
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

export function AuthProvider({
  children,
}: AuthProviderProps) {
  const [state, setState] = useState<AuthState>({
    status: 'loading',
  })

  const loadSession = useCallback(
    async (signal?: AbortSignal): Promise<void> => {
      setState({ status: 'loading' })

      try {
        const session = await getCurrentSession(signal)

        if (!signal?.aborted) {
          setState({
            status: 'authenticated',
            session,
          })
        }
      } catch (error) {
        if (signal?.aborted) return

        if (
          error instanceof ApiRequestError &&
          error.status === 401
        ) {
          setState({ status: 'anonymous' })
          return
        }

        setState({
          status: 'error',
          message: authErrorMessage(error),
        })
      }
    },
    [],
  )

  useEffect(() => {
    const controller = new AbortController()

    void loadSession(controller.signal)

    return () => controller.abort()
  }, [loadSession])

  const refresh = useCallback(async () => {
    await loadSession()
  }, [loadSession])

  const login = useCallback(
    async (username: string, password: string) => {
      await loginOnServer(username, password)

      try {
        // Read the authoritative session after login.
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
          setState({ status: 'anonymous' })
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
      // A missing/expired session is already logged out.
      if (
        !(error instanceof ApiRequestError) ||
        error.status !== 401
      ) {
        throw error
      }
    }

    setState({ status: 'anonymous' })
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

      // Backend revokes every session and clears
      // the browser cookie after a successful change.
      setState({ status: 'anonymous' })
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
