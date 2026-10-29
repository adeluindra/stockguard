
-- StockGuard DB-3D Order Constraint Tests
-- Run ONLY against stockguard_test.
-- All test data is rolled back.

BEGIN;

DO $$
DECLARE
    test_user_id BIGINT;
    test_product_id BIGINT;
    test_order_id BIGINT;
BEGIN

    -- Test fixtures: synthetic user and product.
    INSERT INTO users (
        username, password_hash, role
    )
    VALUES (
        'db3d_test_staff',
        'TEST_ONLY_NOT_REAL_HASH',
        'staff'
    )
    RETURNING id INTO test_user_id;

    INSERT INTO products (sku, name)
    VALUES ('DB3D-ORDER-001', 'Test Product')
    RETURNING id INTO test_product_id;

    INSERT INTO inventory_balances (
        product_id, on_hand, reserved
    )
    VALUES (test_product_id, 10, 0);

    -- TEST 1: Create valid Reserved order.
    INSERT INTO orders (created_by)
    VALUES (test_user_id)
    RETURNING id INTO test_order_id;

    IF NOT EXISTS (
        SELECT 1 FROM orders
        WHERE id = test_order_id
          AND status = 'reserved'
          AND fulfilled_at IS NULL
          AND cancelled_at IS NULL
    ) THEN
        RAISE EXCEPTION 'FAIL 1: Invalid default order state';
    END IF;

    RAISE NOTICE 'PASS 1: Valid Reserved order';

    -- TEST 2: Valid order item.
    INSERT INTO order_items (
        order_id, product_id, quantity
    )
    VALUES (test_order_id, test_product_id, 2);

    RAISE NOTICE 'PASS 2: Valid order item';

    -- TEST 3: Duplicate item.
    BEGIN
        INSERT INTO order_items (
            order_id, product_id, quantity
        )
        VALUES (test_order_id, test_product_id, 1);

        RAISE EXCEPTION 'FAIL 3: Duplicate item accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 3: Duplicate item rejected';
    END;

    -- TEST 4: Zero quantity.
    BEGIN
        INSERT INTO order_items (
            order_id, product_id, quantity
        )
        VALUES (test_order_id, test_product_id, 0);

        RAISE EXCEPTION 'FAIL 4: Zero quantity accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 4: Zero quantity rejected';
    END;

    -- TEST 5: Negative quantity.
    BEGIN
        INSERT INTO order_items (
            order_id, product_id, quantity
        )
        VALUES (test_order_id, test_product_id, -3);

        RAISE EXCEPTION 'FAIL 5: Negative quantity accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 5: Negative quantity rejected';
    END;

    -- TEST 6: Unknown product.
    BEGIN
        INSERT INTO order_items (
            order_id, product_id, quantity
        )
        VALUES (test_order_id, -1, 1);

        RAISE EXCEPTION 'FAIL 6: Unknown product accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 6: Unknown product rejected';
    END;

    -- TEST 7: Unknown order.
    BEGIN
        INSERT INTO order_items (
            order_id, product_id, quantity
        )
        VALUES (-1, test_product_id, 1);

        RAISE EXCEPTION 'FAIL 7: Unknown order accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 7: Unknown order rejected';
    END;

    -- TEST 8: Unknown order creator.
    BEGIN
        INSERT INTO orders (created_by)
        VALUES (-1);

        RAISE EXCEPTION 'FAIL 8: Unknown creator accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 8: Unknown creator rejected';
    END;

    -- TEST 9: Invalid status.
    BEGIN
        INSERT INTO orders (created_by, status)
        VALUES (test_user_id, 'processing');

        RAISE EXCEPTION 'FAIL 9: Invalid status accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 9: Invalid status rejected';
    END;

    -- TEST 10: Fulfilled without timestamp.
    BEGIN
        INSERT INTO orders (created_by, status)
        VALUES (test_user_id, 'fulfilled');

        RAISE EXCEPTION 'FAIL 10: Missing fulfilled_at accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 10: Missing fulfilled_at rejected';
    END;

    -- TEST 11: Cancelled without timestamp.
    BEGIN
        INSERT INTO orders (created_by, status)
        VALUES (test_user_id, 'cancelled');

        RAISE EXCEPTION 'FAIL 11: Missing cancelled_at accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 11: Missing cancelled_at rejected';
    END;

    -- TEST 12: Reserved with terminal timestamp.
    BEGIN
        UPDATE orders
        SET fulfilled_at = NOW()
        WHERE id = test_order_id;

        RAISE EXCEPTION 'FAIL 12: Reserved terminal timestamp accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 12: Invalid timestamp rejected';
    END;

    -- TEST 13: Valid Fulfilled state.
    UPDATE orders
    SET status = 'fulfilled',
        fulfilled_at = NOW(),
        updated_at = NOW()
    WHERE id = test_order_id;

    IF NOT EXISTS (
        SELECT 1 FROM orders
        WHERE id = test_order_id
          AND status = 'fulfilled'
          AND fulfilled_at IS NOT NULL
          AND cancelled_at IS NULL
    ) THEN
        RAISE EXCEPTION 'FAIL 13: Fulfilled state incorrect';
    END IF;

    RAISE NOTICE 'PASS 13: Valid Fulfilled state';

    -- TEST 14: Both terminal timestamps.
    BEGIN
        UPDATE orders
        SET cancelled_at = NOW()
        WHERE id = test_order_id;

        RAISE EXCEPTION 'FAIL 14: Both timestamps accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 14: Both timestamps rejected';
    END;

    -- TEST 15: Fulfillment before creation.
    BEGIN
        UPDATE orders
        SET fulfilled_at = created_at - INTERVAL '1 second'
        WHERE id = test_order_id;

        RAISE EXCEPTION 'FAIL 15: Invalid terminal time accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 15: Invalid terminal time rejected';
    END;

    -- Schema-only test: no reservation logic exists yet.
    IF NOT EXISTS (
        SELECT 1 FROM inventory_balances
        WHERE product_id = test_product_id
          AND on_hand = 10
          AND reserved = 0
    ) THEN
        RAISE EXCEPTION 'FAIL: Inventory changed unexpectedly';
    END IF;

    RAISE NOTICE 'ALL 15 ORDER CONSTRAINT TESTS PASSED';

END;
$$;

ROLLBACK;
