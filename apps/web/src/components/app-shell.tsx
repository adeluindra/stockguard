
import { Link, Outlet } from '@tanstack/react-router'

const navigation = [
  { to: '/app', label: 'Overview', short: '01' },
  { to: '/app/products', label: 'Products', short: '02' },
  { to: '/app/inventory', label: 'Inventory', short: '03' },
  { to: '/app/orders', label: 'Orders', short: '04' },
  { to: '/app/staff', label: 'Team', short: '05' },
] as const

export function AppShell() {
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
          {navigation.map((item) => (
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
            Development
          </p>
          <p className="mt-1 text-sm text-slate-300">
            Frontend Foundation
          </p>
          <p className="mt-2 text-xs text-slate-500">
            FE-1 · Local environment
          </p>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-slate-200 bg-white px-5 py-5 sm:px-8">
          <div>
            <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">
              StockGuard Workspace
            </p>
            <p className="mt-1 text-sm text-slate-500">
              Inventory & Order Management
            </p>
          </div>

          <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-semibold text-slate-600">
            FE-1
          </span>
        </header>

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
