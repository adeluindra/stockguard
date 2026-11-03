package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidPage = errors.New("invalid dashboard pagination")

type StockTotals struct {
	ActiveProducts   int64 `json:"active_products"`
	InactiveProducts int64 `json:"inactive_products"`
	LowStockProducts int64 `json:"low_stock_products"`
	OnHand           int64 `json:"on_hand"`
	Reserved         int64 `json:"reserved"`
	Available        int64 `json:"available"`
}

type OrderTotals struct {
	Total     int64 `json:"total"`
	Reserved  int64 `json:"reserved"`
	Fulfilled int64 `json:"fulfilled"`
	Cancelled int64 `json:"cancelled"`
}

type Summary struct {
	Inventory StockTotals `json:"inventory"`
	Orders    OrderTotals `json:"orders"`
}

type LowStockProduct struct {
	ProductID         int64  `json:"product_id"`
	SKU               string `json:"sku"`
	Name              string `json:"name"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
	OnHand            int64  `json:"on_hand"`
	Reserved          int64  `json:"reserved"`
	Available         int64  `json:"available"`
}

type Activity struct {
	ID            int64     `json:"id"`
	ProductID     int64     `json:"product_id"`
	SKU           string    `json:"sku"`
	ProductName   string    `json:"product_name"`
	OrderID       *int64    `json:"order_id"`
	ActorUserID   int64     `json:"actor_user_id"`
	ActorUsername string    `json:"actor_username"`
	MovementType  string    `json:"movement_type"`
	DeltaOnHand   int64     `json:"delta_on_hand"`
	DeltaReserved int64     `json:"delta_reserved"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type Page[T any] struct {
	Items   []T  `json:"items"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func ValidatePage(limit, offset int) error {
	if limit < 1 || limit > 100 ||
		offset < 0 || offset > 1000000 {
		return ErrInvalidPage
	}

	return nil
}

// GetSummary calculates inventory and order totals
// using one PostgreSQL statement and one MVCC snapshot.
//
// Inventory totals include only active products.
// Order totals include all historical orders.
func (s *Store) GetSummary(
	ctx context.Context,
) (Summary, error) {
	const query = `
		WITH inventory_stats AS (
			SELECT
				COUNT(*) FILTER (
					WHERE p.is_active
				) AS active_products,

				COUNT(*) FILTER (
					WHERE NOT p.is_active
				) AS inactive_products,

				COUNT(*) FILTER (
					WHERE p.is_active
					  AND b.on_hand - b.reserved
						  <= p.low_stock_threshold
				) AS low_stock_products,

				COALESCE(SUM(b.on_hand) FILTER (
					WHERE p.is_active
				), 0) AS on_hand,

				COALESCE(SUM(b.reserved) FILTER (
					WHERE p.is_active
				), 0) AS reserved,

				COALESCE(SUM(
					b.on_hand - b.reserved
				) FILTER (
					WHERE p.is_active
				), 0) AS available

			FROM products AS p
			JOIN inventory_balances AS b
				ON b.product_id = p.id
		),
		order_stats AS (
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (
					WHERE status = 'reserved'
				) AS reserved,
				COUNT(*) FILTER (
					WHERE status = 'fulfilled'
				) AS fulfilled,
				COUNT(*) FILTER (
					WHERE status = 'cancelled'
				) AS cancelled
			FROM orders
		)
		SELECT
			i.active_products,
			i.inactive_products,
			i.low_stock_products,
			i.on_hand,
			i.reserved,
			i.available,
			o.total,
			o.reserved,
			o.fulfilled,
			o.cancelled
		FROM inventory_stats AS i
		CROSS JOIN order_stats AS o
	`

	var summary Summary

	err := s.db.QueryRowContext(ctx, query).Scan(
		&summary.Inventory.ActiveProducts,
		&summary.Inventory.InactiveProducts,
		&summary.Inventory.LowStockProducts,
		&summary.Inventory.OnHand,
		&summary.Inventory.Reserved,
		&summary.Inventory.Available,
		&summary.Orders.Total,
		&summary.Orders.Reserved,
		&summary.Orders.Fulfilled,
		&summary.Orders.Cancelled,
	)

	if err != nil {
		return Summary{},
			fmt.Errorf("read dashboard summary: %w", err)
	}

	return summary, nil
}

// ListLowStock returns active products whose available
// stock is at or below the configured threshold.
//
// Products with less available stock appear first.
func (s *Store) ListLowStock(
	ctx context.Context,
	limit, offset int,
) (Page[LowStockProduct], error) {
	if err := ValidatePage(limit, offset); err != nil {
		return Page[LowStockProduct]{}, err
	}

	const query = `
		SELECT
			p.id,
			p.sku,
			p.name,
			p.low_stock_threshold,
			b.on_hand,
			b.reserved,
			b.on_hand - b.reserved AS available
		FROM products AS p
		JOIN inventory_balances AS b
			ON b.product_id = p.id
		WHERE p.is_active = TRUE
		  AND b.on_hand - b.reserved
			  <= p.low_stock_threshold
		ORDER BY
			available ASC,
			p.id ASC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		limit+1,
		offset,
	)
	if err != nil {
		return Page[LowStockProduct]{},
			fmt.Errorf("list low-stock products: %w", err)
	}
	defer rows.Close()

	page := Page[LowStockProduct]{
		Items:  make([]LowStockProduct, 0, limit),
		Limit:  limit,
		Offset: offset,
	}

	for rows.Next() {
		var product LowStockProduct

		if err := rows.Scan(
			&product.ProductID,
			&product.SKU,
			&product.Name,
			&product.LowStockThreshold,
			&product.OnHand,
			&product.Reserved,
			&product.Available,
		); err != nil {
			return Page[LowStockProduct]{},
				fmt.Errorf("scan low-stock product: %w", err)
		}

		if len(page.Items) == limit {
			page.HasMore = true
			break
		}

		page.Items = append(page.Items, product)
	}

	if err := rows.Err(); err != nil {
		return Page[LowStockProduct]{},
			fmt.Errorf("iterate low-stock products: %w", err)
	}

	return page, nil
}

