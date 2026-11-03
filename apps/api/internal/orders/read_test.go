package orders

import (
	"errors"
	"testing"
)

func TestValidateOrderQuery(t *testing.T) {
	valid := []struct {
		status string
		limit  int
		offset int
	}{
		{"", 20, 0},
		{"reserved", 20, 0},
		{"fulfilled", 100, 100},
		{"cancelled", 1, 1000000},
	}

	for _, tc := range valid {
		if err := ValidateOrderQuery(
			tc.status, tc.limit, tc.offset,
		); err != nil {
			t.Fatalf("valid query rejected: %+v: %v", tc, err)
		}
	}

	invalid := []struct {
		status string
		limit  int
		offset int
	}{
		{"pending", 20, 0},
		{"RESERVED", 20, 0},
		{"", 0, 0},
		{"", 101, 0},
		{"", 20, -1},
		{"", 20, 1000001},
	}

	for _, tc := range invalid {
		err := ValidateOrderQuery(
			tc.status, tc.limit, tc.offset,
		)

		if !errors.Is(err, ErrInvalidOrderQuery) {
			t.Fatalf(
				"invalid query accepted: %+v, err=%v",
				tc, err,
			)
		}
	}
}
