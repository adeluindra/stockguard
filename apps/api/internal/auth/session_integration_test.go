package auth

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSessionLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("TEST_DATABASE_URL must point to stockguard_test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(), 20*time.Second,
	)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	// Create synthetic test user.
	suffix, _, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}

	var userID int64

	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (
			username, password_hash, role
		)
		VALUES ($1, $2, 'staff')
		RETURNING id
	`,
		"be2b_"+suffix[:16],
		"TEST_ONLY_NOT_REAL_HASH",
	).Scan(&userID)

	if err != nil {
		t.Fatal(err)
	}

	store := NewSessionStore(tx)

	// Test creation.
	token, expires, err := store.Create(ctx, userID)
	if err != nil || !expires.After(time.Now()) {
		t.Fatalf("create: %v", err)
	}

	// Confirm only the token digest is stored.
	digest, err := hashSessionToken(token)
	if err != nil {
		t.Fatal(err)
	}

	var storedHash []byte

	err = tx.QueryRowContext(ctx, `
		SELECT token_hash
		FROM sessions
		WHERE user_id = $1
	`, userID).Scan(&storedHash)

	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(storedHash, digest[:]) {
		t.Fatal("incorrect stored token digest")
	}

	// Test authentication.
	identity, err := store.Authenticate(ctx, token)
	if err != nil ||
		identity.UserID != userID ||
		identity.Role != "staff" ||
		identity.MustChangePassword {
		t.Fatalf("authenticate: %+v, %v", identity, err)
	}

	// Test logout.
	if err := store.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}

	_, err = store.Authenticate(ctx, token)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked session accepted: %v", err)
	}

	// Test mandatory password-change flag.
	token2, _, err := store.Create(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE users
		SET must_change_password = TRUE
		WHERE id = $1
	`, userID)

	if err != nil {
		t.Fatal(err)
	}

	identity, err = store.Authenticate(ctx, token2)
	if err != nil || !identity.MustChangePassword {
		t.Fatalf(
			"password-change flag: %+v, %v",
			identity, err,
		)
	}

	// Test revoking every session.
	if err := store.RevokeAllForUser(ctx, userID); err != nil {
		t.Fatal(err)
	}

	_, err = store.Authenticate(ctx, token2)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoke all failed: %v", err)
	}

	// Test idle timeout using a synthetic old session.
	token3, _, err := store.Create(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	hash3, err := hashSessionToken(token3)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET
			created_at =
				statement_timestamp() - INTERVAL '1 hour',
			last_activity_at =
				statement_timestamp() - INTERVAL '31 minutes'
		WHERE token_hash = $1
	`, hash3[:])

	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Authenticate(ctx, token3)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("idle timeout not enforced: %v", err)
	}

	// Test inactive users.
	_, err = tx.ExecContext(ctx, `
		UPDATE users
		SET is_active = FALSE
		WHERE id = $1
	`, userID)

	if err != nil {
		t.Fatal(err)
	}

	_, _, err = store.Create(ctx, userID)
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("inactive user created session: %v", err)
	}

	// Transaction rollback removes all fixtures.
}
