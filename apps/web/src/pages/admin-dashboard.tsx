
import { useState } from 'react'

import { useAuth } from '../auth/auth-context'
import { useDashboardQuery } from '../hooks/use-dashboard-query'

import type { DashboardQueryState } from '../hooks/use-dashboard-query'
import type {
  DashboardSummary,
  InventoryActivity,
  LowStockProduct,
} from '../lib/dashboard'

import {
  DASHBOARD_PAGE_LIMIT,
  formatActivityDate,
  formatDelta,
  movementLabel,
  quantityFormatter,
} from '../lib/dashboard'

interface MetricCardProps {
  label: string
  value: number
  description: string
  warning?: boolean
}

function MetricCard({
  label,
  value,
  description,
  warning = false,
}: MetricCardProps) {
  return (
    <div
      className={`rounded-2xl border p-5 shadow-sm ${
        warning
          ? 'border-amber-200 bg-amber-50'
          : 'border-slate-200 bg-white'
      }`}
    >
      <p className="text-sm font-medium text-slate-500">
        {label}
      </p>

      <p
        className={`mt-3 text-3xl font-bold tracking-tight ${
          warning ? 'text-amber-700' : 'text-slate-950'
        }`}
      >
        {quantityFormatter.format(value)}
      </p>

      <p className="mt-2 text-xs leading-5 text-slate-500">
        {description}
      </p>
    </div>
  )
}

interface ErrorPanelProps {
  message: string
  httpStatus: number | null
  onRetry: () => void
  onVerifySession: () => void
}

function ErrorPanel({
  message,
  httpStatus,
  onRetry,
  onVerifySession,
}: ErrorPanelProps) {
  const needsSessionCheck =
    httpStatus === 401 || httpStatus === 403

  return (
    <div
      role="alert"
      className="rounded-2xl border border-rose-200 bg-rose-50 p-6"
    >
      <p className="font-semibold text-rose-800">
        Gagal memuat data
      </p>

      <p className="mt-2 text-sm leading-6 text-rose-700">
        {message}
      </p>

      <button
        type="button"
        onClick={
          needsSessionCheck ? onVerifySession : onRetry
        }
        className="mt-4 rounded-xl bg-white px-4 py-2 text-sm font-semibold text-rose-700 shadow-sm hover:bg-rose-100"
      >
        {needsSessionCheck ? 'Periksa sesi' : 'Coba lagi'}
      </button>
    </div>
  )
}

function LoadingPanel({ label }: { label: string }) {
  return (
    <div
      role="status"
      className="rounded-2xl border border-slate-200 bg-white p-8 text-sm text-slate-500 shadow-sm"
    >
      Memuat {label}...
    </div>
  )
}

interface PaginationControlsProps {
  offset: number
  hasMore: boolean
  disabled: boolean
  onPrevious: () => void
  onNext: () => void
}

function PaginationControls({
  offset,
  hasMore,
  disabled,
  onPrevious,
  onNext,
}: PaginationControlsProps) {
  const buttonClass =
    'rounded-lg border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-600 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40'

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-4">
      <p className="text-xs text-slate-500">
        Halaman{' '}
        {Math.floor(offset / DASHBOARD_PAGE_LIMIT) + 1}
      </p>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onPrevious}
          disabled={disabled || offset === 0}
          className={buttonClass}
        >
          Sebelumnya
        </button>

        <button
          type="button"
          onClick={onNext}
          disabled={disabled || !hasMore}
          className={buttonClass}
        >
          Berikutnya
        </button>
      </div>
    </div>
  )
}

interface DashboardPanelProps<T> {
  state: DashboardQueryState<T>
  onRetry: () => void
  onVerifySession: () => void
}

function SummaryPanel({
  state,
  onRetry,
  onVerifySession,
}: DashboardPanelProps<DashboardSummary>) {
  if (state.status === 'loading') {
    return <LoadingPanel label="ringkasan operasional" />
  }

  if (state.status === 'error') {
    return (
      <ErrorPanel
        message={state.message}
        httpStatus={state.httpStatus}
        onRetry={onRetry}
        onVerifySession={onVerifySession}
      />
    )
  }

  const { inventory, orders } = state.response.data

  const inventoryMetrics = [
    {
      label: 'Produk Aktif',
      value: inventory.active_products,
      description: 'Produk yang masih aktif',
    },
    {
      label: 'Total On Hand',
      value: inventory.on_hand,
      description: 'Stok fisik produk aktif',
    },
    {
      label: 'Stok Reserved',
      value: inventory.reserved,
      description: 'Stok untuk order yang dipesan',
    },
    {
      label: 'Stok Available',
      value: inventory.available,
      description: 'Stok yang dapat dipesan',
    },
    {
      label: 'Low Stock',
      value: inventory.low_stock_products,
      description: 'Produk pada atau di bawah batas stok',
      warning: inventory.low_stock_products > 0,
    },
    {
      label: 'Produk Nonaktif',
      value: inventory.inactive_products,
      description: 'Produk yang dinonaktifkan',
    },
  ]

  const orderMetrics = [
    {
      label: 'Total Orders',
      value: orders.total,
      description: 'Seluruh riwayat pesanan',
    },
    {
      label: 'Reserved',
      value: orders.reserved,
      description: 'Order menunggu fulfillment',
    },
    {
      label: 'Fulfilled',
      value: orders.fulfilled,
      description: 'Order telah dipenuhi',
    },
    {
      label: 'Cancelled',
      value: orders.cancelled,
      description: 'Order yang dibatalkan',
    },
  ]

  return (
    <div className="space-y-7">
      <section aria-labelledby="inventory-heading">
        <h2
          id="inventory-heading"
          className="mb-4 text-lg font-bold text-slate-950"
        >
          Inventory Summary
        </h2>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {inventoryMetrics.map((metric) => (
            <MetricCard
              key={metric.label}
              {...metric}
            />
          ))}
        </div>
      </section>

      <section aria-labelledby="orders-heading">
        <h2
          id="orders-heading"
          className="mb-4 text-lg font-bold text-slate-950"
        >
          Order Summary
        </h2>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {orderMetrics.map((metric) => (
            <MetricCard
              key={metric.label}
              {...metric}
            />
          ))}
        </div>
      </section>
    </div>
  )
}

