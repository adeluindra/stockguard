package orders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const MaxOrderItems = 50

var (
	ErrInvalidOrder        = errors.New("invalid order request")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
	ErrProductNotFound     = errors.New("product not found")
	ErrProductInactive     = errors.New("product is inactive")
	ErrInsufficientStock   = errors.New("insufficient available stock")
)

type Item struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type Order struct {
	ID          int64      `json:"id"`
	CreatedBy   int64      `json:"created_by"`
	Status      string     `json:"status"`
	Items       []Item     `json:"items"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	FulfilledAt *time.Time `json:"fulfilled_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
}

type CreateResult struct {
	Order    Order
	Replayed bool
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func NormalizeIdempotencyKey(value string) (string, error) {
	if len(value) != 36 {
		return "", ErrInvalidOrder
	}

	for i := 0; i < len(value); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if value[i] != '-' {
				return "", ErrInvalidOrder
			}
			continue
		}

		c := value[i]
		hex := (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')

		if !hex {
			return "", ErrInvalidOrder
		}
	}

	return strings.ToLower(value), nil
}

// normalizeItems rejects duplicates and sorts by product ID.
// This provides deterministic DB locking and payload hashing.
func normalizeItems(items []Item) ([]Item, error) {
	if len(items) < 1 || len(items) > MaxOrderItems {
		return nil, ErrInvalidOrder
	}

	normalized := append([]Item(nil), items...)

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ProductID <
			normalized[j].ProductID
	})

	for i, item := range normalized {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			return nil, ErrInvalidOrder
		}

		if i > 0 &&
			normalized[i-1].ProductID == item.ProductID {
			return nil, ErrInvalidOrder
		}
	}

	return normalized, nil
}

func payloadHash(items []Item) ([32]byte, error) {
	payload := struct {
		Items []Item `json:"items"`
	}{
		Items: items,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return [32]byte{}, err
	}

	return sha256.Sum256(encoded), nil
}

