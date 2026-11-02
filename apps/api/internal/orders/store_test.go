package orders

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeIdempotencyKey(t *testing.T) {
	const upper = "3B6D0429-82C1-4B4F-B20F-2DA29F1B8170"
	const lower = "3b6d0429-82c1-4b4f-b20f-2da29f1b8170"

	got, err := NormalizeIdempotencyKey(upper)
	if err != nil || got != lower {
		t.Fatalf("unexpected normalized key: %q, %v", got, err)
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"3b6d0429-82c1-4b4f-b20f-2da29f1b817Z",
		" " + lower,
		lower + " ",
		strings.Replace(lower, "-", "", 1),
	}

	for _, key := range invalid {
		if _, err := NormalizeIdempotencyKey(key); !errors.Is(
			err, ErrInvalidOrder,
		) {
			t.Fatalf("invalid key accepted: %q", key)
		}
	}
}

func TestNormalizeOrderItems(t *testing.T) {
	items, err := normalizeItems([]Item{
		{ProductID: 5, Quantity: 2},
		{ProductID: 1, Quantity: 3},
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 2 ||
		items[0].ProductID != 1 ||
		items[1].ProductID != 5 {
		t.Fatalf("items not sorted: %+v", items)
	}

	invalid := [][]Item{
		nil,
		{},
		{{ProductID: 0, Quantity: 1}},
		{{ProductID: 1, Quantity: 0}},
		{{ProductID: 1, Quantity: -1}},
		{
			{ProductID: 1, Quantity: 1},
			{ProductID: 1, Quantity: 2},
		},
	}

	tooMany := make([]Item, MaxOrderItems+1)
	for i := range tooMany {
		tooMany[i] = Item{
			ProductID: int64(i + 1),
			Quantity:  1,
		}
	}
	invalid = append(invalid, tooMany)

	for _, input := range invalid {
		if _, err := normalizeItems(input); !errors.Is(
			err, ErrInvalidOrder,
		) {
			t.Fatalf("invalid items accepted: %+v", input)
		}
	}
}

func TestCanonicalPayloadHash(t *testing.T) {
	first, err := normalizeItems([]Item{
		{ProductID: 2, Quantity: 4},
		{ProductID: 1, Quantity: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := normalizeItems([]Item{
		{ProductID: 1, Quantity: 3},
		{ProductID: 2, Quantity: 4},
	})
	if err != nil {
		t.Fatal(err)
	}

	a, err := payloadHash(first)
	if err != nil {
		t.Fatal(err)
	}

	b, err := payloadHash(second)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(a[:], b[:]) {
		t.Fatal("same logical order generated different hashes")
	}

	changed, err := normalizeItems([]Item{
		{ProductID: 1, Quantity: 3},
		{ProductID: 2, Quantity: 5},
	})
	if err != nil {
		t.Fatal(err)
	}

	c, err := payloadHash(changed)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(a[:], c[:]) {
		t.Fatal("different order generated identical hash")
	}
}
