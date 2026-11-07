
import { useRef, useState } from 'react'

import { useAuth } from '../auth/auth-context'
import { ResourceError } from '../components/resource-ui'
import { useApiQuery } from '../hooks/use-api-query'
import { ApiRequestError } from '../lib/api'
import { writeApi } from '../lib/mutations'
import { formatQuantity } from '../lib/stock'

import {
  formatOrderDate,
  orderStatusClass,
  orderStatusLabel,
} from '../lib/orders'

import type {
  Order,
  OrderWriteResult,
} from '../lib/orders'

type TerminalAction = 'fulfill' | 'cancel'

interface Props {
  orderId: number
  onClose: () => void
  onChanged: () => void
}

export function OrderDetail({
  orderId,
  onClose,
  onChanged,
}: Props) {
  const { state: authState, refresh } = useAuth()

  const isAdmin =
    authState.status === 'authenticated' &&
    authState.session.role === 'admin'

  const [revision, setRevision] = useState(0)
  const [pendingAction, setPendingAction] =
    useState<TerminalAction | null>(null)

  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)

  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const query = useApiQuery<Order>(
    `/api/v1/orders/${orderId}`,
    revision,
  )

  function reload() {
    setRevision((n) => n + 1)
  }

  async function sendTransition(
    action: TerminalAction,
  ) {
    if (inFlight.current) return

    inFlight.current = true
    setBusy(true)
    setError('')
    setNotice('')

    try {
      // Repeated identical terminal actions are safe
      // replays according to the Go backend.
      const result = await writeApi<OrderWriteResult>(
        `/api/v1/orders/${orderId}/${action}`,
        'POST',
      )

      if (
        !result ||
        !result.data ||
        !result.data.id
      ) {
        throw new Error(
          'Respons transisi order tidak lengkap.',
        )
      }

      setPendingAction(null)

      setNotice(
        result.replayed
          ? `Order #${orderId} sudah ${result.data.status}; ` +
              'stok tidak diubah kembali.'
          : `Order #${orderId} berhasil diubah menjadi ` +
              `${result.data.status}.`,
      )

      reload()
      onChanged()
    } catch (cause) {
      const definitiveRejection =
        cause instanceof ApiRequestError &&
        cause.status >= 400 &&
        cause.status < 500

      if (definitiveRejection) {
        setPendingAction(null)
        reload()
        onChanged()

        if (
          cause instanceof ApiRequestError &&
          (
            cause.status === 401 ||
            cause.status === 403
          )
        ) {
          void refresh()
        }
      }

      const description =
        cause instanceof Error
          ? cause.message
          : 'Gagal mengubah status order.'

      setError(
        definitiveRejection
          ? `Transisi ditolak: ${description}. ` +
              'Data order dimuat ulang.'
          : `${description}. Hasil belum pasti. ` +
              `Ulangi aksi ${action} yang sama; ` +
              'jangan pilih aksi kebalikannya.',
      )
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  function requestTransition(
    action: TerminalAction,
  ) {
    if (
      !isAdmin ||
      pendingAction ||
      busy ||
      query.status !== 'ready'
    ) {
      return
    }

    if (
      query.response.data.status !== 'reserved'
    ) {
      return
    }

    const description =
      action === 'fulfill'
        ? 'Stok fisik dan reservasi akan dikurangi.'
        : 'Stok yang dipesan akan dilepas kembali.'

    const confirmed = window.confirm(
      `Yakin ${action} order #${orderId}?\n\n${description}`,
    )

    if (!confirmed) return

    setPendingAction(action)
    void sendTransition(action)
  }

  const order =
    query.status === 'ready'
      ? query.response.data
      : null

  const actionLocked =
    busy || pendingAction !== null

  return (
    <section className="space-y-5 rounded-2xl border border-indigo-200 bg-white p-6 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-indigo-600">
            Detail Order
          </p>

          <h2 className="mt-2 text-2xl font-bold text-slate-950">
            Order #{orderId}
          </h2>
        </div>

        <div className="flex gap-3">
          <button
            type="button"
            disabled={actionLocked}
            onClick={reload}
            className="rounded-xl border border-slate-200 px-4 py-2 text-sm disabled:opacity-40"
          >
            Muat ulang
          </button>

          <button
            type="button"
            disabled={actionLocked}
            onClick={onClose}
            className="rounded-xl border border-slate-200 px-4 py-2 text-sm disabled:opacity-40"
          >
            Tutup
          </button>
        </div>
      </div>

      {query.status === 'loading' && (
        <p
          role="status"
          className="text-sm text-slate-500"
        >
          Memuat detail order...
        </p>
      )}

      {query.status === 'error' && (
        <ResourceError
          message={query.message}
          onRetry={() => {
            if (
              query.httpStatus === 401 ||
              query.httpStatus === 403
            ) {
              void refresh()
            } else {
              reload()
            }
          }}
        />
      )}

      {order && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-xl bg-slate-50 p-4">
              <p className="text-xs text-slate-500">
                Status
              </p>

              <span
                className={
                  'mt-2 inline-block rounded-lg px-3 py-1 ' +
                  'text-xs font-bold ' +
                  orderStatusClass[order.status]
                }
              >
                {orderStatusLabel[order.status]}
              </span>
            </div>

            <div className="rounded-xl bg-slate-50 p-4">
              <p className="text-xs text-slate-500">
                Dibuat oleh
              </p>

              <p className="mt-2 text-sm font-semibold text-slate-950">
                User #{order.created_by}
              </p>
            </div>

            <div className="rounded-xl bg-slate-50 p-4">
              <p className="text-xs text-slate-500">
                Dibuat pada
              </p>

              <p className="mt-2 text-sm font-semibold text-slate-950">
                {formatOrderDate(order.created_at)}
              </p>
            </div>

            <div className="rounded-xl bg-slate-50 p-4">
              <p className="text-xs text-slate-500">
                Item berbeda
              </p>

              <p className="mt-2 text-sm font-semibold text-slate-950">
                {order.items.length}
              </p>
            </div>
          </div>

          <div className="overflow-x-auto rounded-xl border border-slate-200">
            <table className="w-full min-w-[440px] text-left text-sm">
              <thead className="bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th
                    scope="col"
                    className="px-5 py-4"
                  >
                    Product ID
                  </th>

                  <th
                    scope="col"
                    className="px-5 py-4 text-right"
                  >
                    Qty
                  </th>
                </tr>
              </thead>

              <tbody className="divide-y divide-slate-100">
                {order.items.map((item) => (
                  <tr key={item.product_id}>
                    <td className="px-5 py-4 font-mono">
                      #{item.product_id}
                    </td>

                    <td className="px-5 py-4 text-right font-semibold">
                      {formatQuantity.format(
                        item.quantity,
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="flex flex-wrap gap-4 text-xs text-slate-500">
            {order.fulfilled_at && (
              <p>
                Fulfilled:{' '}
                {formatOrderDate(
                  order.fulfilled_at,
                )}
              </p>
            )}

            {order.cancelled_at && (
              <p>
                Cancelled:{' '}
                {formatOrderDate(
                  order.cancelled_at,
                )}
              </p>
            )}
          </div>

          {error && (
            <p
              role="alert"
              className="rounded-xl bg-rose-50 p-4 text-sm leading-6 text-rose-700"
            >
              {error}
            </p>
          )}

          {notice && (
            <p
              role="status"
              className="rounded-xl bg-emerald-50 p-4 text-sm text-emerald-700"
            >
              {notice}
            </p>
          )}

          {isAdmin && order.status === 'reserved' && (
            <div className="space-y-3 border-t border-slate-100 pt-5">
              <p className="text-sm font-semibold text-slate-900">
                Finalisasi Order (Admin)
              </p>

              {pendingAction && (
                <p className="text-xs leading-5 text-amber-700">
                  Aksi {pendingAction} belum terkonfirmasi.
                  Retry hanya dengan aksi yang sama.
                </p>
              )}

              <div className="flex flex-wrap gap-3">
                {pendingAction ? (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      void sendTransition(
                        pendingAction,
                      )
                    }
                    className="rounded-xl bg-amber-700 px-5 py-3 text-sm font-semibold text-white disabled:opacity-40"
                  >
                    {busy
                      ? 'Mengirim...'
                      : `Coba ulang ${pendingAction}`}
                  </button>
                ) : (
                  <>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        requestTransition('fulfill')
                      }
                      className="rounded-xl bg-emerald-600 px-5 py-3 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-40"
                    >
                      Fulfill Order
                    </button>

                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        requestTransition('cancel')
                      }
                      className="rounded-xl bg-rose-600 px-5 py-3 text-sm font-semibold text-white hover:bg-rose-500 disabled:opacity-40"
                    >
                      Cancel Order
                    </button>
                  </>
                )}
              </div>

              <p className="text-xs leading-5 text-slate-500">
                Fulfill mengurangi on-hand dan reserved.
                Cancel hanya melepas reserved.
                Aksi final tidak bisa dibalik.
              </p>
            </div>
          )}
        </>
      )}
    </section>
  )
}