// Advisory lock is scoped to the current transaction.
// Hash collisions only cause additional serialization;
// uniqueness is still enforced by PostgreSQL.
func advisoryLockID(
	userID int64,
	key string,
) int64 {
	source := strconv.FormatInt(userID, 10) + ":" + key
	digest := sha256.Sum256([]byte(source))

	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func loadOrder(
	ctx context.Context,
	tx *sql.Tx,
	id int64,
) (Order, error) {
	var result Order
	var fulfilledAt sql.NullTime
	var cancelledAt sql.NullTime

	err := tx.QueryRowContext(ctx, `
		SELECT
			id,
			created_by,
			status,
			created_at,
			updated_at,
			fulfilled_at,
			cancelled_at
		FROM orders
		WHERE id = $1
	`, id).Scan(
		&result.ID,
		&result.CreatedBy,
		&result.Status,
		&result.CreatedAt,
		&result.UpdatedAt,
		&fulfilledAt,
		&cancelledAt,
	)

	if err != nil {
		return Order{}, fmt.Errorf("load order: %w", err)
	}

	if fulfilledAt.Valid {
		value := fulfilledAt.Time
		result.FulfilledAt = &value
	}

	if cancelledAt.Valid {
		value := cancelledAt.Time
		result.CancelledAt = &value
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT product_id, quantity
		FROM order_items
		WHERE order_id = $1
		ORDER BY product_id ASC
	`, id)

	if err != nil {
		return Order{}, fmt.Errorf("load order items: %w", err)
	}
	defer rows.Close()

	result.Items = make([]Item, 0)

	for rows.Next() {
		var item Item

		if err := rows.Scan(
			&item.ProductID,
			&item.Quantity,
		); err != nil {
			return Order{}, fmt.Errorf(
				"scan order item: %w", err,
			)
		}

		result.Items = append(result.Items, item)
	}

	if err := rows.Err(); err != nil {
		return Order{}, fmt.Errorf(
			"iterate order items: %w", err,
		)
	}

	return result, nil
}

// Create atomically reserves every item, records all movements,
// and stores one idempotency record.
//
// A repeated key + equivalent payload returns the existing order.
// A repeated key + different payload returns a conflict.
func (s *Service) Create(
	ctx context.Context,
	actorID int64,
	rawKey string,
	rawItems []Item,
) (CreateResult, error) {
	if actorID <= 0 {
		return CreateResult{}, ErrInvalidOrder
	}

	key, err := NormalizeIdempotencyKey(rawKey)
	if err != nil {
		return CreateResult{}, err
	}

	items, err := normalizeItems(rawItems)
	if err != nil {
		return CreateResult{}, err
	}

	hash, err := payloadHash(items)
	if err != nil {
		return CreateResult{}, fmt.Errorf(
			"hash order payload: %w", err,
		)
	}

	tx, err := s.db.BeginTx(
		ctx,
		&sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		},
	)
	if err != nil {
		return CreateResult{}, fmt.Errorf(
			"begin order transaction: %w", err,
		)
	}
	defer tx.Rollback()

	// Serialize requests with the same user + key.
	_, err = tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock($1)
	`, advisoryLockID(actorID, key))

	if err != nil {
		return CreateResult{}, fmt.Errorf(
			"acquire idempotency lock: %w", err,
		)
	}

	var existingOrderID int64
	var existingHash []byte

	err = tx.QueryRowContext(ctx, `
		SELECT order_id, payload_hash
		FROM idempotency_records
		WHERE user_id = $1
		  AND idempotency_key = $2
	`, actorID, key).Scan(
		&existingOrderID,
		&existingHash,
	)

	if err == nil {
		if !bytes.Equal(existingHash, hash[:]) {
			return CreateResult{},
				ErrIdempotencyConflict
		}

		existing, err := loadOrder(
			ctx, tx, existingOrderID,
		)
		if err != nil {
			return CreateResult{}, err
		}

		if err := tx.Commit(); err != nil {
			return CreateResult{}, fmt.Errorf(
				"commit idempotent replay: %w", err,
			)
		}

		return CreateResult{
			Order:    existing,
			Replayed: true,
		}, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return CreateResult{}, fmt.Errorf(
			"check idempotency record: %w", err,
		)
	}

	// Acquire locks in ascending product ID order.
	// This matches the inventory adjustment lock order:
	// product first, then inventory balance.
	for _, item := range items {
		var active bool

		err := tx.QueryRowContext(ctx, `
			SELECT is_active
			FROM products
			WHERE id = $1
			FOR SHARE
		`, item.ProductID).Scan(&active)

		if errors.Is(err, sql.ErrNoRows) {
			return CreateResult{}, ErrProductNotFound
		}
		if err != nil {
			return CreateResult{}, fmt.Errorf(
				"lock order product: %w", err,
			)
		}

		if !active {
			return CreateResult{}, ErrProductInactive
		}

		var onHand int64
		var reserved int64

		err = tx.QueryRowContext(ctx, `
			SELECT on_hand, reserved
			FROM inventory_balances
			WHERE product_id = $1
			FOR UPDATE
		`, item.ProductID).Scan(
			&onHand,
			&reserved,
		)

		if err != nil {
			return CreateResult{}, fmt.Errorf(
				"lock inventory balance: %w", err,
			)
		}

		if reserved > onHand ||
			item.Quantity > onHand-reserved {
			return CreateResult{}, ErrInsufficientStock
		}
	}

	var order Order

	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (
			created_by,
			status
		)
		VALUES ($1, 'reserved')
		RETURNING id, created_at, updated_at
	`, actorID).Scan(
		&order.ID,
		&order.CreatedAt,
		&order.UpdatedAt,
	)

	if err != nil {
		return CreateResult{}, fmt.Errorf(
			"insert order: %w", err,
		)
	}

	order.CreatedBy = actorID
	order.Status = "reserved"
	order.Items = items

	for _, item := range items {
		// The order item must exist before its movement
		// because movements have a composite FK.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO order_items (
				order_id,
				product_id,
				quantity
			)
			VALUES ($1, $2, $3)
		`, order.ID, item.ProductID, item.Quantity)

		if err != nil {
			return CreateResult{}, fmt.Errorf(
				"insert order item: %w", err,
			)
		}

		// Conditional update maintains the DB invariant
		// even if future code changes the locking flow.
		var newReserved int64

		err = tx.QueryRowContext(ctx, `
			UPDATE inventory_balances
			SET
				reserved = reserved + $2,
				updated_at = statement_timestamp()
			WHERE product_id = $1
			  AND reserved <= on_hand - $2
			RETURNING reserved
		`, item.ProductID, item.Quantity).Scan(
			&newReserved,
		)

		if errors.Is(err, sql.ErrNoRows) {
			return CreateResult{}, ErrInsufficientStock
		}
		if err != nil {
			return CreateResult{}, fmt.Errorf(
				"reserve inventory: %w", err,
			)
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
			VALUES (
				$1, $2, $3, 'reserve', 0, $4
			)
		`,
			item.ProductID,
			order.ID,
			actorID,
			item.Quantity,
		)

		if err != nil {
			return CreateResult{}, fmt.Errorf(
				"insert reservation movement: %w",
				err,
			)
		}
	}

	// Idempotency record and reservation commit together.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO idempotency_records (
			user_id,
			idempotency_key,
			payload_hash,
			order_id
		)
		VALUES ($1, $2, $3, $4)
	`, actorID, key, hash[:], order.ID)

	if err != nil {
		return CreateResult{}, fmt.Errorf(
			"insert idempotency record: %w", err,
		)
	}

	if err := tx.Commit(); err != nil {
		return CreateResult{}, fmt.Errorf(
			"commit order reservation: %w", err,
		)
	}

	return CreateResult{
		Order:    order,
		Replayed: false,
	}, nil
}
