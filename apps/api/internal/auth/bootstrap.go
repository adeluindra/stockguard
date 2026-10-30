package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrAlreadyInitialized = errors.New(
		"StockGuard already has a user; bootstrap is disabled",
	)

	ErrInvalidAdminInput = errors.New(
		"invalid bootstrap admin credentials",
	)
)

const bootstrapLockID int64 = 741919042001

func BootstrapAdmin(
	ctx context.Context,
	db *sql.DB,
	username string,
	password string,
) (int64, error) {
	if err := validateBootstrapInput(username, password); err != nil {
		return 0, err
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("hash admin password: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer tx.Rollback()

	// Prevent concurrent bootstrap attempts.
	_, err = tx.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock($1)",
		bootstrapLockID,
	)
	if err != nil {
		return 0, fmt.Errorf("acquire bootstrap lock: %w", err)
	}

	var hasUsers bool

	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
		)
	`).Scan(&hasUsers)

	if err != nil {
		return 0, fmt.Errorf("check existing users: %w", err)
	}

	if hasUsers {
		return 0, ErrAlreadyInitialized
	}

	var adminID int64

	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role,
			is_active,
			must_change_password
		)
		VALUES (
			$1,
			$2,
			'admin',
			TRUE,
			FALSE
		)
		RETURNING id
	`, username, passwordHash).Scan(&adminID)

	if err != nil {
		return 0, fmt.Errorf("insert initial admin: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit bootstrap: %w", err)
	}

	return adminID, nil
}

func validateBootstrapInput(
	username string,
	password string,
) error {
	if len(username) < 3 || len(username) > 50 {
		return fmt.Errorf(
			"%w: username must contain 3-50 characters",
			ErrInvalidAdminInput,
		)
	}

	for _, character := range username {
		valid := (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' ||
			character == '-'

		if !valid {
			return fmt.Errorf(
				"%w: username supports only letters, numbers, _ and -",
				ErrInvalidAdminInput,
			)
		}
	}

	switch {
	case len(password) == 0:
		return fmt.Errorf(
			"%w: no password was read from the terminal",
			ErrInvalidAdminInput,
		)

	case len(password) < 16:
		return fmt.Errorf(
			"%w: password must contain at least 16 bytes",
			ErrInvalidAdminInput,
		)

	case len(password) > 1024:
		return fmt.Errorf(
			"%w: password must not exceed 1024 bytes",
			ErrInvalidAdminInput,
		)

	case !utf8.ValidString(password):
		return fmt.Errorf(
			"%w: password input contains invalid UTF-8",
			ErrInvalidAdminInput,
		)

	case strings.TrimSpace(password) == "":
		return fmt.Errorf(
			"%w: password cannot consist only of whitespace",
			ErrInvalidAdminInput,
		)
	}

	return nil
}
