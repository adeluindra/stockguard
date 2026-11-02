package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

var ErrCredentialsChanged = errors.New(
	"credentials changed during login",
)

// CreateVerified creates a session only when the password hash
// verified during login still matches the current database hash.
//
// FOR SHARE serializes session creation with password changes,
// password resets, and account deactivation.
func (s *SessionStore) CreateVerified(
	ctx context.Context,
	userID int64,
	verifiedHash string,
) (string, time.Time, error) {
	if userID <= 0 || verifiedHash == "" {
		return "", time.Time{}, ErrCredentialsChanged
	}

	var raw [sessionTokenBytes]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{},
			fmt.Errorf("generate session token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw[:])
	digest := sha256.Sum256(raw[:])

	const query = `
		WITH matched_user AS MATERIALIZED (
			SELECT id
			FROM users
			WHERE id = $1
			  AND is_active = TRUE
			  AND password_hash = $3
			FOR SHARE
		)
		INSERT INTO sessions (
			user_id,
			token_hash,
			created_at,
			last_activity_at,
			expires_at
		)
		SELECT
			id,
			$2,
			statement_timestamp(),
			statement_timestamp(),
			statement_timestamp()
				+ ($4::bigint * INTERVAL '1 second')
		FROM matched_user
		RETURNING expires_at
	`

	var expiresAt time.Time

	err := s.db.QueryRowContext(
		ctx,
		query,
		userID,
		digest[:],
		verifiedHash,
		int64(SessionAbsoluteTTL/time.Second),
	).Scan(&expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrCredentialsChanged
	}

	if err != nil {
		return "", time.Time{},
			fmt.Errorf("create verified session: %w", err)
	}

	return token, expiresAt, nil
}
