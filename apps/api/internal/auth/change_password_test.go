package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateNewPassword(t *testing.T) {
	cases := []struct {
		name      string
		password  string
		wantError bool
	}{
		{
			"good passphrase",
			"fresh secure passphrase",
			false,
		},
		{
			"too short",
			"short",
			true,
		},
		{
			"only spaces",
			strings.Repeat(" ", 20),
			true,
		},
		{
			"too long",
			strings.Repeat("a", 1025),
			true,
		},
		{
			"invalid UTF-8",
			string([]byte{0xff, 0xfe}) +
				strings.Repeat("a", 16),
			true,
		},
		{
			"unicode short in characters",
			strings.Repeat("猫", 15),
			true,
		},
		{
			"unicode valid",
			strings.Repeat("猫", 16),
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewPassword(tc.password)

			if tc.wantError !=
				errors.Is(err, ErrNewPasswordInvalid) {
				t.Fatalf(
					"got error %v; want error=%t",
					err,
					tc.wantError,
				)
			}
		})
	}
}
