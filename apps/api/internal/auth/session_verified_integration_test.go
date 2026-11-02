package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestVerifiedSessionPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal(
			"TEST_DATABASE_URL must point to stockguard_test",
		)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		40*time.Second,
	)
	defer cancel()

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}

	username := "verified_" +
		hex.EncodeToString(suffix[:])

	users := NewUserStore(db)
	sessions := NewSessionStore(db)

	staff, firstPassword, err := users.CreateStaff(
		ctx,
		username,
	)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer done()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM sessions WHERE user_id = $1",
			staff.ID,
		); err != nil {
			t.Errorf("cleanup sessions: %v", err)
		}

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE id = $1",
			staff.ID,
		); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	}()

	// Simulate successful password verification.
	account, err := users.FindActiveByUsername(
		ctx,
		username,
	)
	if err != nil {
		t.Fatal(err)
	}

	valid, err := VerifyPassword(
		firstPassword,
		account.PasswordHash,
	)
	if err != nil || !valid {
		t.Fatal("initial password verification failed")
	}

	// Creating a session with the current hash succeeds.
	oldToken, _, err := sessions.CreateVerified(
		ctx,
		staff.ID,
		account.PasswordHash,
	)
	if err != nil {
		t.Fatalf("initial verified session: %v", err)
	}

	if _, err := sessions.Authenticate(
		ctx,
		oldToken,
	); err != nil {
		t.Fatalf("initial session invalid: %v", err)
	}

	// Admin resets the password and revokes existing sessions.
	newPassword, err := users.ResetStaffPassword(
		ctx,
		staff.ID,
	)
	if err != nil {
		t.Fatalf("reset staff password: %v", err)
	}

	// A login that verified the old hash must not
	// create a new session after the reset.
	_, _, err = sessions.CreateVerified(
		ctx,
		staff.ID,
		account.PasswordHash,
	)
	if !errors.Is(err, ErrCredentialsChanged) {
		t.Fatalf(
			"stale password hash accepted: %v",
			err,
		)
	}

	// Existing sessions must also be revoked.
	_, err = sessions.Authenticate(ctx, oldToken)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf(
			"old session remains valid after reset: %v",
			err,
		)
	}

	updated, err := users.FindActiveByUsername(
		ctx,
		username,
	)
	if err != nil {
		t.Fatal(err)
	}

	valid, err = VerifyPassword(
		newPassword,
		updated.PasswordHash,
	)
	if err != nil || !valid {
		t.Fatal("replacement password verification failed")
	}

	// Only the new hash can authorize session creation.
	newToken, _, err := sessions.CreateVerified(
		ctx,
		staff.ID,
		updated.PasswordHash,
	)
	if err != nil {
		t.Fatalf("new password session: %v", err)
	}

	if _, err := sessions.Authenticate(
		ctx,
		newToken,
	); err != nil {
		t.Fatalf("new session invalid: %v", err)
	}

	// Deactivation must prevent another session,
	// even when the password hash still matches.
	if err := users.DeactivateStaff(ctx, staff.ID); err != nil {
		t.Fatal(err)
	}

	_, _, err = sessions.CreateVerified(
		ctx,
		staff.ID,
		updated.PasswordHash,
	)
	if !errors.Is(err, ErrCredentialsChanged) {
		t.Fatalf(
			"inactive staff created a session: %v",
			err,
		)
	}

	_, err = sessions.Authenticate(ctx, newToken)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf(
			"deactivated staff session still valid: %v",
			err,
		)
	}
}
