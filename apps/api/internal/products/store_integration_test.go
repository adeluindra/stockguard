package products

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProductLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)

	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal(
			"TEST_DATABASE_URL must target stockguard_test",
		)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	var randomBytes [8]byte

	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatal(err)
	}

	sku := "BE3-" +
		hex.EncodeToString(randomBytes[:])

	store := NewStore(db)

	p, err := store.Create(ctx, CreateInput{
		SKU:               sku,
		Name:              "Test Product",
		LowStockThreshold: 5,
	})

	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	defer func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer done()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM inventory_balances WHERE product_id = $1",
			p.ID,
		); err != nil {
			t.Errorf("cleanup balance: %v", err)
		}

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM products WHERE id = $1",
			p.ID,
		); err != nil {
			t.Errorf("cleanup product: %v", err)
		}
	}()

	// Product creation must initialize zero balance.
	if p.ID <= 0 ||
		p.OnHand != 0 ||
		p.Reserved != 0 ||
		p.Available != 0 ||
		!p.IsActive {
		t.Fatalf(
			"incorrect new product: %+v",
			p,
		)
	}

	// Case-insensitive SKU uniqueness.
	_, err = store.Create(ctx, CreateInput{
		SKU:  strings.ToLower(sku),
		Name: "Duplicate",
	})

	if !errors.Is(err, ErrSKUExists) {
		t.Fatalf(
			"duplicate SKU was accepted: %v",
			err,
		)
	}

	// Read product and initial balance.
	got, err := store.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.SKU != sku || got.Available != 0 {
		t.Fatal("GET returned incorrect product")
	}

	// Read through paginated listing.
	page, err := store.List(ctx, 100, 0, false)
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, item := range page.Items {
		if item.ID == p.ID {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("created product not present in list")
	}

	// Update metadata without changing SKU or inventory.
	updated, err := store.Update(
		ctx,
		p.ID,
		UpdateInput{
			Name:              "Updated Product",
			LowStockThreshold: 10,
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if updated.Name != "Updated Product" ||
		updated.LowStockThreshold != 10 ||
		updated.SKU != sku ||
		updated.OnHand != 0 ||
		updated.Reserved != 0 {
		t.Fatalf(
			"incorrect product update: %+v",
			updated,
		)
	}

	// Deactivation must preserve the record.
	if err := store.Deactivate(ctx, p.ID); err != nil {
		t.Fatal(err)
	}

	// Repeated deactivation should be safe.
	if err := store.Deactivate(ctx, p.ID); err != nil {
		t.Fatalf(
			"repeated deactivation failed: %v",
			err,
		)
	}

	inactive, err := store.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if inactive.IsActive {
		t.Fatal("product still active")
	}

	// Inactive products cannot be modified.
	_, err = store.Update(
		ctx,
		p.ID,
		UpdateInput{Name: "Should Fail"},
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf(
			"inactive product update: %v",
			err,
		)
	}

	// Admin listing must still be able to find it.
	page, err = store.List(ctx, 100, 0, true)
	if err != nil {
		t.Fatal(err)
	}

	found = false

	for _, item := range page.Items {
		if item.ID == p.ID {
			found = true
			break
		}
	}

	if !found {
		t.Fatal(
			"inactive product missing from admin list",
		)
	}

	// Database record and inventory must still exist.
	var balances int

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_balances
		WHERE product_id = $1
	`, p.ID).Scan(&balances)

	if err != nil || balances != 1 {
		t.Fatalf(
			"inventory balance missing: %v",
			err,
		)
	}
}
