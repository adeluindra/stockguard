import { useRef, useState } from 'react'
import type { FormEvent } from 'react'

import { useAuth } from '../auth/auth-context'
import { ResourceError, Pager } from '../components/resource-ui'
import { useApiQuery } from '../hooks/use-api-query'
import { ApiRequestError } from '../lib/api'
import { formatActivityDate, movementLabel } from '../lib/dashboard'
import { writeApi } from '../lib/mutations'

import {
    PRODUCT_PAGE_SIZE,
    MOVEMENT_PAGE_SIZE,
    formatQuantity,
    validateAdjustment,
} from '../lib/stock'

import type {
    Product,
    StockBalance,
    StockMovement,
    AdjustmentResult,
} from '../lib/stock'

interface PendingAdjustment {
    productID: number
    delta: number
    reason: string
    key: string
}

function MovementHistory({
    productID,
    revision,
}: {
    productID: number
    revision: number
}) {
    const [offset, setOffset] = useState(0)

    const path =
        `/api/v1/products/${productID}/inventory/movements` +
        `?limit=${MOVEMENT_PAGE_SIZE}&offset=${offset}`

    const [retry, setRetry] = useState(0)

    const result = useApiQuery<StockMovement[]>(
        path,
        revision + retry,
    )

    return (
        <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 px-5 py-5">
                <h2 className="text-lg font-bold text-slate-950">
                    Riwayat Pergerakan Stok
                </h2>
                <p className="mt-1 text-sm text-slate-500">
                    Semua perubahan stok yang tercatat pada produk ini.
                </p>
            </div>

            {result.status === 'loading' && (
                <p role="status" className="p-6 text-sm text-slate-500">
                    Memuat riwayat...
                </p>
            )}

            {result.status === 'error' && (
                <div className="p-5">
                    <ResourceError
                        message={result.message}
                        onRetry={() => setRetry((value) => value + 1)}
                    />
                </div>
            )}

            {result.status === 'ready' && (
                <>
                    {result.response.data.length === 0 ? (
                        <p className="p-8 text-center text-sm text-slate-500">
                            Belum ada movement pada halaman ini.
                        </p>
                    ) : (
                        <ol className="divide-y divide-slate-100">
                            {result.response.data.map((movement) => (
                                <li key={movement.id} className="p-5">
                                    <div className="flex flex-wrap justify-between gap-3">
                                        <div>
                                            <p className="font-semibold text-slate-900">
                                                {movementLabel(movement.movement_type)}
                                            </p>

                                            <p className="mt-1 text-xs text-slate-500">
                                                {formatActivityDate(movement.created_at)}
                                                {' · '}
                                                User #{movement.actor_user_id}
                                                {movement.order_id !== null
                                                    ? ` · Order #${movement.order_id}`
                                                    : ''}
                                            </p>

                                            {movement.reason && (
                                                <p className="mt-2 text-sm text-slate-600">
                                                    {movement.reason}
                                                </p>
                                            )}
                                        </div>

                                        <div className="text-right text-xs text-slate-600">
                                            <p>
                                                On hand:{' '}
                                                <strong>
                                                    {movement.delta_on_hand > 0 ? '+' : ''}
                                                    {formatQuantity.format(
                                                        movement.delta_on_hand,
                                                    )}
                                                </strong>
                                            </p>

                                            <p className="mt-1">
                                                Reserved:{' '}
                                                <strong>
                                                    {movement.delta_reserved > 0 ? '+' : ''}
                                                    {formatQuantity.format(
                                                        movement.delta_reserved,
                                                    )}
                                                </strong>
                                            </p>
                                        </div>
                                    </div>
                                </li>
                            ))}
                        </ol>
                    )}

                    <Pager
                        offset={offset}
                        pageSize={MOVEMENT_PAGE_SIZE}
                        hasMore={result.response.pagination?.has_more ?? false}
                        onChange={setOffset}
                    />
                </>
            )}
        </section>
    )
}

