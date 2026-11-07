
import { useRef, useState } from 'react'
import type { FormEvent } from 'react'

import {
  ResourceError,
  Pager,
} from '../components/resource-ui'

import { useApiQuery } from '../hooks/use-api-query'
import { ApiRequestError } from '../lib/api'
import { writeApi } from '../lib/mutations'
import { formatQuantity } from '../lib/stock'

import type { Product } from '../lib/stock'

import {
  MAX_ORDER_ITEMS,
  ORDER_PRODUCT_PAGE_SIZE,
  validateOrderDraft,
} from '../lib/orders'

import type {
  Order,
  OrderDraftLine,
  OrderItem,
  OrderWriteResult,
} from '../lib/orders'

interface PendingOrder {
  key: string
  items: OrderItem[]
}

interface Props {
  onCreated: (order: Order, replayed: boolean) => void
  onClose: () => void
}

const inputClass =
  'w-full rounded-xl border border-slate-300 bg-white px-3 py-2 ' +
  'text-sm outline-none focus:border-indigo-500 disabled:bg-slate-100'

export function OrderCreate({
  onCreated,
  onClose,
}: Props) {
  const [offset, setOffset] = useState(0)
  const [revision, setRevision] = useState(0)
  const [lines, setLines] = useState<OrderDraftLine[]>([])
  const [pending, setPending] =
    useState<PendingOrder | null>(null)

  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)
  const [error, setError] = useState('')

  const productPath =
    `/api/v1/products?limit=${ORDER_PRODUCT_PAGE_SIZE}` +
    `&offset=${offset}`

  const products = useApiQuery<Product[]>(
    productPath,
    revision,
  )

  const locked = pending !== null || busy

  function addProduct(product: Product) {
    if (
      locked ||
      lines.some((line) => line.productId === product.id)
    ) {
      return
    }

    if (lines.length >= MAX_ORDER_ITEMS) return
    if (!product.is_active || product.available <= 0) return

    setLines((current) => [
      ...current,
      {
        productId: product.id,
        sku: product.sku,
        name: product.name,
        availableAtSelection: product.available,
        quantity: '1',
      },
    ])

    setError('')
  }

  function changeQuantity(
    productId: number,
    value: string,
  ) {
    if (locked) return

    setLines((current) =>
      current.map((line) =>
        line.productId === productId
          ? { ...line, quantity: value }
          : line,
      ),
    )
  }

  function removeProduct(productId: number) {
    if (locked) return

    setLines((current) =>
      current.filter(
        (line) => line.productId !== productId,
      ),
    )
  }

  async function sendOrder(request: PendingOrder) {
    if (inFlight.current) return

    inFlight.current = true
    setBusy(true)
    setError('')

    try {
      const result = await writeApi<OrderWriteResult>(
        '/api/v1/orders',
        'POST',
        { items: request.items },
        request.key,
      )

      if (
        !result ||
        !result.data ||
        !result.data.id
      ) {
        throw new Error(
          'Respons pembuatan order tidak lengkap.',
        )
      }

      setPending(null)
      onCreated(result.data, result.replayed)
    } catch (cause) {
      const definitiveRejection =
        cause instanceof ApiRequestError &&
        cause.status >= 400 &&
        cause.status < 500

      if (definitiveRejection) {
        setPending(null)
      }

      const description =
        cause instanceof Error
          ? cause.message
          : 'Tidak dapat membuat order.'

      setError(
        definitiveRejection
          ? `Order ditolak: ${description}. ` +
              'Perbarui stok dan koreksi item jika perlu.'
          : `${description}. Hasil transaksi belum pasti. ` +
              'Gunakan tombol ulangi request yang sama, ' +
              'jangan membuat order baru.',
      )
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (locked || inFlight.current) return

    let items: OrderItem[]

    try {
      items = validateOrderDraft(lines)
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : 'Item order tidak valid.',
      )
      return
    }

    const request: PendingOrder = {
      key: crypto.randomUUID(),
      items,
    }

    // Bekukan payload dan key untuk retry aman.
    setPending(request)
    void sendOrder(request)
  }

  return (
    <section className="rounded-2xl border border-indigo-200 bg-white p-6 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-slate-950">
            Buat Order Baru
          </h2>

          <p className="mt-2 text-sm text-slate-500">
            Pilih 1–50 produk aktif. Kuantitas setiap
            produk harus tersedia. Reservasi dilakukan
            secara atomik di backend.
          </p>
        </div>

        <button
          type="button"
          onClick={onClose}
          disabled={locked}
          className="rounded-xl border border-slate-200 px-4 py-2 text-sm disabled:opacity-40"
        >
          Tutup
        </button>
      </div>

      <div className="mt-7 grid gap-6 xl:grid-cols-2">
        <div className="overflow-hidden rounded-xl border border-slate-200">
          <div className="flex items-center justify-between gap-3 border-b p-4">
            <h3 className="font-semibold text-slate-900">
              Produk Tersedia
            </h3>

            <button
              type="button"
              disabled={locked}
              onClick={() =>
                setRevision((n) => n + 1)
              }
              className="text-xs font-semibold text-indigo-600 disabled:opacity-40"
            >
              Muat ulang stok
            </button>
          </div>

          {products.status === 'loading' && (
            <p
              role="status"
              className="p-5 text-sm text-slate-500"
            >
              Memuat produk...
            </p>
          )}

          {products.status === 'error' && (
            <div className="p-4">
              <ResourceError
                message={products.message}
                onRetry={() =>
                  setRevision((n) => n + 1)
                }
              />
            </div>
          )}

          {products.status === 'ready' && (
            <>
              {products.response.data.length === 0 ? (
                <p className="p-5 text-sm text-slate-500">
                  Belum ada produk aktif pada halaman ini.
                </p>
              ) : (
                <ul className="max-h-80 divide-y divide-slate-100 overflow-y-auto">
                  {products.response.data.map((product) => {
                    const alreadyAdded = lines.some(
                      (line) =>
                        line.productId === product.id,
                    )

                    const unavailable =
                      product.available <= 0 ||
                      !product.is_active

                    return (
                      <li
                        key={product.id}
                        className="flex items-center justify-between gap-3 p-4"
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-semibold text-slate-900">
                            {product.name}
                          </p>

                          <p className="mt-1 font-mono text-xs text-slate-500">
                            {product.sku}
                          </p>

                          <p className="mt-1 text-xs text-slate-500">
                            Available:{' '}
                            {formatQuantity.format(
                              product.available,
                            )}
                          </p>
                        </div>

                        <button
                          type="button"
                          disabled={
                            locked ||
                            alreadyAdded ||
                            unavailable ||
                            lines.length >= MAX_ORDER_ITEMS
                          }
                          onClick={() =>
                            addProduct(product)
                          }
                          className="shrink-0 rounded-lg bg-indigo-50 px-3 py-2 text-xs font-semibold text-indigo-700 disabled:opacity-40"
                        >
                          {alreadyAdded
                            ? 'Dipilih'
                            : unavailable
                              ? 'Stok 0'
                              : 'Tambahkan'}
                        </button>
                      </li>
                    )
                  })}
                </ul>
              )}

              <Pager
                offset={offset}
                pageSize={ORDER_PRODUCT_PAGE_SIZE}
                hasMore={
                  products.response.pagination
                    ?.has_more ?? false
                }
                disabled={locked}
                onChange={setOffset}
              />
            </>
          )}
        </div>

        <form
          onSubmit={submit}
          className="space-y-4"
        >
          <div className="flex items-center justify-between gap-2">
            <h3 className="font-semibold text-slate-900">
              Item Order
            </h3>

            <span className="text-xs text-slate-500">
              {lines.length}/{MAX_ORDER_ITEMS} produk
            </span>
          </div>

          {lines.length === 0 ? (
            <div className="rounded-xl border border-dashed border-slate-200 p-8 text-center text-sm text-slate-500">
              Pilih produk di kolom sebelah.
            </div>
          ) : (
            <div className="space-y-3">
              {lines.map((line) => (
                <div
                  key={line.productId}
                  className="rounded-xl border border-slate-200 p-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-semibold text-slate-900">
                        {line.name}
                      </p>

                      <p className="mt-1 font-mono text-xs text-slate-500">
                        {line.sku}
                      </p>
                    </div>

                    <button
                      type="button"
                      disabled={locked}
                      onClick={() =>
                        removeProduct(line.productId)
                      }
                      className="text-xs font-semibold text-rose-600 disabled:opacity-40"
                    >
                      Hapus
                    </button>
                  </div>

                  <label className="mt-3 block text-xs font-medium text-slate-600">
                    Kuantitas (tersedia saat dipilih:{' '}
                    {formatQuantity.format(
                      line.availableAtSelection,
                    )})

                    <input
                      type="number"
                      min="1"
                      max={line.availableAtSelection}
                      step="1"
                      required
                      disabled={locked}
                      value={line.quantity}
                      onChange={(event) =>
                        changeQuantity(
                          line.productId,
                          event.target.value,
                        )
                      }
                      className={`mt-2 ${inputClass}`}
                    />
                  </label>
                </div>
              ))}
            </div>
          )}

          {error && (
            <p
              role="alert"
              className="rounded-xl bg-rose-50 p-4 text-sm leading-6 text-rose-700"
            >
              {error}
            </p>
          )}

          {pending ? (
            <div className="space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
              <p className="text-sm font-semibold text-amber-900">
                {busy
                  ? 'Mengirim order...'
                  : 'Hasil order belum pasti'}
              </p>

              <p className="text-xs leading-5 text-amber-800">
                Retry memakai idempotency key dan payload
                yang sama. Jangan refresh halaman atau
                membuat order lain sebelum menyelesaikan
                request ini.
              </p>

              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  void sendOrder(pending)
                }
                className="rounded-lg bg-amber-700 px-4 py-2 text-sm font-semibold text-white disabled:opacity-40"
              >
                Coba ulang request yang sama
              </button>
            </div>
          ) : (
            <button
              type="submit"
              disabled={busy || lines.length === 0}
              className="w-full rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-40"
            >
              Buat & Reservasi Order
            </button>
          )}
        </form>
      </div>
    </section>
  )
}
