package inventory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

func TestInventoryAdjustmentPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	u, err := url.Parse(dsn)
	if err != nil ||
		strings.Trim(u.Path, "/") != "stockguard_test" {
		t.Fatal("TEST_DATABASE_URL must point to stockguard_test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		45*time.Second,
	)
	defer cancel()

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}

	name := hex.EncodeToString(suffix[:])

	var actorID, productID, orderID int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		if productID > 0 {
			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM inventory_movements WHERE product_id=$1",
				productID,
			); err != nil {
				t.Errorf("cleanup movements: %v", err)
			}

			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM order_items WHERE product_id=$1",
				productID,
			); err != nil {
				t.Errorf("cleanup order items: %v", err)
			}
		}

		if orderID > 0 {
			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM orders WHERE id=$1",
				orderID,
			); err != nil {
				t.Errorf("cleanup order: %v", err)
			}
		}

		if productID > 0 {
			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM inventory_balances WHERE product_id=$1",
				productID,
			); err != nil {
				t.Errorf("cleanup balance: %v", err)
			}

			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM products WHERE id=$1",
				productID,
			); err != nil {
				t.Errorf("cleanup product: %v", err)
			}
		}

		if actorID > 0 {
			if _, err := db.ExecContext(
				cleanupCtx,
				"DELETE FROM users WHERE id=$1",
				actorID,
			); err != nil {
				t.Errorf("cleanup actor: %v", err)
			}
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
	`, "inv_"+name).Scan(&actorID)

	if err != nil {
		t.Fatal(err)
	}

	product, err := products.NewStore(db).Create(
		ctx,
		products.CreateInput{
			SKU:               "INV-" + name,
			Name:              "Inventory fixture",
			LowStockThreshold: 5,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	productID = product.ID
	store := NewStore(db)

	// Initial quantity is recorded as an adjustment
	// because BE-3 creates products with zero stock.
	received, err := store.Adjust(
		ctx,
		productID,
		actorID,
		10,
		"Received ten items for testing",
	)
	if err != nil {
		t.Fatal(err)
	}

	if received.Balance.OnHand != 10 ||
		received.Balance.Available != 10 ||
		received.Movement.DeltaOnHand != 10 ||
		received.Movement.ActorUserID != actorID {
		t.Fatalf(
			"incorrect initial adjustment: %+v",
			received,
		)
	}

	// Two concurrent withdrawals cannot both spend
	// the same available stock.
	outcomes := make(chan error, 2)

	for i := 0; i < 2; i++ {
		go func() {
			_, err := store.Adjust(
				ctx,
				productID,
				actorID,
				-7,
				"Concurrent stock reduction test",
			)
			outcomes <- err
		}()
	}

	success, conflicts := 0, 0

	for i := 0; i < 2; i++ {
		err := <-outcomes

		switch {
		case err == nil:
			success++

		case errors.Is(
			err,
			ErrInsufficientAvailable,
		):
			conflicts++

		default:
			t.Fatalf(
				"unexpected concurrent result: %v",
				err,
			)
		}
	}

	if success != 1 || conflicts != 1 {
		t.Fatalf(
			"concurrency: successes=%d conflicts=%d",
			success,
			conflicts,
		)
	}

	b, err := store.GetBalance(ctx, productID)

	if err != nil ||
		b.OnHand != 3 ||
		b.Reserved != 0 {
		t.Fatalf(
			"concurrency corrupted balance: %+v, %v",
			b, err,
		)
	}

	// Restore stock for reservation testing.
	_, err = store.Adjust(
		ctx,
		productID,
		actorID,
		7,
		"Restore stock for reservation test",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create an order reservation fixture, including
	// its matching inventory movement, atomically.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (created_by)
		VALUES ($1)
		RETURNING id
	`, actorID).Scan(&orderID)

	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO order_items (
			order_id,
			product_id,
			quantity
		)
		VALUES ($1, $2, 4)
	`, orderID, productID)

	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE inventory_balances
		SET
			reserved = reserved + 4,
			updated_at = statement_timestamp()
		WHERE product_id = $1
	`, productID)

	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (
			product_id,
			order_id,
			actor_user_id,
			movement_type,
			delta_on_hand,
			delta_reserved
		)
		VALUES ($1, $2, $3, 'reserve', 0, 4)
	`, productID, orderID, actorID)

	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Available = 10 - 4 = 6.
	// Taking 7 units must fail.
	_, err = store.Adjust(
		ctx,
		productID,
		actorID,
		-7,
		"Attempt to overspend reserved units",
	)

	if !errors.Is(err, ErrInsufficientAvailable) {
		t.Fatalf(
			"reserved stock consumed: %v",
			err,
		)
	}

	// Taking exactly 6 units is allowed.
	result, err := store.Adjust(
		ctx,
		productID,
		actorID,
		-6,
		"Reduce only unreserved stock",
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Balance.OnHand != 4 ||
		result.Balance.Reserved != 4 ||
		result.Balance.Available != 0 {
		t.Fatalf(
			"incorrect reserved boundary: %+v",
			result.Balance,
		)
	}

	// Integer overflow must not change the balance.
	_, err = store.Adjust(
		ctx,
		productID,
		actorID,
		math.MaxInt64,
		"Quantity overflow scenario",
	)

	if !errors.Is(err, ErrQuantityOverflow) {
		t.Fatalf("overflow permitted: %v", err)
	}

	page, err := store.ListMovements(
		ctx, productID, 2, 0,
	)

	if err != nil ||
		len(page.Items) != 2 ||
		!page.HasMore {
		t.Fatalf(
			"movement pagination: %+v %v",
			page, err,
		)
	}

	// Reconcile final quantities with the audit trail.
	var totalOnHand, totalReserved int64

	err = db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(delta_on_hand), 0),
			COALESCE(SUM(delta_reserved), 0)
		FROM inventory_movements
		WHERE product_id = $1
	`, productID).Scan(
		&totalOnHand,
		&totalReserved,
	)

	if err != nil ||
		totalOnHand != 4 ||
		totalReserved != 4 {
		t.Fatalf(
			"audit reconciliation failed: on_hand=%d reserved=%d err=%v",
			totalOnHand,
			totalReserved,
			err,
		)
	}

	// Inactive products must reject adjustments.
	_, err = db.ExecContext(ctx, `
		UPDATE products
		SET is_active = FALSE
		WHERE id = $1
	`, productID)

	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Adjust(
		ctx,
		productID,
		actorID,
		1,
		"Change inactive product stock",
	)

	if !errors.Is(err, ErrProductInactive) {
		t.Fatalf(
			"inactive product adjusted: %v",
			err,
		)
	}
}