export function InventoryPage() {
    const { state, refresh } = useAuth()

    const isAdmin =
        state.status === 'authenticated' &&
        state.session.role === 'admin'

    const [productOffset, setProductOffset] = useState(0)
    const [includeInactive, setIncludeInactive] = useState(false)
    const [selectedID, setSelectedID] = useState<number | null>(null)
    const [movementOffsetKey, setMovementOffsetKey] = useState(0)
    const [revision, setRevision] = useState(0)

    const [deltaText, setDeltaText] = useState('')
    const [reason, setReason] = useState('')
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState('')
    const [notice, setNotice] = useState('')

    const pendingRef = useRef<PendingAdjustment | null>(null)
    const [pending, setPending] =
        useState<PendingAdjustment | null>(null)

    const productPath =
        `/api/v1/products?limit=${PRODUCT_PAGE_SIZE}` +
        `&offset=${productOffset}` +
        `&include_inactive=${isAdmin && includeInactive}`

    const products = useApiQuery<Product[]>(productPath, revision)

    const balance = useApiQuery<StockBalance>(
        selectedID === null
            ? ''
            : `/api/v1/products/${selectedID}/inventory`,
        revision,
    )

    function reload() {
        setRevision((value) => value + 1)
    }

    function clearPending() {
        pendingRef.current = null
        setPending(null)
    }

    function selectProduct(id: number) {
        if (busy || pendingRef.current !== null) return

        setSelectedID(id)
        setMovementOffsetKey((value) => value + 1)
        setDeltaText('')
        setReason('')
        setError('')
        setNotice('')
    }

    async function sendAdjustment(request: PendingAdjustment) {
        if (busy) return

        setBusy(true)
        setError('')
        setNotice('')

        try {
            const result = await writeApi<{
                data: AdjustmentResult
                replayed: boolean
            }>(
                `/api/v1/products/${request.productID}/inventory/adjustments`,
                'POST',
                {
                    delta: request.delta,
                    reason: request.reason,
                },
                request.key,
            )

            if (!result) {
                throw new Error('Respons adjustment tidak lengkap.')
            }

            clearPending()
            setDeltaText('')
            setReason('')

            setNotice(
                result.replayed
                    ? 'Request berhasil diverifikasi melalui replay. Stok tidak diubah dua kali.'
                    : `Adjustment berhasil. Saldo on hand: ${formatQuantity.format(result.data.balance.on_hand)}.`,
            )

            reload()
        } catch (cause) {
            const knownRejection =
                cause instanceof ApiRequestError &&
                [400, 401, 403, 404, 409].includes(cause.status)

            if (knownRejection) {
                clearPending()
            }

            const details =
                cause instanceof Error
                    ? cause.message
                    : 'Gagal mengirim adjustment.'

            setError(
                knownRejection
                    ? `Adjustment ditolak: ${details}`
                    : `${details} Hasil request belum pasti. Gunakan "Coba ulang request yang sama" sebelum membuat adjustment baru.`,
            )
        } finally {
            setBusy(false)
        }
    }

    function submitAdjustment(event: FormEvent<HTMLFormElement>) {
        event.preventDefault()

        if (
            !isAdmin ||
            selectedID === null ||
            busy ||
            pendingRef.current !== null ||
            balance.status !== 'ready' ||
            !balance.response.data.is_active
        ) {
            return
        }

        let input

        try {
            input = validateAdjustment(deltaText, reason)

            if (
                input.delta < 0 &&
                -input.delta > balance.response.data.available
            ) {
                throw new Error(
                    'Pengurangan melebihi stok available.',
                )
            }
        } catch (cause) {
            setError(
                cause instanceof Error
                    ? cause.message
                    : 'Adjustment tidak valid.',
            )
            return
        }

        const request: PendingAdjustment = {
            productID: selectedID,
            delta: input.delta,
            reason: input.reason,
            key: crypto.randomUUID(),
        }

        // Freeze the original request until its outcome is known.
        pendingRef.current = request
        setPending(request)

        void sendAdjustment(request)
    }

    function abandonUncertainRequest() {
        if (busy || !pending) return

        const confirmed = window.confirm(
            'Pastikan kamu sudah memeriksa saldo dan riwayat stok.\n\n' +
            'Jika request sebelumnya sebenarnya berhasil, ' +
            'membuat adjustment baru dapat mengubah stok sekali lagi.\n\n' +
            'Abaikan request yang belum pasti?',
        )

        if (!confirmed) return

        clearPending()
        setError('')
        setNotice(
            'Request lama diabaikan. Periksa saldo terbaru sebelum melakukan adjustment baru.',
        )
        reload()
    }

    return (
        <div className="space-y-7">
            <header className="flex flex-wrap items-start justify-between gap-4">
                <div>
                    <p className="text-xs font-bold uppercase tracking-widest text-indigo-600">
                        Workspace / Inventory
                    </p>

                    <h1 className="mt-2 text-3xl font-bold text-slate-950">
                        Inventory Control
                    </h1>

                    <p className="mt-2 text-sm text-slate-500">
                        Pilih produk untuk melihat saldo stok dan
                        {isAdmin
                            ? ' mencatat adjustment.'
                            : ' informasi persediaan.'}
                    </p>
                </div>

                <button
                    type="button"
                    onClick={reload}
                    className="rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-medium"
                >
                    Muat ulang
                </button>
            </header>

            <section className="rounded-2xl border border-slate-200 bg-white shadow-sm">
                <div className="flex flex-wrap items-center justify-between gap-4 border-b border-slate-100 p-5">
                    <h2 className="font-bold text-slate-950">
                        Pilih Produk
                    </h2>

                    {isAdmin && (
                        <label className="flex items-center gap-2 text-xs text-slate-600">
                            <input
                                type="checkbox"
                                checked={includeInactive}
                                disabled={busy || pending !== null}
                                onChange={(event) => {
                                    setIncludeInactive(event.target.checked)
                                    setProductOffset(0)
                                }}
                            />
                            Sertakan nonaktif
                        </label>
                    )}
                </div>

                {products.status === 'loading' && (
                    <p role="status" className="p-6 text-sm text-slate-500">
                        Memuat daftar produk...
                    </p>
                )}

                {products.status === 'error' && (
                    <div className="p-5">
                        <ResourceError
                            message={products.message}
                            onRetry={() => {
                                if (
                                    products.httpStatus === 401 ||
                                    products.httpStatus === 403
                                ) {
                                    void refresh()
                                } else {
                                    reload()
                                }
                            }}
                        />
                    </div>
                )}

                {products.status === 'ready' && (
                    <>
                        {products.response.data.length === 0 ? (
                            <p className="p-8 text-sm text-slate-500">
                                Tidak ada produk pada halaman ini.
                            </p>
                        ) : (
                            <div className="grid gap-2 p-4 sm:grid-cols-2 xl:grid-cols-3">
                                {products.response.data.map((product) => (
                                    <button
                                        key={product.id}
                                        type="button"
                                        disabled={busy || pending !== null}
                                        onClick={() => selectProduct(product.id)}
                                        className={`rounded-xl border p-4 text-left transition disabled:opacity-60 ${selectedID === product.id
                                                ? 'border-indigo-500 bg-indigo-50'
                                                : 'border-slate-200 hover:border-indigo-300'
                                            }`}
                                    >
                                        <p className="font-semibold text-slate-900">
                                            {product.name}
                                        </p>

                                        <p className="mt-1 font-mono text-xs text-slate-500">
                                            {product.sku}
                                        </p>

                                        <p className="mt-3 text-xs text-slate-600">
                                            Available:{' '}
                                            {formatQuantity.format(product.available)}
                                        </p>

                                        {!product.is_active && (
                                            <p className="mt-2 text-xs font-semibold text-slate-500">
                                                Nonaktif
                                            </p>
                                        )}
                                    </button>
                                ))}
                            </div>
                        )}

                        <Pager
                            offset={productOffset}
                            pageSize={PRODUCT_PAGE_SIZE}
                            hasMore={products.response.pagination?.has_more ?? false}
                            disabled={busy || pending !== null}
                            onChange={setProductOffset}
                        />
                    </>
                )}
            </section>

            {selectedID === null ? (
                <p className="rounded-2xl border border-dashed border-slate-300 p-10 text-center text-sm text-slate-500">
                    Pilih produk terlebih dahulu untuk melihat inventory.
                </p>
            ) : (
                <>
                    {balance.status === 'loading' && (
                        <p role="status" className="rounded-2xl bg-white p-6 text-sm text-slate-500">
                            Memuat saldo inventory...
                        </p>
                    )}

                    {balance.status === 'error' && (
                        <ResourceError
                            message={balance.message}
                            onRetry={reload}
                        />
                    )}

                    {balance.status === 'ready' && (
                        <section className="space-y-5 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
                            <div>
                                <h2 className="text-xl font-bold text-slate-950">
                                    {balance.response.data.name}
                                </h2>

                                <p className="mt-1 font-mono text-xs text-slate-500">
                                    {balance.response.data.sku}
                                </p>
                            </div>

                            <div className="grid gap-3 sm:grid-cols-3">
                                {[
                                    {
                                        label: 'On Hand',
                                        value: balance.response.data.on_hand,
                                    },
                                    {
                                        label: 'Reserved',
                                        value: balance.response.data.reserved,
                                    },
                                    {
                                        label: 'Available',
                                        value: balance.response.data.available,
                                    },
                                ].map((metric) => (
                                    <div
                                        key={metric.label}
                                        className="rounded-xl bg-slate-50 p-4"
                                    >
                                        <p className="text-xs text-slate-500">
                                            {metric.label}
                                        </p>
                                        <p className="mt-2 text-2xl font-bold text-slate-950">
                                            {formatQuantity.format(metric.value)}
                                        </p>
                                    </div>
                                ))}
                            </div>

                            <p className="text-xs text-slate-500">
                                Threshold:{' '}
                                {formatQuantity.format(
                                    balance.response.data.low_stock_threshold,
                                )}
                                {' · '}
                                {balance.response.data.is_active
                                    ? balance.response.data.low_stock
                                        ? 'Stok pada/di bawah threshold'
                                        : 'Stok normal'
                                    : 'Produk nonaktif'}
                            </p>

                            {isAdmin && balance.response.data.is_active && (
                                <form
                                    onSubmit={submitAdjustment}
                                    className="space-y-4 border-t border-slate-100 pt-6"
                                >
                                    <h3 className="text-lg font-bold text-slate-950">
                                        Stock Adjustment
                                    </h3>

                                    <p className="text-sm text-slate-500">
                                        Gunakan angka positif untuk menambah
                                        dan negatif untuk mengurangi stok.
                                    </p>

                                    <label className="block text-sm font-medium">
                                        Perubahan stok (delta)
                                        <input
                                            type="number"
                                            step={1}
                                            required
                                            value={deltaText}
                                            disabled={busy || pending !== null}
                                            onChange={(event) =>
                                                setDeltaText(event.target.value)
                                            }
                                            className="mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 outline-none disabled:bg-slate-100"
                                            placeholder="Contoh: 10 atau -3"
                                        />
                                    </label>

                                    <label className="block text-sm font-medium">
                                        Alasan adjustment
                                        <textarea
                                            required
                                            minLength={10}
                                            maxLength={500}
                                            rows={3}
                                            value={reason}
                                            disabled={busy || pending !== null}
                                            onChange={(event) =>
                                                setReason(event.target.value)
                                            }
                                            className="mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 outline-none disabled:bg-slate-100"
                                            placeholder="Contoh: Barang diterima dari pemasok"
                                        />
                                    </label>

                                    {error && (
                                        <p role="alert" className="rounded-xl bg-rose-50 p-4 text-sm text-rose-700">
                                            {error}
                                        </p>
                                    )}

                                    {notice && (
                                        <p role="status" className="rounded-xl bg-emerald-50 p-4 text-sm text-emerald-700">
                                            {notice}
                                        </p>
                                    )}

                                    {pending && (
                                        <div className="space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
                                            <p className="text-sm font-semibold text-amber-800">
                                                Request belum dikonfirmasi
                                            </p>

                                            <p className="text-xs leading-6 text-amber-700">
                                                Perubahan {pending.delta} unit sedang
                                                menunggu hasil yang pasti. Retry memakai
                                                idempotency key dan payload yang sama.
                                            </p>

                                            <button
                                                type="button"
                                                disabled={busy}
                                                onClick={() =>
                                                    void sendAdjustment(pending)
                                                }
                                                className="rounded-lg bg-amber-700 px-4 py-2 text-sm font-semibold text-white disabled:opacity-50"
                                            >
                                                {busy
                                                    ? 'Mengirim...'
                                                    : 'Coba ulang request yang sama'}
                                            </button>

                                            <button
                                                type="button"
                                                disabled={busy}
                                                onClick={abandonUncertainRequest}
                                                className="ml-3 text-xs font-semibold text-amber-800 underline disabled:opacity-50"
                                            >
                                                Abaikan setelah verifikasi manual
                                            </button>
                                        </div>
                                    )}

                                    {!pending && (
                                        <button
                                            type="submit"
                                            disabled={busy}
                                            className="rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50"
                                        >
                                            {busy
                                                ? 'Menyimpan...'
                                                : 'Simpan Adjustment'}
                                        </button>
                                    )}
                                </form>
                            )}
                        </section>
                    )}

                    {isAdmin && (
                        <MovementHistory
                            key={`${selectedID}-${movementOffsetKey}`}
                            productID={selectedID}
                            revision={revision}
                        />
                    )}
                </>
            )}
        </div>
    )
}
