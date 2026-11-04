
import { useEffect, useState } from 'react'

import {
  ApiRequestError,
  getApiResponse,
} from '../lib/api'

import type { ApiResponse } from '../lib/api'

export type DashboardQueryState<T> =
  | { status: 'loading' }
  | {
      status: 'ready'
      response: ApiResponse<T>
    }
  | {
      status: 'error'
      message: string
      httpStatus: number | null
    }

function dashboardError(error: unknown): {
  message: string
  httpStatus: number | null
} {
  if (error instanceof ApiRequestError) {
    if (error.status === 401) {
      return {
        message:
          'Sesi sudah tidak valid. Periksa sesi dan login kembali.',
        httpStatus: 401,
      }
    }

    if (error.status === 403) {
      return {
        message:
          'Akun ini tidak mempunyai izin membaca dashboard.',
        httpStatus: 403,
      }
    }

    if (error.status === 503) {
      return {
        message:
          'Dashboard belum tersedia. Periksa backend dan database.',
        httpStatus: 503,
      }
    }

    return {
      message: error.message,
      httpStatus: error.status,
    }
  }

  return {
    message:
      'Gagal memuat data dashboard. Periksa koneksi lalu coba lagi.',
    httpStatus: null,
  }
}

export function useDashboardQuery<T>(
  path: string,
  revision: number,
): DashboardQueryState<T> {
  const [state, setState] = useState<
    DashboardQueryState<T>
  >({
    status: 'loading',
  })

  useEffect(() => {
    const controller = new AbortController()

    setState({ status: 'loading' })

    void getApiResponse<T>(path, controller.signal)
      .then((response) => {
        if (controller.signal.aborted) return

        setState({
          status: 'ready',
          response,
        })
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return

        const result = dashboardError(error)

        setState({
          status: 'error',
          ...result,
        })
      })

    return () => {
      controller.abort()
    }
  }, [path, revision])

  return state
}
