package inventory

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestValidateAdjustment(t *testing.T) {
	cases := []struct {
		name   string
		delta  int64
		reason string
		valid  bool
	}{
		{
			"positive",
			7,
			"Stock received from supplier",
			true,
		},
		{
			"negative",
			-3,
			"Damaged units removed",
			true,
		},
		{
			"zero",
			0,
			"Stock count corrected",
			false,
		},
		{
			"short reason",
			1,
			"short",
			false,
		},
		{
			"blank",
			1,
			"            ",
			false,
		},
		{
			"leading space",
			1,
			" Stock count adjusted",
			false,
		},
		{
			"newline",
			1,
			"Stock count\ncorrected",
			false,
		},
		{
			"too long",
			1,
			strings.Repeat("x", 501),
			false,
		},
		{
			"invalid UTF-8",
			1,
			string([]byte{0xff, 0xfe}) +
				"rest of description",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAdjustment(
				tc.delta, tc.reason,
			)

			if tc.valid && err != nil {
				t.Fatalf(
					"valid adjustment rejected: %v",
					err,
				)
			}

			if !tc.valid &&
				!errors.Is(err, ErrInvalidAdjustment) {
				t.Fatalf(
					"invalid adjustment accepted: %v",
					err,
				)
			}
		})
	}
}

func TestApplyDeltaPreservesReserved(t *testing.T) {
	cases := []struct {
		name     string
		onHand   int64
		reserved int64
		delta    int64
		want     int64
		wantErr  error
	}{
		{
			name:   "increase",
			onHand: 10, reserved: 4,
			delta: 5, want: 15,
		},
		{
			name:   "reduce to reserved",
			onHand: 10, reserved: 4,
			delta: -6, want: 4,
		},
		{
			name:   "oversell",
			onHand: 10, reserved: 4,
			delta:   -7,
			wantErr: ErrInsufficientAvailable,
		},
		{
			name:     "overflow",
			onHand:   math.MaxInt64,
			reserved: 0, delta: 1,
			wantErr: ErrQuantityOverflow,
		},
		{
			name:   "max negative",
			onHand: 0, reserved: 0,
			delta:   math.MinInt64,
			wantErr: ErrInsufficientAvailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyDelta(
				tc.onHand,
				tc.reserved,
				tc.delta,
			)

			if tc.wantErr != nil &&
				!errors.Is(err, tc.wantErr) {
				t.Fatalf(
					"want %v, got %v",
					tc.wantErr, err,
				)
			}

			if tc.wantErr == nil &&
				(err != nil || got != tc.want) {
				t.Fatalf(
					"got %d, %v; want %d",
					got, err, tc.want,
				)
			}
		})
	}
}

func TestValidatePage(t *testing.T) {
	if err := validatePage(20, 0); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		limit  int
		offset int
	}{
		{0, 0},
		{101, 0},
		{20, -1},
		{20, 1000001},
	}

	for _, tc := range cases {
		if !errors.Is(
			validatePage(tc.limit, tc.offset),
			ErrInvalidPagination,
		) {
			t.Fatalf(
				"invalid page accepted: %+v",
				tc,
			)
		}
	}
}
