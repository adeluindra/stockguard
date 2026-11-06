
import { useState } from 'react'
import type { FormEvent } from 'react'

import { useAuth } from '../auth/auth-context'
import { ResourceError, Pager } from '../components/resource-ui'
import { useApiQuery } from '../hooks/use-api-query'
import { ApiRequestError } from '../lib/api'
import { writeApi } from '../lib/mutations'

import {
  PRODUCT_PAGE_SIZE,
  formatQuantity,
  validateProductForm,
} from '../lib/stock'

import type {
  Product,
  ProductFormValues,
} from '../lib/stock'

type Editor =
  | { kind: 'create' }
  | { kind: 'edit'; product: Product }

interface ProductEditorProps {
  editor: Editor
  onCancel: () => void
  onSaved: (message: string) => void
}

function ProductEditor({
  editor,
  onCancel,
  onSaved,
}: ProductEditorProps) {
  const original =
    editor.kind === 'edit' ? editor.product : null

  const [values, setValues] = useState<ProductFormValues>({
    sku: original?.sku ?? '',
    name: original?.name ?? '',
    description: original?.description ?? '',
    threshold: String(original?.low_stock_threshold ?? 0),
  })

  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  function update(
    field: keyof ProductFormValues,
    value: string,
  ) {
    setValues((current) => ({
      ...current,
      [field]: value,
    }))
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy) return

    let payload

    try {
      payload = validateProductForm(values)
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : 'Data produk tidak valid.',
      )
      return
    }

    setBusy(true)
    setError('')

    try {
      if (editor.kind === 'create') {
        const result = await writeApi<{ data: Product }>(
          '/api/v1/products',
          'POST',
          payload,
        )

        if (!result) {
          throw new Error('Respons pembuatan produk tidak valid.')
        }

        onSaved(`Produk ${result.data.sku} berhasil dibuat.`)
      } else {
        const result = await writeApi<{ data: Product }>(
          `/api/v1/products/${editor.product.id}`,
          'PUT',
          {
            name: payload.name,
            description: payload.description,
            low_stock_threshold:
              payload.low_stock_threshold,
          },
        )

        if (!result) {
          throw new Error('Respons pembaruan produk tidak valid.')
        }

        onSaved(`Produk ${result.data.sku} berhasil diperbarui.`)
      }
    } catch (cause) {
      let message =
        cause instanceof Error
          ? cause.message
          : 'Gagal menyimpan produk.'

      if (
        editor.kind === 'create' &&
        (
          !(cause instanceof ApiRequestError) ||
          cause.status >= 500
        )
      ) {
        message +=
          ' Jika respons jaringan tidak pasti, periksa daftar produk sebelum mencoba membuat ulang.'
      }

      setError(message)
    } finally {
      setBusy(false)
    }
  }

  const fieldClass =
    'mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 disabled:bg-slate-100'

  return (
    <section className="rounded-2xl border border-indigo-200 bg-white p-6 shadow-sm">
      <h2 className="text-lg font-bold">
        {editor.kind === 'create'
          ? 'Tambah Produk'
          : `Edit Produk · ${editor.product.sku}`}
      </h2>

      <p className="mt-2 text-sm text-slate-500">
        Produk baru dimulai dengan stok 0. Gunakan menu
        Inventory untuk menambahkan stok.
      </p>

      <form
        onSubmit={(event) => void handleSubmit(event)}
        className="mt-6 space-y-5"
      >
        <div className="grid gap-5 md:grid-cols-2">
          <label className="block text-sm font-medium text-slate-700">
            SKU
            <input
              value={values.sku}
              onChange={(event) =>
                update('sku', event.target.value)
              }
              disabled={busy || editor.kind === 'edit'}
              required
              minLength={3}
              maxLength={64}
              className={fieldClass}
            />
          </label>

          <label className="block text-sm font-medium text-slate-700">
            Nama Produk
            <input
              value={values.name}
              onChange={(event) =>
                update('name', event.target.value)
              }
              disabled={busy}
              required
              maxLength={200}
              className={fieldClass}
            />
          </label>
        </div>

        <label className="block text-sm font-medium text-slate-700">
          Deskripsi
          <textarea
            value={values.description}
            onChange={(event) =>
              update('description', event.target.value)
            }
            disabled={busy}
            maxLength={2000}
            rows={3}
            className={fieldClass}
          />
        </label>

        <label className="block text-sm font-medium text-slate-700">
          Low-stock threshold
          <input
            type="number"
            min={0}
            step={1}
            required
            value={values.threshold}
            onChange={(event) =>
              update('threshold', event.target.value)
            }
            disabled={busy}
            className={fieldClass}
          />
        </label>

        {error && (
          <p role="alert" className="rounded-xl bg-rose-50 p-3 text-sm text-rose-700">
            {error}
          </p>
        )}

        <div className="flex flex-wrap gap-3">
          <button
            type="submit"
            disabled={busy}
            className="rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50"
          >
            {busy ? 'Menyimpan...' : 'Simpan Produk'}
          </button>

          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            className="rounded-xl border border-slate-200 px-5 py-3 text-sm font-medium disabled:opacity-50"
          >
            Batal
          </button>
        </div>
      </form>
    </section>
  )
}

