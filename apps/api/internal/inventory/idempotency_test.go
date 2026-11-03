package inventory

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAdjustmentKey(t *testing.T) {
	const upper = "71FA94A1-8C33-45A6-B915-ECF719CF48C1"
	const lower = "71fa94a1-8c33-45a6-b915-ecf719cf48c1"

	got, err := NormalizeAdjustmentKey(upper)
	if err != nil || got != lower {
		t.Fatalf("normalization: %q %v", got, err)
	}

	invalid := []string{
		"",
		"invalid",
		" " + lower,
		lower + " ",
		strings.Replace(lower, "-", "x", 1),
		"71fa94a1-8c33-45a6-b915-ecf719cf48cz",
	}

	for _, key := range invalid {
		if _, err := NormalizeAdjustmentKey(key); !errors.Is(
			err, ErrInvalidAdjustmentKey,
		) {
			t.Fatalf("invalid key accepted: %q", key)
		}
	}
}

func TestAdjustmentLockScope(t *testing.T) {
	const key = "71fa94a1-8c33-45a6-b915-ecf719cf48c1"

	a := adjustmentLockID(1, key)
	b := adjustmentLockID(1, strings.ToLower(key))
	c := adjustmentLockID(2, key)

	if a != b {
		t.Fatal("identical scoped keys differ")
	}

	if a == c {
		t.Fatal("different actors share lock ID")
	}
}
