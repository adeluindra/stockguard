
import { useEffect, useState } from 'react'

import {
  ApiRequestError,
  getApiResponse,
} from '../lib/api'

import type { ApiResponse } from '../lib/api'

export type ApiQueryState<T> =
  | {
      status: 'loading'
    }
  | {
      status: 'ready'
      response: ApiResponse<T>
    }
  | {
      status: 'error'
      message: string
      httpStatus: number | null
    }

interface QuerySnapshot<T> {
  key: string
  value: ApiQueryState<T>
}

export function useApiQuery<T>(
  path: string,
  revision: number,
): ApiQueryState<T> {
  const key = `${path}\u0000${revision}`

  const [snapshot, setSnapshot] =
    useState<QuerySnapshot<T>>({
      key,
      value: {
        status: 'loading',
      },
    })

  useEffect(() => {
    const controller = new AbortController()

    if (!path) {
      return () => controller.abort()
    }

    void getApiResponse<T>(
      path,
      controller.signal,
    )
      .then((response) => {
        if (controller.signal.aborted) {
          return
        }

        setSnapshot({
          key,
          value: {
            status: 'ready',
            response,
          },
        })
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) {
          return
        }

        const httpStatus =
          error instanceof ApiRequestError
            ? error.status
            : null

        let message =
          'Gagal mengambil data dari server.'

        if (httpStatus === 401) {
          message =
            'Sesi tidak valid. Silakan periksa login.'
        } else if (httpStatus === 403) {
          message = 'Akses tidak diizinkan.'
        } else if (httpStatus === 503) {
          message =
            'Backend atau database tidak tersedia.'
        } else if (error instanceof Error) {
          message = error.message
        }

        setSnapshot({
          key,
          value: {
            status: 'error',
            message,
            httpStatus,
          },
        })
      })

    return () => {
      controller.abort()
    }
  }, [key, path])

  // Respons dari request sebelumnya tidak
  // boleh tampil untuk URL/revision yang baru.
  if (snapshot.key !== key) {
    return {
      status: 'loading',
    }
  }

  return snapshot.value
}