export function ProductsPage() {
  const { state, refresh } = useAuth()
  const isAdmin =
    state.status === 'authenticated' &&
    state.session.role === 'admin'

  const [offset, setOffset] = useState(0)
  const [revision, setRevision] = useState(0)
  const [includeInactive, setIncludeInactive] = useState(false)
  const [editor, setEditor] = useState<Editor | null>(null)
  const [busyID, setBusyID] = useState<number | null>(null)
  const [notice, setNotice] = useState('')

  const path =
    `/api/v1/products?limit=${PRODUCT_PAGE_SIZE}` +
    `&offset=${offset}` +
    `&include_inactive=${isAdmin && includeInactive}`

  const query = useApiQuery<Product[]>(path, revision)

  function reload() {
    setRevision((value) => value + 1)
  }

  function handleSaved(message: string) {
    setEditor(null)
    setNotice(message)
    reload()
  }

  async function deactivate(product: Product) {
    if (!isAdmin || !product.is_active || busyID !== null) {
      return
    }

    const confirmed = window.confirm(
      `Nonaktifkan "${product.name}"?\n\n` +
      'Produk tidak akan tersedia untuk order baru. ' +
      'Riwayat tetap disimpan.',
    )

    if (!confirmed) return

    setNotice('')
    setBusyID(product.id)

    try {
      await writeApi<never>(
        `/api/v1/products/${product.id}/deactivate`,
        'POST',
      )

      setNotice(`Produk ${product.sku} dinonaktifkan.`)
      reload()
    } catch (cause) {
      setNotice(
        cause instanceof Error
          ? `Gagal menonaktifkan produk: ${cause.message}`
          : 'Gagal menonaktifkan produk.',
      )
    } finally {
      setBusyID(null)
    }
  }

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-indigo-600">
            Workspace / Products
          </p>

          <h1 className="mt-2 text-3xl font-bold text-slate-950">
            Product Catalog
          </h1>

          <p className="mt-2 text-sm text-slate-500">
            Data produk dan stok yang berasal langsung
            dari backend StockGuard.
          </p>
        </div>

        {isAdmin && (
          <button
            type="button"
            onClick={() => {
              setEditor({ kind: 'create' })
              setNotice('')
            }}
            className="rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500"
          >
            + Tambah Produk
          </button>
        )}
      </header>

      {editor && isAdmin && (
        <ProductEditor
          key={
            editor.kind === 'create'
              ? 'create'
              : `edit-${editor.product.id}`
          }
          editor={editor}
          onCancel={() => setEditor(null)}
          onSaved={handleSaved}
        />
      )}

      {notice && (
        <p role="status" className="rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-700">
          {notice}
        </p>
      )}

      <div className="flex flex-wrap items-center justify-between gap-4">
        {isAdmin ? (
          <label className="flex items-center gap-2 text-sm text-slate-600">
            <input
              type="checkbox"
              checked={includeInactive}
              onChange={(event) => {
                setIncludeInactive(event.target.checked)
                setOffset(0)
              }}
            />
            Tampilkan produk nonaktif
          </label>
        ) : (
          <p className="text-sm text-slate-500">
            Menampilkan produk aktif.
          </p>
        )}

        <button
          type="button"
          onClick={reload}
          className="rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-600"
        >
          Muat ulang
        </button>
      </div>

      {query.status === 'loading' && (
        <p role="status" className="rounded-xl bg-white p-8 text-sm text-slate-500">
          Memuat produk...
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

      {query.status === 'ready' && (
        <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
          {query.response.data.length === 0 ? (
            <p className="p-10 text-center text-sm text-slate-500">
              Belum ada produk pada halaman ini.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[850px] text-left text-sm">
                <thead className="bg-slate-50 text-xs uppercase text-slate-500">
                  <tr>
                    <th scope="col" className="px-5 py-4">Produk</th>
                    <th scope="col" className="px-5 py-4 text-right">On Hand</th>
                    <th scope="col" className="px-5 py-4 text-right">Reserved</th>
                    <th scope="col" className="px-5 py-4 text-right">Available</th>
                    <th scope="col" className="px-5 py-4 text-right">Threshold</th>
                    <th scope="col" className="px-5 py-4">Status</th>
                    {isAdmin && (
                      <th scope="col" className="px-5 py-4">Aksi</th>
                    )}
                  </tr>
                </thead>

                <tbody className="divide-y divide-slate-100">
                  {query.response.data.map((product) => (
                    <tr key={product.id}>
                      <td className="px-5 py-4">
                        <p className="font-semibold text-slate-900">
                          {product.name}
                        </p>
                        <p className="mt-1 font-mono text-xs text-slate-400">
                          {product.sku}
                        </p>
                      </td>

                      <td className="px-5 py-4 text-right">
                        {formatQuantity.format(product.on_hand)}
                      </td>

                      <td className="px-5 py-4 text-right">
                        {formatQuantity.format(product.reserved)}
                      </td>

                      <td className="px-5 py-4 text-right font-semibold">
                        {formatQuantity.format(product.available)}
                      </td>

                      <td className="px-5 py-4 text-right">
                        {formatQuantity.format(product.low_stock_threshold)}
                      </td>

                      <td className="px-5 py-4">
                        <span className={
                          product.is_active
                            ? 'text-emerald-700'
                            : 'text-slate-400'
                        }>
                          {product.is_active ? 'Aktif' : 'Nonaktif'}
                        </span>
                      </td>

                      {isAdmin && (
                        <td className="px-5 py-4">
                          <div className="flex gap-3">
                            {product.is_active && (
                              <>
                                <button
                                  type="button"
                                  onClick={() => {
                                    setEditor({
                                      kind: 'edit',
                                      product,
                                    })
                                    setNotice('')
                                  }}
                                  className="font-semibold text-indigo-600"
                                >
                                  Edit
                                </button>

                                <button
                                  type="button"
                                  onClick={() =>
                                    void deactivate(product)
                                  }
                                  disabled={busyID !== null}
                                  className="font-semibold text-rose-600 disabled:opacity-40"
                                >
                                  Nonaktifkan
                                </button>
                              </>
                            )}
                          </div>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <Pager
            offset={offset}
            pageSize={PRODUCT_PAGE_SIZE}
            hasMore={query.response.pagination?.has_more ?? false}
            onChange={setOffset}
          />
        </section>
      )}
    </div>
  )
}
