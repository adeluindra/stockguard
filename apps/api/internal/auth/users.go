package auth

import (
	"context"
	"database/sql"
	"fmt"
)

type LoginAccount struct {
	ID                 int64
	Username           string
	PasswordHash       string
	Role               string
	MustChangePassword bool
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) FindActiveByUsername(
	ctx context.Context,
	username string,
) (LoginAccount, error) {
	const query = `
		SELECT
			id,
			username,
			password_hash,
			role,
			must_change_password
		FROM users
		WHERE LOWER(username) = LOWER($1)
		  AND is_active = TRUE
	`

	var user LoginAccount

	err := s.db.QueryRowContext(
		ctx,
		query,
		username,
	).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.MustChangePassword,
	)

	if err != nil {
		return LoginAccount{},
			fmt.Errorf("find active user: %w", err)
	}

	return user, nil
}
