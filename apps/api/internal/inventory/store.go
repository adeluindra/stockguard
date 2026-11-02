package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidAdjustment     = errors.New("invalid inventory adjustment")
	ErrProductNotFound       = errors.New("product not found")
	ErrProductInactive       = errors.New("product is inactive")
	ErrInsufficientAvailable = errors.New("adjustment would reduce on-hand stock below reserved stock")
	ErrQuantityOverflow      = errors.New("stock quantity overflow")
	ErrInvalidPagination     = errors.New("invalid pagination")
)

type Balance struct {
	ProductID         int64  `json:"product_id"`
	SKU               string `json:"sku"`
	Name              string `json:"name"`
	IsActive          bool   `json:"is_active"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
	OnHand            int64  `json:"on_hand"`
	Reserved          int64  `json:"reserved"`
	Available         int64  `json:"available"`
	LowStock          bool   `json:"low_stock"`
}

type Movement struct {
	ID            int64     `json:"id"`
	ProductID     int64     `json:"product_id"`
	OrderID       *int64    `json:"order_id"`
	ActorUserID   int64     `json:"actor_user_id"`
	MovementType  string    `json:"movement_type"`
	DeltaOnHand   int64     `json:"delta_on_hand"`
	DeltaReserved int64     `json:"delta_reserved"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type AdjustmentResult struct {
	Balance  Balance  `json:"balance"`
	Movement Movement `json:"movement"`
}

type MovementPage struct {
	Items   []Movement `json:"items"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	HasMore bool       `json:"has_more"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func ValidateAdjustment(delta int64, reason string) error {
	if delta == 0 ||
		!utf8.ValidString(reason) ||
		strings.TrimSpace(reason) != reason ||
		utf8.RuneCountInString(reason) < 10 ||
		utf8.RuneCountInString(reason) > 500 {
		return ErrInvalidAdjustment
	}

	for _, r := range reason {
		if unicode.IsControl(r) {
			return ErrInvalidAdjustment
		}
	}

	return nil
}

func validatePage(limit, offset int) error {
	if limit < 1 || limit > 100 ||
		offset < 0 || offset > 1000000 {
		return ErrInvalidPagination
	}
	return nil
}

func applyDelta(
	onHand, reserved, delta int64,
) (int64, error) {
	if onHand < 0 || reserved < 0 || reserved > onHand {
		return 0, errors.New("invalid stored inventory balance")
	}

	available := onHand - reserved

	if delta < 0 && delta < -available {
		return 0, ErrInsufficientAvailable
	}

	if delta > 0 && onHand > math.MaxInt64-delta {
		return 0, ErrQuantityOverflow
	}

	return onHand + delta, nil
}

func scanBalance(
	row interface{ Scan(...any) error },
) (Balance, error) {
	var b Balance

	err := row.Scan(
		&b.ProductID,
		&b.SKU,
		&b.Name,
		&b.IsActive,
		&b.LowStockThreshold,
		&b.OnHand,
		&b.Reserved,
	)
	if err != nil {
		return Balance{}, err
	}

	b.Available = b.OnHand - b.Reserved
	b.LowStock = b.Available <= b.LowStockThreshold

	return b, nil
}

const balanceQuery = `
	SELECT
		p.id,
		p.sku,
		p.name,
		p.is_active,
		p.low_stock_threshold,
		b.on_hand,
		b.reserved
	FROM products AS p
	JOIN inventory_balances AS b
		ON b.product_id = p.id
	WHERE p.id = $1
`

func (s *Store) GetBalance(
	ctx context.Context,
	productID int64,
) (Balance, error) {
	if productID <= 0 {
		return Balance{}, ErrProductNotFound
	}

	b, err := scanBalance(
		s.db.QueryRowContext(ctx, balanceQuery, productID),
	)

	if errors.Is(err, sql.ErrNoRows) {
		return Balance{}, ErrProductNotFound
	}
	if err != nil {
		return Balance{},
			fmt.Errorf("get inventory balance: %w", err)
	}

	return b, nil
}

// Adjust locks the product and inventory balance.
// The balance update and movement insert commit together.
func (s *Store) Adjust(
	ctx context.Context,
	productID int64,
	actorID int64,
	delta int64,
	reason string,
) (AdjustmentResult, error) {
	if productID <= 0 {
		return AdjustmentResult{}, ErrProductNotFound
	}

	if actorID <= 0 ||
		ValidateAdjustment(delta, reason) != nil {
		return AdjustmentResult{}, ErrInvalidAdjustment
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("begin adjustment: %w", err)
	}
	defer tx.Rollback()

	// Lock product before inventory.
	// This also coordinates with product deactivation.
	var active bool

	err = tx.QueryRowContext(ctx, `
		SELECT is_active
		FROM products
		WHERE id = $1
		FOR SHARE
	`, productID).Scan(&active)

	if errors.Is(err, sql.ErrNoRows) {
		return AdjustmentResult{}, ErrProductNotFound
	}
	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("lock product: %w", err)
	}
	if !active {
		return AdjustmentResult{}, ErrProductInactive
	}

	var onHand, reserved int64

	err = tx.QueryRowContext(ctx, `
		SELECT on_hand, reserved
		FROM inventory_balances
		WHERE product_id = $1
		FOR UPDATE
	`, productID).Scan(&onHand, &reserved)

	if errors.Is(err, sql.ErrNoRows) {
		return AdjustmentResult{},
			errors.New("product has no inventory balance")
	}
	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("lock inventory balance: %w", err)
	}

	next, err := applyDelta(onHand, reserved, delta)
	if err != nil {
		return AdjustmentResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE inventory_balances
		SET
			on_hand = $2,
			updated_at = statement_timestamp()
		WHERE product_id = $1
	`, productID, next)

	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("update inventory balance: %w", err)
	}

	var movement Movement
	movement.ProductID = productID
	movement.ActorUserID = actorID
	movement.MovementType = "adjustment"
	movement.DeltaOnHand = delta
	movement.DeltaReserved = 0
	movement.Reason = &reason

	err = tx.QueryRowContext(ctx, `
		INSERT INTO inventory_movements (
			product_id,
			actor_user_id,
			movement_type,
			delta_on_hand,
			delta_reserved,
			reason
		)
		VALUES (
			$1, $2, 'adjustment', $3, 0, $4
		)
		RETURNING id, created_at
	`, productID, actorID, delta, reason).Scan(
		&movement.ID,
		&movement.CreatedAt,
	)

	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("insert inventory movement: %w", err)
	}

	b, err := scanBalance(
		tx.QueryRowContext(ctx, balanceQuery, productID),
	)
	if err != nil {
		return AdjustmentResult{},
			fmt.Errorf("read updated balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AdjustmentResult{},
			fmt.Errorf("commit adjustment: %w", err)
	}

	return AdjustmentResult{
		Balance:  b,
		Movement: movement,
	}, nil
}

func scanMovement(
	row interface{ Scan(...any) error },
) (Movement, error) {
	var m Movement
	var orderID sql.NullInt64
	var reason sql.NullString

	err := row.Scan(
		&m.ID,
		&m.ProductID,
		&orderID,
		&m.ActorUserID,
		&m.MovementType,
		&m.DeltaOnHand,
		&m.DeltaReserved,
		&reason,
		&m.CreatedAt,
	)
	if err != nil {
		return Movement{}, err
	}

	if orderID.Valid {
		id := orderID.Int64
		m.OrderID = &id
	}

	if reason.Valid {
		value := reason.String
		m.Reason = &value
	}

	return m, nil
}

func (s *Store) ListMovements(
	ctx context.Context,
	productID int64,
	limit int,
	offset int,
) (MovementPage, error) {
	if productID <= 0 {
		return MovementPage{}, ErrProductNotFound
	}

	if err := validatePage(limit, offset); err != nil {
		return MovementPage{}, err
	}

	var exists bool

	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM products WHERE id = $1
		)
	`, productID).Scan(&exists)

	if err != nil {
		return MovementPage{},
			fmt.Errorf("check product: %w", err)
	}
	if !exists {
		return MovementPage{}, ErrProductNotFound
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			product_id,
			order_id,
			actor_user_id,
			movement_type,
			delta_on_hand,
			delta_reserved,
			reason,
			created_at
		FROM inventory_movements
		WHERE product_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, productID, limit+1, offset)

	if err != nil {
		return MovementPage{},
			fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()

	page := MovementPage{
		Items:  make([]Movement, 0, limit),
		Limit:  limit,
		Offset: offset,
	}

	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return MovementPage{},
				fmt.Errorf("scan movement: %w", err)
		}

		if len(page.Items) == limit {
			page.HasMore = true
			break
		}

		page.Items = append(page.Items, m)
	}

	if err := rows.Err(); err != nil {
		return MovementPage{},
			fmt.Errorf("iterate movements: %w", err)
	}

	return page, nil
}