// ListActivity reads committed inventory movements.
// It includes activity for inactive products as well,
// because their historical audit trail must remain visible.
func (s *Store) ListActivity(
	ctx context.Context,
	limit, offset int,
) (Page[Activity], error) {
	if err := ValidatePage(limit, offset); err != nil {
		return Page[Activity]{}, err
	}

	const query = `
		SELECT
			m.id,
			m.product_id,
			p.sku,
			p.name,
			m.order_id,
			m.actor_user_id,
			u.username,
			m.movement_type,
			m.delta_on_hand,
			m.delta_reserved,
			m.reason,
			m.created_at
		FROM inventory_movements AS m
		JOIN products AS p
			ON p.id = m.product_id
		JOIN users AS u
			ON u.id = m.actor_user_id
		ORDER BY
			m.created_at DESC,
			m.id DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		limit+1,
		offset,
	)
	if err != nil {
		return Page[Activity]{},
			fmt.Errorf("list dashboard activity: %w", err)
	}
	defer rows.Close()

	page := Page[Activity]{
		Items:  make([]Activity, 0, limit),
		Limit:  limit,
		Offset: offset,
	}

	for rows.Next() {
		var activity Activity
		var orderID sql.NullInt64
		var reason sql.NullString

		err := rows.Scan(
			&activity.ID,
			&activity.ProductID,
			&activity.SKU,
			&activity.ProductName,
			&orderID,
			&activity.ActorUserID,
			&activity.ActorUsername,
			&activity.MovementType,
			&activity.DeltaOnHand,
			&activity.DeltaReserved,
			&reason,
			&activity.CreatedAt,
		)
		if err != nil {
			return Page[Activity]{},
				fmt.Errorf("scan dashboard activity: %w", err)
		}

		if len(page.Items) == limit {
			page.HasMore = true
			break
		}

		if orderID.Valid {
			value := orderID.Int64
			activity.OrderID = &value
		}

		if reason.Valid {
			value := reason.String
			activity.Reason = &value
		}

		page.Items = append(page.Items, activity)
	}

	if err := rows.Err(); err != nil {
		return Page[Activity]{},
			fmt.Errorf("iterate dashboard activity: %w", err)
	}

	return page, nil
}
