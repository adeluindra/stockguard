
import { ApiRequestError, getApiData } from './api'

export interface SessionIdentity {
  user_id: number
  role: 'admin' | 'staff'
  must_change_password: boolean
  expires_at: string
}

interface CsrfResponse {
  csrf_token: string
}

type AuthPostPath =
  | '/api/v1/auth/login'
  | '/api/v1/auth/logout'
  | '/api/v1/auth/change-password'

async function getCsrfToken(
  path: '/api/v1/auth/login-csrf' | '/api/v1/auth/csrf',
): Promise<string> {
  const result = await getApiData<CsrfResponse>(path)

  if (!result.csrf_token) {
    throw new Error('CSRF token missing from server')
  }

  return result.csrf_token
}

async function postAuth(
  path: AuthPostPath,
  csrfToken: string,
  body?: Record<string, string>,
): Promise<void> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-CSRF-Token': csrfToken,
  }

  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  const response = await fetch(path, {
    method: 'POST',
    credentials: 'same-origin',
    cache: 'no-store',
    headers,
    body: body === undefined
      ? undefined
      : JSON.stringify(body),
  })

  if (response.ok) {
    return
  }

  const payload: unknown = await response
    .json()
    .catch(() => null)

  let message = `Request failed (${response.status})`

  if (
    payload !== null &&
    typeof payload === 'object' &&
    'error' in payload
  ) {
    const error = payload.error

    if (
      error !== null &&
      typeof error === 'object' &&
      'message' in error &&
      typeof error.message === 'string'
    ) {
      message = error.message
    }
  }

  throw new ApiRequestError(response.status, message)
}

export function getCurrentSession(
  signal?: AbortSignal,
): Promise<SessionIdentity> {
  return getApiData<SessionIdentity>(
    '/api/v1/auth/me',
    signal,
  )
}

export async function loginOnServer(
  username: string,
  password: string,
): Promise<void> {
  // Backend sets the login CSRF cookie.
  const token = await getCsrfToken(
    '/api/v1/auth/login-csrf',
  )

  await postAuth(
    '/api/v1/auth/login',
    token,
    { username, password },
  )
}

export async function logoutOnServer(): Promise<void> {
  const token = await getCsrfToken(
    '/api/v1/auth/csrf',
  )

  await postAuth('/api/v1/auth/logout', token)
}

export async function changePasswordOnServer(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  const token = await getCsrfToken(
    '/api/v1/auth/csrf',
  )

  await postAuth(
    '/api/v1/auth/change-password',
    token,
    {
      current_password: currentPassword,
      new_password: newPassword,
    },
  )
}

export function authErrorMessage(error: unknown): string {
  if (error instanceof ApiRequestError) {
    switch (error.status) {
      case 401:
        return 'Username, password, atau sesi tidak valid.'
      case 429:
        return 'Terlalu banyak percobaan login. Coba kembali nanti.'
      case 503:
        return 'Layanan sedang tidak tersedia. Periksa backend.'
      default:
        return error.message
    }
  }

  return 'Tidak dapat menghubungi server. Periksa koneksi backend.'
}
