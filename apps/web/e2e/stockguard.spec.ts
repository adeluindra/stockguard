
import {
  expect,
  test,
} from '@playwright/test'

import type { Page } from '@playwright/test'

const origin = 'http://127.0.0.1:5173'

const adminUsername =
  process.env.E2E_ADMIN_USERNAME || 'admin'

const adminPassword =
  process.env.E2E_ADMIN_PASSWORD || ''

const staffUsername =
  process.env.E2E_STAFF_USERNAME || ''

const staffPassword =
  process.env.E2E_STAFF_PASSWORD || ''

async function login(
  page: Page,
  username: string,
  password: string,
): Promise<void> {
  if (!password) {
    throw new Error(
      'Password E2E belum disediakan melalui environment variable.',
    )
  }

  await page.goto('/login')

  await expect(
    page.getByRole('heading', {
      name: 'Masuk ke akun',
    }),
  ).toBeVisible()

  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Password').fill(password)

  await page.getByRole('button', {
    name: 'Masuk',
    exact: true,
  }).click()

  await expect(page).toHaveURL(/\/app\/?$/)
}

test.describe('Public access', () => {
  test('API dan PostgreSQL siap', async ({ request }) => {
    const response = await request.get('/readyz')

    expect(response.status()).toBe(200)

    const body = await response.json()

    expect(body.data.status).toBe('ok')
    expect(body.data.database).toBe('ok')
  })

  test('halaman privat mengarahkan anonymous ke login', async ({
    page,
  }) => {
    await page.goto('/app/orders')

    await expect(page).toHaveURL(/\/login\/?$/)

    await expect(
      page.getByRole('heading', {
        name: 'Masuk ke akun',
      }),
    ).toBeVisible()
  })

  test('anonymous ditolak oleh endpoint privat', async ({
    request,
  }) => {
    const paths = [
      '/api/v1/auth/me',
      '/api/v1/products',
      '/api/v1/orders',
      '/api/v1/dashboard/summary',
      '/api/v1/admin/staff',
    ]

    for (const path of paths) {
      const response = await request.get(path)

      expect(
        response.status(),
        `${path} harus mengembalikan 401`,
      ).toBe(401)
    }
  })
})

test.describe('Admin authentication and navigation', () => {
  test.beforeEach(async ({ page }) => {
    await login(
      page,
      adminUsername,
      adminPassword,
    )
  })

  test('login menghasilkan sesi Admin yang valid', async ({
    page,
  }) => {
    const response = await page.request.get(
      '/api/v1/auth/me',
    )

    expect(response.status()).toBe(200)

    const body = await response.json()

    expect(body.data.role).toBe('admin')
    expect(body.data.must_change_password).toBe(false)

    await expect(
      page.getByRole('link', {
        name: /Team/,
      }),
    ).toBeVisible()
  })

  test('Admin dapat membaca dashboard dan seluruh modul', async ({
    page,
  }) => {
    await expect(
      page.getByRole('heading', {
        name: 'Dashboard Operasional',
      }),
    ).toBeVisible()

    const summary = await page.request.get(
      '/api/v1/dashboard/summary',
    )

    expect(summary.status()).toBe(200)

    const body = await summary.json()

    expect(
      typeof body.data.inventory.active_products,
    ).toBe('number')

    await page.goto('/app/products')

    await expect(
      page.getByRole('heading', {
        name: 'Product Catalog',
      }),
    ).toBeVisible()

    await page.goto('/app/inventory')

    await expect(
      page.getByRole('heading', {
        name: 'Inventory Control',
      }),
    ).toBeVisible()

    await page.goto('/app/orders')

    await expect(
      page.getByRole('heading', {
        name: 'Order Management',
      }),
    ).toBeVisible()

    await page.goto('/app/staff')

    await expect(
      page.getByRole('heading', {
        name: 'Team Management',
      }),
    ).toBeVisible()
  })

  test('logout tanpa CSRF harus ditolak', async ({
    page,
  }) => {
    const response = await page.request.post(
      '/api/v1/auth/logout',
      {
        headers: {
          Origin: origin,
        },
      },
    )

    expect(response.status()).toBe(403)

    // Request yang ditolak tidak boleh
    // mencabut sesi Admin.
    const session = await page.request.get(
      '/api/v1/auth/me',
    )

    expect(session.status()).toBe(200)
  })

  test('logout dengan Origin berbeda harus ditolak', async ({
    page,
  }) => {
    const csrfResponse = await page.request.get(
      '/api/v1/auth/csrf',
    )

    expect(csrfResponse.status()).toBe(200)

    const csrf = await csrfResponse.json()

    const response = await page.request.post(
      '/api/v1/auth/logout',
      {
        headers: {
          Origin: 'https://attacker.example',
          'X-CSRF-Token': csrf.data.csrf_token,
        },
      },
    )

    expect(response.status()).toBe(403)

    const session = await page.request.get(
      '/api/v1/auth/me',
    )

    expect(session.status()).toBe(200)
  })

  test('logout mencabut sesi dan kembali ke login', async ({
    page,
  }) => {
    await page.getByRole('button', {
      name: 'Logout',
    }).click()

    await expect(page).toHaveURL(/\/login\/?$/)

    const response = await page.request.get(
      '/api/v1/auth/me',
    )

    expect(response.status()).toBe(401)

    await page.goto('/app')

    await expect(page).toHaveURL(/\/login\/?$/)
  })
})

test.describe('Staff authorization', () => {
  test('Staff tidak dapat membaca resource khusus Admin', async ({
    page,
  }) => {
    test.skip(
      !staffUsername || !staffPassword,
      'Set E2E_STAFF_USERNAME dan E2E_STAFF_PASSWORD untuk menjalankan pengujian Staff.',
    )

    await login(
      page,
      staffUsername,
      staffPassword,
    )

    await expect(
      page.getByRole('heading', {
        name: 'Selamat datang di StockGuard',
      }),
    ).toBeVisible()

    await expect(
      page.getByRole('link', {
        name: /Team/,
      }),
    ).toHaveCount(0)

    const forbiddenPaths = [
      '/api/v1/admin/staff',
      '/api/v1/dashboard/summary',
      '/api/v1/dashboard/activity',
      '/api/v1/products?include_inactive=true',
    ]

    for (const path of forbiddenPaths) {
      const response = await page.request.get(path)

      expect(
        response.status(),
        `Staff tidak boleh mengakses ${path}`,
      ).toBe(403)
    }

    await page.goto('/app/staff')

    await expect(
      page.getByRole('heading', {
        name: 'Akses khusus Admin',
      }),
    ).toBeVisible()
  })
})
