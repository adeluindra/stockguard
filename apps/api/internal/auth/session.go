package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	SessionCookieName  = "stockguard_session"
	SessionAbsoluteTTL = 12 * time.Hour
	SessionIdleTTL     = 30 * time.Minute
	sessionTokenBytes  = 32
)

var (
	ErrInvalidSession = errors.New("invalid or expired session")
	ErrInactiveUser   = errors.New("user not found or inactive")
)

// DBTX supports both *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SessionStore struct {
	db DBTX
}

func NewSessionStore(db DBTX) *SessionStore {
	return &SessionStore{db: db}
}

type SessionIdentity struct {
	UserID             int64
	Role               string
	MustChangePassword bool
	ExpiresAt          time.Time
}

// Create generates a random bearer token.
// Only SHA-256(token) is persisted in PostgreSQL.
// Call this only after successful password verification.
func (s *SessionStore) Create(
	ctx context.Context,
	userID int64,
) (string, time.Time, error) {
	if userID <= 0 {
		return "", time.Time{}, ErrInactiveUser
	}

	token, digest, err := newSessionToken()
	if err != nil {
		return "", time.Time{},
			fmt.Errorf("generate session token: %w", err)
	}

	const query = `
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
				+ ($3::bigint * INTERVAL '1 second')
		FROM users
		WHERE id = $1
		  AND is_active = TRUE
		RETURNING expires_at
	`

	var expiresAt time.Time

	err = s.db.QueryRowContext(
		ctx,
		query,
		userID,
		digest[:],
		int64(SessionAbsoluteTTL.Seconds()),
	).Scan(&expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrInactiveUser
	}
	if err != nil {
		return "", time.Time{},
			fmt.Errorf("create session: %w", err)
	}

	return token, expiresAt, nil
}

// Authenticate validates the token and atomically refreshes
// last_activity_at if the session is still valid.
func (s *SessionStore) Authenticate(
	ctx context.Context,
	token string,
) (SessionIdentity, error) {
	digest, err := hashSessionToken(token)
	if err != nil {
		return SessionIdentity{}, ErrInvalidSession
	}

	const query = `
		UPDATE sessions AS s
		SET last_activity_at = statement_timestamp()
		FROM users AS u
		WHERE s.token_hash = $1
		  AND s.user_id = u.id
		  AND u.is_active = TRUE
		  AND s.revoked_at IS NULL
		  AND s.expires_at > statement_timestamp()
		  AND s.last_activity_at >
			  statement_timestamp()
			  - ($2::bigint * INTERVAL '1 second')
		RETURNING
			s.user_id,
			u.role,
			u.must_change_password,
			s.expires_at
	`

	var identity SessionIdentity

	err = s.db.QueryRowContext(
		ctx,
		query,
		digest[:],
		int64(SessionIdleTTL.Seconds()),
	).Scan(
		&identity.UserID,
		&identity.Role,
		&identity.MustChangePassword,
		&identity.ExpiresAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return SessionIdentity{}, ErrInvalidSession
	}
	if err != nil {
		return SessionIdentity{},
			fmt.Errorf("authenticate session: %w", err)
	}

	return identity, nil
}

// Revoke invalidates a session.
// Repeating logout for an already revoked token is safe.
func (s *SessionStore) Revoke(
	ctx context.Context,
	token string,
) error {
	digest, err := hashSessionToken(token)
	if err != nil {
		return ErrInvalidSession
	}

	const query = `
		UPDATE sessions
		SET revoked_at = statement_timestamp()
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`

	if _, err := s.db.ExecContext(
		ctx, query, digest[:],
	); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

// RevokeAllForUser invalidates all sessions belonging to a user.
// Useful for password reset and security operations.
func (s *SessionStore) RevokeAllForUser(
	ctx context.Context,
	userID int64,
) error {
	if userID <= 0 {
		return ErrInactiveUser
	}

	const query = `
		UPDATE sessions
		SET revoked_at = statement_timestamp()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`

	if _, err := s.db.ExecContext(
		ctx, query, userID,
	); err != nil {
		return fmt.Errorf(
			"revoke all user sessions: %w", err,
		)
	}

	return nil
}

func newSessionToken() (string, [32]byte, error) {
	var raw [sessionTokenBytes]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}

	token := base64.RawURLEncoding.EncodeToString(raw[:])
	digest := sha256.Sum256(raw[:])

	return token, digest, nil
}

func hashSessionToken(token string) ([32]byte, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(
		sessionTokenBytes,
	) {
		return [32]byte{}, ErrInvalidSession
	}

	decoded, err := base64.RawURLEncoding.Strict().
		DecodeString(token)

	if err != nil || len(decoded) != sessionTokenBytes {
		return [32]byte{}, ErrInvalidSession
	}

	return sha256.Sum256(decoded), nil
}

// Secure must be true when using HTTPS in production.
func SessionCookie(
	token string,
	expiresAt time.Time,
	secure bool,
) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func ClearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
