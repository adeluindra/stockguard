package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidStaffUsername = errors.New("staff username must be 3-50 characters (letters, digits, _ or -)")
	ErrUsernameTaken        = errors.New("username already exists")
	ErrStaffNotFound        = errors.New("active staff account not found")
)

type StaffUser struct {
	ID                 int64     `json:"id"`
	Username           string    `json:"username"`
	Role               string    `json:"role"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
}

func ValidateStaffUsername(username string) error {
	if len(username) < 3 || len(username) > 50 {
		return ErrInvalidStaffUsername
	}

	for _, r := range username {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-'

		if !valid {
			return ErrInvalidStaffUsername
		}
	}

	return nil
}

func newTemporaryPassword() (string, error) {
	var raw [24]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate temporary password: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *UserStore) ListStaff(
	ctx context.Context,
) ([]StaffUser, error) {
	const query = `
		SELECT
			id,
			username,
			role,
			is_active,
			must_change_password,
			created_at
		FROM users
		WHERE role = 'staff'
		ORDER BY id ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer rows.Close()

	result := make([]StaffUser, 0)

	for rows.Next() {
		var u StaffUser

		err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.Role,
			&u.IsActive,
			&u.MustChangePassword,
			&u.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("scan staff: %w", err)
		}

		result = append(result, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate staff: %w", err)
	}

	return result, nil
}

func (s *UserStore) CreateStaff(
	ctx context.Context,
	username string,
) (StaffUser, string, error) {
	if err := ValidateStaffUsername(username); err != nil {
		return StaffUser{}, "", err
	}

	temp, err := newTemporaryPassword()
	if err != nil {
		return StaffUser{}, "", err
	}

	hash, err := HashPassword(temp)
	if err != nil {
		return StaffUser{}, "", fmt.Errorf(
			"hash temporary password: %w", err,
		)
	}

	const query = `
		INSERT INTO users (
			username,
			password_hash,
			role,
			is_active,
			must_change_password
		)
		VALUES ($1, $2, 'staff', TRUE, TRUE)
		ON CONFLICT DO NOTHING
		RETURNING
			id,
			username,
			role,
			is_active,
			must_change_password,
			created_at
	`

	var u StaffUser

	err = s.db.QueryRowContext(
		ctx, query, username, hash,
	).Scan(
		&u.ID,
		&u.Username,
		&u.Role,
		&u.IsActive,
		&u.MustChangePassword,
		&u.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return StaffUser{}, "", ErrUsernameTaken
	}
	if err != nil {
		return StaffUser{}, "", fmt.Errorf("create staff: %w", err)
	}

	return u, temp, nil
}

func (s *UserStore) ResetStaffPassword(
	ctx context.Context,
	id int64,
) (string, error) {
	if id <= 0 {
		return "", ErrStaffNotFound
	}

	temp, err := newTemporaryPassword()
	if err != nil {
		return "", err
	}

	hash, err := HashPassword(temp)
	if err != nil {
		return "", fmt.Errorf(
			"hash temporary password: %w", err,
		)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf(
			"start staff reset transaction: %w", err,
		)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET
			password_hash = $2,
			must_change_password = TRUE,
			updated_at = statement_timestamp()
		WHERE id = $1
		  AND role = 'staff'
		  AND is_active = TRUE
	`, id, hash)

	if err != nil {
		return "", fmt.Errorf(
			"reset staff password: %w", err,
		)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}

	if affected != 1 {
		return "", ErrStaffNotFound
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = statement_timestamp()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, id)

	if err != nil {
		return "", fmt.Errorf(
			"revoke staff sessions: %w", err,
		)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf(
			"commit staff reset: %w", err,
		)
	}

	return temp, nil
}

func (s *UserStore) DeactivateStaff(
	ctx context.Context,
	id int64,
) error {
	if id <= 0 {
		return ErrStaffNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"start staff deactivation: %w", err,
		)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET
			is_active = FALSE,
			updated_at = statement_timestamp()
		WHERE id = $1
		  AND role = 'staff'
		  AND is_active = TRUE
	`, id)

	if err != nil {
		return fmt.Errorf("deactivate staff: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected != 1 {
		return ErrStaffNotFound
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = statement_timestamp()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, id)

	if err != nil {
		return fmt.Errorf(
			"revoke staff sessions: %w", err,
		)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf(
			"commit staff deactivation: %w", err,
		)
	}

	return nil
}
