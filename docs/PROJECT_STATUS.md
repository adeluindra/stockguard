
# StockGuard — Project Status

Last reviewed: 2026-11-03
Current checkpoint: BE-7C

## Project Scope

StockGuard is an inventory reservation and order management
system for one store and one warehouse.

Roles:
- Admin
- Staff

Order states:
- reserved
- fulfilled
- cancelled

## Implementation Progress

| Phase | Scope | Status |
|---|---|---|
| BE-1 | API foundation and PostgreSQL | Implemented |
| BE-2 | Authentication, sessions, CSRF and RBAC | Implemented |
| BE-3 | Product management | Implemented |
| BE-4 | Inventory adjustment and audit trail | Implemented |
| BE-5A | Order creation and reservations | Implemented |
| BE-5B | Fulfillment and cancellation | Implemented |
| BE-5C | Order queries and access scoping | Implemented |
| BE-6 | Dashboard and reporting | Implemented |
| BE-7A | Adjustment idempotency | Implemented |
| BE-7B | Security and concurrency regression tests | Implemented |
| BE-7C | CI, documentation and release verification | In progress |
| FE-1 | Frontend foundation | Not started |

Backend test results have been reported as passing locally.
GitHub Actions execution must be verified separately.

## Technology Stack

Backend:
- Go, net/http, database/sql
- PostgreSQL 17
- pgx driver
- golang-migrate
- Docker Compose

Frontend target:
- React
- TypeScript
- Vite
- Tailwind CSS
- TanStack Router

## Database

Current expected schema version: 5

Core entities:
- users
- sessions
- products
- inventory_balances
- orders
- order_items
- inventory_movements
- idempotency_records
- inventory_adjustment_idempotency

## Product Invariants

- available = on_hand - reserved
- 0 <= reserved <= on_hand
- Creating an order reserves every item atomically
- Fulfillment decreases on_hand and reserved
- Cancellation decreases reserved only
- Terminal order actions cannot double-apply changes
- Every committed stock mutation has an audit movement
- Repeated successful requests with the same idempotency
  key must not duplicate their stock effects

## Documentation

- API reference: docs/API.md
- Release checklist: docs/BACKEND_RELEASE_CHECKLIST.md
- Local setup: README.md

## Documentation Gaps

The previously approved PRD, amendments, design documents,
and ADRs should be transferred or linked to authoritative
repository documentation.

Do not treat this status summary as a replacement for them.

## Next Milestone

1. Verify go-checks and postgres-integration in GitHub Actions.
2. Review backend release checklist.
3. Start FE-1 after backend integration criteria pass.

Public production deployment is a separate milestone.
