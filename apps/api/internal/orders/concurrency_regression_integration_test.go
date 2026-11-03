package orders

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/adeluindra/stockguard/apps/api/internal/inventory"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

func TestCrossWorkflowConcurrencyPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	u, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(u.Path, "/") != "stockguard_test" {
		t.Fatal("test must target stockguard_test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(12)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(seed[:])
	var actorID int64
	var productIDs []int64

	defer func() {
		cctx, done := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer done()

		cleanup := func(query string, args ...any) {
			if _, err := db.ExecContext(
				cctx, query, args...,
			); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}

		if actorID > 0 {
			cleanup(
				"DELETE FROM idempotency_records WHERE user_id=$1",
				actorID,
			)
		}

		for _, id := range productIDs {
			cleanup(
				"DELETE FROM inventory_movements WHERE product_id=$1",
				id,
			)
		}

		if actorID > 0 {
			cleanup(`
				DELETE FROM order_items
				WHERE order_id IN (
					SELECT id FROM orders WHERE created_by=$1
				)
			`, actorID)

			cleanup(
				"DELETE FROM orders WHERE created_by=$1",
				actorID,
			)
		}

		for _, id := range productIDs {
			cleanup(
				"DELETE FROM inventory_balances WHERE product_id=$1",
				id,
			)
			cleanup(
				"DELETE FROM products WHERE id=$1",
				id,
			)
		}

		if actorID > 0 {
			cleanup("DELETE FROM users WHERE id=$1", actorID)
		}
	}()

	err = db.QueryRowContext(ctx, `
		INSERT INTO users (
			username, password_hash, role
		)
		VALUES ($1, 'TEST_ONLY_NOT_REAL_HASH', 'admin')
		RETURNING id
	`, "regression_"+suffix).Scan(&actorID)
	if err != nil {
		t.Fatal(err)
	}

	productStore := products.NewStore(db)
	inventoryStore := inventory.NewStore(db)
	orderStore := NewService(db)

	addProduct := func(label string) int64 {
		t.Helper()

		p, err := productStore.Create(
			ctx,
			products.CreateInput{
				SKU:  "RACE-" + suffix + "-" + label,
				Name: "Concurrency fixture " + label,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		productIDs = append(productIDs, p.ID)

		_, err = inventoryStore.Adjust(
			ctx, p.ID, actorID, 10,
			"Initial stock for concurrency test",
		)
		if err != nil {
			t.Fatal(err)
		}

		return p.ID
	}

	serial := 0
	nextKey := func() string {
		serial++
		return fmt.Sprintf(
			"00000000-0000-4000-8000-%012d",
			serial,
		)
	}

	createOrder := func(items []Item) Order {
		t.Helper()

		result, err := orderStore.Create(
			ctx, actorID, nextKey(), items,
		)
		if err != nil {
			t.Fatal(err)
		}

		return result.Order
	}

	checkBalance := func(
		id, expectedOnHand, expectedReserved int64,
	) {
		t.Helper()

		b, err := inventoryStore.GetBalance(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		if b.OnHand != expectedOnHand ||
			b.Reserved != expectedReserved ||
			b.Available != expectedOnHand-expectedReserved {
			t.Fatalf(
				"balance mismatch product=%d got=%+v want on_hand=%d reserved=%d",
				id, b,
				expectedOnHand, expectedReserved,
			)
		}
	}

	type outcome struct {
		name string
		err  error
	}

	// CASE 1: Two multi-item orders compete for
	// the same inventory, with reversed input order.
	p1 := addProduct("A")
	p2 := addProduct("B")

	key1, key2 := nextKey(), nextKey()
	results := make(chan outcome, 2)
	start := make(chan struct{})

	go func() {
		<-start
		_, err := orderStore.Create(
			ctx, actorID, key1,
			[]Item{
				{ProductID: p1, Quantity: 7},
				{ProductID: p2, Quantity: 7},
			},
		)
		results <- outcome{"order1", err}
	}()

	go func() {
		<-start
		_, err := orderStore.Create(
			ctx, actorID, key2,
			[]Item{
				{ProductID: p2, Quantity: 7},
				{ProductID: p1, Quantity: 7},
			},
		)
		results <- outcome{"order2", err}
	}()

	close(start)

	success, insufficient := 0, 0

	for i := 0; i < 2; i++ {
		result := <-results

		switch {
		case result.err == nil:
			success++

		case errors.Is(result.err, ErrInsufficientStock):
			insufficient++

		default:
			t.Fatalf(
				"unexpected %s result: %v",
				result.name, result.err,
			)
		}
	}

	if success != 1 || insufficient != 1 {
		t.Fatalf(
			"multi-item contention: success=%d insufficient=%d",
			success, insufficient,
		)
	}

	checkBalance(p1, 10, 7)
	checkBalance(p2, 10, 7)

	// CASE 2: New order competes with stock reduction.
	// One operation must fail rather than oversell.
	p3 := addProduct("C")
	key3 := nextKey()

	results = make(chan outcome, 2)
	start = make(chan struct{})

	go func() {
		<-start
		_, err := orderStore.Create(
			ctx, actorID, key3,
			[]Item{
				{ProductID: p3, Quantity: 7},
			},
		)
		results <- outcome{"create", err}
	}()

	go func() {
		<-start
		_, err := inventoryStore.Adjust(
			ctx, p3, actorID, -4,
			"Concurrent stock reduction attempt",
		)
		results <- outcome{"adjust", err}
	}()

	close(start)

	createSucceeded := false
	adjustSucceeded := false

	for i := 0; i < 2; i++ {
		result := <-results

		switch result.name {
		case "create":
			if result.err == nil {
				createSucceeded = true
			} else if !errors.Is(
				result.err, ErrInsufficientStock,
			) {
				t.Fatalf("create failed unexpectedly: %v", result.err)
			}

		case "adjust":
			if result.err == nil {
				adjustSucceeded = true
			} else if !errors.Is(
				result.err, inventory.ErrInsufficientAvailable,
			) {
				t.Fatalf("adjust failed unexpectedly: %v", result.err)
			}
		}
	}

	if createSucceeded == adjustSucceeded {
		t.Fatal("expected exactly one successful operation")
	}

	if createSucceeded {
		checkBalance(p3, 10, 7)
	} else {
		checkBalance(p3, 6, 0)
	}

	// CASE 3: Fulfillment competes with a withdrawal
	// larger than available stock.
	p4 := addProduct("D")
	fulfillOrder := createOrder([]Item{
		{ProductID: p4, Quantity: 7},
	})

	results = make(chan outcome, 2)
	start = make(chan struct{})

	go func() {
		<-start
		_, err := orderStore.Transition(
			ctx, actorID, fulfillOrder.ID, ActionFulfill,
		)
		results <- outcome{"fulfill", err}
	}()

	go func() {
		<-start
		_, err := inventoryStore.Adjust(
			ctx, p4, actorID, -4,
			"Withdrawal competing with fulfillment",
		)
		results <- outcome{"adjust", err}
	}()

	close(start)

	for i := 0; i < 2; i++ {
		result := <-results

		if result.name == "fulfill" && result.err != nil {
			t.Fatalf("fulfill: %v", result.err)
		}

		if result.name == "adjust" &&
			!errors.Is(
				result.err, inventory.ErrInsufficientAvailable,
			) {
			t.Fatalf(
				"reserved stock was not protected: %v",
				result.err,
			)
		}
	}

	checkBalance(p4, 3, 0)

	// CASE 4: Cancellation competes with adjustment.
	// Both valid serial execution orders are accepted.
	p5 := addProduct("E")
	cancelOrder := createOrder([]Item{
		{ProductID: p5, Quantity: 7},
	})

	results = make(chan outcome, 2)
	start = make(chan struct{})

	go func() {
		<-start
		_, err := orderStore.Transition(
			ctx, actorID, cancelOrder.ID, ActionCancel,
		)
		results <- outcome{"cancel", err}
	}()

	go func() {
		<-start
		_, err := inventoryStore.Adjust(
			ctx, p5, actorID, -5,
			"Withdrawal competing with cancellation",
		)
		results <- outcome{"adjust", err}
	}()

	close(start)

	withdrawalSucceeded := false

	for i := 0; i < 2; i++ {
		result := <-results

		if result.name == "cancel" && result.err != nil {
			t.Fatalf("cancel: %v", result.err)
		}

		if result.name == "adjust" {
			switch {
			case result.err == nil:
				withdrawalSucceeded = true

			case errors.Is(
				result.err,
				inventory.ErrInsufficientAvailable,
			):
				// Adjustment ran before cancellation.

			default:
				t.Fatalf(
					"unexpected adjustment result: %v",
					result.err,
				)
			}
		}
	}

	if withdrawalSucceeded {
		checkBalance(p5, 5, 0)
	} else {
		checkBalance(p5, 10, 0)
	}

	cancelled, err := orderStore.Get(
		ctx, actorID, true, cancelOrder.ID,
	)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("incorrect terminal status: %+v %v",
			cancelled, err)
	}

	// CASE 5: Every committed balance must reconcile
	// against all recorded movements.
	for _, id := range productIDs {
		b, err := inventoryStore.GetBalance(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		var totalOnHand, totalReserved int64

		err = db.QueryRowContext(ctx, `
			SELECT
				COALESCE(SUM(delta_on_hand), 0),
				COALESCE(SUM(delta_reserved), 0)
			FROM inventory_movements
			WHERE product_id = $1
		`, id).Scan(&totalOnHand, &totalReserved)

		if err != nil {
			t.Fatal(err)
		}

		if b.OnHand != totalOnHand ||
			b.Reserved != totalReserved {
			t.Fatalf(
				"movement reconciliation failed for product %d: balance=%+v movements=(%d,%d)",
				id, b, totalOnHand, totalReserved,
			)
		}
	}
}
