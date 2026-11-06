
interface ResourceErrorProps {
  message: string
  onRetry: () => void
}

export function ResourceError({
  message,
  onRetry,
}: ResourceErrorProps) {
  return (
    <div
      role="alert"
      className="rounded-2xl border border-rose-200 bg-rose-50 p-6"
    >
      <p className="font-semibold text-rose-800">
        Gagal memuat data
      </p>
      <p className="mt-2 text-sm text-rose-700">{message}</p>
      <button
        type="button"
        onClick={onRetry}
        className="mt-4 rounded-lg bg-white px-4 py-2 text-sm font-medium text-rose-700"
      >
        Coba lagi
      </button>
    </div>
  )
}

interface PagerProps {
  offset: number
  pageSize: number
  hasMore: boolean
  disabled?: boolean
  onChange: (next: number) => void
}

export function Pager({
  offset,
  pageSize,
  hasMore,
  disabled = false,
  onChange,
}: PagerProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-4">
      <span className="text-xs text-slate-500">
        Halaman {Math.floor(offset / pageSize) + 1}
      </span>

      <div className="flex gap-2">
        <button
          type="button"
          disabled={disabled || offset === 0}
          onClick={() => onChange(Math.max(0, offset - pageSize))}
          className="rounded-lg border px-3 py-2 text-xs disabled:opacity-40"
        >
          Sebelumnya
        </button>
        <button
          type="button"
          disabled={disabled || !hasMore}
          onClick={() => onChange(offset + pageSize)}
          className="rounded-lg border px-3 py-2 text-xs disabled:opacity-40"
        >
          Berikutnya
        </button>
      </div>
    </div>
  )
}
