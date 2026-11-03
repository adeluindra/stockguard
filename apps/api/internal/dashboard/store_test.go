package dashboard

import (
	"errors"
	"testing"
)

func TestValidatePage(t *testing.T) {
	valid := []struct {
		limit  int
		offset int
	}{
		{1, 0},
		{20, 10},
		{100, 1000000},
	}

	for _, tc := range valid {
		if err := ValidatePage(
			tc.limit,
			tc.offset,
		); err != nil {
			t.Fatalf(
				"valid pagination rejected: %+v: %v",
				tc,
				err,
			)
		}
	}

	invalid := []struct {
		limit  int
		offset int
	}{
		{0, 0},
		{101, 0},
		{-1, 0},
		{20, -1},
		{20, 1000001},
	}

	for _, tc := range invalid {
		err := ValidatePage(tc.limit, tc.offset)

		if !errors.Is(err, ErrInvalidPage) {
			t.Fatalf(
				"invalid pagination accepted: %+v",
				tc,
			)
		}
	}
}
