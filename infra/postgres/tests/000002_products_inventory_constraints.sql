
-- StockGuard DB-3C Constraint Tests
-- Execute ONLY against stockguard_test.

BEGIN;

DO $$
DECLARE
    test_product_id BIGINT;
    available_stock BIGINT;
BEGIN
    -- TEST 1: Valid product and inventory balance.
    INSERT INTO products (
        sku, name, low_stock_threshold
    )
    VALUES (
        'DB3C-001', 'Produk Pengujian', 3
    )
    RETURNING id INTO test_product_id;

    INSERT INTO inventory_balances (
        product_id, on_hand, reserved
    )
    VALUES (test_product_id, 10, 3);

    SELECT on_hand - reserved
    INTO available_stock
    FROM inventory_balances
    WHERE product_id = test_product_id;

    IF available_stock <> 7 THEN
        RAISE EXCEPTION 'FAIL 1: Available stock incorrect';
    END IF;

    RAISE NOTICE 'PASS 1: Valid product and balance';

    -- TEST 2: Duplicate SKU (case-insensitive).
    BEGIN
        INSERT INTO products (sku, name)
        VALUES ('db3c-001', 'Produk Duplikat');

        RAISE EXCEPTION 'FAIL 2: Duplicate SKU accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 2: Duplicate SKU rejected';
    END;

    -- TEST 3: Blank product name.
    BEGIN
        INSERT INTO products (sku, name)
        VALUES ('DB3C-BAD-NAME', '   ');

        RAISE EXCEPTION 'FAIL 3: Blank name accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 3: Blank name rejected';
    END;

    -- TEST 4: Negative low-stock threshold.
    BEGIN
        INSERT INTO products (
            sku, name, low_stock_threshold
        )
        VALUES ('DB3C-BAD-THRESHOLD', 'Invalid', -1);

        RAISE EXCEPTION 'FAIL 4: Negative threshold accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 4: Negative threshold rejected';
    END;

    -- TEST 5: Inventory for nonexistent product.
    BEGIN
        INSERT INTO inventory_balances (
            product_id, on_hand, reserved
        )
        VALUES (-1, 10, 0);

        RAISE EXCEPTION 'FAIL 5: Invalid FK accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS 5: Invalid FK rejected';
    END;

    -- TEST 6: Negative on-hand.
    BEGIN
        UPDATE inventory_balances
        SET on_hand = -1
        WHERE product_id = test_product_id;

        RAISE EXCEPTION 'FAIL 6: Negative on_hand accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 6: Negative on_hand rejected';
    END;

    -- TEST 7: Reserved greater than on-hand.
    BEGIN
        UPDATE inventory_balances
        SET reserved = 11
        WHERE product_id = test_product_id;

        RAISE EXCEPTION 'FAIL 7: Overselling invariant violated';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 7: Reserved > on_hand rejected';
    END;

    -- TEST 8: Negative reserved.
    BEGIN
        UPDATE inventory_balances
        SET reserved = -1
        WHERE product_id = test_product_id;

        RAISE EXCEPTION 'FAIL 8: Negative reserved accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 8: Negative reserved rejected';
    END;

    -- TEST 9: Duplicate inventory balance.
    BEGIN
        INSERT INTO inventory_balances (product_id)
        VALUES (test_product_id);

        RAISE EXCEPTION 'FAIL 9: Duplicate balance accepted';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS 9: Duplicate balance rejected';
    END;

    -- TEST 10: Adjustment below reserved.
    BEGIN
        UPDATE inventory_balances
        SET on_hand = 2
        WHERE product_id = test_product_id;

        RAISE EXCEPTION 'FAIL 10: Invalid adjustment accepted';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS 10: Adjustment below reserved rejected';
    END;

    -- Ensure invalid operations did not modify the balance.
    IF NOT EXISTS (
        SELECT 1
        FROM inventory_balances
        WHERE product_id = test_product_id
          AND on_hand = 10
          AND reserved = 3
    ) THEN
        RAISE EXCEPTION 'FAIL: Balance changed unexpectedly';
    END IF;

    RAISE NOTICE 'ALL 10 TESTS PASSED';
END;
$$;

-- Undo test data.
ROLLBACK;
