
# StockGuard Backend Release Checklist

Status: BE-7C verification pending.

## 1. Code Quality

- [ ] `go mod verify` passes
- [ ] All Go files pass gofmt
- [ ] `go vet ./...` passes
- [ ] `go build ./...` passes
- [ ] Unit tests pass
- [ ] Go race detector passes

## 2. Database

- [ ] PostgreSQL 17 test service starts successfully
- [ ] Migrations 000001–000005 apply in order
- [ ] schema_migrations reports version 5, dirty=false
- [ ] SQL constraint tests 000001–000004 pass
- [ ] PostgreSQL integration tests run without SKIP
- [ ] Inventory balances reconcile with movement totals
- [ ] No development or production database is used by CI

## 3. Authentication & Authorization

- [ ] Anonymous access to protected routes returns 401
- [ ] Staff cannot use Admin-only endpoints
- [ ] Staff cannot read another Staff member's orders
- [ ] Unsafe session-based requests require Origin and CSRF
- [ ] Mandatory password change is enforced
- [ ] Password reset and deactivation revoke sessions
- [ ] No session token or password is written to logs

## 4. Inventory & Orders

- [ ] Reserved quantity never exceeds on-hand stock
- [ ] Concurrent orders do not oversell
- [ ] Multi-item orders are atomic
- [ ] Concurrent fulfill/cancel cannot double-apply stock
- [ ] Every committed stock mutation has an audit movement
- [ ] Adjustment idempotency prevents double execution
- [ ] Order creation idempotency prevents duplicate orders
- [ ] Failed operations roll back completely

## 5. GitHub Actions

- [ ] Workflow triggers on push
- [ ] Workflow triggers on pull requests
- [ ] go-checks job passes
- [ ] postgres-integration job passes
- [ ] Both jobs are required checks on protected main
- [ ] CI uses only disposable credentials and database data

## 6. API Integration Readiness

- [ ] API contract reviewed against implementation
- [ ] Error response structure is consistent
- [ ] Frontend authentication flow is documented
- [ ] CSRF and Idempotency-Key requirements are documented
- [ ] `/healthz` and `/readyz` respond correctly
- [ ] Frontend API client handles 401, 403, 409, 429, 503
- [ ] No secrets are embedded in frontend code

## 7. Before Public Production Deployment

- [ ] Use HTTPS/TLS for all external traffic
- [ ] Set SESSION_COOKIE_SECURE=true
- [ ] Configure APP_ORIGIN with the trusted HTTPS origin
- [ ] Restrict database network access
- [ ] Store credentials in secure environment/secrets storage
- [ ] Configure external rate limiting at the reverse proxy
- [ ] Review distributed rate limiting if multiple API replicas
- [ ] Implement database backup and restore procedures
- [ ] Test a database restore in a separate environment
- [ ] Configure monitoring, alerting, and log retention
- [ ] Review security headers at the reverse proxy
- [ ] Review dependency vulnerability scan results
- [ ] Test migration and rollback procedures in staging
- [ ] Perform an independent security review

## Known limitations / follow-up items

1. The login rate limiter is currently in process memory.
   It does not coordinate across multiple application instances.

2. CI does not perform deployment to a public server.

3. The web frontend is still a placeholder.

4. Historical migrations must not be edited after deployment.
   Add new versioned migrations for future schema changes.

5. Inspect the nested `apps/api/apps/api/` directory.
   It contains additional tracked source copies. Determine
   whether they are required before removing anything.

6. The approved PRD, amendments, and architecture decisions
   should be included or linked to authoritative project
   documentation before a formal production release.

## Release decision

Backend may be considered ready for frontend integration after
all CI and API integration acceptance criteria pass.

Public production readiness requires completing the additional
security, deployment, backup, and operations checks above.
