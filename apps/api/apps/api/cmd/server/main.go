package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/adeluindra/stockguard/apps/api/internal/httpapi"
)

const minimumSchemaVersion = 4

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server_failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("failed to configure PostgreSQL connection")
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	checkReady := func(ctx context.Context) error {
		var version int
		var dirty bool
		err := db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
		if err != nil {
			return fmt.Errorf("read database schema migration state: %w", err)
		}
		if dirty || version < minimumSchemaVersion {
			return fmt.Errorf("database migration state not ready (version=%d, dirty=%t)", version, dirty)
		}
		return nil
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(startupCtx); err != nil {
		cancel()
		return fmt.Errorf("PostgreSQL ping failed: %w", err)
	}
	if err := checkReady(startupCtx); err != nil {
		cancel()
		return fmt.Errorf("PostgreSQL schema not ready: %w", err)
	}
	cancel()

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouter(checkReady, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	logger.Info("api_start_requested", "address", addr)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	case <-ctx.Done():
		stop()
		logger.Info("shutdown_started")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}

		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server stopped: %w", err)
		}
		logger.Info("shutdown_completed")
		return nil
	}
}
