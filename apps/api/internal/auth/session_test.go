package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSessionTokenIsRandomAndHashed(t *testing.T) {
	first, hash1, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}

	second, hash2, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}

	if first == second || bytes.Equal(hash1[:], hash2[:]) {
		t.Fatal("tokens must be unique")
	}

	raw, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(raw) != sessionTokenBytes {
		t.Fatal("token must be 32 bytes")
	}

	want := sha256.Sum256(raw)
	if hash1 != want {
		t.Fatal("incorrect token digest")
	}

	if strings.Contains(first, "+") ||
		strings.Contains(first, "/") {
		t.Fatal("token is not URL-safe")
	}
}

func TestInvalidSessionTokensRejected(t *testing.T) {
	tokens := []string{
		"",
		"not-a-token",
		strings.Repeat("x", 43),
		strings.Repeat("=", 43),
	}

	for _, token := range tokens {
		_, err := hashSessionToken(token)
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf(
				"token %q: expected invalid session, got %v",
				token, err,
			)
		}

		store := NewSessionStore(nil)

		_, err = store.Authenticate(
			context.Background(), token,
		)

		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf(
				"authenticate malformed token: %v", err,
			)
		}
	}
}

func TestSessionCookieSecurity(t *testing.T) {
	token, _, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}

	expires := time.Now().Add(time.Hour)
	cookie := SessionCookie(token, expires, true)

	if cookie.Name != SessionCookieName ||
		cookie.Value != token ||
		cookie.Path != "/" ||
		!cookie.HttpOnly ||
		!cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode ||
		cookie.Domain != "" {
		t.Fatalf("insecure session cookie: %+v", cookie)
	}

	cleared := ClearSessionCookie(true)

	if cleared.MaxAge != -1 ||
		cleared.Value != "" ||
		!cleared.Secure ||
		!cleared.HttpOnly {
		t.Fatalf("invalid logout cookie: %+v", cleared)
	}
}

func TestInvalidUserIDRejected(t *testing.T) {
	store := NewSessionStore(nil)

	_, _, err := store.Create(context.Background(), 0)
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("Create: %v", err)
	}

	err = store.RevokeAllForUser(context.Background(), 0)
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
}
