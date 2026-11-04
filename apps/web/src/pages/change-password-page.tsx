import type { FormEvent } from 'react'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'

import { useAuth } from '../auth/auth-context'
import { authErrorMessage } from '../lib/auth-client'

export function ChangePasswordPage() {
    const { state, changePassword } = useAuth()
    const navigate = useNavigate()

    const [currentPassword, setCurrentPassword] = useState('')
    const [newPassword, setNewPassword] = useState('')
    const [confirmation, setConfirmation] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState('')

    const required =
        state.status === 'authenticated' &&
        state.session.must_change_password

    async function handleSubmit(
        event: FormEvent<HTMLFormElement>,
    ) {
        event.preventDefault()

        if (submitting) return

        const characterCount = Array.from(newPassword).length
        const byteCount = new TextEncoder().encode(newPassword).length

        if (characterCount < 16 || byteCount > 1024) {
            setError(
                'Password baru harus minimal 16 karakter dan maksimal 1024 byte.',
            )
            return
        }

        if (newPassword !== confirmation) {
            setError('Konfirmasi password tidak cocok.')
            return
        }

        if (newPassword === currentPassword) {
            setError('Password baru harus berbeda dari password lama.')
            return
        }

        setError('')
        setSubmitting(true)

        try {
            await changePassword(currentPassword, newPassword)

            setCurrentPassword('')
            setNewPassword('')
            setConfirmation('')

            await navigate({
                to: '/login',
                replace: true,
            })
        } catch (cause) {
            setError(authErrorMessage(cause))
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <main className="flex min-h-screen items-center justify-center bg-slate-100 px-5 py-10">
            <div className="w-full max-w-lg rounded-3xl border border-slate-200 bg-white p-8 shadow-xl shadow-slate-200/70 sm:p-10">
                <div className="mb-7 flex h-12 w-12 items-center justify-center rounded-2xl bg-indigo-100 text-xl font-bold text-indigo-700">
                    S
                </div>

                <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">
                    Account security
                </p>

                <h1 className="mt-3 text-2xl font-bold text-slate-950">
                    Ganti password
                </h1>

                <p className="mt-3 text-sm leading-7 text-slate-500">
                    {required
                        ? 'Akun ini menggunakan password sementara. Kamu wajib menggantinya sebelum membuka workspace.'
                        : 'Perbarui password akun untuk menjaga keamanan akses.'}
                </p>

                <form
                    onSubmit={(event) => void handleSubmit(event)}
                    className="mt-8 space-y-5"
                >
                    <div>
                        <label
                            htmlFor="current-password"
                            className="block text-sm font-semibold text-slate-700"
                        >
                            Password saat ini
                        </label>
                        <input
                            id="current-password"
                            type="password"
                            autoComplete="current-password"
                            required
                            value={currentPassword}
                            onChange={(event) =>
                                setCurrentPassword(event.target.value)
                            }
                            disabled={submitting}
                            className="mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100"
                        />
                    </div>

                    <div>
                        <label
                            htmlFor="new-password"
                            className="block text-sm font-semibold text-slate-700"
                        >
                            Password baru
                        </label>
                        <input
                            id="new-password"
                            type="password"
                            autoComplete="new-password"
                            required
                            minLength={16}
                            value={newPassword}
                            onChange={(event) =>
                                setNewPassword(event.target.value)
                            }
                            disabled={submitting}
                            className="mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100"
                        />
                        <p className="mt-2 text-xs text-slate-500">
                            Minimal 16 karakter. Gunakan password yang unik.
                        </p>
                    </div>

                    <div>
                        <label
                            htmlFor="confirm-password"
                            className="block text-sm font-semibold text-slate-700"
                        >
                            Konfirmasi password baru
                        </label>
                        <input
                            id="confirm-password"
                            type="password"
                            autoComplete="new-password"
                            required
                            value={confirmation}
                            onChange={(event) =>
                                setConfirmation(event.target.value)
                            }
                            disabled={submitting}
                            className="mt-2 w-full rounded-xl border border-slate-300 px-4 py-3 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100"
                        />
                    </div>

                    {error && (
                        <p
                            role="alert"
                            className="rounded-xl bg-rose-50 p-3 text-sm text-rose-700"
                        >
                            {error}
                        </p>
                    )}

                    <button
                        type="submit"
                        disabled={submitting}
                        className="w-full rounded-xl bg-indigo-600 px-5 py-3.5 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
                    >
                        {submitting ? 'Menyimpan...' : 'Simpan password baru'}
                    </button>

                    {!required && (
                        <button
                            type="button"
                            disabled={submitting}
                            onClick={() =>
                                void navigate({ to: '/app' })
                            }
                            className="w-full py-2 text-sm font-medium text-slate-500 hover:text-slate-900"
                        >
                            Kembali ke workspace
                        </button>
                    )}
                </form>
            </div>
        </main>
    )
}
