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

func TestOrderTransitionsPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)

	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("test must use stockguard_test")
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

	var seed [8]byte

	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(seed[:])

	var actorID int64
	var ids []int64

	defer func() {
		cctx, done := context.WithTimeout(
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
			"DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE created_by=$1)",
			"DELETE FROM orders WHERE created_by=$1",
		}

		for _, query := range statements {
			if _, err := db.ExecContext(
				cctx, query, actorID,
			); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}

		for _, id := range ids {
			if _, err := db.ExecContext(
				cctx,
				"DELETE FROM inventory_balances WHERE product_id=$1",
				id,
			); err != nil {
				t.Errorf("cleanup balance: %v", err)
			}

			if _, err := db.ExecContext(
				cctx,
				"DELETE FROM products WHERE id=$1",
				id,
			); err != nil {
				t.Errorf("cleanup product: %v", err)
			}
		}

		if _, err := db.ExecContext(
			cctx,
			"DELETE FROM users WHERE id=$1",
			actorID,
		); err != nil {
			t.Errorf("cleanup actor: %v", err)
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
	`, "term_"+suffix).Scan(&actorID)

	if err != nil {
		t.Fatal(err)
	}

	productStore := products.NewStore(db)
	inventoryStore := inventory.NewStore(db)
	service := NewService(db)

	addProduct := func(label string) int64 {
		t.Helper()

		p, err := productStore.Create(
			ctx,
			products.CreateInput{
				SKU:  "TERM-" + suffix + "-" + label,
				Name: "Terminal test " + label,
			},
		)

		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, p.ID)

		_, err = inventoryStore.Adjust(
			ctx,
			p.ID,
			actorID,
			10,
			"Receive stock for terminal test",
		)

		if err != nil {
			t.Fatal(err)
		}

		return p.ID
	}

	a := addProduct("A")
	b := addProduct("B")
	c := addProduct("C")

	serial := 0

	create := func(items []Item) Order {
		t.Helper()

		serial++

		key := fmt.Sprintf(
			"00000000-0000-4000-8000-%012d",
			serial,
		)

		result, err := service.Create(
			ctx, actorID, key, items,
		)
		if err != nil {
			t.Fatalf("reserve: %v", err)
		}

		return result.Order
	}

	checkBalance := func(
		productID, onHand, reserved int64,
	) {
		t.Helper()

		balance, err := inventoryStore.GetBalance(
			ctx, productID,
		)
		if err != nil {
			t.Fatal(err)
		}

		if balance.OnHand != onHand ||
			balance.Reserved != reserved ||
			balance.Available != onHand-reserved {
			t.Fatalf(
				"product %d balance %+v, expected onHand=%d reserved=%d",
				productID, balance, onHand, reserved,
			)
		}
	}

	countMovements := func(
		orderID int64,
		movementType string,
	) int {
		t.Helper()

		var count int

		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM inventory_movements
			WHERE order_id = $1
			  AND movement_type = $2
		`, orderID, movementType).Scan(&count)

		if err != nil {
			t.Fatal(err)
		}

		return count
	}

	// Case 1: Fulfill two items.
	fulfilledOrder := create([]Item{
		{ProductID: b, Quantity: 4},
		{ProductID: a, Quantity: 3},
	})

	result, err := service.Transition(
		ctx,
		actorID,
		fulfilledOrder.ID,
		ActionFulfill,
	)

	if err != nil ||
		result.Replayed ||
		result.Order.Status != "fulfilled" ||
		result.Order.FulfilledAt == nil ||
		result.Order.CancelledAt != nil {
		t.Fatalf("fulfill: %+v, %v", result, err)
	}

	checkBalance(a, 7, 0)
	checkBalance(b, 6, 0)

	if countMovements(
		fulfilledOrder.ID, "fulfill",
	) != 2 {
		t.Fatal("missing fulfillment movements")
	}

	// Repeating fulfillment must not subtract stock again.
	replay, err := service.Transition(
		ctx,
		actorID,
		fulfilledOrder.ID,
		ActionFulfill,
	)

	if err != nil ||
		!replay.Replayed ||
		replay.Order.ID != fulfilledOrder.ID {
		t.Fatalf("fulfill replay: %+v %v", replay, err)
	}

	checkBalance(a, 7, 0)
	checkBalance(b, 6, 0)

	if countMovements(
		fulfilledOrder.ID, "fulfill",
	) != 2 {
		t.Fatal("replay inserted movement")
	}

	_, err = service.Transition(
		ctx, actorID, fulfilledOrder.ID, ActionCancel,
	)

	if !errors.Is(err, ErrOrderNotReserved) {
		t.Fatalf("cancel after fulfill: %v", err)
	}

	// Case 2: Cancel two items.
	cancelledOrder := create([]Item{
		{ProductID: a, Quantity: 2},
		{ProductID: b, Quantity: 3},
	})

	result, err = service.Transition(
		ctx,
		actorID,
		cancelledOrder.ID,
		ActionCancel,
	)

	if err != nil ||
		result.Replayed ||
		result.Order.Status != "cancelled" ||
		result.Order.CancelledAt == nil ||
		result.Order.FulfilledAt != nil {
		t.Fatalf("cancel: %+v %v", result, err)
	}

	// On-hand stock must remain unchanged.
	checkBalance(a, 7, 0)
	checkBalance(b, 6, 0)

	if countMovements(
		cancelledOrder.ID, "release",
	) != 2 {
		t.Fatal("missing release movements")
	}

	replay, err = service.Transition(
		ctx,
		actorID,
		cancelledOrder.ID,
		ActionCancel,
	)

	if err != nil || !replay.Replayed {
		t.Fatalf("cancel replay: %+v %v", replay, err)
	}

	if countMovements(
		cancelledOrder.ID, "release",
	) != 2 {
		t.Fatal("duplicate release movement")
	}

	_, err = service.Transition(
		ctx, actorID, cancelledOrder.ID, ActionFulfill,
	)

	if !errors.Is(err, ErrOrderNotReserved) {
		t.Fatalf("fulfill after cancellation: %v", err)
	}

	// Case 3: Fulfill and cancel compete concurrently.
	raceOrder := create([]Item{
		{ProductID: a, Quantity: 2},
	})

	type outcome struct {
		action TransitionAction
		err    error
	}

	results := make(chan outcome, 2)

	for _, action := range []TransitionAction{
		ActionFulfill,
		ActionCancel,
	} {
		go func(action TransitionAction) {
			_, err := service.Transition(
				ctx, actorID, raceOrder.ID, action,
			)
			results <- outcome{
				action: action,
				err:    err,
			}
		}(action)
	}

	success := 0
	conflict := 0
	winningAction := TransitionAction("")

	for i := 0; i < 2; i++ {
		result := <-results

		switch {
		case result.err == nil:
			success++
			winningAction = result.action

		case errors.Is(
			result.err,
			ErrOrderNotReserved,
		):
			conflict++

		default:
			t.Fatalf(
				"unexpected race error: %v",
				result.err,
			)
		}
	}

	if success != 1 || conflict != 1 {
		t.Fatalf(
			"race: successes=%d conflicts=%d",
			success, conflict,
		)
	}

	if winningAction == ActionFulfill {
		checkBalance(a, 5, 0)

		if countMovements(raceOrder.ID, "fulfill") != 1 ||
			countMovements(raceOrder.ID, "release") != 0 {
			t.Fatal("incorrect fulfillment race movements")
		}
	} else {
		checkBalance(a, 7, 0)

		if countMovements(raceOrder.ID, "release") != 1 ||
			countMovements(raceOrder.ID, "fulfill") != 0 {
			t.Fatal("incorrect cancellation race movements")
		}
	}

	// Case 4: Force item B of a multi-item transaction
	// to fail, then verify item A was rolled back.
	rollbackOrder := create([]Item{
		{ProductID: b, Quantity: 2},
		{ProductID: c, Quantity: 2},
	})

	// Deliberately corrupt only this temporary test fixture.
	_, err = db.ExecContext(ctx, `
		UPDATE inventory_balances
		SET reserved = 0
		WHERE product_id = $1
	`, c)

	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Transition(
		ctx, actorID, rollbackOrder.ID, ActionCancel,
	)

	if !errors.Is(err, ErrReservationInconsistent) {
		t.Fatalf(
			"expected failed second item: %v",
			err,
		)
	}

	checkBalance(b, 6, 2)

	if countMovements(rollbackOrder.ID, "release") != 0 {
		t.Fatal("partial release movement committed")
	}

	var storedStatus string

	err = db.QueryRowContext(ctx, `
		SELECT status
		FROM orders
		WHERE id = $1
	`, rollbackOrder.ID).Scan(&storedStatus)

	if err != nil {
		t.Fatal(err)
	}

	if storedStatus != "reserved" {
		t.Fatal("status changed despite rollback")
	}

	// Repair the deliberately corrupted test fixture.
	_, err = db.ExecContext(ctx, `
		UPDATE inventory_balances
		SET reserved = 2
		WHERE product_id = $1
	`, c)

	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Transition(
		ctx, actorID, rollbackOrder.ID, ActionCancel,
	)

	if err != nil {
		t.Fatal(err)
	}

	checkBalance(b, 6, 0)
	checkBalance(c, 10, 0)

	// Case 5: Reconcile every balance against its movements.
	for _, id := range ids {
		balance, err := inventoryStore.GetBalance(
			ctx, id,
		)
		if err != nil {
			t.Fatal(err)
		}

		var onHand, reserved int64

		err = db.QueryRowContext(ctx, `
			SELECT
				COALESCE(SUM(delta_on_hand), 0),
				COALESCE(SUM(delta_reserved), 0)
			FROM inventory_movements
			WHERE product_id = $1
		`, id).Scan(&onHand, &reserved)

		if err != nil {
			t.Fatal(err)
		}

		if balance.OnHand != onHand ||
			balance.Reserved != reserved {
			t.Fatalf(
				"movement mismatch for %d: %+v vs %d,%d",
				id, balance, onHand, reserved,
			)
		}
	}

	_, err = service.Transition(
		ctx, actorID, -1, ActionCancel,
	)

	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid ID: %v", err)
	}

	_, err = service.Transition(
		ctx,
		actorID,
		9223372036854775807,
		ActionCancel,
	)

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("missing order: %v", err)
	}
}
