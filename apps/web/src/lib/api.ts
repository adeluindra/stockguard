
export interface Pagination {
  limit: number
  offset: number
  has_more: boolean
}

export interface ApiResponse<T> {
  data: T
  pagination?: Pagination
}

export class ApiRequestError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiRequestError'
    this.status = status
  }
}

function isRecord(
  value: unknown,
): value is Record<string, unknown> {
  return (
    value !== null &&
    typeof value === 'object' &&
    !Array.isArray(value)
  )
}

export async function getApiResponse<T>(
  path: string,
  signal?: AbortSignal,
): Promise<ApiResponse<T>> {
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

    if (isRecord(payload) && isRecord(payload.error)) {
      const serverMessage = payload.error.message

      if (typeof serverMessage === 'string') {
        message = serverMessage
      }
    }

    throw new ApiRequestError(response.status, message)
  }

  if (!isRecord(payload) || !('data' in payload)) {
    throw new Error('Invalid API response')
  }

  const result: ApiResponse<T> = {
    data: payload.data as T,
  }

  if ('pagination' in payload) {
    const page = payload.pagination

    if (
      !isRecord(page) ||
      typeof page.limit !== 'number' ||
      !Number.isInteger(page.limit) ||
      typeof page.offset !== 'number' ||
      !Number.isInteger(page.offset) ||
      typeof page.has_more !== 'boolean'
    ) {
      throw new Error('Invalid API pagination')
    }

    result.pagination = {
      limit: page.limit,
      offset: page.offset,
      has_more: page.has_more,
    }
  }

  return result
}

export async function getApiData<T>(
  path: string,
  signal?: AbortSignal,
): Promise<T> {
  const response = await getApiResponse<T>(path, signal)
  return response.data
}
