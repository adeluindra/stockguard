
import { useRef, useState } from 'react'
import type { FormEvent } from 'react'

import { useAuth } from '../auth/auth-context'
import { ResourceError } from '../components/resource-ui'
import { useApiQuery } from '../hooks/use-api-query'
import { ApiRequestError } from '../lib/api'
import { writeApi } from '../lib/mutations'

import {
  formatStaffDate,
  validateStaffUsername,
} from '../lib/staff'

import type {
  StaffUser,
  CreateStaffResponse,
  ResetStaffPasswordResponse,
} from '../lib/staff'

interface TemporaryCredential {
  username: string
  password: string
  action: 'create' | 'reset'
}

function mutationError(
  error: unknown,
  operation: 'create' | 'reset' | 'deactivate',
): string {
  if (error instanceof ApiRequestError) {
    switch (error.status) {
      case 400:
        return 'Permintaan tidak valid. Periksa kembali data.'
      case 401:
        return 'Sesi login sudah tidak valid. Login kembali.'
      case 403:
        return 'Akun ini tidak memiliki izin Admin.'
      case 404:
        return 'Akun Staff aktif tidak ditemukan.'
      case 409:
        return 'Username sudah digunakan. Pilih username lain.'
    }
  }

  const details =
    error instanceof Error
      ? error.message
      : 'Tidak dapat menghubungi server.'

  const advice = {
    create:
      'Periksa daftar Staff sebelum mencoba lagi. ' +
      'Jika akun sudah dibuat tetapi password sementara hilang, ' +
      'gunakan Reset Password.',
    reset:
      'Hasil reset belum pasti. Periksa status akun, ' +
      'lalu lakukan reset kembali jika diperlukan.',
    deactivate:
      'Periksa status akun setelah memuat ulang daftar Staff.',
  }[operation]

  return `${details} ${advice}`
}

function TemporaryPasswordPanel({
  credential,
  onClose,
}: {
  credential: TemporaryCredential
  onClose: () => void
}) {
  const [visible, setVisible] = useState(false)
  const [copyMessage, setCopyMessage] = useState('')

  async function copyPassword() {
    try {
      await navigator.clipboard.writeText(
        credential.password,
      )

      setCopyMessage(
        'Password disalin. Bersihkan clipboard setelah digunakan.',
      )
    } catch {
      setCopyMessage(
        'Tidak dapat menyalin otomatis. Tampilkan password dan salin manual.',
      )
    }
  }

  return (
    <section
      aria-labelledby="credential-heading"
      className="rounded-2xl border border-amber-300 bg-amber-50 p-6"
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-amber-700">
            Kredensial Sementara
          </p>

          <h2
            id="credential-heading"
            className="mt-2 text-xl font-bold text-amber-950"
          >
            {credential.action === 'create'
              ? 'Akun Staff berhasil dibuat'
              : 'Password Staff berhasil direset'}
          </h2>

          <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900">
            Simpan kredensial ini di password manager
            yang aman. Password sementara hanya diberikan
            saat pembuatan atau reset dan tidak dapat
            dilihat lagi dari daftar akun.
          </p>
        </div>
      </div>

      <div className="mt-6 grid gap-4 md:grid-cols-2">
        <div>
          <label
            htmlFor="temporary-username"
            className="text-xs font-semibold text-amber-900"
          >
            Username
          </label>

          <input
            id="temporary-username"
            value={credential.username}
            readOnly
            className="mt-2 w-full rounded-xl border border-amber-200 bg-white px-4 py-3 font-mono text-sm text-slate-900"
          />
        </div>

        <div>
          <label
            htmlFor="temporary-password"
            className="text-xs font-semibold text-amber-900"
          >
            Password sementara
          </label>

          <input
            id="temporary-password"
            type={visible ? 'text' : 'password'}
            value={credential.password}
            readOnly
            autoComplete="off"
            className="mt-2 w-full rounded-xl border border-amber-200 bg-white px-4 py-3 font-mono text-sm text-slate-900"
          />
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-3">
        <button
          type="button"
          onClick={() => setVisible((value) => !value)}
          className="rounded-xl border border-amber-300 bg-white px-4 py-2 text-sm font-semibold text-amber-900"
        >
          {visible ? 'Sembunyikan' : 'Tampilkan'} password
        </button>

        <button
          type="button"
          onClick={() => void copyPassword()}
          className="rounded-xl border border-amber-300 bg-white px-4 py-2 text-sm font-semibold text-amber-900"
        >
          Salin password
        </button>
      </div>

      {copyMessage && (
        <p
          role="status"
          className="mt-3 text-xs text-amber-800"
        >
          {copyMessage}
        </p>
      )}

      <p className="mt-5 text-sm leading-6 text-amber-900">
        Saat login menggunakan password sementara,
        Staff akan diarahkan ke halaman ganti password.
        Password baru harus memiliki minimal 16 karakter.
      </p>

      <button
        type="button"
        onClick={onClose}
        className="mt-5 rounded-xl bg-amber-800 px-5 py-3 text-sm font-semibold text-white hover:bg-amber-900"
      >
        Sudah disimpan — Tutup kredensial
      </button>
    </section>
  )
}

