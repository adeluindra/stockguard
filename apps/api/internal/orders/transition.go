package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrInvalidTransition = errors.New(
		"invalid order transition",
	)
	ErrOrderNotFound = errors.New(
		"order not found",
	)
	ErrOrderNotReserved = errors.New(
		"order is already in a terminal state",
	)
	ErrReservationInconsistent = errors.New(
		"order reservation and inventory balance are inconsistent",
	)
)

type TransitionAction string

const (
	ActionFulfill TransitionAction = "fulfill"
	ActionCancel  TransitionAction = "cancel"
)

type TransitionResult struct {
	Order    Order `json:"order"`
	Replayed bool  `json:"replayed"`
}

// Transition finalizes a reserved order.
// The order row is locked before inventory is updated.
// Repeated identical transitions are read-only.
func (s *Service) Transition(
	ctx context.Context,
	actorID int64,
	orderID int64,
	action TransitionAction,
) (TransitionResult, error) {
	if actorID <= 0 || orderID <= 0 {
		return TransitionResult{}, ErrInvalidTransition
	}

	var target, movementType string

	switch action {
	case ActionFulfill:
		target, movementType = "fulfilled", "fulfill"
	case ActionCancel:
		target, movementType = "cancelled", "release"
	default:
		return TransitionResult{}, ErrInvalidTransition
	}

	tx, err := s.db.BeginTx(
		ctx,
		&sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		},
	)
	if err != nil {
		return TransitionResult{},
			fmt.Errorf("begin order transition: %w", err)
	}
	defer tx.Rollback()

	// Serialize simultaneous terminal actions
	// targeting the same order.
	var status string

	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(&status)

	if errors.Is(err, sql.ErrNoRows) {
		return TransitionResult{}, ErrOrderNotFound
	}
	if err != nil {
		return TransitionResult{},
			fmt.Errorf("lock order: %w", err)
	}

	// Same terminal action: safe retry without
	// updating stock or inserting movements.
	if status == target {
		current, err := loadOrder(ctx, tx, orderID)
		if err != nil {
			return TransitionResult{}, err
		}

		if err := tx.Commit(); err != nil {
			return TransitionResult{},
				fmt.Errorf("commit order replay: %w", err)
		}

		return TransitionResult{
			Order:    current,
			Replayed: true,
		}, nil
	}

	// Fulfilled -> cancelled, or cancelled -> fulfilled,
	// is never allowed.
	if status != "reserved" {
		return TransitionResult{}, ErrOrderNotReserved
	}

	order, err := loadOrder(ctx, tx, orderID)
	if err != nil {
		return TransitionResult{}, err
	}

	if len(order.Items) == 0 {
		return TransitionResult{},
			ErrReservationInconsistent
	}

	// loadOrder already returns items sorted
	// by ascending product_id.
	for _, item := range order.Items {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			return TransitionResult{},
				ErrReservationInconsistent
		}

		var changedID int64

		if action == ActionFulfill {
			// Actual stock leaves the warehouse.
			err = tx.QueryRowContext(ctx, `
				UPDATE inventory_balances
				SET
					on_hand = on_hand - $2,
					reserved = reserved - $2,
					updated_at = statement_timestamp()
				WHERE product_id = $1
				  AND on_hand >= $2
				  AND reserved >= $2
				RETURNING product_id
			`,
				item.ProductID,
				item.Quantity,
			).Scan(&changedID)
		} else {
			// Cancellation releases reservation only.
			err = tx.QueryRowContext(ctx, `
				UPDATE inventory_balances
				SET
					reserved = reserved - $2,
					updated_at = statement_timestamp()
				WHERE product_id = $1
				  AND reserved >= $2
				RETURNING product_id
			`,
				item.ProductID,
				item.Quantity,
			).Scan(&changedID)
		}

		if errors.Is(err, sql.ErrNoRows) {
			return TransitionResult{},
				ErrReservationInconsistent
		}
		if err != nil {
			return TransitionResult{},
				fmt.Errorf("update reserved inventory: %w", err)
		}

		deltaOnHand := int64(0)

		if action == ActionFulfill {
			deltaOnHand = -item.Quantity
		}

		// The movement must be committed together
		// with the inventory balance change.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (
				product_id,
				order_id,
				actor_user_id,
				movement_type,
				delta_on_hand,
				delta_reserved
			)
			VALUES ($1, $2, $3, $4, $5, $6)
		`,
			item.ProductID,
			orderID,
			actorID,
			movementType,
			deltaOnHand,
			-item.Quantity,
		)

		if err != nil {
			return TransitionResult{},
				fmt.Errorf("insert terminal movement: %w", err)
		}
	}

	// Status and timestamps commit in the same
	// transaction as every inventory mutation.
	var updatedID int64

	err = tx.QueryRowContext(ctx, `
		UPDATE orders
		SET
			status = $2,
			updated_at = statement_timestamp(),
			fulfilled_at = CASE
				WHEN $2 = 'fulfilled'
					THEN statement_timestamp()
				ELSE NULL
			END,
			cancelled_at = CASE
				WHEN $2 = 'cancelled'
					THEN statement_timestamp()
				ELSE NULL
			END
		WHERE id = $1
		  AND status = 'reserved'
		RETURNING id
	`, orderID, target).Scan(&updatedID)

	if errors.Is(err, sql.ErrNoRows) {
		return TransitionResult{}, ErrOrderNotReserved
	}
	if err != nil {
		return TransitionResult{},
			fmt.Errorf("update order status: %w", err)
	}

	updated, err := loadOrder(ctx, tx, orderID)
	if err != nil {
		return TransitionResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return TransitionResult{},
			fmt.Errorf("commit order transition: %w", err)
	}

	return TransitionResult{
		Order:    updated,
		Replayed: false,
	}, nil
}
