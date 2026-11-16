import { Route, Routes, useLocation } from 'react-router'
import { loginUrl } from './api'
import { AuthProvider, useAuth } from './auth'
import Layout from './components/Layout'
import AdminSLA from './routes/AdminSLA'
import AdminUsers from './routes/AdminUsers'
import Audit from './routes/Audit'
import Dashboard from './routes/Dashboard'
import FindingDetail from './routes/FindingDetail'
import FindingNew from './routes/FindingNew'
import Findings from './routes/Findings'
import Import from './routes/Import'
import Report from './routes/Report'
import type { Action } from './permissions'

function Gate({ action, children }: { action: Action; children: React.ReactNode }) {
  const { can } = useAuth()
  return can(action) ? children : <p className="notice">Your role does not have access to this page.</p>
}

function Staff() {
  const { user, loading, error } = useAuth()
  if (loading) return <p className="notice">Loading…</p>
  if (error) return <p className="notice error">{error}</p>
  if (!user) {
    window.location.assign(loginUrl())
    return <p className="notice">Signing you in…</p>
  }
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/findings" element={<Findings />} />
        <Route path="/findings/new" element={<Gate action="create"><FindingNew /></Gate>} />
        <Route path="/findings/:id" element={<FindingDetail />} />
        <Route path="/import" element={<Gate action="import"><Import /></Gate>} />
        <Route path="/audit" element={<Gate action="view_audit"><Audit /></Gate>} />
        <Route path="/admin/users" element={<Gate action="manage_roles"><AdminUsers /></Gate>} />
        <Route path="/admin/sla" element={<Gate action="set_sla"><AdminSLA /></Gate>} />
        <Route path="*" element={<p className="notice">Page not found.</p>} />
      </Routes>
    </Layout>
  )
}

export default function App() {
  // The public reporting form needs no login (AUTH-03).
  if (useLocation().pathname === '/report') return <Report />
  return (
    <AuthProvider>
      <Staff />
    </AuthProvider>
  )
}