export function StaffPage() {
  const { refresh } = useAuth()

  const [revision, setRevision] = useState(0)
  const [username, setUsername] = useState('')

  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)

  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [credential, setCredential] =
    useState<TemporaryCredential | null>(null)

  const staffQuery = useApiQuery<StaffUser[]>(
    '/api/v1/admin/staff',
    revision,
  )

  const staff =
    staffQuery.status === 'ready'
      ? staffQuery.response.data
      : []

  const activeCount = staff.filter(
    (user) => user.is_active,
  ).length

  function reload() {
    setRevision((value) => value + 1)
  }

  async function execute(
    operation: 'create' | 'reset' | 'deactivate',
    action: () => Promise<void>,
  ) {
    if (inFlight.current || credential !== null) return

    inFlight.current = true
    setBusy(true)
    setError('')
    setNotice('')

    try {
      await action()
    } catch (cause) {
      setError(mutationError(cause, operation))

      if (
        cause instanceof ApiRequestError &&
        (cause.status === 401 || cause.status === 403)
      ) {
        void refresh()
      }

      // Periksa state server setelah hasil operasi tidak pasti.
      reload()
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  function handleCreate(
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault()

    if (busy || credential !== null) return

    let validUsername: string

    try {
      validUsername = validateStaffUsername(username)
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : 'Username tidak valid.',
      )
      return
    }

    void execute('create', async () => {
      const response =
        await writeApi<CreateStaffResponse>(
          '/api/v1/admin/staff',
          'POST',
          { username: validUsername },
        )

      if (
        !response ||
        !response.data?.user ||
        !response.data.temporary_password
      ) {
        throw new Error(
          'Respons pembuatan Staff tidak lengkap.',
        )
      }

      setCredential({
        username: response.data.user.username,
        password: response.data.temporary_password,
        action: 'create',
      })

      setUsername('')
      reload()
    })
  }

  function handleReset(user: StaffUser) {
    if (
      busy ||
      credential !== null ||
      !user.is_active
    ) {
      return
    }

    const confirmed = window.confirm(
      `Reset password untuk "${user.username}"?\n\n` +
      'Semua sesi login Staff ini akan dicabut. ' +
      'Password lama tidak dapat digunakan lagi.',
    )

    if (!confirmed) return

    void execute('reset', async () => {
      const response =
        await writeApi<ResetStaffPasswordResponse>(
          `/api/v1/admin/staff/${user.id}/reset-password`,
          'POST',
        )

      if (
        !response ||
        response.data?.user_id !== user.id ||
        !response.data.temporary_password
      ) {
        throw new Error(
          'Respons reset password tidak lengkap.',
        )
      }

      setCredential({
        username: user.username,
        password: response.data.temporary_password,
        action: 'reset',
      })

      reload()
    })
  }

  function handleDeactivate(user: StaffUser) {
    if (
      busy ||
      credential !== null ||
      !user.is_active
    ) {
      return
    }

    const confirmed = window.confirm(
      `Nonaktifkan akun "${user.username}"?\n\n` +
      'Akun tidak dapat login lagi dan seluruh sesi ' +
      'aktifnya akan dicabut.\n\n' +
      'Tindakan ini tidak menghapus riwayat transaksi.',
    )

    if (!confirmed) return

    void execute('deactivate', async () => {
      await writeApi<never>(
        `/api/v1/admin/staff/${user.id}/deactivate`,
        'POST',
      )

      setNotice(
        `Akun "${user.username}" berhasil dinonaktifkan.`,
      )

      reload()
    })
  }

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-widest text-indigo-600">
            Workspace / Team
          </p>

          <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">
            Team Management
          </h1>

          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">
            Kelola akun Staff, password sementara,
            dan akses pengguna StockGuard.
            Halaman ini khusus administrator.
          </p>
        </div>

        <button
          type="button"
          onClick={reload}
          disabled={busy || credential !== null}
          className="rounded-xl border border-slate-200 bg-white px-5 py-3 text-sm font-semibold text-slate-700 disabled:opacity-40"
        >
          Muat ulang
        </button>
      </header>

      {credential && (
        <TemporaryPasswordPanel
          credential={credential}
          onClose={() => {
            setCredential(null)
            setError('')
          }}
        />
      )}

      {error && (
        <p
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm leading-6 text-rose-700"
        >
          {error}
        </p>
      )}

      {notice && (
        <p
          role="status"
          className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800"
        >
          {notice}
        </p>
      )}

      <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <div className="mb-5">
          <h2 className="text-lg font-bold text-slate-950">
            Tambah Akun Staff
          </h2>

          <p className="mt-2 text-sm text-slate-500">
            Backend akan membuat password sementara secara
            acak. Admin tidak menentukan password pengguna.
          </p>
        </div>

        <form
          onSubmit={handleCreate}
          className="flex flex-col gap-4 md:flex-row md:items-end"
        >
          <label className="flex-1 text-sm font-medium text-slate-700">
            Username Staff

            <input
              type="text"
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              required
              minLength={3}
              maxLength={50}
              pattern="[A-Za-z0-9_-]{3,50}"
              value={username}
              onChange={(event) =>
                setUsername(event.target.value)
              }
              disabled={busy || credential !== null}
              placeholder="Contoh: staff_gudang"
              className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none focus:border-indigo-500 disabled:bg-slate-100"
            />
          </label>

          <button
            type="submit"
            disabled={busy || credential !== null}
            className="rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-40"
          >
            {busy ? 'Memproses...' : '+ Buat Staff'}
          </button>
        </form>

        <p className="mt-3 text-xs text-slate-500">
          Gunakan 3–50 karakter: A–Z, a–z, 0–9,
          underscore, atau tanda hubung.
        </p>
      </section>

      <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-6 py-5">
          <div>
            <h2 className="text-lg font-bold text-slate-950">
              Daftar Staff
            </h2>

            <p className="mt-1 text-sm text-slate-500">
              Termasuk akun yang sudah dinonaktifkan.
            </p>
          </div>

          {staffQuery.status === 'ready' && (
            <p className="text-xs font-semibold text-slate-600">
              {activeCount} aktif / {staff.length} total
            </p>
          )}
        </div>

        {staffQuery.status === 'loading' && (
          <p
            role="status"
            className="p-8 text-sm text-slate-500"
          >
            Memuat daftar Staff...
          </p>
        )}

        {staffQuery.status === 'error' && (
          <div className="p-5">
            <ResourceError
              message={staffQuery.message}
              onRetry={() => {
                if (
                  staffQuery.httpStatus === 401 ||
                  staffQuery.httpStatus === 403
                ) {
                  void refresh()
                } else {
                  reload()
                }
              }}
            />
          </div>
        )}

        {staffQuery.status === 'ready' && (
          <>
            {staff.length === 0 ? (
              <div className="px-6 py-12 text-center">
                <p className="font-semibold text-slate-800">
                  Belum ada akun Staff
                </p>

                <p className="mt-2 text-sm text-slate-500">
                  Buat akun pertama menggunakan form
                  di bagian atas.
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[780px] text-left text-sm">
                  <thead className="bg-slate-50 text-xs uppercase tracking-wider text-slate-500">
                    <tr>
                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Pengguna
                      </th>

                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Status
                      </th>

                      <th
                        scope="col"
                        className="px-5 py-4"
                      >
                        Password
                      </th>

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
                    {staff.map((user) => (
                      <tr key={user.id}>
                        <td className="px-5 py-4">
                          <p className="font-semibold text-slate-900">
                            {user.username}
                          </p>

                          <p className="mt-1 text-xs text-slate-500">
                            ID #{user.id} · Staff
                          </p>
                        </td>

                        <td className="px-5 py-4">
                          <span
                            className={
                              user.is_active
                                ? 'rounded-lg bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-700'
                                : 'rounded-lg bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-500'
                            }
                          >
                            {user.is_active
                              ? 'Aktif'
                              : 'Nonaktif'}
                          </span>
                        </td>

                        <td className="px-5 py-4">
                          {user.must_change_password ? (
                            <span className="text-xs font-medium text-amber-700">
                              Wajib ganti password
                            </span>
                          ) : (
                            <span className="text-xs text-slate-500">
                              Sudah diatur
                            </span>
                          )}
                        </td>

                        <td className="px-5 py-4 text-sm text-slate-600">
                          {formatStaffDate(user.created_at)}
                        </td>

                        <td className="px-5 py-4">
                          {user.is_active ? (
                            <div className="flex flex-wrap gap-3">
                              <button
                                type="button"
                                disabled={
                                  busy ||
                                  credential !== null
                                }
                                onClick={() =>
                                  handleReset(user)
                                }
                                className="text-xs font-semibold text-indigo-600 hover:underline disabled:opacity-40"
                              >
                                Reset Password
                              </button>

                              <button
                                type="button"
                                disabled={
                                  busy ||
                                  credential !== null
                                }
                                onClick={() =>
                                  handleDeactivate(user)
                                }
                                className="text-xs font-semibold text-rose-600 hover:underline disabled:opacity-40"
                              >
                                Nonaktifkan
                              </button>
                            </div>
                          ) : (
                            <span className="text-xs text-slate-400">
                              Tidak ada aksi
                            </span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </section>

      <p className="text-xs leading-6 text-slate-500">
        Keamanan: hanya Admin dapat mengelola Staff.
        Password sementara tidak disimpan di browser
        secara permanen. Penonaktifan akun tidak menghapus
        riwayat order maupun inventory.
      </p>
    </div>
  )
}
