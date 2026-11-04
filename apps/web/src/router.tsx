
import {
  createRootRoute,
  createRoute,
  createRouter,
  Navigate,
} from '@tanstack/react-router'

import { RootLayout } from './components/root-layout'
import {
  AdminOnly,
  LoginRoute,
  PasswordChangeRoute,
  ProtectedApp,
} from './components/auth-guards'

import { OverviewPage } from './pages/overview-page'
import { SectionPage } from './pages/section-page'
import { NotFoundPage } from './pages/not-found-page'

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
  component: LoginRoute,
})

const passwordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'change-password',
  component: PasswordChangeRoute,
})

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'app',
  component: ProtectedApp,
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
    <AdminOnly>
      <SectionPage
        title="Team"
        description="Manajemen akun Staff untuk administrator."
        nextPhase="FE-6"
      />
    </AdminOnly>
  ),
})

const routeTree = rootRoute.addChildren([
  homeRoute,
  loginRoute,
  passwordRoute,
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
