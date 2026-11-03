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

func TestOrderQueriesPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("TEST_DATABASE_URL must target stockguard_test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(seed[:])

	var staffA, staffB, adminID int64
	productIDs := make([]int64, 0, 2)

	// Cleanup only the fixtures created by this test.
	defer func() {
		cctx, done := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer done()

		run := func(query string, args ...any) {
			if _, err := db.ExecContext(
				cctx, query, args...,
			); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}

		if staffA > 0 && staffB > 0 {
			run(`
				DELETE FROM idempotency_records
				WHERE user_id = $1 OR user_id = $2
			`, staffA, staffB)
		}

		for _, id := range productIDs {
			run(
				"DELETE FROM inventory_movements WHERE product_id=$1",
				id,
			)
		}

		if staffA > 0 && staffB > 0 {
			run(`
				DELETE FROM order_items
				WHERE order_id IN (
					SELECT id FROM orders
					WHERE created_by=$1 OR created_by=$2
				)
			`, staffA, staffB)

			run(`
				DELETE FROM orders
				WHERE created_by=$1 OR created_by=$2
			`, staffA, staffB)
		}

		for _, id := range productIDs {
			run(
				"DELETE FROM inventory_balances WHERE product_id=$1",
				id,
			)
			run(
				"DELETE FROM products WHERE id=$1",
				id,
			)
		}

		for _, id := range []int64{staffA, staffB, adminID} {
			if id > 0 {
				run("DELETE FROM users WHERE id=$1", id)
			}
		}
	}()

	createUser := func(label, role string) int64 {
		t.Helper()

		var id int64

		err := db.QueryRowContext(ctx, `
			INSERT INTO users (
				username, password_hash, role
			)
			VALUES (
				$1, 'TEST_ONLY_NOT_REAL_HASH', $2
			)
			RETURNING id
		`, "read_"+suffix+"_"+label, role).Scan(&id)

		if err != nil {
			t.Fatal(err)
		}

		return id
	}

	staffA = createUser("a", "staff")
	staffB = createUser("b", "staff")
	adminID = createUser("admin", "admin")

	productStore := products.NewStore(db)
	inventoryStore := inventory.NewStore(db)
	service := NewService(db)

	addProduct := func(label string) int64 {
		t.Helper()

		p, err := productStore.Create(
			ctx,
			products.CreateInput{
				SKU:  "READ-" + suffix + "-" + label,
				Name: "Query test product " + label,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		productIDs = append(productIDs, p.ID)

		_, err = inventoryStore.Adjust(
			ctx,
			p.ID,
			adminID,
			20,
			"Receive inventory for order query test",
		)
		if err != nil {
			t.Fatal(err)
		}

		return p.ID
	}

	firstProduct := addProduct("A")
	secondProduct := addProduct("B")

	counter := 0

	createOrder := func(
		creator int64,
		items []Item,
	) Order {
		t.Helper()

		counter++

		key := fmt.Sprintf(
			"be5c0000-0000-4000-8000-%012d",
			counter,
		)

		result, err := service.Create(
			ctx, creator, key, items,
		)
		if err != nil {
			t.Fatal(err)
		}

		return result.Order
	}

	// Order 1: Staff A, still reserved, two items.
	reservedOrder := createOrder(
		staffA,
		[]Item{
			{ProductID: secondProduct, Quantity: 3},
			{ProductID: firstProduct, Quantity: 2},
		},
	)

	// Order 2: Staff B, fulfilled by Admin.
	fulfilledOrder := createOrder(
		staffB,
		[]Item{
			{ProductID: secondProduct, Quantity: 4},
		},
	)

	_, err = service.Transition(
		ctx, adminID,
		fulfilledOrder.ID, ActionFulfill,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Order 3: Staff A, cancelled by Admin.
	cancelledOrder := createOrder(
		staffA,
		[]Item{
			{ProductID: firstProduct, Quantity: 1},
		},
	)

	_, err = service.Transition(
		ctx, adminID,
		cancelledOrder.ID, ActionCancel,
	)
	if err != nil {
		t.Fatal(err)
	}

	var movementsBefore int64

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE product_id IN ($1, $2)
	`, firstProduct, secondProduct).Scan(&movementsBefore)
	if err != nil {
		t.Fatal(err)
	}

	// Staff A may see their two orders only.
	pageA, err := service.List(
		ctx, staffA, false, "", 20, 0,
	)

	if err != nil || len(pageA.Items) != 2 {
		t.Fatalf("staff A list: %+v %v", pageA, err)
	}

	for _, summary := range pageA.Items {
		if summary.CreatedBy != staffA {
			t.Fatal("staff A can see another user's order")
		}
	}

	// Staff B may see only their fulfilled order.
	pageB, err := service.List(
		ctx, staffB, false, "", 20, 0,
	)

	if err != nil ||
		len(pageB.Items) != 1 ||
		pageB.Items[0].ID != fulfilledOrder.ID ||
		pageB.Items[0].Status != "fulfilled" {
		t.Fatalf("staff B list: %+v %v", pageB, err)
	}

	reservedPage, err := service.List(
		ctx, staffA, false, "reserved", 20, 0,
	)

	if err != nil ||
		len(reservedPage.Items) != 1 ||
		reservedPage.Items[0].ID != reservedOrder.ID ||
		reservedPage.Items[0].ItemCount != 2 {
		t.Fatalf(
			"reserved status filter: %+v %v",
			reservedPage, err,
		)
	}

	cancelledPage, err := service.List(
		ctx, staffA, false, "cancelled", 20, 0,
	)

	if err != nil ||
		len(cancelledPage.Items) != 1 ||
		cancelledPage.Items[0].ID != cancelledOrder.ID {
		t.Fatalf(
			"cancelled status filter: %+v %v",
			cancelledPage, err,
		)
	}

	emptyPage, err := service.List(
		ctx, staffA, false, "fulfilled", 20, 0,
	)

	if err != nil ||
		len(emptyPage.Items) != 0 ||
		emptyPage.HasMore {
		t.Fatalf("empty status filter: %+v %v", emptyPage, err)
	}

	// Staff A pagination must not duplicate results.
	firstPage, err := service.List(
		ctx, staffA, false, "", 1, 0,
	)
	if err != nil ||
		len(firstPage.Items) != 1 ||
		!firstPage.HasMore {
		t.Fatalf("first page: %+v %v", firstPage, err)
	}

	secondPage, err := service.List(
		ctx, staffA, false, "", 1, 1,
	)
	if err != nil ||
		len(secondPage.Items) != 1 ||
		secondPage.HasMore ||
		firstPage.Items[0].ID == secondPage.Items[0].ID {
		t.Fatalf("second page: %+v %v", secondPage, err)
	}

	// Admin can see all three orders.
	adminPage, err := service.List(
		ctx, adminID, true, "", 100, 0,
	)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[int64]bool{}

	for _, summary := range adminPage.Items {
		seen[summary.ID] = true
	}

	for _, id := range []int64{
		reservedOrder.ID,
		fulfilledOrder.ID,
		cancelledOrder.ID,
	} {
		if !seen[id] {
			t.Fatalf("admin cannot see order %d", id)
		}
	}

	// Detailed order must include sorted line items.
	detail, err := service.Get(
		ctx, staffA, false, reservedOrder.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if detail.Status != "reserved" ||
		detail.CreatedBy != staffA ||
		len(detail.Items) != 2 ||
		detail.Items[0].ProductID != firstProduct ||
		detail.Items[0].Quantity != 2 ||
		detail.Items[1].ProductID != secondProduct ||
		detail.Items[1].Quantity != 3 {
		t.Fatalf("invalid order detail: %+v", detail)
	}

	// Another Staff's order must be hidden.
	_, err = service.Get(
		ctx, staffA, false, fulfilledOrder.ID,
	)

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("cross-user read allowed: %v", err)
	}

	// Admin can read the same order including its status.
	adminDetail, err := service.Get(
		ctx, adminID, true, fulfilledOrder.ID,
	)

	if err != nil ||
		adminDetail.Status != "fulfilled" ||
		adminDetail.FulfilledAt == nil ||
		len(adminDetail.Items) != 1 {
		t.Fatalf("admin order detail: %+v %v", adminDetail, err)
	}

	_, err = service.Get(
		ctx, adminID, true, 9223372036854775807,
	)

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("nonexistent order: %v", err)
	}

	_, err = service.List(
		ctx, adminID, true, "pending", 20, 0,
	)

	if !errors.Is(err, ErrInvalidOrderQuery) {
		t.Fatalf("invalid status accepted: %v", err)
	}

	// Reading orders must not modify inventory.
	firstBalance, err := inventoryStore.GetBalance(
		ctx, firstProduct,
	)
	if err != nil {
		t.Fatal(err)
	}

	secondBalance, err := inventoryStore.GetBalance(
		ctx, secondProduct,
	)
	if err != nil {
		t.Fatal(err)
	}

	if firstBalance.OnHand != 20 ||
		firstBalance.Reserved != 2 ||
		secondBalance.OnHand != 16 ||
		secondBalance.Reserved != 3 {
		t.Fatalf(
			"unexpected inventory: first=%+v second=%+v",
			firstBalance, secondBalance,
		)
	}

	var movementsAfter int64

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE product_id IN ($1, $2)
	`, firstProduct, secondProduct).Scan(&movementsAfter)

	if err != nil || movementsBefore != movementsAfter {
		t.Fatalf(
			"read changed movement history: before=%d after=%d err=%v",
			movementsBefore, movementsAfter, err,
		)
	}
}
