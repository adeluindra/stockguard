import { ApiRequestError, getApiData } from './api'

type WriteMethod = 'POST' | 'PUT'

interface CsrfResponse {
  csrf_token: string
}

function isObject(
  value: unknown,
): value is Record<string, unknown> {
  return (
    value !== null &&
    typeof value === 'object' &&
    !Array.isArray(value)
  )
}

export async function writeApi<T>(
  path: string,
  method: WriteMethod,
  payload?: unknown,
  idempotencyKey?: string,
): Promise<T | null> {
  if (!path.startsWith('/api/v1/')) {
    throw new Error('Invalid API path')
  }

  const csrf = await getApiData<CsrfResponse>(
    '/api/v1/auth/csrf',
  )

  if (
    typeof csrf.csrf_token !== 'string' ||
    csrf.csrf_token.length === 0
  ) {
    throw new Error('Session CSRF token not available')
  }

  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-CSRF-Token': csrf.csrf_token,
  }

  if (payload !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  if (idempotencyKey !== undefined) {
    headers['Idempotency-Key'] = idempotencyKey
  }

  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    cache: 'no-store',
    headers,
    body:
      payload === undefined
        ? undefined
        : JSON.stringify(payload),
  })

  if (!response.ok) {
    const errorPayload: unknown = await response
      .json()
      .catch(() => null)

    let message = `Request failed (${response.status})`

    if (
      isObject(errorPayload) &&
      isObject(errorPayload.error) &&
      typeof errorPayload.error.message === 'string'
    ) {
      message = errorPayload.error.message
    }

    throw new ApiRequestError(response.status, message)
  }

  if (response.status === 204) {
    return null
  }

  const result: unknown = await response.json()

  if (!isObject(result) || !('data' in result)) {
    throw new Error('Invalid API mutation response')
  }

  return result as T
}
