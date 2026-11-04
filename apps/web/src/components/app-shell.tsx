
import { useState } from 'react'

import {
  Link,
  Outlet,
  useNavigate,
} from '@tanstack/react-router'

import { useAuth } from '../auth/auth-context'
import { authErrorMessage } from '../lib/auth-client'

const navigation = [
  { to: '/app', label: 'Overview', short: '01' },
  { to: '/app/products', label: 'Products', short: '02' },
  { to: '/app/inventory', label: 'Inventory', short: '03' },
  { to: '/app/orders', label: 'Orders', short: '04' },
  { to: '/app/staff', label: 'Team', short: '05' },
] as const

export function AppShell() {
  const { state, logout } = useAuth()
  const navigate = useNavigate()

  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutError, setLogoutError] = useState('')

  if (state.status !== 'authenticated') {
    return null
  }

  const session = state.session
  const isAdmin = session.role === 'admin'

  const visibleNavigation = navigation.filter(
    (item) => item.to !== '/app/staff' || isAdmin,
  )

  async function handleLogout() {
    if (loggingOut) return

    setLogoutError('')
    setLoggingOut(true)

    try {
      await logout()

      await navigate({
        to: '/login',
        replace: true,
      })
    } catch (error) {
      setLogoutError(authErrorMessage(error))
    } finally {
      setLoggingOut(false)
    }
  }

  return (
    <div className="min-h-screen md:flex">
      <aside className="border-b border-slate-800 bg-slate-950 text-white md:flex md:w-64 md:shrink-0 md:flex-col md:border-b-0">
        <div className="flex items-center gap-3 px-5 py-5 md:px-7 md:py-8">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-indigo-500 text-lg font-black shadow-lg shadow-indigo-950/30">
            S
          </div>

          <div>
            <p className="text-lg font-bold tracking-tight">
              StockGuard
            </p>
            <p className="text-xs text-slate-400">
              Inventory workspace
            </p>
          </div>
        </div>

        <nav
          aria-label="Main navigation"
          className="flex gap-1 overflow-x-auto px-4 pb-4 md:mt-5 md:flex-col md:gap-2 md:px-4"
        >
          {visibleNavigation.map((item) => (
            <Link
              key={item.to}
              to={item.to}
              activeOptions={{ exact: true }}
              className="flex shrink-0 items-center gap-3 rounded-xl px-4 py-3 text-sm font-medium text-slate-300 transition-colors hover:bg-slate-800 hover:text-white data-[status=active]:bg-indigo-500 data-[status=active]:text-white"
            >
              <span className="text-xs opacity-70">
                {item.short}
              </span>
              <span>{item.label}</span>
            </Link>
          ))}
        </nav>

        <div className="mt-auto hidden border-t border-slate-800 px-7 py-6 md:block">
          <p className="text-xs font-semibold uppercase tracking-wider text-slate-400">
            Signed in
          </p>

          <p className="mt-2 text-sm font-semibold capitalize text-white">
            {session.role}
          </p>

          <p className="mt-1 text-xs text-slate-400">
            User ID #{session.user_id}
          </p>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex flex-wrap items-center justify-between gap-4 border-b border-slate-200 bg-white px-5 py-5 sm:px-8">
          <div>
            <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">
              StockGuard Workspace
            </p>

            <p className="mt-1 text-sm text-slate-500">
              Inventory & Order Management
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <span className="rounded-full bg-indigo-50 px-3 py-1.5 text-xs font-semibold capitalize text-indigo-700">
              {session.role}
            </span>

            <Link
              to="/change-password"
              className="text-xs font-semibold text-slate-600 hover:text-indigo-600"
            >
              Ganti password
            </Link>

            <button
              type="button"
              onClick={() => void handleLogout()}
              disabled={loggingOut}
              className="rounded-xl border border-slate-200 px-4 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-60"
            >
              {loggingOut ? 'Keluar...' : 'Logout'}
            </button>
          </div>
        </header>

        {logoutError && (
          <p
            role="alert"
            className="mx-5 mt-4 rounded-xl bg-rose-50 p-4 text-sm text-rose-700 sm:mx-8"
          >
            {logoutError}
          </p>
        )}

        <main
          id="main-content"
          className="mx-auto w-full max-w-7xl flex-1 px-5 py-7 sm:px-8 sm:py-10"
        >
          <Outlet />
        </main>
      </div>
    </div>
  )
}
