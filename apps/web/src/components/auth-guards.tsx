import type { ReactNode } from 'react'

import { Link, Navigate } from '@tanstack/react-router'

import { useAuth } from '../auth/auth-context'
import { AppShell } from './app-shell'
import { LoginPage } from '../pages/login-page'
import { ChangePasswordPage } from '../pages/change-password-page'

function LoadingScreen() {
  return (
    <main
      role="status"
      className="flex min-h-screen items-center justify-center bg-slate-50 px-5"
    >
      <p className="text-sm font-medium text-slate-600">
        Memeriksa sesi StockGuard...
      </p>
    </main>
  )
}

function UnavailableScreen() {
  const { state, refresh } = useAuth()

  return (
    <main className="flex min-h-screen items-center justify-center px-5">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-8 text-center shadow-sm">
        <h1 className="text-xl font-bold">
          Verifikasi sesi gagal
        </h1>

        <p role="alert" className="mt-3 text-sm text-slate-600">
          {state.status === 'error'
            ? state.message
            : 'Tidak dapat memeriksa sesi.'}
        </p>

        <button
          type="button"
          onClick={() => void refresh()}
          className="mt-6 rounded-xl bg-indigo-600 px-5 py-3 text-sm font-semibold text-white hover:bg-indigo-500"
        >
          Coba lagi
        </button>
      </div>
    </main>
  )
}

export function LoginRoute() {
  const { state } = useAuth()

  if (state.status === 'loading') return <LoadingScreen />
  if (state.status === 'error') return <UnavailableScreen />

  if (state.status === 'authenticated') {
    return (
      <Navigate
        to={
          state.session.must_change_password
            ? '/change-password'
            : '/app'
        }
        replace
      />
    )
  }

  return <LoginPage />
}

export function ProtectedApp() {
  const { state } = useAuth()

  if (state.status === 'loading') return <LoadingScreen />
  if (state.status === 'error') return <UnavailableScreen />

  if (state.status === 'anonymous') {
    return <Navigate to="/login" replace />
  }

  if (state.session.must_change_password) {
    return <Navigate to="/change-password" replace />
  }

  return <AppShell />
}

export function PasswordChangeRoute() {
  const { state } = useAuth()

  if (state.status === 'loading') return <LoadingScreen />
  if (state.status === 'error') return <UnavailableScreen />

  if (state.status === 'anonymous') {
    return <Navigate to="/login" replace />
  }

  return <ChangePasswordPage />
}

export function AdminOnly({
  children,
}: {
  children: ReactNode
}) {
  const { state } = useAuth()

  if (state.status !== 'authenticated') {
    return null
  }

  if (state.session.role !== 'admin') {
    return (
      <div className="rounded-2xl border border-amber-200 bg-white p-8">
        <p className="text-sm font-semibold text-amber-700">
          403 · Access denied
        </p>

        <h1 className="mt-3 text-2xl font-bold text-slate-950">
          Akses khusus Admin
        </h1>

        <p className="mt-3 text-sm text-slate-500">
          Akun Staff tidak memiliki izin mengakses
          manajemen pengguna.
        </p>

        <Link
          to="/app"
          className="mt-6 inline-block text-sm font-semibold text-indigo-600 hover:underline"
        >
          Kembali ke Overview
        </Link>
      </div>
    )
  }

  return <>{children}</>
}
