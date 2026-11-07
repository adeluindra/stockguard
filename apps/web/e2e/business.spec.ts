
import {
  expect,
  request,
  test,
} from '@playwright/test'

import type {
  APIRequestContext,
  APIResponse,
} from '@playwright/test'

import { randomUUID } from 'node:crypto'

const BASE_URL = 'http://127.0.0.1:18080'

interface Product {
  id: number
  sku: string
  name: string
  is_active: boolean
}

interface Balance {
  on_hand: number
  reserved: number
  available: number
}

interface Order {
  id: number
  status: 'reserved' | 'fulfilled' | 'cancelled'
}

interface Session {
  role: string
  must_change_password: boolean
  csrf: string
}

interface OrderItem {
  product_id: number
  quantity: number
}

async function data<T>(
  response: APIResponse,
  expectedStatus: number,
): Promise<T> {
  expect(response.status()).toBe(expectedStatus)

  const payload = (await response.json()) as {
    data: T
  }

  return payload.data
}

async function login(
  client: APIRequestContext,
  username: string,
  password: string,
): Promise<Session> {
  const loginCsrf = await data<{ csrf_token: string }>(
    await client.get('/api/v1/auth/login-csrf'),
    200,
  )

  const identity = await data<{
    role: string
    must_change_password: boolean
  }>(
    await client.post('/api/v1/auth/login', {
      headers: {
        'X-CSRF-Token': loginCsrf.csrf_token,
      },
      data: {
        username,
        password,
      },
    }),
    200,
  )

  const sessionCsrf = await data<{
    csrf_token: string
  }>(
    await client.get('/api/v1/auth/csrf'),
    200,
  )

  return {
    ...identity,
    csrf: sessionCsrf.csrf_token,
  }
}

function post(
  client: APIRequestContext,
  csrf: string,
  path: string,
  payload?: unknown,
  key?: string,
): Promise<APIResponse> {
  return client.post(path, {
    headers: {
      'X-CSRF-Token': csrf,
      ...(key
        ? { 'Idempotency-Key': key }
        : {}),
    },
    ...(payload === undefined
      ? {}
      : { data: payload }),
  })
}

async function getBalance(
  client: APIRequestContext,
  productId: number,
): Promise<Balance> {
  return data<Balance>(
    await client.get(
      `/api/v1/products/${productId}/inventory`,
    ),
    200,
  )
}

function expectBalance(
  balance: Balance,
  onHand: number,
  reserved: number,
  available: number,
): void {
  expect(balance).toMatchObject({
    on_hand: onHand,
    reserved,
    available,
  })
}

