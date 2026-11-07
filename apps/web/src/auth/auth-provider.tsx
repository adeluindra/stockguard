
import {
  useCallback,
  useEffect,
  useState,
} from 'react'

import type { ReactNode } from 'react'

import { AuthContext } from './auth-context'
import type { AuthState } from './auth-context'

import { ApiRequestError } from '../lib/api'

import {
  authErrorMessage,
  changePasswordOnServer,
  getCurrentSession,
  loginOnServer,
  logoutOnServer,
} from '../lib/auth-client'

interface AuthProviderProps {
  children: ReactNode
}

// Hanya membaca sesi dan mengembalikan state.
// Tidak melakukan setState di dalam fungsi ini.
async function readSessionState(
  signal?: AbortSignal,
): Promise<AuthState> {
  try {
    const session = await getCurrentSession(signal)

    return {
      status: 'authenticated',
      session,
    }
  } catch (error) {
    if (
      error instanceof ApiRequestError &&
      error.status === 401
    ) {
      return {
        status: 'anonymous',
      }
    }

    return {
      status: 'error',
      message: authErrorMessage(error),
    }
  }
}

export function AuthProvider({
  children,
}: AuthProviderProps) {
  const [state, setState] = useState<AuthState>({
    status: 'loading',
  })

  // Verifikasi sesi saat aplikasi pertama dibuka.
  useEffect(() => {
    const controller = new AbortController()

    void readSessionState(controller.signal).then(
      (nextState) => {
        // Jangan memperbarui state bila effect
        // sudah dibersihkan atau request dibatalkan.
        if (controller.signal.aborted) {
          return
        }

        setState(nextState)
      },
    )

    return () => {
      controller.abort()
    }
  }, [])

  // Refresh dipicu oleh aksi aplikasi.
  const refresh = useCallback(async () => {
    setState({
      status: 'loading',
    })

    const nextState = await readSessionState()

    setState(nextState)
  }, [])

  const login = useCallback(
    async (
      username: string,
      password: string,
    ) => {
      await loginOnServer(username, password)

      try {
        // Backend adalah sumber kebenaran sesi.
        const session = await getCurrentSession()

        setState({
          status: 'authenticated',
          session,
        })

        return session
      } catch (error) {
        if (
          error instanceof ApiRequestError &&
          error.status === 401
        ) {
          setState({
            status: 'anonymous',
          })
        } else {
          setState({
            status: 'error',
            message: authErrorMessage(error),
          })
        }

        throw error
      }
    },
    [],
  )

  const logout = useCallback(async () => {
    try {
      await logoutOnServer()
    } catch (error) {
      // Sesi yang sudah tidak valid dianggap logout.
      if (
        !(error instanceof ApiRequestError) ||
        error.status !== 401
      ) {
        throw error
      }
    }

    setState({
      status: 'anonymous',
    })
  }, [])

  const changePassword = useCallback(
    async (
      currentPassword: string,
      newPassword: string,
    ) => {
      await changePasswordOnServer(
        currentPassword,
        newPassword,
      )

      // Backend mencabut seluruh sesi setelah
      // password berhasil diganti.
      setState({
        status: 'anonymous',
      })
    },
    [],
  )

  return (
    <AuthContext.Provider
      value={{
        state,
        refresh,
        login,
        logout,
        changePassword,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}
