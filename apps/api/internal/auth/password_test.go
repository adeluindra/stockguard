package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "Example passphrase 🔐 for StockGuard"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if strings.Contains(encoded, password) {
		t.Fatal("encoded password must not contain plaintext")
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding prefix: %q", encoded)
	}

	matched, err := VerifyPassword(password, encoded)
	if err != nil || !matched {
		t.Fatalf("valid password not verified: matched=%t err=%v", matched, err)
	}
}

func TestDifferentSaltsForSamePassword(t *testing.T) {
	password := "same-password-test-only"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different hash calls unexpectedly returned identical encodings")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	encoded, err := HashPassword("correct-password-test")
	if err != nil {
		t.Fatal(err)
	}
	matched, err := VerifyPassword("wrong-password", encoded)
	if err != nil || matched {
		t.Fatalf("wrong password must not match: matched=%t err=%v", matched, err)
	}
}

func TestRejectInvalidPasswords(t *testing.T) {
	for _, password := range []string{"", strings.Repeat("x", maxPasswordBytes+1)} {
		_, err := HashPassword(password)
		if !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("expected ErrInvalidPassword, got %v", err)
		}
	}
	matched, err := VerifyPassword("", "anything")
	if err != nil || matched {
		t.Fatalf("invalid candidate must not match: matched=%t err=%v", matched, err)
	}
}

func TestRejectMalformedOrExpensiveHashes(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"wrong algorithm":  "$argon2i$v=19$m=19456,t=2,p=1$YWJj$YWJj",
		"unbounded memory": "$argon2id$v=19$m=4294967295,t=2,p=1$YWJj$YWJj",
		"wrong version":    "$argon2id$v=18$m=19456,t=2,p=1$YWJj$YWJj",
		"invalid base64":   "$argon2id$v=19$m=19456,t=2,p=1$??$??",
		"short salt":       "$argon2id$v=19$m=19456,t=2,p=1$YWJj$YWJj",
		"extra field":      "$argon2id$v=19$m=19456,t=2,p=1$YWJj$YWJj$extra",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			matched, err := VerifyPassword("sample-password", encoded)
			if matched || !errors.Is(err, ErrInvalidHash) {
				t.Fatalf("want invalid hash, got matched=%t err=%v", matched, err)
			}
		})
	}
}