test('FE-7B: transaksi stok, order, concurrency, dan RBAC', async () => {
  // Guard: agar tidak otomatis mengubah database
  // saat menjalankan tes FE-7A biasa.
  test.skip(
    process.env.E2E_WRITE_TARGET !== 'stockguard_e2e',
    'FE-7B hanya boleh dijalankan pada database khusus E2E.',
  )

  const adminPassword =
    process.env.E2E_BUSINESS_ADMIN_PASSWORD

  if (!adminPassword) {
    throw new Error(
      'E2E_BUSINESS_ADMIN_PASSWORD belum diatur.',
    )
  }

  const admin = await request.newContext({
    baseURL: BASE_URL,
    extraHTTPHeaders: {
      Origin: BASE_URL,
    },
  })

  const staff = await request.newContext({
    baseURL: BASE_URL,
    extraHTTPHeaders: {
      Origin: BASE_URL,
    },
  })

  try {
    const adminSession = await login(
      admin,
      'e2e_admin',
      adminPassword,
    )

    expect(adminSession.role).toBe('admin')
    expect(
      adminSession.must_change_password,
    ).toBe(false)

    const csrf = adminSession.csrf
    const suffix = randomUUID()
      .replaceAll('-', '')
      .slice(0, 10)

    let productA = 0
    let productB = 0
    let firstOrderId = 0

    await test.step('Products dan inventory adjustment', async () => {
      const first = await data<Product>(
        await post(
          admin,
          csrf,
          '/api/v1/products',
          {
            sku: `E2EA-${suffix}`,
            name: 'FE7B Test Product A',
            description: 'Produk pengujian terisolasi',
            low_stock_threshold: 2,
          },
        ),
        201,
      )

      const second = await data<Product>(
        await post(
          admin,
          csrf,
          '/api/v1/products',
          {
            sku: `E2EB-${suffix}`,
            name: 'FE7B Test Product B',
            description: null,
            low_stock_threshold: 1,
          },
        ),
        201,
      )

      productA = first.id
      productB = second.id

      expect(productA).toBeGreaterThan(0)
      expect(productB).toBeGreaterThan(0)

      expectBalance(
        await getBalance(admin, productA),
        0, 0, 0,
      )

      const updated = await admin.put(
        `/api/v1/products/${productA}`,
        {
          headers: {
            'X-CSRF-Token': csrf,
          },
          data: {
            name: 'FE7B Product A Updated',
            description: 'Updated by business test',
            low_stock_threshold: 3,
          },
        },
      )

      const updatedProduct =
        await data<Product>(updated, 200)

      expect(updatedProduct.name).toBe(
        'FE7B Product A Updated',
      )

      const adjustmentKey = randomUUID()
      const adjustment = {
        delta: 10,
        reason: 'FE7B initial stock product A',
      }

      const adjustmentPath =
        `/api/v1/products/${productA}` +
        '/inventory/adjustments'

      const firstAdjustment = await post(
        admin,
        csrf,
        adjustmentPath,
        adjustment,
        adjustmentKey,
      )

      expect(firstAdjustment.status()).toBe(200)

      expectBalance(
        await getBalance(admin, productA),
        10, 0, 10,
      )

      const replay = await post(
        admin,
        csrf,
        adjustmentPath,
        adjustment,
        adjustmentKey,
      )

      expect(replay.status()).toBe(200)

      const replayBody = await replay.json()

      expect(replayBody.replayed).toBe(true)

      expectBalance(
        await getBalance(admin, productA),
        10, 0, 10,
      )

      const keyConflict = await post(
        admin,
        csrf,
        adjustmentPath,
        {
          delta: 9,
          reason: adjustment.reason,
        },
        adjustmentKey,
      )

      expect(keyConflict.status()).toBe(409)

      const overdraw = await post(
        admin,
        csrf,
        adjustmentPath,
        {
          delta: -100,
          reason: 'FE7B overdraw rejection test',
        },
        randomUUID(),
      )

      expect(overdraw.status()).toBe(409)

      expectBalance(
        await getBalance(admin, productA),
        10, 0, 10,
      )

      const secondAdjustment = await post(
        admin,
        csrf,
        `/api/v1/products/${productB}` +
          '/inventory/adjustments',
        {
          delta: 5,
          reason: 'FE7B initial stock product B',
        },
        randomUUID(),
      )

      expect(secondAdjustment.status()).toBe(200)

      expectBalance(
        await getBalance(admin, productB),
        5, 0, 5,
      )

      // Proteksi backend tetap wajib meskipun
      // request dikirim langsung tanpa frontend.
      const withoutCsrf = await admin.post(
        '/api/v1/orders',
        {
          headers: {
            'Idempotency-Key': randomUUID(),
          },
          data: {
            items: [
              {
                product_id: productA,
                quantity: 1,
              },
            ],
          },
        },
      )

      expect(withoutCsrf.status()).toBe(403)
    })

    await test.step('Order multi-item dan fulfillment', async () => {
      const items: OrderItem[] = [
        { product_id: productA, quantity: 3 },
        { product_id: productB, quantity: 2 },
      ]

      const orderKey = randomUUID()

      const created = await data<Order>(
        await post(
          admin,
          csrf,
          '/api/v1/orders',
          { items },
          orderKey,
        ),
        201,
      )

      firstOrderId = created.id

      expect(created.status).toBe('reserved')

      expectBalance(
        await getBalance(admin, productA),
        10, 3, 7,
      )

      expectBalance(
        await getBalance(admin, productB),
        5, 2, 3,
      )

      const replay = await post(
        admin,
        csrf,
        '/api/v1/orders',
        { items },
        orderKey,
      )

      expect(replay.status()).toBe(200)

      const replayBody = await replay.json()

      expect(replayBody.replayed).toBe(true)
      expect(replayBody.data.id).toBe(firstOrderId)

      const conflict = await post(
        admin,
        csrf,
        '/api/v1/orders',
        {
          items: [
            { product_id: productA, quantity: 2 },
            { product_id: productB, quantity: 2 },
          ],
        },
        orderKey,
      )

      expect(conflict.status()).toBe(409)

      const insufficient = await post(
        admin,
        csrf,
        '/api/v1/orders',
        {
          items: [
            { product_id: productA, quantity: 100 },
            { product_id: productB, quantity: 1 },
          ],
        },
        randomUUID(),
      )

      expect(insufficient.status()).toBe(409)

      const path =
        `/api/v1/orders/${firstOrderId}`

      const fulfilled = await data<Order>(
        await post(
          admin,
          csrf,
          `${path}/fulfill`,
        ),
        200,
      )

      expect(fulfilled.status).toBe('fulfilled')

      expectBalance(
        await getBalance(admin, productA),
        7, 0, 7,
      )

      expectBalance(
        await getBalance(admin, productB),
        3, 0, 3,
      )

      const repeated = await post(
        admin,
        csrf,
        `${path}/fulfill`,
      )

      expect(repeated.status()).toBe(200)

      const repeatedBody = await repeated.json()

      expect(repeatedBody.replayed).toBe(true)

      const invalidCancel = await post(
        admin,
        csrf,
        `${path}/cancel`,
      )

      expect(invalidCancel.status()).toBe(409)

      expectBalance(
        await getBalance(admin, productA),
        7, 0, 7,
      )
    })

    await test.step('Cancellation melepas reservasi', async () => {
      const order = await data<Order>(
        await post(
          admin,
          csrf,
          '/api/v1/orders',
          {
            items: [
              { product_id: productA, quantity: 2 },
              { product_id: productB, quantity: 1 },
            ],
          },
          randomUUID(),
        ),
        201,
      )

      expectBalance(
        await getBalance(admin, productA),
        7, 2, 5,
      )

      const path =
        `/api/v1/orders/${order.id}`

      const cancelled = await data<Order>(
        await post(
          admin,
          csrf,
          `${path}/cancel`,
        ),
        200,
      )

      expect(cancelled.status).toBe('cancelled')

      expectBalance(
        await getBalance(admin, productA),
        7, 0, 7,
      )

      expectBalance(
        await getBalance(admin, productB),
        3, 0, 3,
      )

      const replay = await post(
        admin,
        csrf,
        `${path}/cancel`,
      )

      expect(replay.status()).toBe(200)

      expect(
        (await replay.json()).replayed,
      ).toBe(true)

      const invalidFulfill = await post(
        admin,
        csrf,
        `${path}/fulfill`,
      )

      expect(invalidFulfill.status()).toBe(409)
    })

    await test.step('Staff RBAC dan kepemilikan order', async () => {
      const username = `e2e_${suffix}`

      const staffCreated = await data<{
        user: {
          id: number
          username: string
        }
        temporary_password: string
      }>(
        await post(
          admin,
          csrf,
          '/api/v1/admin/staff',
          { username },
        ),
        201,
      )

      const staffId = staffCreated.user.id

      const initial = await login(
        staff,
        username,
        staffCreated.temporary_password,
      )

      expect(initial.role).toBe('staff')
      expect(initial.must_change_password).toBe(true)

      // Staff belum boleh mengakses bisnis
      // sebelum mengganti password sementara.
      const blocked = await staff.get(
        '/api/v1/orders',
      )

      expect(blocked.status()).toBe(403)

      const newPassword =
        `FE7B-${randomUUID()}-staff-password`

      const changed = await post(
        staff,
        initial.csrf,
        '/api/v1/auth/change-password',
        {
          current_password:
            staffCreated.temporary_password,
          new_password: newPassword,
        },
      )

      expect(changed.status()).toBe(204)

      const oldSession = await staff.get(
        '/api/v1/auth/me',
      )

      expect(oldSession.status()).toBe(401)

      const active = await login(
        staff,
        username,
        newPassword,
      )

      expect(active.must_change_password).toBe(false)

      const adminOnly = await staff.get(
        '/api/v1/admin/staff',
      )

      expect(adminOnly.status()).toBe(403)

      const otherOrder = await staff.get(
        `/api/v1/orders/${firstOrderId}`,
      )

      expect(otherOrder.status()).toBe(404)

      const forbiddenCreate = await post(
        staff,
        active.csrf,
        '/api/v1/products',
        {
          sku: `NOAUTH-${suffix}`,
          name: 'Forbidden Product',
          description: null,
          low_stock_threshold: 0,
        },
      )

      expect(forbiddenCreate.status()).toBe(403)

      const staffOrder = await data<Order>(
        await post(
          staff,
          active.csrf,
          '/api/v1/orders',
          {
            items: [
              { product_id: productA, quantity: 1 },
            ],
          },
          randomUUID(),
        ),
        201,
      )

      expect(staffOrder.status).toBe('reserved')

      const ownOrder = await data<Order>(
        await staff.get(
          `/api/v1/orders/${staffOrder.id}`,
        ),
        200,
      )

      expect(ownOrder.id).toBe(staffOrder.id)

      const ownList = await data<Array<{ id: number }>>(
        await staff.get(
          '/api/v1/orders?limit=20&offset=0',
        ),
        200,
      )

      expect(
        ownList.some(
          (order) => order.id === staffOrder.id,
        ),
      ).toBe(true)

      expect(
        ownList.some(
          (order) => order.id === firstOrderId,
        ),
      ).toBe(false)

      const forbiddenFulfill = await post(
        staff,
        active.csrf,
        `/api/v1/orders/${staffOrder.id}/fulfill`,
      )

      expect(forbiddenFulfill.status()).toBe(403)

      // Admin dapat menyelesaikan order Staff.
      const fulfilled = await post(
        admin,
        csrf,
        `/api/v1/orders/${staffOrder.id}/fulfill`,
      )

      expect(fulfilled.status()).toBe(200)

      expectBalance(
        await getBalance(admin, productA),
        6, 0, 6,
      )

      // Deaktivasi juga harus mencabut sesi Staff.
      const deactivate = await post(
        admin,
        csrf,
        `/api/v1/admin/staff/${staffId}/deactivate`,
      )

      expect(deactivate.status()).toBe(204)

      const revoked = await staff.get(
        '/api/v1/auth/me',
      )

      expect(revoked.status()).toBe(401)
    })

    await test.step('Concurrent orders tidak oversell', async () => {
      // Available A = 6. Dua order masing-masing
      // meminta 5 unit. Hanya satu boleh berhasil.
      const payload = {
        items: [
          { product_id: productA, quantity: 5 },
        ],
      }

      const results = await Promise.all([
        post(
          admin,
          csrf,
          '/api/v1/orders',
          payload,
          randomUUID(),
        ),
        post(
          admin,
          csrf,
          '/api/v1/orders',
          payload,
          randomUUID(),
        ),
      ])

      expect(
        results
          .map((response) => response.status())
          .sort((a, b) => a - b),
      ).toEqual([201, 409])

      const winner = results.find(
        (response) => response.status() === 201,
      )

      expect(winner).toBeDefined()

      const winningOrder = await data<Order>(
        winner!,
        201,
      )

      expectBalance(
        await getBalance(admin, productA),
        6, 5, 1,
      )

      const finalized = await post(
        admin,
        csrf,
        `/api/v1/orders/${winningOrder.id}/fulfill`,
      )

      expect(finalized.status()).toBe(200)

      expectBalance(
        await getBalance(admin, productA),
        1, 0, 1,
      )
    })

    await test.step('Produk nonaktif dan movement audit', async () => {
      const history = await data<Array<{
        movement_type: string
      }>>(
        await admin.get(
          `/api/v1/products/${productA}` +
          '/inventory/movements?limit=100&offset=0',
        ),
        200,
      )

      const movementTypes = history.map(
        (movement) => movement.movement_type,
      )

      expect(movementTypes).toContain('adjustment')
      expect(movementTypes).toContain('reserve')
      expect(movementTypes).toContain('fulfill')
      expect(movementTypes).toContain('release')

      const deactivation = await post(
        admin,
        csrf,
        `/api/v1/products/${productA}/deactivate`,
      )

      expect(deactivation.status()).toBe(204)

      const product = await data<Product>(
        await admin.get(
          `/api/v1/products/${productA}`,
        ),
        200,
      )

      expect(product.is_active).toBe(false)

      const inactiveOrder = await post(
        admin,
        csrf,
        '/api/v1/orders',
        {
          items: [
            { product_id: productA, quantity: 1 },
          ],
        },
        randomUUID(),
      )

      expect(inactiveOrder.status()).toBe(409)
    })
  } finally {
    await Promise.all([
      admin.dispose(),
      staff.dispose(),
    ])
  }
})
