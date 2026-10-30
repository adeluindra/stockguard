package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateStaffUsername(t *testing.T) {
	valid := []string{
		"staff01",
		"A_user-2",
		"ABC",
	}

	for _, username := range valid {
		if err := ValidateStaffUsername(username); err != nil {
			t.Fatalf(
				"valid username %q rejected: %v",
				username,
				err,
			)
		}
	}

	invalid := []string{
		"",
		"ab",
		"has space",
		"üsername",
		"../admin",
		strings.Repeat("a", 51),
	}

	for _, username := range invalid {
		err := ValidateStaffUsername(username)

		if !errors.Is(err, ErrInvalidStaffUsername) {
			t.Fatalf(
				"invalid username %q accepted: %v",
				username,
				err,
			)
		}
	}
}

func TestTemporaryPasswordIsUniqueAndVerifiable(t *testing.T) {
	first, err := newTemporaryPassword()
	if err != nil {
		t.Fatal(err)
	}

	second, err := newTemporaryPassword()
	if err != nil {
		t.Fatal(err)
	}

	if first == second || len(first) < 16 {
		t.Fatal(
			"temporary passwords must be strong and unique",
		)
	}

	hash, err := HashPassword(first)
	if err != nil {
		t.Fatal(err)
	}

	valid, err := VerifyPassword(first, hash)
	if err != nil || !valid {
		t.Fatal(
			"generated temporary password failed verification",
		)
	}
}
