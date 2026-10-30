package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Bootstrap failed:", err)
		os.Exit(1)
	}
}

func run() error {
	username := flag.String(
		"username",
		"admin",
		"Username for the initial administrator",
	)
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	fd := int(os.Stdin.Fd())

	if !term.IsTerminal(fd) {
		return errors.New(
			"bootstrap requires an interactive terminal",
		)
	}

	fmt.Println("StockGuard Initial Admin Bootstrap")
	fmt.Println("Password must contain at least 16 bytes.")
	fmt.Println("Password will not be displayed.")

	fmt.Print("Admin password: ")

	password, err := term.ReadPassword(fd)
	fmt.Println()

	if err != nil {
		return fmt.Errorf("read admin password: %w", err)
	}
	defer clearBytes(password)

	fmt.Print("Confirm password: ")

	confirmation, err := term.ReadPassword(fd)
	fmt.Println()

	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	defer clearBytes(confirmation)

	if !bytes.Equal(password, confirmation) {
		return errors.New("password confirmation does not match")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	id, err := auth.BootstrapAdmin(
		ctx,
		db,
		*username,
		string(password),
	)
	if err != nil {
		return err
	}

	fmt.Printf(
		"Initial admin created successfully (ID: %d).\n",
		id,
	)

	fmt.Println("You can now test the HTTP login endpoint.")

	return nil
}

func clearBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
