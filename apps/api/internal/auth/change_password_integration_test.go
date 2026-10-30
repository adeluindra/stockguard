package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestChangePasswordPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/stockguard_test" {
		t.Fatal("Test must use stockguard_test database")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatal(err)
	}

	username := "pwchange_" +
		hex.EncodeToString(randomBytes[:])

	oldPassword := "Old-Test-Password-12345"
	newPassword := "New-Test-Password-67890"

	oldHash, err := HashPassword(oldPassword)
	if err != nil {
		t.Fatal(err)
	}

	var userID int64

	err = db.QueryRowContext(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role,
			is_active,
			must_change_password
		)
		VALUES ($1, $2, 'staff', TRUE, TRUE)
		RETURNING id
	`, username, oldHash).Scan(&userID)

	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel :=
			context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
		defer cleanupCancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM sessions WHERE user_id = $1",
			userID,
		); err != nil {
			t.Errorf("cleanup sessions: %v", err)
		}

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE id = $1",
			userID,
		); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	}()

	users := NewUserStore(db)
	sessions := NewSessionStore(db)

	// Simulate two logged-in devices.
	firstToken, _, err := sessions.Create(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	secondToken, _, err := sessions.Create(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}

	// Wrong current password must not change credentials.
	err = users.ChangePassword(
		ctx,
		userID,
		"incorrect-current-password",
		newPassword,
	)
	if !errors.Is(err, ErrCurrentPasswordIncorrect) {
		t.Fatalf("wrong password: %v", err)
	}

	// Reusing the current password must be rejected.
	err = users.ChangePassword(
		ctx,
		userID,
		oldPassword,
		oldPassword,
	)
	if !errors.Is(err, ErrPasswordReused) {
		t.Fatalf("password reuse: %v", err)
	}

	// Failed attempts must not invalidate sessions.
	if _, err := sessions.Authenticate(
		ctx,
		firstToken,
	); err != nil {
		t.Fatalf(
			"session revoked after failed attempt: %v",
			err,
		)
	}

	// The successful change must update and revoke atomically.
	if err := users.ChangePassword(
		ctx,
		userID,
		oldPassword,
		newPassword,
	); err != nil {
		t.Fatalf("change password: %v", err)
	}

	var storedHash string
	var mustChange bool

	err = db.QueryRowContext(ctx, `
		SELECT password_hash, must_change_password
		FROM users
		WHERE id = $1
	`, userID).Scan(&storedHash, &mustChange)

	if err != nil {
		t.Fatal(err)
	}

	if mustChange {
		t.Fatal("must_change_password should be false")
	}

	newPasswordValid, err := VerifyPassword(
		newPassword,
		storedHash,
	)
	if err != nil || !newPasswordValid {
		t.Fatalf(
			"new password not accepted: %v",
			err,
		)
	}

	oldPasswordValid, err := VerifyPassword(
		oldPassword,
		storedHash,
	)
	if err != nil || oldPasswordValid {
		t.Fatalf(
			"old password still accepted: %v",
			err,
		)
	}

	for _, token := range []string{
		firstToken,
		secondToken,
	} {
		_, err := sessions.Authenticate(ctx, token)

		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf(
				"old session still valid: %v",
				err,
			)
		}
	}
}
