
export interface InventoryTotals {
  active_products: number
  inactive_products: number
  low_stock_products: number
  on_hand: number
  reserved: number
  available: number
}

export interface OrderTotals {
  total: number
  reserved: number
  fulfilled: number
  cancelled: number
}

export interface DashboardSummary {
  inventory: InventoryTotals
  orders: OrderTotals
}

export interface LowStockProduct {
  product_id: number
  sku: string
  name: string
  low_stock_threshold: number
  on_hand: number
  reserved: number
  available: number
}

export interface InventoryActivity {
  id: number
  product_id: number
  sku: string
  product_name: string
  order_id: number | null
  actor_user_id: number
  actor_username: string
  movement_type: string
  delta_on_hand: number
  delta_reserved: number
  reason: string | null
  created_at: string
}

export const DASHBOARD_PAGE_LIMIT = 5

export const quantityFormatter =
  new Intl.NumberFormat('id-ID')

export const activityDateFormatter =
  new Intl.DateTimeFormat('id-ID', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })

export function formatActivityDate(value: string): string {
  const timestamp = Date.parse(value)

  return Number.isNaN(timestamp)
    ? 'Tanggal tidak tersedia'
    : activityDateFormatter.format(timestamp)
}

export function formatDelta(value: number): string {
  const formatted = quantityFormatter.format(value)

  return value > 0 ? `+${formatted}` : formatted
}

const movementLabels: Record<string, string> = {
  initial: 'Saldo awal',
  adjustment: 'Penyesuaian stok',
  reserve: 'Reservasi order',
  release: 'Pembatalan reservasi',
  fulfill: 'Pemenuhan order',
}

export function movementLabel(type: string): string {
  return movementLabels[type] ?? type
}
