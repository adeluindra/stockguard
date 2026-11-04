
# StockGuard API Reference

Status: Backend MVP implemented; final CI verification pending.

## Base URL

Development: `http://127.0.0.1:8080`

API prefix: `/api/v1`

## Authentication

StockGuard uses an HTTP-only session cookie.

### Login flow

1. `GET /api/v1/auth/login-csrf`
   - Receive `csrf_token`.
   - Browser receives the login CSRF cookie.

2. `POST /api/v1/auth/login`
   - Send `Origin`, `X-CSRF-Token`, and `Content-Type: application/json`.
   - Body:

   ```json
   {
     "username": "example",
     "password": "your-password"
   }
   ```

3. After login, the browser receives the session cookie.

4. `GET /api/v1/auth/csrf`
   - Obtain the session CSRF token.

5. For POST and PUT requests protected by the session middleware:
   - Include the session cookie.
   - Include the correct Origin header.
   - Include `X-CSRF-Token`.

Never store the session cookie value in localStorage or frontend logs.

### Authentication endpoints

| Method | Path | Access |
|---|---|---|
| GET | /api/v1/auth/login-csrf | Public |
| POST | /api/v1/auth/login | Public + login CSRF |
| GET | /api/v1/auth/me | Authenticated |
| GET | /api/v1/auth/csrf | Authenticated |
| POST | /api/v1/auth/change-password | Authenticated + CSRF |
| POST | /api/v1/auth/logout | Session + CSRF |

The first login with a temporary password can require an immediate password change.

## Admin Staff Management

| Method | Path | Access |
|---|---|---|
| GET | /api/v1/admin/staff | Admin |
| POST | /api/v1/admin/staff | Admin |
| POST | /api/v1/admin/staff/{id}/reset-password | Admin |
| POST | /api/v1/admin/staff/{id}/deactivate | Admin |

Creating or resetting staff credentials returns a temporary password.
Display this password only to the authorized administrator.

## Products

| Method | Path | Access |
|---|---|---|
| GET | /api/v1/products | Admin, Staff |
| GET | /api/v1/products/{id} | Admin, Staff |
| POST | /api/v1/products | Admin |
| PUT | /api/v1/products/{id} | Admin |
| POST | /api/v1/products/{id}/deactivate | Admin |

Product creation example:

```json
{
  "sku": "SKU-001",
  "name": "Example Product",
  "description": "Optional description",
  "low_stock_threshold": 5
}
```

Product creation initializes inventory at zero.

Product updates do not modify the SKU or inventory quantity.

List parameters:
- `limit`: 1–100, default 20
- `offset`: default 0
- `include_inactive`: Admin-only when true

Inactive products remain stored for historical consistency.

## Inventory

| Method | Path | Access |
|---|---|---|
| GET | /api/v1/products/{id}/inventory | Admin, Staff |
| POST | /api/v1/products/{id}/inventory/adjustments | Admin |
| GET | /api/v1/products/{id}/inventory/movements | Admin |

Inventory adjustment:

```http
POST /api/v1/products/1/inventory/adjustments
Content-Type: application/json
Idempotency-Key: 71fa94a1-8c33-45a6-b915-ecf719cf48c1
X-CSRF-Token: <session-csrf-token>
```

```json
{
  "delta": 10,
  "reason": "Goods received from supplier"
}
```

Response:

```json
{
  "data": {
    "balance": {},
    "movement": {}
  },
  "replayed": false
}
```

The `balance` and `movement` objects contain their respective
inventory fields.

Rules:
- `available = on_hand - reserved`
- `reserved <= on_hand`
- `delta` must be nonzero
- Reason must contain 10–500 characters
- Adjustment cannot consume reserved stock
- Idempotency-Key is mandatory
- Reusing the key with a different payload returns 409
- Reusing the key with the same payload returns the original result

Movement history supports `limit` and `offset`.

## Orders

| Method | Path | Access |
|---|---|---|
| GET | /api/v1/orders | Admin, Staff |
| GET | /api/v1/orders/{id} | Admin, Staff |
| POST | /api/v1/orders | Admin, Staff |
| POST | /api/v1/orders/{id}/fulfill | Admin |
| POST | /api/v1/orders/{id}/cancel | Admin |

Staff can read only orders created by their own user account.
Admin can read all orders.

Order creation requires `Idempotency-Key`.

Example:

```json
{
  "items": [
    {
      "product_id": 1,
      "quantity": 3
    },
    {
      "product_id": 2,
      "quantity": 2
    }
  ]
}
```

Rules:
- Maximum 50 unique products per order
- All items reserve stock atomically
- No partial reservations
- New order returns 201
- Successful idempotent replay returns 200
- Order state is one of: reserved, fulfilled, cancelled
- Fulfillment decreases on_hand and reserved
- Cancellation decreases reserved only
- Repeating an identical terminal action is safe
- Opposite terminal action returns 409

Order listing:
- `status`: reserved, fulfilled, cancelled, or omitted
- `limit`: 1–100, default 20
- `offset`: default 0

The response includes `pagination.has_more`.

## Dashboard

All dashboard endpoints require Admin.

| Method | Path | Function |
|---|---|---|
| GET | /api/v1/dashboard/summary | Inventory and order KPIs |
| GET | /api/v1/dashboard/low-stock | Products at/below threshold |
| GET | /api/v1/dashboard/activity | Inventory audit events |

Low-stock and activity endpoints support limit/offset pagination.

Dashboard queries are read-only.

## Health

| Method | Path |
|---|---|
| GET | /healthz |
| GET | /readyz |

`/healthz` checks process liveness.
`/readyz` checks database readiness and minimum schema version.

## Response conventions

Successful object responses use a `data` field.

Paginated responses use:

```json
{
  "data": [],
  "pagination": {
    "limit": 20,
    "offset": 0,
    "has_more": false
  }
}
```

Error responses use:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Error message",
    "details": [],
    "request_id": "request-id"
  }
}
```

Common HTTP statuses:
- 200: success
- 201: resource created
- 204: successful operation with no body
- 400: invalid input
- 401: authentication required
- 403: insufficient permission or invalid CSRF
- 404: resource not found or hidden by access policy
- 409: state, stock, or idempotency conflict
- 429: login rate limit
- 503: service unavailable

## Security notes

- Use HTTPS outside localhost.
- Never expose PostgreSQL directly to the internet.
- Do not log passwords, session cookies, or CSRF tokens.
- Do not automatically retry a write operation with a new
  idempotency key after an uncertain network failure.
- The frontend must preserve the same key when retrying
  the same order creation or inventory adjustment.
