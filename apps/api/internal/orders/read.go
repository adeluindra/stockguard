package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidOrderQuery = errors.New("invalid order query")

type OrderSummary struct {
	ID          int64      `json:"id"`
	CreatedBy   int64      `json:"created_by"`
	Status      string     `json:"status"`
	ItemCount   int64      `json:"item_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	FulfilledAt *time.Time `json:"fulfilled_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
}

type OrderPage struct {
	Items   []OrderSummary `json:"items"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	HasMore bool           `json:"has_more"`
}

func ValidateOrderQuery(
	status string,
	limit, offset int,
) error {
	if limit < 1 || limit > 100 ||
		offset < 0 || offset > 1000000 {
		return ErrInvalidOrderQuery
	}

	switch status {
	case "", "reserved", "fulfilled", "cancelled":
		return nil
	default:
		return ErrInvalidOrderQuery
	}
}

// Get reads the order and its items in one consistent
// PostgreSQL snapshot.
//
// Admin may read any order. Staff may read only orders
// where created_by equals their authenticated user ID.
func (s *Service) Get(
	ctx context.Context,
	requesterID int64,
	isAdmin bool,
	orderID int64,
) (Order, error) {
	if requesterID <= 0 || orderID <= 0 {
		return Order{}, ErrInvalidOrderQuery
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return Order{}, fmt.Errorf(
			"begin read order transaction: %w", err,
		)
	}
	defer tx.Rollback()

	var visibleID int64

	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM orders
		WHERE id = $1
		  AND ($2::boolean OR created_by = $3)
	`, orderID, isAdmin, requesterID).Scan(&visibleID)

	if errors.Is(err, sql.ErrNoRows) {
		// Do not reveal whether another Staff owns
		// the requested order.
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf(
			"authorize order read: %w", err,
		)
	}

	order, err := loadOrder(ctx, tx, visibleID)
	if err != nil {
		return Order{}, err
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf(
			"commit read order transaction: %w", err,
		)
	}

	return order, nil
}

// List uses a single query and returns only summaries.
// Detailed line items are available through Get.
func (s *Service) List(
	ctx context.Context,
	requesterID int64,
	isAdmin bool,
	status string,
	limit, offset int,
) (OrderPage, error) {
	if requesterID <= 0 {
		return OrderPage{}, ErrInvalidOrderQuery
	}

	if err := ValidateOrderQuery(
		status, limit, offset,
	); err != nil {
		return OrderPage{}, err
	}

	const query = `
		SELECT
			o.id,
			o.created_by,
			o.status,
			o.created_at,
			o.updated_at,
			o.fulfilled_at,
			o.cancelled_at,
			(
				SELECT COUNT(*)
				FROM order_items AS oi
				WHERE oi.order_id = o.id
			) AS item_count
		FROM orders AS o
		WHERE ($1::boolean OR o.created_by = $2)
		  AND ($3::text = '' OR o.status = $3)
		ORDER BY o.created_at DESC, o.id DESC
		LIMIT $4 OFFSET $5
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		isAdmin,
		requesterID,
		status,
		limit+1,
		offset,
	)
	if err != nil {
		return OrderPage{}, fmt.Errorf(
			"list orders: %w", err,
		)
	}
	defer rows.Close()

	page := OrderPage{
		Items:  make([]OrderSummary, 0, limit),
		Limit:  limit,
		Offset: offset,
	}

	for rows.Next() {
		var o OrderSummary
		var fulfilled sql.NullTime
		var cancelled sql.NullTime

		err := rows.Scan(
			&o.ID,
			&o.CreatedBy,
			&o.Status,
			&o.CreatedAt,
			&o.UpdatedAt,
			&fulfilled,
			&cancelled,
			&o.ItemCount,
		)
		if err != nil {
			return OrderPage{}, fmt.Errorf(
				"scan order summary: %w", err,
			)
		}

		if len(page.Items) == limit {
			page.HasMore = true
			break
		}

		if fulfilled.Valid {
			value := fulfilled.Time
			o.FulfilledAt = &value
		}

		if cancelled.Valid {
			value := cancelled.Time
			o.CancelledAt = &value
		}

		page.Items = append(page.Items, o)
	}

	if err := rows.Err(); err != nil {
		return OrderPage{}, fmt.Errorf(
			"iterate order summaries: %w", err,
		)
	}

	return page, nil
}
