# StockGuard

Inventory Reservation & Order Management System.

**Project status:** Phase 3, Checkpoint DB-2 (PostgreSQL development environment only). This scaffold does **not** implement the Go API, React application, database tables, migrations, business logic, or approved acceptance criteria yet.

**Approved baseline:** Go + `net/http` + `database/sql`, PostgreSQL, monorepo, React + TypeScript + Vite + Tailwind CSS + TanStack Router, Docker Compose, GitHub Actions. Development will be incremental with verification at every checkpoint.

## Windows PowerShell quick start (one-time)

1. Open Docker Desktop and wait until the **Engine is running**.
2. Open PowerShell at the `stockguard` root (the directory containing `docker-compose.yml`).
3. Copy the environment example:

   ```powershell
   Copy-Item .env.example .env
   notepad .env
   ```

4. Replace `CHANGE_ME_TO_A_UNIQUE_LOCAL_PASSWORD` with your own local-only password. Do not commit `.env`, or share its contents/screenshots.
5. Start the database:

   ```powershell
   docker compose config --quiet
   docker compose up -d db
   docker compose ps
   ```

6. Check a real PostgreSQL connection **inside the container**:

   ```powershell
   docker compose exec db psql -U stockguard -d stockguard_dev -c "SELECT current_database(), current_user;"
   ```

7. Confirm persistent volume is attached:

   ```powershell
   docker volume inspect stockguard_postgres_data --format '{{.Name}}'
   ```

Expected service health: `healthy` after initialization. Expected query: database `stockguard_dev` and user `stockguard`. Commands are examples, not test results.

## Daily workflow

1. Start Docker Desktop (Docker engine must be available).
2. From this repository's root run `docker compose up -d db` if it has not automatically started.
3. Verify with `docker compose ps`.
4. If desired, stop without deleting the volume using `docker compose stop db`.

Never use `docker compose down -v` or `docker volume rm stockguard_postgres_data` as part of ordinary development. Those commands delete persisted database data.

The database is reachable from **this Windows host only** at `127.0.0.1:5433`. Future host-run Go backend configuration will use this port; API containers use the Docker service name `db` and container port `5432` instead.

**Important:** Initial `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` are applied to an empty Postgres data directory. Changing `.env` after the volume has already been initialized does not automatically change existing database credentials. Never remove volumes just to fix an environment mismatch; inspect the root cause.

## What's next?

DB-3: approve a concrete migration tool, create versioned migration files for the eight approved tables, and verify constraints on a separate test PostgreSQL database. The monorepo subdirectories currently contain only placeholders.

## Project design references

See `docs/PROJECT_STATUS.md`. The approved PRD and architecture decisions were agreed in the conversation, but **this starter is not a replacement for the full approved PRD**. Before DB-3, include the exact approved PRD and Amendment A in source-controlled documentation.
