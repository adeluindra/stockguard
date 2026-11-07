
import { useState } from 'react'

import { useAuth } from '../auth/auth-context'

import {
  ResourceError,
  Pager,
} from '../components/resource-ui'

import { useApiQuery } from '../hooks/use-api-query'
import { formatQuantity } from '../lib/stock'

import {
  ORDER_PAGE_SIZE,
  formatOrderDate,
  orderStatusClass,
  orderStatusLabel,
} from '../lib/orders'

import type {
  Order,
  OrderStatus,
  OrderSummary,
} from '../lib/orders'

import { OrderCreate } from './order-create'
import { OrderDetail } from './order-detail'

type StatusFilter = '' | OrderStatus

export function OrdersPage() {
  const {
    state: authState,
    refresh,
  } = useAuth()

  const isAdmin =
    authState.status === 'authenticated' &&
    authState.session.role === 'admin'

  const [status, setStatus] =
    useState<StatusFilter>('')

  const [offset, setOffset] = useState(0)
  const [revision, setRevision] = useState(0)

  const [showCreate, setShowCreate] =
    useState(false)

  const [
    selectedOrderId,
    setSelectedOrderId,
  ] = useState<number | null>(null)

  const [notice, setNotice] = useState('')

  const path =
    `/api/v1/orders?limit=${ORDER_PAGE_SIZE}&offset=${offset}` +
    (status ? `&status=${status}` : '')

  const list = useApiQuery<OrderSummary[]>(
    path,
    revision,
  )

  function reload() {
    setRevision((n) => n + 1)
  }

  function handleCreated(
    order: Order,
    replayed: boolean,
  ) {
    setShowCreate(false)
    setStatus('')
    setOffset(0)
    setSelectedOrderId(order.id)

    setNotice(
      replayed
        ? `Order #${order.id} ditemukan melalui replay aman; ` +
            'tidak dibuat dua kali.'
        : `Order #${order.id} berhasil dibuat ` +
            'dan stok telah direservasi.',
    )

    reload()
  }

  function filterStatus(next: StatusFilter) {
    setStatus(next)
    setOffset(0)
  }

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-indigo-600">
            Workspace / Orders
          </p>

          <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">
            Order Management
          </h1>

          <p className="mt-3 text-sm text-slate-500">
            {isAdmin
              ? 'Kelola seluruh order, reservasi stok, fulfillment, dan cancellation.'
              : 'Buat dan lihat order milik akunmu.'}
          </p>
        </div>

        <button
          type="button"
          disabled={showCreate}
          onClick={() => {
            setSelectedOrderId(null)
            setNotice('')
            setShowCreate(true)
          }}
          className="rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-40"
        >
          + Buat Order
        </button>
      </header>

      {showCreate && (
        <OrderCreate
          onCreated={handleCreated}
          onClose={() =>
            setShowCreate(false)
          }
        />
      )}

      {notice && (
        <p
          role="status"
          className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800"
        >
          {notice}
        </p>
      )}

      {selectedOrderId !== null && (
        <OrderDetail
          key={selectedOrderId}
          orderId={selectedOrderId}
          onClose={() =>
            setSelectedOrderId(null)
          }
          onChanged={reload}
        />
      )}

      <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-4 border-b border-slate-100 p-5">
          <div>
            <h2 className="text-lg font-bold text-slate-950">
              Daftar Order
            </h2>

            <p className="mt-1 text-xs text-slate-500">
              {isAdmin
                ? 'Seluruh order'
                : 'Hanya order yang dibuat akunmu'}
            </p>
          </div>

          <div className="flex items-center gap-3">
            <label className="text-xs font-medium text-slate-600">
              Status

              <select
                className="ml-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm"
                value={status}
                onChange={(event) =>
                  filterStatus(
                    event.target.value as StatusFilter,
                  )
                }
              >
                <option value="">
                  Semua
                </option>
                <option value="reserved">
                  Reserved
                </option>
                <option value="fulfilled">
                  Fulfilled
                </option>
                <option value="cancelled">
                  Cancelled
                </option>
              </select>
            </label>

            <button
              type="button"
              onClick={reload}
              className="rounded-lg border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-600"
            >
              Muat ulang
            </button>
          </div>
        </div>

        {list.status === 'loading' && (
          <p
            role="status"
            className="p-8 text-sm text-slate-500"
          >
            Memuat order...
          </p>
        )}

        {list.status === 'error' && (
          <div className="p-5">
            <ResourceError
              message={list.message}
              onRetry={() => {
                if (
                  list.httpStatus === 401 ||
                  list.httpStatus === 403
                ) {
                  void refresh()
                } else {
                  reload()
                }
              }}
            />
          </div>
        )}

        {list.status === 'ready' && (
          <>
            {list.response.data.length === 0 ? (
              <div className="p-10 text-center">
                <p className="font-semibold text-slate-800">
                  Belum ada order
                </p>

                <p className="mt-2 text-sm text-slate-500">
                  Buat order baru atau ubah filter status.
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[740px] text-left text-sm">
                  <thead className="bg-slate-50 text-xs uppercase text-slate-500">
                    <tr>
                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Order ID
                      </th>

                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Status
                      </th>

                      <th
                        scope="col"
                        className="px-5 py-4 text-right"
                      >
                        Item
                      </th>

                      {isAdmin && (
                        <th
                          scope="col"
                          className="px-5 py-4"
                        >
                          Dibuat oleh
                        </th>
                      )}

                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Dibuat pada
                      </th>

                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Aksi
                      </th>
                    </tr>
                  </thead>

                  <tbody className="divide-y divide-slate-100">
                    {list.response.data.map((order) => (
                      <tr key={order.id}>
                        <td className="px-5 py-4 font-mono font-semibold text-slate-900">
                          #{order.id}
                        </td>

                        <td className="px-5 py-4">
                          <span
                            className={
                              'rounded-lg px-3 py-1 text-xs ' +
                              'font-semibold ' +
                              orderStatusClass[order.status]
                            }
                          >
                            {orderStatusLabel[order.status]}
                          </span>
                        </td>

                        <td className="px-5 py-4 text-right">
                          {formatQuantity.format(
                            order.item_count,
                          )}
                        </td>

                        {isAdmin && (
                          <td className="px-5 py-4">
                            User #{order.created_by}
                          </td>
                        )}

                        <td className="px-5 py-4 text-slate-600">
                          {formatOrderDate(
                            order.created_at,
                          )}
                        </td>

                        <td className="px-5 py-4">
                          <button
                            type="button"
                            onClick={() =>
                              setSelectedOrderId(
                                order.id,
                              )
                            }
                            className="font-semibold text-indigo-600 hover:underline"
                          >
                            Lihat Detail
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            <Pager
              offset={offset}
              pageSize={ORDER_PAGE_SIZE}
              hasMore={
                list.response.pagination
                  ?.has_more ?? false
              }
              onChange={setOffset}
            />
          </>
        )}
      </section>
    </div>
  )
}
