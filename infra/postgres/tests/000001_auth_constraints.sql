
-- StockGuard DB-3B: Authentication constraint tests.
-- Run ONLY against stockguard_test.
-- All inserted data will be rolled back.

BEGIN;

DO $$
DECLARE
    test_user_id BIGINT;
BEGIN
    -- TEST 1: Valid user
    INSERT INTO users (username, password_hash, role)
    VALUES ('Db3b_Test_Admin', 'TEST_ONLY_NOT_REAL_HASH', 'admin')
    RETURNING id INTO test_user_id;

    RAISE NOTICE 'PASS 1: Valid user inserted';

    -- TEST 2: Case-insensitive username uniqueness
    BEGIN
        INSERT INTO users (username, password_hash, role)
        VALUES ('db3b_test_admin', 'TEST_ONLY_NOT_REAL_HASH', 'staff');

        RAISE EXCEPTION 'FAIL 2: Duplicate username accepted';
    EXCEPTION
        WHEN unique_violation THEN
            RAISE NOTICE 'PASS 2: Duplicate username rejected';
    END;

    -- TEST 3: Invalid role
    BEGIN
        INSERT INTO users (username, password_hash, role)
        VALUES ('invalid_role_user', 'TEST_ONLY_NOT_REAL_HASH', 'owner');

        RAISE EXCEPTION 'FAIL 3: Invalid role accepted';
    EXCEPTION
        WHEN check_violation THEN
            RAISE NOTICE 'PASS 3: Invalid role rejected';
    END;

    -- TEST 4: Valid session
    INSERT INTO sessions (
        user_id,
        token_hash,
        expires_at
    )
    VALUES (
        test_user_id,
        decode(repeat('aa', 32), 'hex'),
        NOW() + INTERVAL '1 hour'
    );

    RAISE NOTICE 'PASS 4: Valid session inserted';

    -- TEST 5: Session references nonexistent user
    BEGIN
        INSERT INTO sessions (user_id, token_hash, expires_at)
        VALUES (
            -1,
            decode(repeat('bb', 32), 'hex'),
            NOW() + INTERVAL '1 hour'
        );

        RAISE EXCEPTION 'FAIL 5: Invalid foreign key accepted';
    EXCEPTION
        WHEN foreign_key_violation THEN
            RAISE NOTICE 'PASS 5: Invalid foreign key rejected';
    END;

    -- TEST 6: Invalid token hash length
    BEGIN
        INSERT INTO sessions (user_id, token_hash, expires_at)
        VALUES (
            test_user_id,
            decode('abcd', 'hex'),
            NOW() + INTERVAL '1 hour'
        );

        RAISE EXCEPTION 'FAIL 6: Invalid token hash accepted';
    EXCEPTION
        WHEN check_violation THEN
            RAISE NOTICE 'PASS 6: Invalid token hash rejected';
    END;

    -- TEST 7: Expiry earlier than creation
    BEGIN
        INSERT INTO sessions (user_id, token_hash, expires_at)
        VALUES (
            test_user_id,
            decode(repeat('cc', 32), 'hex'),
            NOW() - INTERVAL '1 minute'
        );

        RAISE EXCEPTION 'FAIL 7: Invalid expiry accepted';
    EXCEPTION
        WHEN check_violation THEN
            RAISE NOTICE 'PASS 7: Invalid expiry rejected';
    END;

    RAISE NOTICE 'All 7 authentication constraint tests passed';
END;
$$;

-- Preserve the state of the test database.
ROLLBACK;
