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
	ErrCurrentPasswordIncorrect = errors.New(
		"current password is incorrect",
	)

	ErrPasswordReused = errors.New(
		"new password must differ from current password",
	)

	ErrNewPasswordInvalid = errors.New(
		"new password must contain at least 16 characters and at most 1024 bytes",
	)
)

func ValidateNewPassword(password string) error {
	if !utf8.ValidString(password) ||
		len(password) > 1024 ||
		utf8.RuneCountInString(password) < 16 ||
		strings.TrimSpace(password) == "" {
		return ErrNewPasswordInvalid
	}

	return nil
}

// ChangePassword verifies the current password and atomically
// changes the hash, clears the reset flag, and revokes sessions.
func (s *UserStore) ChangePassword(
	ctx context.Context,
	userID int64,
	currentPassword string,
	newPassword string,
) error {
	if userID <= 0 {
		return ErrInactiveUser
	}

	if len(currentPassword) == 0 ||
		len(currentPassword) > 1024 {
		return ErrCurrentPasswordIncorrect
	}

	if err := ValidateNewPassword(newPassword); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(
		ctx,
		&sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"start password change transaction: %w",
			err,
		)
	}
	defer tx.Rollback()

	// Serialize competing password changes for this user.
	var storedHash string

	err = tx.QueryRowContext(ctx, `
		SELECT password_hash
		FROM users
		WHERE id = $1
		  AND is_active = TRUE
		FOR UPDATE
	`, userID).Scan(&storedHash)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrInactiveUser
	}
	if err != nil {
		return fmt.Errorf(
			"read account for password change: %w",
			err,
		)
	}

	matched, err := VerifyPassword(
		currentPassword,
		storedHash,
	)
	if err != nil {
		return fmt.Errorf(
			"verify stored password: %w",
			err,
		)
	}

	if !matched {
		return ErrCurrentPasswordIncorrect
	}

	reused, err := VerifyPassword(
		newPassword,
		storedHash,
	)
	if err != nil {
		return fmt.Errorf(
			"check password reuse: %w",
			err,
		)
	}

	if reused {
		return ErrPasswordReused
	}

	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf(
			"hash replacement password: %w",
			err,
		)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE users
		SET
			password_hash = $2,
			must_change_password = FALSE,
			updated_at = statement_timestamp()
		WHERE id = $1
	`, userID, newHash)

	if err != nil {
		return fmt.Errorf(
			"update account password: %w",
			err,
		)
	}

	// Invalidate every existing session, including
	// the one used to change the password.
	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = statement_timestamp()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, userID)

	if err != nil {
		return fmt.Errorf(
			"revoke sessions after password change: %w",
			err,
		)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf(
			"commit password change: %w",
			err,
		)
	}

	return nil
}