interface LowStockPanelProps
  extends DashboardPanelProps<LowStockProduct[]> {
  offset: number
  onPrevious: () => void
  onNext: () => void
}

function LowStockPanel({
  state,
  offset,
  onPrevious,
  onNext,
  onRetry,
  onVerifySession,
}: LowStockPanelProps) {
  if (state.status === 'loading') {
    return <LoadingPanel label="produk low-stock" />
  }

  if (state.status === 'error') {
    return (
      <ErrorPanel
        message={state.message}
        httpStatus={state.httpStatus}
        onRetry={onRetry}
        onVerifySession={onVerifySession}
      />
    )
  }

  const products = state.response.data
  const hasMore = state.response.pagination?.has_more ?? false

  return (
    <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      {products.length === 0 ? (
        <div className="px-6 py-12 text-center">
          <p className="font-semibold text-slate-800">
            {offset === 0
              ? 'Tidak ada produk low-stock'
              : 'Tidak ada data pada halaman ini'}
          </p>

          <p className="mt-2 text-sm text-slate-500">
            {offset === 0
              ? 'Belum ada produk aktif yang memenuhi kondisi stok menipis.'
              : 'Gunakan tombol Sebelumnya untuk melihat data lain.'}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[650px] text-left text-sm">
            <thead className="bg-slate-50 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th scope="col" className="px-5 py-4">Produk</th>
                <th scope="col" className="px-5 py-4 text-right">On Hand</th>
                <th scope="col" className="px-5 py-4 text-right">Reserved</th>
                <th scope="col" className="px-5 py-4 text-right">Available</th>
                <th scope="col" className="px-5 py-4 text-right">Threshold</th>
              </tr>
            </thead>

            <tbody className="divide-y divide-slate-100">
              {products.map((product) => (
                <tr key={product.product_id}>
                  <td className="px-5 py-4">
                    <p className="font-semibold text-slate-900">
                      {product.name}
                    </p>
                    <p className="mt-1 font-mono text-xs text-slate-400">
                      {product.sku}
                    </p>
                  </td>

                  <td className="px-5 py-4 text-right text-slate-600">
                    {quantityFormatter.format(product.on_hand)}
                  </td>

                  <td className="px-5 py-4 text-right text-slate-600">
                    {quantityFormatter.format(product.reserved)}
                  </td>

                  <td className="px-5 py-4 text-right">
                    <span
                      className={`rounded-lg px-2.5 py-1 text-xs font-bold ${
                        product.available === 0
                          ? 'bg-rose-100 text-rose-700'
                          : 'bg-amber-100 text-amber-700'
                      }`}
                    >
                      {quantityFormatter.format(product.available)}
                    </span>
                  </td>

                  <td className="px-5 py-4 text-right text-slate-600">
                    {quantityFormatter.format(
                      product.low_stock_threshold,
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <PaginationControls
        offset={offset}
        hasMore={hasMore}
        disabled={false}
        onPrevious={onPrevious}
        onNext={onNext}
      />
    </div>
  )
}

interface ActivityPanelProps
  extends DashboardPanelProps<InventoryActivity[]> {
  offset: number
  onPrevious: () => void
  onNext: () => void
}

function ActivityPanel({
  state,
  offset,
  onPrevious,
  onNext,
  onRetry,
  onVerifySession,
}: ActivityPanelProps) {
  if (state.status === 'loading') {
    return <LoadingPanel label="aktivitas stok" />
  }

  if (state.status === 'error') {
    return (
      <ErrorPanel
        message={state.message}
        httpStatus={state.httpStatus}
        onRetry={onRetry}
        onVerifySession={onVerifySession}
      />
    )
  }

  const activities = state.response.data
  const hasMore = state.response.pagination?.has_more ?? false

  return (
    <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      {activities.length === 0 ? (
        <div className="px-6 py-12 text-center">
          <p className="font-semibold text-slate-800">
            Belum ada aktivitas pada halaman ini
          </p>

          <p className="mt-2 text-sm text-slate-500">
            Aktivitas stok akan muncul setelah ada
            transaksi inventory.
          </p>
        </div>
      ) : (
        <ol className="divide-y divide-slate-100">
          {activities.map((activity) => (
            <li key={activity.id} className="px-5 py-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-bold text-slate-900">
                    {movementLabel(activity.movement_type)}
                  </p>

                  <p className="mt-1 text-sm text-slate-600">
                    {activity.product_name}
                    <span className="ml-2 font-mono text-xs text-slate-400">
                      {activity.sku}
                    </span>
                  </p>

                  <p className="mt-2 text-xs text-slate-500">
                    Oleh {activity.actor_username}
                    {activity.order_id !== null
                      ? ` · Order #${activity.order_id}`
                      : ''}
                    {' · '}
                    {formatActivityDate(activity.created_at)}
                  </p>

                  {activity.reason && (
                    <p className="mt-2 text-xs leading-5 text-slate-500">
                      Alasan: {activity.reason}
                    </p>
                  )}
                </div>

                <div className="text-right text-xs">
                  <p className="font-semibold text-slate-700">
                    On hand: {formatDelta(activity.delta_on_hand)}
                  </p>

                  <p className="mt-1 text-slate-500">
                    Reserved: {formatDelta(activity.delta_reserved)}
                  </p>
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}

      <PaginationControls
        offset={offset}
        hasMore={hasMore}
        disabled={false}
        onPrevious={onPrevious}
        onNext={onNext}
      />
    </div>
  )
}

export function AdminDashboard() {
  const { refresh } = useAuth()

  const [revision, setRevision] = useState(0)
  const [lowStockOffset, setLowStockOffset] = useState(0)
  const [activityOffset, setActivityOffset] = useState(0)

  const summary = useDashboardQuery<DashboardSummary>(
    '/api/v1/dashboard/summary',
    revision,
  )

  const lowStock = useDashboardQuery<LowStockProduct[]>(
    `/api/v1/dashboard/low-stock?limit=${DASHBOARD_PAGE_LIMIT}&offset=${lowStockOffset}`,
    revision,
  )

  const activity = useDashboardQuery<InventoryActivity[]>(
    `/api/v1/dashboard/activity?limit=${DASHBOARD_PAGE_LIMIT}&offset=${activityOffset}`,
    revision,
  )

  function reloadDashboard() {
    setRevision((current) => current + 1)
  }

  function previousLowStock() {
    setLowStockOffset((current) =>
      Math.max(0, current - DASHBOARD_PAGE_LIMIT),
    )
  }

  function nextLowStock() {
    setLowStockOffset((current) =>
      current + DASHBOARD_PAGE_LIMIT,
    )
  }

  function previousActivity() {
    setActivityOffset((current) =>
      Math.max(0, current - DASHBOARD_PAGE_LIMIT),
    )
  }

  function nextActivity() {
    setActivityOffset((current) =>
      current + DASHBOARD_PAGE_LIMIT,
    )
  }

  return (
    <div className="space-y-9">
      <header className="flex flex-wrap items-start justify-between gap-5">
        <div>
          <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">
            Operations / Overview
          </p>

          <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">
            Dashboard Operasional
          </h1>

          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">
            Ringkasan stok, pesanan, dan aktivitas yang
            diambil langsung dari database StockGuard.
          </p>
        </div>

        <button
          type="button"
          onClick={reloadDashboard}
          className="rounded-xl border border-slate-200 bg-white px-5 py-3 text-sm font-semibold text-slate-700 shadow-sm transition hover:bg-slate-50"
        >
          Muat ulang data
        </button>
      </header>

      <SummaryPanel
        state={summary}
        onRetry={reloadDashboard}
        onVerifySession={() => void refresh()}
      />

      <section aria-labelledby="low-stock-heading">
        <div className="mb-4">
          <h2
            id="low-stock-heading"
            className="text-xl font-bold text-slate-950"
          >
            Low Stock Alerts
          </h2>

          <p className="mt-1 text-sm text-slate-500">
            Produk aktif dengan available stock pada
            atau di bawah threshold.
          </p>
        </div>

        <LowStockPanel
          state={lowStock}
          offset={lowStockOffset}
          onPrevious={previousLowStock}
          onNext={nextLowStock}
          onRetry={reloadDashboard}
          onVerifySession={() => void refresh()}
        />
      </section>

      <section aria-labelledby="activity-heading">
        <div className="mb-4">
          <h2
            id="activity-heading"
            className="text-xl font-bold text-slate-950"
          >
            Recent Inventory Activity
          </h2>

          <p className="mt-1 text-sm text-slate-500">
            Riwayat perubahan stok terbaru, termasuk
            aktivitas pada produk nonaktif.
          </p>
        </div>

        <ActivityPanel
          state={activity}
          offset={activityOffset}
          onPrevious={previousActivity}
          onNext={nextActivity}
          onRetry={reloadDashboard}
          onVerifySession={() => void refresh()}
        />
      </section>
    </div>
  )
}
