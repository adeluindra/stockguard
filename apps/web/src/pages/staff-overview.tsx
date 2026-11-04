
import { Link } from '@tanstack/react-router'

const staffModules = [
  {
    to: '/app/products',
    title: 'Product Catalog',
    description:
      'Lihat informasi produk dan stok yang tersedia.',
    number: '01',
  },
  {
    to: '/app/inventory',
    title: 'Inventory',
    description:
      'Lihat informasi stok sesuai izin Staff.',
    number: '02',
  },
  {
    to: '/app/orders',
    title: 'My Orders',
    description:
      'Kelola pesanan yang dibuat oleh akunmu.',
    number: '03',
  },
] as const

export function StaffOverview() {
  return (
    <div className="space-y-8">
      <section className="rounded-3xl bg-slate-950 px-8 py-12 text-white">
        <span className="rounded-full bg-indigo-500/20 px-4 py-2 text-xs font-semibold text-indigo-200">
          STAFF WORKSPACE
        </span>

        <h1 className="mt-7 text-3xl font-bold tracking-tight sm:text-4xl">
          Selamat datang di StockGuard
        </h1>

        <p className="mt-4 max-w-xl text-sm leading-7 text-slate-300">
          Akses produk, informasi inventory, dan pesanan
          sesuai izin akun Staff.
        </p>
      </section>

      <div>
        <h2 className="text-xl font-bold text-slate-950">
          Workspace Modules
        </h2>

        <p className="mt-2 text-sm text-slate-500">
          Pilih modul untuk melanjutkan pekerjaan.
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {staffModules.map((module) => (
          <Link
            key={module.to}
            to={module.to}
            className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm transition hover:border-indigo-200 hover:shadow-md"
          >
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-indigo-50 text-sm font-bold text-indigo-600">
              {module.number}
            </div>

            <h3 className="mt-6 text-lg font-semibold text-slate-950">
              {module.title}
            </h3>

            <p className="mt-2 text-sm leading-6 text-slate-500">
              {module.description}
            </p>
          </Link>
        ))}
      </div>

      <p className="text-xs leading-6 text-slate-500">
        Ringkasan stok global, audit aktivitas global,
        dan administrasi pengguna hanya tersedia untuk Admin.
      </p>
    </div>
  )
}
