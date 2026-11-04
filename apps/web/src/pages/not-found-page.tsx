
import { Link } from '@tanstack/react-router'

export function NotFoundPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center px-5 text-center">
      <p className="text-6xl font-extrabold text-indigo-600">
        404
      </p>

      <h1 className="mt-4 text-2xl font-bold text-slate-950">
        Halaman tidak ditemukan
      </h1>

      <p className="mt-3 text-sm text-slate-500">
        URL yang kamu buka tidak tersedia di StockGuard.
      </p>

      <Link
        to="/app"
        className="mt-6 text-sm font-semibold text-indigo-600 hover:underline"
      >
        Kembali ke workspace
      </Link>
    </main>
  )
}
