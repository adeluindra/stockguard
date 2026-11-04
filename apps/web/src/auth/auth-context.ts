
import { createContext, useContext } from 'react'

import type { SessionIdentity } from '../lib/auth-client'

export type AuthState =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'authenticated'; session: SessionIdentity }
  | { status: 'error'; message: string }

export interface AuthContextValue {
  state: AuthState
  refresh: () => Promise<void>
  login: (
    username: string,
    password: string,
  ) => Promise<SessionIdentity>
  logout: () => Promise<void>
  changePassword: (
    currentPassword: string,
    newPassword: string,
  ) => Promise<void>
}

export const AuthContext =
  createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)

  if (!context) {
    throw new Error('AuthProvider is missing')
  }

  return context
}
