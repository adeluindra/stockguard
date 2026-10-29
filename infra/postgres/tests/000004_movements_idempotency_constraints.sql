
-- StockGuard DB-3E Constraint Tests
-- Run ONLY on stockguard_test.

BEGIN;

DO $$
DECLARE
    actor_id BIGINT;
    other_user_id BIGINT;
    product_id_1 BIGINT;
    product_id_2 BIGINT;
    order_id_1 BIGINT;
    order_id_2 BIGINT;
    current_on_hand BIGINT;
    current_reserved BIGINT;
    movement_on_hand BIGINT;
    movement_reserved BIGINT;
BEGIN
    -- Synthetic fixtures.
    INSERT INTO users (username, password_hash, role)
    VALUES ('db3e_staff', 'TEST_ONLY_NOT_REAL_HASH', 'staff')
    RETURNING id INTO actor_id;

    INSERT INTO users (username, password_hash, role)
    VALUES ('db3e_other', 'TEST_ONLY_NOT_REAL_HASH', 'staff')
    RETURNING id INTO other_user_id;

    INSERT INTO products (sku, name)
    VALUES ('DB3E-P001', 'Movement Test Product')
    RETURNING id INTO product_id_1;

    INSERT INTO products (sku, name)
    VALUES ('DB3E-P002', 'Other Test Product')
    RETURNING id INTO product_id_2;

    INSERT INTO inventory_balances
        (product_id, on_hand, reserved)
    VALUES (product_id_1, 10, 0);

    INSERT INTO orders (created_by)
    VALUES (actor_id)
    RETURNING id INTO order_id_1;

    INSERT INTO order_items
        (order_id, product_id, quantity)
    VALUES (order_id_1, product_id_1, 2);

    INSERT INTO orders (created_by)
    VALUES (actor_id)
    RETURNING id INTO order_id_2;

    INSERT INTO order_items
        (order_id, product_id, quantity)
    VALUES (order_id_2, product_id_1, 1);

    -- TEST 1: Valid initial movement.
    INSERT INTO inventory_movements (
        product_id, actor_user_id, movement_type,
        delta_on_hand, delta_reserved
    )
    VALUES (product_id_1, actor_id, 'initial', 10, 0);

    RAISE NOTICE 'PASS 1: Valid initial movement';

    -- TEST 2: Duplicate initial movement.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, actor_user_id, movement_type,
            delta_on_hand, delta_reserved
        )
        VALUES (product_id_1, actor_id, 'initial', 5, 0);

        RAISE EXCEPTION 'FAIL 2: Duplicate initial accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 2: Duplicate initial rejected';
    END;

    -- TEST 3: Valid reservation.
    INSERT INTO inventory_movements (
        product_id, order_id, actor_user_id,
        movement_type, delta_on_hand, delta_reserved
    )
    VALUES (
        product_id_1, order_id_1, actor_id,
        'reserve', 0, 2
    );

    UPDATE inventory_balances
    SET reserved = 2
    WHERE product_id = product_id_1;

    RAISE NOTICE 'PASS 3: Valid reservation movement';

    -- TEST 4: Duplicate reservation movement.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, order_id, actor_user_id,
            movement_type, delta_on_hand, delta_reserved
        )
        VALUES (
            product_id_1, order_id_1, actor_id,
            'reserve', 0, 2
        );

        RAISE EXCEPTION 'FAIL 4: Duplicate reserve accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 4: Duplicate reserve rejected';
    END;

    -- TEST 5: Movement references product not in order.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, order_id, actor_user_id,
            movement_type, delta_on_hand, delta_reserved
        )
        VALUES (
            product_id_2, order_id_1, actor_id,
            'reserve', 0, 1
        );

        RAISE EXCEPTION 'FAIL 5: Invalid order item accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 5: Invalid order item rejected';
    END;

    -- TEST 6: Adjustment requires a reason.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, actor_user_id, movement_type,
            delta_on_hand, delta_reserved
        )
        VALUES (
            product_id_1, actor_id, 'adjustment', 3, 0
        );

        RAISE EXCEPTION 'FAIL 6: Missing reason accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 6: Missing reason rejected';
    END;

    -- TEST 7: Invalid fulfillment deltas.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, order_id, actor_user_id,
            movement_type, delta_on_hand, delta_reserved
        )
        VALUES (
            product_id_1, order_id_1, actor_id,
            'fulfill', -2, -1
        );

        RAISE EXCEPTION 'FAIL 7: Incorrect deltas accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 7: Incorrect deltas rejected';
    END;

    -- TEST 8: Valid fulfillment movement.
    INSERT INTO inventory_movements (
        product_id, order_id, actor_user_id,
        movement_type, delta_on_hand, delta_reserved
    )
    VALUES (
        product_id_1, order_id_1, actor_id,
        'fulfill', -2, -2
    );

    UPDATE inventory_balances
    SET on_hand = 8, reserved = 0
    WHERE product_id = product_id_1;

    UPDATE orders
    SET status = 'fulfilled',
        fulfilled_at = NOW(),
        updated_at = NOW()
    WHERE id = order_id_1;

    RAISE NOTICE 'PASS 8: Valid fulfillment movement';

    -- TEST 9: Duplicate fulfillment movement.
    BEGIN
        INSERT INTO inventory_movements (
            product_id, order_id, actor_user_id,
            movement_type, delta_on_hand, delta_reserved
        )
        VALUES (
            product_id_1, order_id_1, actor_id,
            'fulfill', -2, -2
        );

        RAISE EXCEPTION 'FAIL 9: Duplicate fulfill accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 9: Duplicate fulfill rejected';
    END;

    -- TEST 10: Wrong idempotency owner.
    BEGIN
        INSERT INTO idempotency_records (
            user_id, idempotency_key,
            payload_hash, order_id
        )
        VALUES (
            other_user_id,
            '11111111-1111-4111-8111-111111111111',
            decode(repeat('ab', 32), 'hex'),
            order_id_1
        );

        RAISE EXCEPTION 'FAIL 10: Owner mismatch accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 10: Owner mismatch rejected';
    END;

    -- TEST 11: Valid idempotency record.
    INSERT INTO idempotency_records (
        user_id, idempotency_key,
        payload_hash, order_id
    )
    VALUES (
        actor_id,
        '11111111-1111-4111-8111-111111111111',
        decode(repeat('ab', 32), 'hex'),
        order_id_1
    );

    RAISE NOTICE 'PASS 11: Valid idempotency record';

    -- TEST 12: Same user and key, different order.
    BEGIN
        INSERT INTO idempotency_records (
            user_id, idempotency_key,
            payload_hash, order_id
        )
        VALUES (
            actor_id,
            '11111111-1111-4111-8111-111111111111',
            decode(repeat('ab', 32), 'hex'),
            order_id_2
        );

        RAISE EXCEPTION 'FAIL 12: Duplicate key accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 12: Duplicate key rejected';
    END;

    -- TEST 13: Same order with different key.
    BEGIN
        INSERT INTO idempotency_records (
            user_id, idempotency_key,
            payload_hash, order_id
        )
        VALUES (
            actor_id,
            '22222222-2222-4222-8222-222222222222',
            decode(repeat('cd', 32), 'hex'),
            order_id_1
        );

        RAISE EXCEPTION 'FAIL 13: Duplicate order reference';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 13: Duplicate order rejected';
    END;

    -- TEST 14: Invalid payload hash length.
    BEGIN
        INSERT INTO idempotency_records (
            user_id, idempotency_key,
            payload_hash, order_id
        )
        VALUES (
            actor_id,
            '22222222-2222-4222-8222-222222222222',
            decode('abcd', 'hex'),
            order_id_2
        );

        RAISE EXCEPTION 'FAIL 14: Invalid hash accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 14: Invalid hash rejected';
    END;

    -- TEST 15: Idempotency record without order.
    BEGIN
        INSERT INTO idempotency_records (
            user_id, idempotency_key,
            payload_hash, order_id
        )
        VALUES (
            actor_id,
            '33333333-3333-4333-8333-333333333333',
            decode(repeat('ef', 32), 'hex'),
            NULL
        );

        RAISE EXCEPTION 'FAIL 15: Null order accepted';
    EXCEPTION WHEN not_null_violation THEN
        RAISE NOTICE 'PASS 15: Null order rejected';
    END;

    -- Reconcile the movement totals with final balance.
    SELECT on_hand, reserved
    INTO current_on_hand, current_reserved
    FROM inventory_balances
    WHERE product_id = product_id_1;

    SELECT SUM(delta_on_hand), SUM(delta_reserved)
    INTO movement_on_hand, movement_reserved
    FROM inventory_movements
    WHERE product_id = product_id_1;

    IF current_on_hand <> movement_on_hand
       OR current_reserved <> movement_reserved THEN
        RAISE EXCEPTION 'FAIL: Movement reconciliation mismatch';
    END IF;

    RAISE NOTICE 'PASS: Test fixture balances reconciled';
    RAISE NOTICE 'ALL 15 DB-3E TESTS PASSED';
END;
$$;

ROLLBACK;
