
export interface StaffUser {
  id: number
  username: string
  role: 'staff'
  is_active: boolean
  must_change_password: boolean
  created_at: string
}

export interface CreateStaffResponse {
  data: {
    user: StaffUser
    temporary_password: string
  }
}

export interface ResetStaffPasswordResponse {
  data: {
    user_id: number
    temporary_password: string
    must_change_password: boolean
  }
}

export function validateStaffUsername(
  username: string,
): string {
  if (!/^[A-Za-z0-9_-]{3,50}$/.test(username)) {
    throw new Error(
      'Username harus 3–50 karakter, hanya huruf, angka, underscore (_), atau tanda hubung (-).',
    )
  }

  return username
}

const dateFormatter = new Intl.DateTimeFormat('id-ID', {
  dateStyle: 'medium',
  timeStyle: 'short',
})

export function formatStaffDate(value: string): string {
  const timestamp = Date.parse(value)

  return Number.isNaN(timestamp)
    ? 'Tanggal tidak tersedia'
    : dateFormatter.format(timestamp)
}
