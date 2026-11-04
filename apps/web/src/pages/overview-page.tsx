import { useAuth } from '../auth/auth-context'
import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { getApiData } from '../lib/api'

type BackendState = 'checking' | 'online' | 'offline'

interface Readiness {
    status: string
    database: string
}

const modules = [
    {
        title: 'Product Catalog',
        description: 'Kelola data barang dan status produk.',
        to: '/app/products',
        number: '01',
    },
    {
        title: 'Inventory Control',
        description: 'Monitor stok, reservasi, dan pergerakan.',
        to: '/app/inventory',
        number: '02',
    },
    {
        title: 'Order Management',
        description: 'Kelola reservasi dan status pesanan.',
        to: '/app/orders',
        number: '03',
    },
    {
        title: 'Team Administration',
        description: 'Kelola akun Staff dan hak akses.',
        to: '/app/staff',
        number: '04',
    },
] as const

export function OverviewPage() {

    const { state } = useAuth()

    const isAdmin =
        state.status === 'authenticated' &&
        state.session.role === 'admin'

    const visibleModules = modules.filter(
        (module) =>
            module.to !== '/app/staff' || isAdmin,
    )

    const [backend, setBackend] =
        useState<BackendState>('checking')

    const [revision, setRevision] = useState(0)

    useEffect(() => {
        const controller = new AbortController()

        getApiData<Readiness>('/readyz', controller.signal)
            .then((result) => {
                if (controller.signal.aborted) return

                const ready =
                    result.status === 'ok' &&
                    result.database === 'ok'

                setBackend(ready ? 'online' : 'offline')
            })
            .catch(() => {
                if (!controller.signal.aborted) {
                    setBackend('offline')
                }
            })

        return () => controller.abort()
    }, [revision])

    function checkAgain() {
        setBackend('checking')
        setRevision((value) => value + 1)
    }

    const backendLabel = {
        checking: 'Checking connection',
        online: 'Backend connected',
        offline: 'Backend unavailable',
    }[backend]

    const backendStyle = {
        checking: 'bg-amber-50 text-amber-700',
        online: 'bg-emerald-50 text-emerald-700',
        offline: 'bg-rose-50 text-rose-700',
    }[backend]

    return (
        <div className="space-y-8">
            <section className="relative overflow-hidden rounded-3xl bg-slate-950 px-7 py-10 text-white shadow-xl shadow-slate-200 sm:px-10 sm:py-12">
                <div className="pointer-events-none absolute -right-20 -top-32 h-80 w-80 rounded-full bg-indigo-500/20 blur-3xl" />

                <div className="relative">
                    <span className="inline-flex rounded-full border border-indigo-400/30 bg-indigo-400/10 px-4 py-2 text-xs font-semibold text-indigo-200">
                        FRONTEND FOUNDATION
                    </span>

                    <h1 className="mt-6 max-w-2xl text-3xl font-bold tracking-tight sm:text-4xl">
                        Welcome to StockGuard
                    </h1>

                    <p className="mt-4 max-w-xl text-sm leading-7 text-slate-300 sm:text-base">
                        Satu workspace untuk pengelolaan inventory,
                        order reservation, dan operasional stok.
                        Fondasi frontend sudah disiapkan untuk
                        terhubung dengan backend Go.
                    </p>

                    <div className="mt-7 flex flex-wrap gap-3">
                        <Link
                            to="/app/products"
                            className="rounded-xl bg-indigo-500 px-5 py-3 text-sm font-semibold text-white transition-colors hover:bg-indigo-400"
                        >
                            Explore workspace
                        </Link>

                        <span className="rounded-xl border border-slate-700 px-5 py-3 text-sm text-slate-300">
                            React · TypeScript · Vite
                        </span>
                    </div>
                </div>
            </section>

            <section className="grid gap-4 lg:grid-cols-3">
                <div className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm lg:col-span-2">
                    <div className="flex flex-wrap items-start justify-between gap-4">
                        <div>
                            <h2 className="text-lg font-bold text-slate-950">
                                Backend Connection
                            </h2>
                            <p className="mt-1 text-sm text-slate-500">
                                Pemeriksaan aktual melalui endpoint /readyz
                            </p>
                        </div>

                        <span
                            role="status"
                            aria-live="polite"
                            className={`rounded-full px-3 py-2 text-xs font-semibold ${backendStyle}`}
                        >
                            {backendLabel}
                        </span>
                    </div>

                    <div className="mt-7 flex flex-wrap items-center justify-between gap-4 border-t border-slate-100 pt-5">
                        <div>
                            <p className="text-xs text-slate-400">
                                Development API
                            </p>
                            <p className="mt-1 font-mono text-sm text-slate-700">
                                /readyz via Vite proxy
                            </p>
                        </div>

                        <button
                            type="button"
                            onClick={checkAgain}
                            disabled={backend === 'checking'}
                            className="rounded-xl border border-slate-200 px-4 py-2 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50"
                        >
                            Check again
                        </button>
                    </div>
                </div>

                <div className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
                    <p className="text-sm font-semibold text-slate-500">
                        Current milestone
                    </p>

                    <p className="mt-4 text-3xl font-bold text-slate-950">
                        FE-1
                    </p>

                    <p className="mt-2 text-sm leading-6 text-slate-500">
                        Routing, responsive layout, styling, dan
                        integrasi koneksi development.
                    </p>
                </div>
            </section>

            <section>
                <div className="mb-5">
                    <h2 className="text-xl font-bold text-slate-950">
                        Workspace Modules
                    </h2>
                    <p className="mt-1 text-sm text-slate-500">
                        Navigasi siap digunakan; fitur bisnis UI
                        diimplementasikan bertahap.
                    </p>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                    {visibleModules.map((module) => (
                        <Link
                            key={module.to}
                            to={module.to}
                            className="group rounded-2xl border border-slate-200 bg-white p-6 shadow-sm transition-all hover:-translate-y-0.5 hover:border-indigo-200 hover:shadow-md"
                        >
                            <div className="flex items-start justify-between gap-4">
                                <span className="flex h-11 w-11 items-center justify-center rounded-xl bg-indigo-50 text-sm font-bold text-indigo-600">
                                    {module.number}
                                </span>

                                <span className="text-slate-400 transition-transform group-hover:translate-x-1">
                                    →
                                </span>
                            </div>

                            <h3 className="mt-6 text-lg font-semibold text-slate-900">
                                {module.title}
                            </h3>

                            <p className="mt-2 text-sm leading-6 text-slate-500">
                                {module.description}
                            </p>
                        </Link>
                    ))}
                </div>
            </section>
        </div>
    )
}
