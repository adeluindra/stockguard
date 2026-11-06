
export interface Product {
  id: number
  sku: string
  name: string
  description: string | null
  is_active: boolean
  low_stock_threshold: number
  on_hand: number
  reserved: number
  available: number
  created_at: string
  updated_at: string
}

export interface ProductFormValues {
  sku: string
  name: string
  description: string
  threshold: string
}

export interface ProductPayload {
  sku: string
  name: string
  description: string | null
  low_stock_threshold: number
}

export interface StockBalance {
  product_id: number
  sku: string
  name: string
  is_active: boolean
  low_stock_threshold: number
  on_hand: number
  reserved: number
  available: number
  low_stock: boolean
}

export interface StockMovement {
  id: number
  product_id: number
  order_id: number | null
  actor_user_id: number
  movement_type: string
  delta_on_hand: number
  delta_reserved: number
  reason: string | null
  created_at: string
}

export interface AdjustmentResult {
  balance: StockBalance
  movement: StockMovement
}

export const PRODUCT_PAGE_SIZE = 20
export const MOVEMENT_PAGE_SIZE = 10

export const formatQuantity = new Intl.NumberFormat('id-ID')

export function safeInteger(
  raw: string,
  allowNegative = false,
): number {
  if (!/^-?\d+$/.test(raw)) {
    throw new Error('Masukkan bilangan bulat yang valid.')
  }

  const value = Number(raw)

  if (
    !Number.isSafeInteger(value) ||
    (!allowNegative && value < 0)
  ) {
    throw new Error('Jumlah di luar batas angka yang didukung.')
  }

  return value
}

export function validateProductForm(
  values: ProductFormValues,
): ProductPayload {
  const sku = values.sku
  const name = values.name
  const description = values.description

  if (
    Array.from(sku).length < 3 ||
    Array.from(sku).length > 64 ||
    /\s/u.test(sku) ||
    /[\u0000-\u001F\u007F]/u.test(sku)
  ) {
    throw new Error(
      'SKU harus 3–64 karakter tanpa spasi atau karakter kontrol.',
    )
  }

  if (
    name !== name.trim() ||
    Array.from(name).length < 1 ||
    Array.from(name).length > 200 ||
    /[\u0000-\u001F\u007F]/u.test(name)
  ) {
    throw new Error('Nama produk harus 1–200 karakter yang valid.')
  }

  if (
    Array.from(description).length > 2000 ||
    description.includes('\0')
  ) {
    throw new Error('Deskripsi maksimal 2.000 karakter.')
  }

  return {
    sku,
    name,
    description: description === '' ? null : description,
    low_stock_threshold: safeInteger(values.threshold),
  }
}

export function validateAdjustment(
  rawDelta: string,
  reason: string,
): { delta: number; reason: string } {
  const delta = safeInteger(rawDelta, true)

  if (delta === 0) {
    throw new Error('Perubahan stok tidak boleh 0.')
  }

  if (
    reason !== reason.trim() ||
    Array.from(reason).length < 10 ||
    Array.from(reason).length > 500 ||
    /[\u0000-\u001F\u007F]/u.test(reason)
  ) {
    throw new Error(
      'Alasan harus 10–500 karakter tanpa spasi di awal/akhir.',
    )
  }

  return { delta, reason }
}
