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

func TestStaffManagementPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("TEST_DATABASE_URL must target stockguard_test")
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

	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	store := NewUserStore(db)
	sessions := NewSessionStore(db)

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}

	username := "staff_" +
		hex.EncodeToString(suffix[:])

	user, temporary, err := store.CreateStaff(
		ctx, username,
	)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM sessions WHERE user_id=$1",
			user.ID,
		); err != nil {
			t.Errorf("cleanup sessions: %v", err)
		}

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE id=$1",
			user.ID,
		); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	}()

	if user.Role != "staff" ||
		!user.IsActive ||
		!user.MustChangePassword {
		t.Fatal("created staff attributes incorrect")
	}

	// Username uniqueness must be case-insensitive.
	_, _, err = store.CreateStaff(
		ctx,
		strings.ToUpper(username),
	)

	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf(
			"case-insensitive uniqueness: %v",
			err,
		)
	}

	// Staff must appear in the list.
	staff, err := store.ListStaff(ctx)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, item := range staff {
		if item.ID == user.ID {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("staff not returned in list")
	}

	// The first temporary password must work.
	account, err := store.FindActiveByUsername(
		ctx, username,
	)
	if err != nil {
		t.Fatal(err)
	}

	matched, err := VerifyPassword(
		temporary,
		account.PasswordHash,
	)

	if err != nil || !matched {
		t.Fatal(
			"initial temporary password not verifiable",
		)
	}

	// Simulate a logged-in Staff.
	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Reset must invalidate the old session.
	replacement, err := store.ResetStaffPassword(
		ctx, user.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if replacement == temporary {
		t.Fatal("reset reused temporary password")
	}

	_, err = sessions.Authenticate(ctx, token)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf(
			"reset failed to revoke existing session: %v",
			err,
		)
	}

	account, err = store.FindActiveByUsername(
		ctx, username,
	)
	if err != nil {
		t.Fatal(err)
	}

	matched, err = VerifyPassword(
		replacement,
		account.PasswordHash,
	)

	if err != nil ||
		!matched ||
		!account.MustChangePassword {
		t.Fatal(
			"reset password or must_change flag invalid",
		)
	}

	oldMatched, err := VerifyPassword(
		temporary,
		account.PasswordHash,
	)

	if err != nil || oldMatched {
		t.Fatal(
			"initial password remains valid after reset",
		)
	}

	// Create another session before deactivation.
	newToken, _, err := sessions.Create(
		ctx, user.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeactivateStaff(
		ctx, user.ID,
	); err != nil {
		t.Fatal(err)
	}

	_, err = sessions.Authenticate(ctx, newToken)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf(
			"deactivated staff session still valid: %v",
			err,
		)
	}

	// Inactive Staff cannot create new sessions.
	_, _, err = sessions.Create(ctx, user.ID)
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf(
			"inactive staff can create session: %v",
			err,
		)
	}

	// Reset must not reactivate an inactive account.
	_, err = store.ResetStaffPassword(
		ctx, user.ID,
	)

	if !errors.Is(err, ErrStaffNotFound) {
		t.Fatalf(
			"reset inactive staff: %v",
			err,
		)
	}
}
