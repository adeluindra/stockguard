package inventory

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

	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

func TestAdjustmentIdempotencyPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	u, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(u.Path, "/") != "stockguard_test" {
		t.Fatal("test must use stockguard_test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(raw[:])

	var actorID, productID int64

	defer func() {
		cctx, done := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer done()

		cleanup := func(query string, args ...any) {
			if _, err := db.ExecContext(
				cctx, query, args...,
			); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}

		if actorID > 0 {
			cleanup(
				"DELETE FROM inventory_adjustment_idempotency WHERE actor_user_id=$1",
				actorID,
			)
		}

		if productID > 0 {
			cleanup(
				"DELETE FROM inventory_movements WHERE product_id=$1",
				productID,
			)
			cleanup(
				"DELETE FROM inventory_balances WHERE product_id=$1",
				productID,
			)
			cleanup(
				"DELETE FROM products WHERE id=$1",
				productID,
			)
		}

		if actorID > 0 {
			cleanup(
				"DELETE FROM users WHERE id=$1",
				actorID,
			)
		}
	}()

	err = db.QueryRowContext(ctx, `
		INSERT INTO users (
			username, password_hash, role
		)
		VALUES (
			$1, 'TEST_ONLY_NOT_REAL_HASH', 'admin'
		)
		RETURNING id
	`, "idem_"+suffix).Scan(&actorID)

	if err != nil {
		t.Fatal(err)
	}

	product, err := products.NewStore(db).Create(
		ctx,
		products.CreateInput{
			SKU:  "IDEM-" + suffix,
			Name: "Idempotency test product",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	productID = product.ID
	store := NewStore(db)

	const key1 = "11111111-1111-4111-8111-111111111111"
	const key2 = "22222222-2222-4222-8222-222222222222"
	const key3 = "33333333-3333-4333-8333-333333333333"
	const reason = "Supplier stock received for testing"

	first, err := store.AdjustIdempotent(
		ctx, productID, actorID,
		10, reason, key1,
	)
	if err != nil ||
		first.Replayed ||
		first.Balance.OnHand != 10 {
		t.Fatalf("first adjustment: %+v %v", first, err)
	}

	replay, err := store.AdjustIdempotent(
		ctx, productID, actorID,
		10, reason, strings.ToUpper(key1),
	)

	if err != nil ||
		!replay.Replayed ||
		replay.Movement.ID != first.Movement.ID ||
		replay.Balance.OnHand != 10 {
		t.Fatalf("first replay: %+v %v", replay, err)
	}

	// Reusing a key for a different adjustment is forbidden.
	_, err = store.AdjustIdempotent(
		ctx, productID, actorID,
		11, reason, key1,
	)
	if !errors.Is(err, ErrAdjustmentKeyConflict) {
		t.Fatalf("changed payload accepted: %v", err)
	}

	// Another legitimate internal change occurs afterwards.
	_, err = store.Adjust(
		ctx,
		productID, actorID,
		2, "Separate stock adjustment for testing",
	)
	if err != nil {
		t.Fatal(err)
	}

	// A retry must retain its ORIGINAL response snapshot.
	replay, err = store.AdjustIdempotent(
		ctx, productID, actorID,
		10, reason, key1,
	)

	if err != nil ||
		!replay.Replayed ||
		replay.Balance.OnHand != 10 {
		t.Fatalf("stale response replay: %+v %v", replay, err)
	}

	current, err := store.GetBalance(ctx, productID)
	if err != nil || current.OnHand != 12 {
		t.Fatalf("replay changed current balance: %+v %v",
			current, err)
	}

	// Two requests using one key must produce
	// exactly one -4 stock movement.
	type outcome struct {
		result IdempotentAdjustment
		err    error
	}

	outcomes := make(chan outcome, 2)

	for i := 0; i < 2; i++ {
		go func() {
			result, err := store.AdjustIdempotent(
				ctx,
				productID,
				actorID,
				-4,
				"Concurrent inventory reduction test",
				key2,
			)
			outcomes <- outcome{result, err}
		}()
	}

	newCount := 0
	replayedCount := 0
	var movementID int64

	for i := 0; i < 2; i++ {
		result := <-outcomes

		if result.err != nil {
			t.Fatalf("concurrent adjustment: %v", result.err)
		}

		if movementID == 0 {
			movementID = result.result.Movement.ID
		} else if movementID != result.result.Movement.ID {
			t.Fatal("same key produced two movements")
		}

		if result.result.Replayed {
			replayedCount++
		} else {
			newCount++
		}
	}

	if newCount != 1 || replayedCount != 1 {
		t.Fatalf("new=%d replayed=%d",
			newCount, replayedCount)
	}

	current, err = store.GetBalance(ctx, productID)
	if err != nil || current.OnHand != 8 {
		t.Fatalf("concurrent stock change: %+v %v",
			current, err)
	}

	// A failed adjustment must not consume its key.
	_, err = store.AdjustIdempotent(
		ctx, productID, actorID,
		-100, "Failed reduction due to insufficient stock",
		key3,
	)

	if !errors.Is(err, ErrInsufficientAvailable) {
		t.Fatalf("expected insufficient stock: %v", err)
	}

	// The same unused key can subsequently succeed.
	final, err := store.AdjustIdempotent(
		ctx, productID, actorID,
		-3, "Successful reduction after failed attempt",
		key3,
	)

	if err != nil ||
		final.Replayed ||
		final.Balance.OnHand != 5 {
		t.Fatalf("key was consumed by failure: %+v %v",
			final, err)
	}

	var keyCount, movementCount int64

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_adjustment_idempotency
		WHERE actor_user_id = $1
	`, actorID).Scan(&keyCount)
	if err != nil {
		t.Fatal(err)
	}

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE product_id = $1
		  AND movement_type = 'adjustment'
	`, productID).Scan(&movementCount)
	if err != nil {
		t.Fatal(err)
	}

	// 3 idempotent adjustments and one trusted
	// internal adjustment.
	if keyCount != 3 || movementCount != 4 {
		t.Fatalf("keys=%d movements=%d",
			keyCount, movementCount)
	}

	var netChange int64

	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(delta_on_hand), 0)
		FROM inventory_movements
		WHERE product_id = $1
	`, productID).Scan(&netChange)
	if err != nil || netChange != 5 {
		t.Fatalf("movement reconciliation: %d %v",
			netChange, err)
	}

	// Even after deactivation, replaying an existing
	// successful request must still work.
	if err := products.NewStore(db).Deactivate(
		ctx, productID,
	); err != nil {
		t.Fatal(err)
	}

	replay, err = store.AdjustIdempotent(
		ctx, productID, actorID,
		10, reason, key1,
	)

	if err != nil ||
		!replay.Replayed ||
		replay.Balance.OnHand != 10 {
		t.Fatalf("replay after deactivation: %+v %v",
			replay, err)
	}

	_, err = store.AdjustIdempotent(
		ctx, productID, actorID,
		1, "New adjustment on inactive product",
		"44444444-4444-4444-8444-444444444444",
	)

	if !errors.Is(err, ErrProductInactive) {
		t.Fatalf("inactive product adjusted: %v", err)
	}

	current, err = store.GetBalance(ctx, productID)
	if err != nil || current.OnHand != 5 {
		t.Fatalf("final balance changed: %+v %v",
			current, err)
	}
}
