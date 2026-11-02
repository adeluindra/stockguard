package orders

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

	"github.com/adeluindra/stockguard/apps/api/internal/inventory"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

func TestOrderReservationsPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("test must use stockguard_test database")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(randomBytes[:])
	var actorID int64
	productIDs := make([]int64, 0, 4)

	// Cleanup occurs only in the isolated test database.
	defer func() {
		cleanupCtx, done := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer done()

		if actorID == 0 {
			return
		}

		statements := []string{
			"DELETE FROM idempotency_records WHERE user_id=$1",
			"DELETE FROM inventory_movements WHERE actor_user_id=$1",
			`DELETE FROM order_items
			 WHERE order_id IN (
			     SELECT id FROM orders WHERE created_by=$1
			 )`,
			"DELETE FROM orders WHERE created_by=$1",
		}

		for _, query := range statements {
			if _, err := db.ExecContext(
				cleanupCtx, query, actorID,
			); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}

		for _, id := range productIDs {
			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM inventory_balances WHERE product_id=$1",
				id,
			); err != nil {
				t.Errorf("cleanup balance: %v", err)
			}

			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM products WHERE id=$1",
				id,
			); err != nil {
				t.Errorf("cleanup product: %v", err)
			}
		}

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE id=$1",
			actorID,
		); err != nil {
			t.Errorf("cleanup actor: %v", err)
		}
	}()

	err = db.QueryRowContext(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role
		)
		VALUES ($1, 'TEST_ONLY_NOT_REAL_HASH', 'staff')
		RETURNING id
	`, "order_"+suffix).Scan(&actorID)

	if err != nil {
		t.Fatal(err)
	}

	productStore := products.NewStore(db)
	inventoryStore := inventory.NewStore(db)
	orderStore := NewService(db)

	makeProduct := func(label string, quantity int64) int64 {
		t.Helper()

		product, err := productStore.Create(
			ctx,
			products.CreateInput{
				SKU:  "ORD-" + suffix + "-" + label,
				Name: "Order fixture " + label,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		productIDs = append(productIDs, product.ID)

		_, err = inventoryStore.Adjust(
			ctx,
			product.ID,
			actorID,
			quantity,
			"Receive initial test inventory",
		)

		if err != nil {
			t.Fatal(err)
		}

		return product.ID
	}

	firstID := makeProduct("A", 8)
	secondID := makeProduct("B", 5)
	thirdID := makeProduct("C", 4)
	fourthID := makeProduct("D", 4)

	const firstKey = "11111111-1111-4111-8111-111111111111"

	items := []Item{
		{ProductID: secondID, Quantity: 2},
		{ProductID: firstID, Quantity: 3},
	}

	created, err := orderStore.Create(
		ctx, actorID, firstKey, items,
	)
	if err != nil {
		t.Fatalf("create multi-item order: %v", err)
	}

	if created.Replayed ||
		created.Order.ID <= 0 ||
		created.Order.Status != "reserved" ||
		len(created.Order.Items) != 2 {
		t.Fatalf("invalid created order: %+v", created)
	}

	checkReserved := func(
		productID int64,
		want int64,
	) {
		t.Helper()

		balance, err := inventoryStore.GetBalance(
			ctx, productID,
		)
		if err != nil {
			t.Fatal(err)
		}

		if balance.Reserved != want ||
			balance.Available != balance.OnHand-want {
			t.Fatalf(
				"product %d: balance=%+v want reserved=%d",
				productID, balance, want,
			)
		}
	}

	checkReserved(firstID, 3)
	checkReserved(secondID, 2)

	// Equivalent payload in different item order:
	// return the same order without another reservation.
	replayed, err := orderStore.Create(
		ctx,
		actorID,
		strings.ToUpper(firstKey),
		[]Item{
			{ProductID: firstID, Quantity: 3},
			{ProductID: secondID, Quantity: 2},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !replayed.Replayed ||
		replayed.Order.ID != created.Order.ID {
		t.Fatalf(
			"idempotent replay failed: %+v",
			replayed,
		)
	}

	checkReserved(firstID, 3)
	checkReserved(secondID, 2)

	// Same key with a different payload must be rejected.
	_, err = orderStore.Create(
		ctx,
		actorID,
		firstKey,
		[]Item{
			{ProductID: firstID, Quantity: 1},
		},
	)

	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf(
			"key reuse with different payload: %v",
			err,
		)
	}

	// One unavailable item must prevent the entire order.
	_, err = orderStore.Create(
		ctx,
		actorID,
		"22222222-2222-4222-8222-222222222222",
		[]Item{
			{ProductID: firstID, Quantity: 1},
			{ProductID: secondID, Quantity: 100},
		},
	)

	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf(
			"insufficient stock not rejected: %v",
			err,
		)
	}

	checkReserved(firstID, 3)
	checkReserved(secondID, 2)

	// Two simultaneous requests using the same key:
	// only one new order may be created.
	type outcome struct {
		result CreateResult
		err    error
	}

	sameKeyResults := make(chan outcome, 2)

	for i := 0; i < 2; i++ {
		go func() {
			result, err := orderStore.Create(
				ctx,
				actorID,
				"33333333-3333-4333-8333-333333333333",
				[]Item{
					{ProductID: thirdID, Quantity: 3},
				},
			)

			sameKeyResults <- outcome{
				result: result,
				err:    err,
			}
		}()
	}

	var sameKeyOrderID int64
	newCount := 0
	replayCount := 0

	for i := 0; i < 2; i++ {
		result := <-sameKeyResults

		if result.err != nil {
			t.Fatalf(
				"concurrent same-key request: %v",
				result.err,
			)
		}

		if sameKeyOrderID == 0 {
			sameKeyOrderID = result.result.Order.ID
		} else if sameKeyOrderID != result.result.Order.ID {
			t.Fatal("same key created different orders")
		}

		if result.result.Replayed {
			replayCount++
		} else {
			newCount++
		}
	}

	if newCount != 1 || replayCount != 1 {
		t.Fatalf(
			"same-key result: new=%d replay=%d",
			newCount, replayCount,
		)
	}

	checkReserved(thirdID, 3)

	// Different keys competing for 4 units of stock.
	// Each order wants 3; only one can succeed.
	differentKeys := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	}

	competingResults := make(chan error, 2)

	for _, key := range differentKeys {
		go func(k string) {
			_, err := orderStore.Create(
				ctx,
				actorID,
				k,
				[]Item{
					{ProductID: fourthID, Quantity: 3},
				},
			)
			competingResults <- err
		}(key)
	}

	successCount := 0
	insufficientCount := 0

	for i := 0; i < 2; i++ {
		err := <-competingResults

		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrInsufficientStock):
			insufficientCount++
		default:
			t.Fatalf(
				"unexpected competing request result: %v",
				err,
			)
		}
	}

	if successCount != 1 || insufficientCount != 1 {
		t.Fatalf(
			"competing orders: success=%d insufficient=%d",
			successCount, insufficientCount,
		)
	}

	checkReserved(fourthID, 3)

	// 3 successful orders:
	// first multi-item, same-key order, competing order.
	var orderCount, keyCount, reserveMovements int

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM orders
		WHERE created_by = $1
	`, actorID).Scan(&orderCount)
	if err != nil {
		t.Fatal(err)
	}

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM idempotency_records
		WHERE user_id = $1
	`, actorID).Scan(&keyCount)
	if err != nil {
		t.Fatal(err)
	}

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE actor_user_id = $1
		  AND movement_type = 'reserve'
	`, actorID).Scan(&reserveMovements)
	if err != nil {
		t.Fatal(err)
	}

	if orderCount != 3 ||
		keyCount != 3 ||
		reserveMovements != 4 {
		t.Fatalf(
			"orders=%d keys=%d reserve movements=%d",
			orderCount, keyCount, reserveMovements,
		)
	}

	// Every balance must reconcile with its movement history.
	for _, productID := range productIDs {
		balance, err := inventoryStore.GetBalance(
			ctx, productID,
		)
		if err != nil {
			t.Fatal(err)
		}

		var onHandTotal, reservedTotal int64

		err = db.QueryRowContext(ctx, `
			SELECT
				COALESCE(SUM(delta_on_hand), 0),
				COALESCE(SUM(delta_reserved), 0)
			FROM inventory_movements
			WHERE product_id = $1
		`, productID).Scan(
			&onHandTotal,
			&reservedTotal,
		)

		if err != nil {
			t.Fatal(err)
		}

		if balance.OnHand != onHandTotal ||
			balance.Reserved != reservedTotal {
			t.Fatalf(
				"product %d reconciliation mismatch: balance=%+v movements=(%d,%d)",
				productID,
				balance,
				onHandTotal,
				reservedTotal,
			)
		}
	}
}
