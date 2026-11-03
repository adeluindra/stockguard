package dashboard

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/adeluindra/stockguard/apps/api/internal/inventory"
	"github.com/adeluindra/stockguard/apps/api/internal/orders"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

func TestDashboardPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}

	parsed, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(parsed.Path, "/") != "stockguard_test" {
		t.Fatal("must use stockguard_test database")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	dashboardStore := NewStore(db)

	before, err := dashboardStore.GetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatal(err)
	}

	suffix := hex.EncodeToString(randomBytes[:])

	var actorID int64
	productIDs := make([]int64, 0, 3)

	// Cleanup touches only the temporary test fixtures.
	defer func() {
		cctx, done := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer done()

		if actorID == 0 {
			return
		}

		queries := []string{
			"DELETE FROM idempotency_records WHERE user_id=$1",
			"DELETE FROM inventory_movements WHERE actor_user_id=$1",
			`DELETE FROM order_items
			 WHERE order_id IN (
				SELECT id FROM orders WHERE created_by=$1
			 )`,
			"DELETE FROM orders WHERE created_by=$1",
		}

		for _, query := range queries {
			if _, err := db.ExecContext(
				cctx,
				query,
				actorID,
			); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}

		for _, id := range productIDs {
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
			username,
			password_hash,
			role
		)
		VALUES (
			$1,
			'TEST_ONLY_NOT_REAL_HASH',
			'admin'
		)
		RETURNING id
	`, "dashboard_"+suffix).Scan(&actorID)

	if err != nil {
		t.Fatal(err)
	}

	productStore := products.NewStore(db)
	inventoryStore := inventory.NewStore(db)
	orderStore := orders.NewService(db)

	makeProduct := func(
		label string,
		threshold int64,
		quantity int64,
	) int64 {
		t.Helper()

		product, err := productStore.Create(
			ctx,
			products.CreateInput{
				SKU:               "DASH-" + suffix + "-" + label,
				Name:              "Dashboard fixture " + label,
				LowStockThreshold: threshold,
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
			"Receive stock for dashboard test",
		)
		if err != nil {
			t.Fatal(err)
		}

		return product.ID
	}

	// A: available 3, threshold 5 -> low-stock.
	a := makeProduct("A", 5, 3)

	// B: starts with 10; reserve 9 later.
	b := makeProduct("B", 2, 10)

	// C: will be deactivated after completing orders.
	c := makeProduct("C", 0, 5)

	counter := 0

	createOrder := func(
		items []orders.Item,
	) orders.Order {
		t.Helper()

		counter++

		key := fmt.Sprintf(
			"00000000-0000-4000-8000-%012d",
			counter,
		)

		result, err := orderStore.Create(
			ctx,
			actorID,
			key,
			items,
		)
		if err != nil {
			t.Fatal(err)
		}

		return result.Order
	}

	// Reserved: B available 10 - 9 = 1 -> low-stock.
	_ = createOrder([]orders.Item{
		{ProductID: b, Quantity: 9},
	})

	// Fulfilled: C stock 5 -> 3.
	fulfilled := createOrder([]orders.Item{
		{ProductID: c, Quantity: 2},
	})

	_, err = orderStore.Transition(
		ctx,
		actorID,
		fulfilled.ID,
		orders.ActionFulfill,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Cancelled: reservation released without
	// reducing on-hand stock.
	cancelled := createOrder([]orders.Item{
		{ProductID: c, Quantity: 1},
	})

	_, err = orderStore.Transition(
		ctx,
		actorID,
		cancelled.ID,
		orders.ActionCancel,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Historical activities should remain visible
	// even after product C becomes inactive.
	if err := productStore.Deactivate(ctx, c); err != nil {
		t.Fatal(err)
	}

	after, err := dashboardStore.GetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Compare against baseline, not an assumed empty DB.
	checks := []struct {
		name string
		got  int64
		want int64
	}{
		{
			"active products",
			after.Inventory.ActiveProducts -
				before.Inventory.ActiveProducts,
			2,
		},
		{
			"inactive products",
			after.Inventory.InactiveProducts -
				before.Inventory.InactiveProducts,
			1,
		},
		{
			"low-stock products",
			after.Inventory.LowStockProducts -
				before.Inventory.LowStockProducts,
			2,
		},
		{
			"on hand",
			after.Inventory.OnHand -
				before.Inventory.OnHand,
			13,
		},
		{
			"reserved",
			after.Inventory.Reserved -
				before.Inventory.Reserved,
			9,
		},
		{
			"available",
			after.Inventory.Available -
				before.Inventory.Available,
			4,
		},
		{
			"total orders",
			after.Orders.Total - before.Orders.Total,
			3,
		},
		{
			"reserved orders",
			after.Orders.Reserved - before.Orders.Reserved,
			1,
		},
		{
			"fulfilled orders",
			after.Orders.Fulfilled - before.Orders.Fulfilled,
			1,
		},
		{
			"cancelled orders",
			after.Orders.Cancelled - before.Orders.Cancelled,
			1,
		},
	}

	for _, tc := range checks {
		if tc.got != tc.want {
			t.Errorf(
				"%s: got %d, want %d",
				tc.name,
				tc.got,
				tc.want,
			)
		}
	}

	// At least two fixtures are low-stock,
	// so the first page with limit 1 must have more.
	firstPage, err := dashboardStore.ListLowStock(
		ctx, 1, 0,
	)
	if err != nil ||
		len(firstPage.Items) != 1 ||
		!firstPage.HasMore {
		t.Fatalf(
			"low-stock pagination: %+v %v",
			firstPage,
			err,
		)
	}

	foundA := false
	foundB := false
	foundInactive := false

	// Scan paginated results; pre-existing test data
	// must not affect the fixture search.
	for offset := 0; offset <= 10000; offset += 100 {
		page, err := dashboardStore.ListLowStock(
			ctx, 100, offset,
		)
		if err != nil {
			t.Fatal(err)
		}

		for _, item := range page.Items {
			switch item.ProductID {
			case a:
				foundA = item.Available == 3
			case b:
				foundB = item.Available == 1
			case c:
				foundInactive = true
			}
		}

		if !page.HasMore {
			break
		}
	}

	if !foundA || !foundB || foundInactive {
		t.Fatalf(
			"low-stock results: A=%t B=%t inactive=%t",
			foundA,
			foundB,
			foundInactive,
		)
	}

	// 3 adjustments + 3 reserve movements
	// + 1 fulfill + 1 release = 8 movements.
	var movementsBefore int64

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE actor_user_id = $1
	`, actorID).Scan(&movementsBefore)

	if err != nil || movementsBefore != 8 {
		t.Fatalf(
			"movement fixture count: %d %v",
			movementsBefore, err,
		)
	}

	activityPage, err := dashboardStore.ListActivity(
		ctx, 2, 0,
	)
	if err != nil ||
		len(activityPage.Items) != 2 ||
		!activityPage.HasMore {
		t.Fatalf(
			"activity pagination: %+v %v",
			activityPage, err,
		)
	}

	seen := make(map[int64]bool)

	for offset := 0; offset <= 10000; offset += 100 {
		page, err := dashboardStore.ListActivity(
			ctx, 100, offset,
		)
		if err != nil {
			t.Fatal(err)
		}

		for _, item := range page.Items {
			if item.ActorUserID == actorID {
				if seen[item.ID] {
					t.Fatal("duplicate movement in pagination")
				}

				seen[item.ID] = true

				if item.ActorUsername !=
					"dashboard_"+suffix {
					t.Fatal("incorrect movement actor")
				}
			}
		}

		if !page.HasMore {
			break
		}
	}

	if len(seen) != 8 {
		t.Fatalf(
			"activity history incomplete: got %d movements",
			len(seen),
		)
	}

	var movementsAfter int64

	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory_movements
		WHERE actor_user_id = $1
	`, actorID).Scan(&movementsAfter)

	if err != nil || movementsAfter != movementsBefore {
		t.Fatalf(
			"dashboard read modified movements: before=%d after=%d err=%v",
			movementsBefore,
			movementsAfter,
			err,
		)
	}
}
