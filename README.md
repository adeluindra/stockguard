
# StockGuard

Inventory Reservation & Order Management System.

StockGuard is a full-stack application for managing products,
inventory quantities, stock reservations, orders, and audit trails.

## Project Status

Backend MVP: implemented, final CI verification in progress.

Frontend: not started.

This repository is not yet approved for public production deployment.

See:
- [Project Status](docs/PROJECT_STATUS.md)
- [API Reference](docs/API.md)
- [Backend Release Checklist](docs/BACKEND_RELEASE_CHECKLIST.md)

## Tech Stack

Backend:
- Go (version specified in apps/api/go.mod)
- net/http
- database/sql with pgx
- PostgreSQL 17
- Docker Compose
- golang-migrate

Frontend (planned):
- React
- TypeScript
- Vite
- Tailwind CSS
- TanStack Router

Quality:
- Go unit and integration tests
- PostgreSQL constraint tests
- Concurrency regression tests
- GitHub Actions

## Repository Structure

```text
stockguard/
├── .github/workflows/
├── apps/
│   ├── api/
│   │   ├── cmd/
│   │   └── internal/
│   │       ├── auth/
│   │       ├── products/
│   │       ├── inventory/
│   │       ├── orders/
│   │       ├── dashboard/
│   │       └── httpapi/
│   └── web/
├── infra/postgres/
│   ├── migrations/
│   └── tests/
├── scripts/
├── docs/
├── docker-compose.yml
└── .env.example
```

## Local Requirements

- Windows with PowerShell
- Docker Desktop with running engine
- Go matching apps/api/go.mod
- golang-migrate CLI
- Git

## Local Development

Run from the repository root.

### 1. Configure environment

```powershell
Copy-Item .env.example .env
notepad .env
```

Set a unique local PostgreSQL password.

Never commit .env.

### 2. Start PostgreSQL

```powershell
docker compose up -d --wait db
docker compose ps
```

Default Windows host port: 5433.

The port is bound to 127.0.0.1 by Docker Compose.

### 3. Apply database migrations

All five migrations must be applied before starting the API.

Use the golang-migrate CLI with the connection string
derived from your local .env.

Use a relative migration path from the repository root:

```powershell
migrate -path "infra/postgres/migrations" -database "<local-postgres-dsn>" up
```

Do not copy the placeholder literally. Replace it with your
locally constructed PostgreSQL DSN.

Do not publish your DSN because it contains credentials.

Verify:

```powershell
docker compose exec -T db psql -U stockguard -d stockguard_dev -Atc "SELECT version, dirty FROM schema_migrations;"
```

Expected schema state: version 5, dirty=false.

Never use `migrate force` to hide a failed migration.

### 4. Start the API

```powershell
.\scripts\start-api.ps1
```

Local API: http://127.0.0.1:8080

Health:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

### 5. Run Go tests

From the repository root:

```powershell
cd apps/api

go mod verify
go vet ./...
go test ./... -count=1
```

PostgreSQL integration tests require TEST_DATABASE_URL pointing
to the separate `stockguard_test` database.

Without TEST_DATABASE_URL, integration tests may be skipped.

For complete regression coverage:

```powershell
go test -p 1 ./... -count=1
```

Run this with a valid TEST_DATABASE_URL.

Do not run database tests against stockguard_dev or production.

## Main Features

- Admin and Staff authentication
- Session and CSRF protection
- Staff account administration
- Product management
- Stock adjustments
- Stock reservation
- Order fulfillment and cancellation
- Order ownership-based access
- Inventory movement audit trail
- Dashboard and low-stock reporting
- Idempotent stock-changing requests

## Inventory Rules

```text
available = on_hand - reserved
0 <= reserved <= on_hand
```

All inventory-changing workflows use PostgreSQL transactions.

The inventory movement history records committed quantity changes.

## API Documentation

See [docs/API.md](docs/API.md).

All protected write operations require authentication and CSRF.

Order creation and stock adjustment also require an
Idempotency-Key header.

## CI

GitHub Actions workflow:
`.github/workflows/backend-ci.yml`

Jobs:
- go-checks
- postgres-integration

The workflow runs on pushes, pull requests, and manual dispatch.

The integration job uses disposable PostgreSQL credentials.
No real database or production secrets are needed.

## Security

- Never commit .env or credential files
- Never log passwords or session tokens
- Use HTTPS and Secure cookies in production
- Restrict database network exposure
- Maintain backups before production migrations
- Review the backend release checklist before deployment

## Roadmap

1. Complete BE-7C CI and final verification
2. Start FE-1 frontend foundation
3. Integrate authentication, products, inventory, and orders
4. Run frontend and end-to-end tests
5. Prepare a separate staging and production release
```
