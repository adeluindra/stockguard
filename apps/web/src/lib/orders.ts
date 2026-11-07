
import { safeInteger } from './stock'

export type OrderStatus = 'reserved' | 'fulfilled' | 'cancelled'

export interface OrderItem {
  product_id: number
  quantity: number
}

export interface Order {
  id: number
  created_by: number
  status: OrderStatus
  items: OrderItem[]
  created_at: string
  updated_at: string
  fulfilled_at: string | null
  cancelled_at: string | null
}

export interface OrderSummary {
  id: number
  created_by: number
  status: OrderStatus
  item_count: number
  created_at: string
  updated_at: string
  fulfilled_at: string | null
  cancelled_at: string | null
}

export interface OrderWriteResult {
  data: Order
  replayed: boolean
}

export interface OrderDraftLine {
  productId: number
  sku: string
  name: string
  availableAtSelection: number
  quantity: string
}

export const ORDER_PAGE_SIZE = 20
export const ORDER_PRODUCT_PAGE_SIZE = 20
export const MAX_ORDER_ITEMS = 50

export const orderStatusLabel: Record<OrderStatus, string> = {
  reserved: 'Reserved',
  fulfilled: 'Fulfilled',
  cancelled: 'Cancelled',
}

export const orderStatusClass: Record<OrderStatus, string> = {
  reserved: 'bg-amber-100 text-amber-800',
  fulfilled: 'bg-emerald-100 text-emerald-800',
  cancelled: 'bg-rose-100 text-rose-800',
}

const dateFormatter = new Intl.DateTimeFormat('id-ID', {
  dateStyle: 'medium',
  timeStyle: 'short',
})

export function formatOrderDate(value: string | null): string {
  if (!value) return '-'

  const timestamp = Date.parse(value)

  return Number.isNaN(timestamp)
    ? 'Tanggal tidak valid'
    : dateFormatter.format(timestamp)
}

export function validateOrderDraft(
  lines: OrderDraftLine[],
): OrderItem[] {
  if (lines.length < 1 || lines.length > MAX_ORDER_ITEMS) {
    throw new Error('Order harus berisi 1–50 produk berbeda.')
  }

  const ids = new Set<number>()
  const items: OrderItem[] = []

  for (const line of lines) {
    if (
      !Number.isSafeInteger(line.productId) ||
      line.productId <= 0
    ) {
      throw new Error('ID produk tidak valid.')
    }

    if (ids.has(line.productId)) {
      throw new Error('Produk tidak boleh ditambahkan dua kali.')
    }

    ids.add(line.productId)

    const quantity = safeInteger(line.quantity)

    if (quantity <= 0) {
      throw new Error(
        `Kuantitas ${line.sku} harus lebih dari 0.`,
      )
    }

    if (quantity > line.availableAtSelection) {
      throw new Error(
        `Kuantitas ${line.sku} melebihi stok tersedia saat dipilih. ` +
          'Muat ulang daftar produk jika stok berubah.',
      )
    }

    items.push({
      product_id: line.productId,
      quantity,
    })
  }

  // Backend juga mengurutkan item berdasarkan product ID.
  return items.sort(
    (a, b) => a.product_id - b.product_id,
  )
}
