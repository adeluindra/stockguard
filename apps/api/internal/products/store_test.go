package products

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCreateProduct(t *testing.T) {
	good := CreateInput{
		SKU:               "SKU-001",
		Name:              "Keyboard Mechanical",
		LowStockThreshold: 5,
	}

	if err := ValidateCreate(good); err != nil {
		t.Fatalf("valid product rejected: %v", err)
	}

	cases := []struct {
		name  string
		input CreateInput
	}{
		{
			"short SKU",
			CreateInput{
				SKU:  "AB",
				Name: "Product",
			},
		},
		{
			"SKU with spaces",
			CreateInput{
				SKU:  "SKU 001",
				Name: "Product",
			},
		},
		{
			"SKU too long",
			CreateInput{
				SKU:  strings.Repeat("A", 65),
				Name: "Product",
			},
		},
		{
			"blank name",
			CreateInput{
				SKU:  "SKU-002",
				Name: "   ",
			},
		},
		{
			"negative threshold",
			CreateInput{
				SKU:               "SKU-003",
				Name:              "Product",
				LowStockThreshold: -1,
			},
		},
		{
			"oversized description",
			CreateInput{
				SKU:         "SKU-004",
				Name:        "Product",
				Description: ptr(strings.Repeat("x", 2001)),
			},
		},
		{
			"NUL in description",
			CreateInput{
				SKU:         "SKU-005",
				Name:        "Product",
				Description: ptr("bad\x00description"),
			},
		},
		{
			"invalid UTF-8",
			CreateInput{
				SKU:  "SKU-006",
				Name: string([]byte{0xff}),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreate(tc.input)

			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf(
					"expected invalid input, got %v",
					err,
				)
			}
		})
	}
}

func TestValidateUpdateProduct(t *testing.T) {
	if err := ValidateUpdate(UpdateInput{
		Name:              "Updated name",
		LowStockThreshold: 0,
	}); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}

	if err := ValidateUpdate(UpdateInput{
		Name:              "Product",
		LowStockThreshold: -5,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative threshold accepted: %v", err)
	}
}

func ptr(value string) *string {
	return &value
}
