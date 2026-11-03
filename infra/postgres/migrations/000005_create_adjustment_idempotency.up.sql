
-- StockGuard Migration 000005
-- Durable idempotency for inventory adjustments.

CREATE TABLE inventory_adjustment_idempotency (
    actor_user_id BIGINT NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    idempotency_key TEXT NOT NULL,

    product_id BIGINT NOT NULL
        REFERENCES products(id) ON DELETE RESTRICT,

    delta_on_hand BIGINT NOT NULL,
    reason TEXT NOT NULL,

    movement_id BIGINT NOT NULL UNIQUE
        REFERENCES inventory_movements(id) ON DELETE RESTRICT,

    -- Preserve the original adjustment response.
    response_snapshot JSONB NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT adjustment_idempotency_pk
        PRIMARY KEY (actor_user_id, idempotency_key),

    CONSTRAINT adjustment_key_length_check
        CHECK (CHAR_LENGTH(idempotency_key) = 36),

    CONSTRAINT adjustment_nonzero_delta_check
        CHECK (delta_on_hand <> 0),

    CONSTRAINT adjustment_reason_length_check
        CHECK (
            CHAR_LENGTH(BTRIM(reason)) BETWEEN 10 AND 500
        ),

    CONSTRAINT adjustment_response_object_check
        CHECK (JSONB_TYPEOF(response_snapshot) = 'object')
);
