# StockGuard — Project status

- Phase 0: Discovery approved — one store, one warehouse, Admin/Staff, manual orders, automatic reservations.
- Phase 1: PRD v1.0 approved.
- Phase 2: SD-1..SD-4 and ADR-001..ADR-035 approved.
- Phase 3 DB-1: approved by user's instruction to accept recommendations and start implementation; candidate eight-table design.
- Phase 3 DB-2: starter PostgreSQL environment, **not yet verified on the user's computer**.

## Product invariants (approved)

- `available = on_hand - reserved`
- `0 <= reserved <= on_hand`
- Order states: `reserved`, `fulfilled`, `cancelled`
- Create order reserves all items atomically or none.
- Fulfillment decreases both `on_hand` and `reserved` for all items exactly once.
- Cancellation decreases `reserved` only.
- Each successful create-order idempotency key maps to one valid order.
- Every committed inventory balance mutation has an associated movement in the same transaction.

## Design entities

`users`, `sessions`, `products`, `inventory_balances`, `orders`, `order_items`, `inventory_movements`, `idempotency_records`.

## Caution

The full PRD, PRD Amendment A, detailed API contracts, and ERD must be transferred verbatim from approved source documents before implementing related domain behavior. This status summary does not supersede those documents.
