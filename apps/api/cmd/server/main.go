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

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/httpapi"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

const minimumSchemaVersion = 4

func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, nil),
	)

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

	origin := os.Getenv("APP_ORIGIN")
	if origin == "" {
		origin = "http://127.0.0.1:8080"
	}

	secureCookies := os.Getenv(
		"SESSION_COOKIE_SECURE",
	) == "true"

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf(
			"configure PostgreSQL: %w", err,
		)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	checkReady := func(ctx context.Context) error {
		var version int
		var dirty bool

		err := db.QueryRowContext(
			ctx,
			"SELECT version, dirty FROM schema_migrations",
		).Scan(&version, &dirty)

		if err != nil {
			return fmt.Errorf(
				"read schema version: %w", err,
			)
		}

		if dirty || version < minimumSchemaVersion {
			return fmt.Errorf(
				"schema version %d (dirty=%t) not ready",
				version, dirty,
			)
		}

		return nil
	}

	startupCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf(
			"ping database: %w", err,
		)
	}

	if err := checkReady(startupCtx); err != nil {
		return err
	}

	users := auth.NewUserStore(db)
	sessions := auth.NewSessionStore(db)
	productStore := products.NewStore(db)

	authHandler, err := httpapi.NewAuthHandler(
		users,
		sessions,
		logger,
		origin,
		secureCookies,
	)
	if err != nil {
		return fmt.Errorf(
			"configure authentication: %w", err,
		)
	}

	server := &http.Server{
		Addr: addr,
		Handler: httpapi.NewRouterWithAuthAndProducts(
			checkReady,
			logger,
			authHandler,
			productStore,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	logger.Info(
		"api_start_requested",
		"address", addr,
	)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)

	case <-ctx.Done():
		stop()
		logger.Info("shutdown_started")

		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf(
				"graceful shutdown: %w", err,
			)
		}

		if err := <-serverErrors; err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf(
				"HTTP server stopped: %w", err,
			)
		}

		logger.Info("shutdown_completed")
		return nil
	}
}
