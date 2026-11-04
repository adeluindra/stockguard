import type { FormEvent } from 'react'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'

import { useAuth } from '../auth/auth-context'
import { authErrorMessage } from '../lib/auth-client'

export function LoginPage() {
    const { login } = useAuth()
    const navigate = useNavigate()

    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState('')

    async function handleSubmit(
        event: FormEvent<HTMLFormElement>,
    ) {
        event.preventDefault()

        if (submitting) return

        setSubmitting(true)
        setError('')

        try {
            const session = await login(username.trim(), password)
            setPassword('')

            if (session.must_change_password) {
                await navigate({
                    to: '/change-password',
                    replace: true,
                })
            } else {
                await navigate({
                    to: '/app',
                    replace: true,
                })
            }
        } catch (cause) {
            setError(authErrorMessage(cause))
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <main className="flex min-h-screen items-center justify-center bg-slate-100 px-5 py-10">
            <div className="w-full max-w-md rounded-3xl border border-slate-200 bg-white p-8 shadow-xl shadow-slate-200/70 sm:p-10">
                <div className="flex items-center gap-3">
                    <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-indigo-600 text-xl font-bold text-white">
                        S
                    </div>

                    <div>
                        <h1 className="text-xl font-bold text-slate-950">
                            StockGuard
                        </h1>
                        <p className="text-xs text-slate-500">
                            Inventory Workspace
                        </p>
                    </div>
                </div>

                <div className="mt-10">
                    <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">
                        Secure access
                    </p>

                    <h2 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">
                        Masuk ke akun
                    </h2>

                    <p className="mt-3 text-sm leading-6 text-slate-500">
                        Gunakan akun Admin atau Staff StockGuard
                        yang sudah terdaftar.
                    </p>
                </div>

                <form
                    onSubmit={(event) => void handleSubmit(event)}
                    className="mt-8 space-y-5"
                >
                    <div>
                        <label
                            htmlFor="username"
                            className="block text-sm font-semibold text-slate-700"
                        >
                            Username
                        </label>

                        <input
                            id="username"
                            name="username"
                            type="text"
                            autoComplete="username"
                            autoCapitalize="none"
                            spellCheck={false}
                            minLength={3}
                            maxLength={200}
                            required
                            value={username}
                            onChange={(event) => setUsername(event.target.value)}
                            disabled={submitting}
                            className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 disabled:opacity-60"
                            placeholder="Masukkan username"
                        />
                    </div>

                    <div>
                        <label
                            htmlFor="password"
                            className="block text-sm font-semibold text-slate-700"
                        >
                            Password
                        </label>

                        <input
                            id="password"
                            name="password"
                            type="password"
                            autoComplete="current-password"
                            required
                            maxLength={1024}
                            value={password}
                            onChange={(event) => setPassword(event.target.value)}
                            disabled={submitting}
                            className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 disabled:opacity-60"
                            placeholder="Masukkan password"
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
                        className="w-full rounded-xl bg-indigo-600 px-5 py-3.5 text-sm font-semibold text-white transition hover:bg-indigo-500 disabled:cursor-wait disabled:opacity-60"
                    >
                        {submitting ? 'Memverifikasi...' : 'Masuk'}
                    </button>
                </form>

                <p className="mt-8 border-t border-slate-100 pt-6 text-center text-xs leading-6 text-slate-400">
                    Akses terbatas untuk pengguna resmi StockGuard.
                    Jangan membagikan kredensial akun.
                </p>
            </div>
        </main>
    )
}
