import {
  createRootRoute,
  createRoute,
  createRouter,
  Link,
  Navigate,
  Outlet,
} from '@tanstack/react-router'

import { AppShell } from './components/app-shell'
import { OverviewPage } from './pages/overview-page'
import { SectionPage } from './pages/section-page'

function RootLayout() {
  return <Outlet />
}

function LoginPlaceholder() {
  return (
    <main className="flex min-h-screen items-center justify-center px-5">
      <div className="w-full max-w-md rounded-3xl border border-slate-200 bg-white p-8 text-center shadow-xl shadow-slate-100">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-600 text-2xl font-bold text-white">
          S
        </div>

        <h1 className="mt-6 text-2xl font-bold text-slate-900">
          StockGuard Login
        </h1>

        <p className="mt-3 text-sm leading-7 text-slate-500">
          Halaman autentikasi akan dibuat pada FE-2,
          termasuk login, session handling, dan CSRF.
        </p>

        <Link
          to="/app"
          className="mt-7 inline-flex rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500"
        >
          Preview workspace
        </Link>
      </div>
    </main>
  )
}

function NotFoundPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center px-5 text-center">
      <p className="text-6xl font-extrabold text-indigo-600">
        404
      </p>

      <h1 className="mt-4 text-2xl font-bold">
        Halaman tidak ditemukan
      </h1>

      <Link
        to="/app"
        className="mt-6 text-sm font-semibold text-indigo-600 hover:underline"
      >
        Kembali ke workspace
      </Link>
    </main>
  )
}

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
})

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: () => <Navigate to="/app" replace />,
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'login',
  component: LoginPlaceholder,
})

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'app',
  component: AppShell,
})

const overviewRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  component: OverviewPage,
})

const productsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'products',
  component: () => (
    <SectionPage
      title="Products"
      description="Pengelolaan katalog, SKU, metadata produk, dan status aktif."
      nextPhase="FE-4"
    />
  ),
})

const inventoryRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'inventory',
  component: () => (
    <SectionPage
      title="Inventory"
      description="Monitoring on-hand, reserved, available stock, dan audit movements."
      nextPhase="FE-4"
    />
  ),
})

const ordersRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'orders',
  component: () => (
    <SectionPage
      title="Orders"
      description="Pembuatan order, reservasi barang, fulfillment, dan cancellation."
      nextPhase="FE-5"
    />
  ),
})

const staffRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'staff',
  component: () => (
    <SectionPage
      title="Team"
      description="Manajemen akun Staff untuk administrator."
      nextPhase="FE-6"
    />
  ),
})

const routeTree = rootRoute.addChildren([
  homeRoute,
  loginRoute,
  appRoute.addChildren([
    overviewRoute,
    productsRoute,
    inventoryRoute,
    ordersRoute,
    staffRoute,
  ]),
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
