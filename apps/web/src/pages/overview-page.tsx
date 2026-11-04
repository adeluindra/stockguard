
import { useAuth } from '../auth/auth-context'

import { AdminDashboard } from './admin-dashboard'
import { StaffOverview } from './staff-overview'

export function OverviewPage() {
  const { state } = useAuth()

  if (state.status !== 'authenticated') {
    return null
  }

  if (state.session.role === 'admin') {
    return <AdminDashboard />
  }

  return <StaffOverview />
}
