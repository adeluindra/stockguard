
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
import { ProductsPage } from './pages/products-page'
import { InventoryPage } from './pages/inventory-page'
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
  component: ProductsPage,
})

const inventoryRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'inventory',
  component: InventoryPage,
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
