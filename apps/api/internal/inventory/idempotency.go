package inventory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidAdjustmentKey = errors.New(
		"invalid inventory adjustment idempotency key",
	)
	ErrAdjustmentKeyConflict = errors.New(
		"idempotency key already used with different adjustment",
	)
)

type IdempotentAdjustment struct {
	AdjustmentResult
	Replayed bool `json:"replayed"`
}

func NormalizeAdjustmentKey(key string) (string, error) {
	if len(key) != 36 {
		return "", ErrInvalidAdjustmentKey
	}

	for i := 0; i < len(key); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if key[i] != '-' {
				return "", ErrInvalidAdjustmentKey
			}
			continue
		}

		c := key[i]
		hex := (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')

		if !hex {
			return "", ErrInvalidAdjustmentKey
		}
	}

	return strings.ToLower(key), nil
}

func adjustmentLockID(actorID int64, key string) int64 {
	source := strconv.FormatInt(actorID, 10) + ":" + key
	digest := sha256.Sum256([]byte(source))

	return int64(binary.BigEndian.Uint64(digest[:8]))
}

// AdjustIdempotent guarantees a single committed adjustment
// per (actor_user_id, idempotency_key).
//
// The stock mutation, movement and response snapshot are
// committed in one transaction. Failed adjustments do not
// consume the idempotency key.
func (s *Store) AdjustIdempotent(
	ctx context.Context,
	productID, actorID, delta int64,
	reason, rawKey string,
) (IdempotentAdjustment, error) {
	if productID <= 0 {
		return IdempotentAdjustment{}, ErrProductNotFound
	}

	if actorID <= 0 ||
		ValidateAdjustment(delta, reason) != nil {
		return IdempotentAdjustment{}, ErrInvalidAdjustment
	}

	key, err := NormalizeAdjustmentKey(rawKey)
	if err != nil {
		return IdempotentAdjustment{}, err
	}

	tx, err := s.db.BeginTx(
		ctx,
		&sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
		},
	)
	if err != nil {
		return IdempotentAdjustment{},
			fmt.Errorf("begin idempotent adjustment: %w", err)
	}
	defer tx.Rollback()

	// Same-key requests serialize across all backend instances.
	_, err = tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock($1)
	`, adjustmentLockID(actorID, key))

	if err != nil {
		return IdempotentAdjustment{},
			fmt.Errorf("lock adjustment key: %w", err)
	}

	var storedProductID, storedDelta int64
	var storedReason string
	var storedJSON []byte

	err = tx.QueryRowContext(ctx, `
		SELECT
			product_id,
			delta_on_hand,
			reason,
			response_snapshot
		FROM inventory_adjustment_idempotency
		WHERE actor_user_id = $1
		  AND idempotency_key = $2
	`, actorID, key).Scan(
		&storedProductID,
		&storedDelta,
		&storedReason,
		&storedJSON,
	)

	if err == nil {
		// A retry must match the entire logical request.
		if storedProductID != productID ||
			storedDelta != delta ||
			storedReason != reason {
			return IdempotentAdjustment{},
				ErrAdjustmentKeyConflict
		}

		var original AdjustmentResult

		if err := json.Unmarshal(
			storedJSON,
			&original,
		); err != nil {
			return IdempotentAdjustment{},
				fmt.Errorf("decode adjustment snapshot: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return IdempotentAdjustment{},
				fmt.Errorf("commit adjustment replay: %w", err)
		}

		return IdempotentAdjustment{
			AdjustmentResult: original,
			Replayed:         true,
		}, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return IdempotentAdjustment{},
			fmt.Errorf("lookup adjustment key: %w", err)
	}

	// The same transaction now owns both the stock
	// mutation and the idempotency record.
	result, err := adjustWithinTx(
		ctx,
		tx,
		productID,
		actorID,
		delta,
		reason,
	)
	if err != nil {
		return IdempotentAdjustment{}, err
	}

	snapshot, err := json.Marshal(result)
	if err != nil {
		return IdempotentAdjustment{},
			fmt.Errorf("encode adjustment snapshot: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_adjustment_idempotency (
			actor_user_id,
			idempotency_key,
			product_id,
			delta_on_hand,
			reason,
			movement_id,
			response_snapshot
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`,
		actorID,
		key,
		productID,
		delta,
		reason,
		result.Movement.ID,
		string(snapshot),
	)

	if err != nil {
		return IdempotentAdjustment{},
			fmt.Errorf("store adjustment idempotency: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return IdempotentAdjustment{},
			fmt.Errorf("commit idempotent adjustment: %w", err)
	}

	return IdempotentAdjustment{
		AdjustmentResult: result,
		Replayed:         false,
	}, nil
}
