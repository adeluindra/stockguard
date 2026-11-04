
export interface ApiResponse<T> {
  data: T
}

export class ApiRequestError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)

    this.name = 'ApiRequestError'
    this.status = status
  }
}

export async function getApiData<T>(
  path: string,
  signal?: AbortSignal,
): Promise<T> {
  if (
    path !== '/readyz' &&
    !path.startsWith('/api/v1/')
  ) {
    throw new Error('Unsupported API path')
  }

  const response = await fetch(path, {
    method: 'GET',
    credentials: 'same-origin',
    cache: 'no-store',
    headers: {
      Accept: 'application/json',
    },
    signal,
  })

  const payload: unknown = await response
    .json()
    .catch(() => null)

  if (!response.ok) {
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

  if (
    payload === null ||
    typeof payload !== 'object' ||
    !('data' in payload)
  ) {
    throw new Error('Invalid API response')
  }

  return (payload as ApiResponse<T>).data
}
