import { Navigate, Route, Routes, useLocation } from 'react-router'
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
import Landing from './routes/Landing'
import Login from './routes/Login'
import Report from './routes/Report'
import type { Action } from './permissions'

function Gate({ action, children, denied = 'Your role does not have access to this page.' }: { action: Action; children: React.ReactNode; denied?: string }) {
  const { can } = useAuth()
  return can(action) ? children : <p className="notice">{denied}</p>
}

// Pages that show findings need "view"; a guest (the pentest account) has none.
const noFindings = 'Your account has no access to findings.'

// Visitors see the product page; signed-in users go straight to the app.
function Home() {
  const { user } = useAuth()
  // Render the page straight away (no blank first paint); a signed-in user
  // is redirected as soon as the session check answers.
  return user ? <Navigate to="/app" replace /> : <Landing />
}

function Staff() {
  const { user, loading, error } = useAuth()
  const loc = useLocation()
  if (loading) return <p className="notice">Loading…</p>
  if (error) return <p className="notice error">{error}</p>
  if (!user) return <Navigate to={'/login?return=' + encodeURIComponent(loc.pathname + loc.search)} replace />
  return (
    <Layout>
      <Routes>
        <Route index element={<Gate action="view" denied={noFindings}><Dashboard /></Gate>} />
        <Route path="findings" element={<Gate action="view" denied={noFindings}><Findings /></Gate>} />
        <Route path="findings/new" element={<Gate action="create"><FindingNew /></Gate>} />
        <Route path="findings/:id" element={<Gate action="view" denied={noFindings}><FindingDetail /></Gate>} />
        <Route path="import" element={<Gate action="import"><Import /></Gate>} />
        <Route path="audit" element={<Gate action="view_audit"><Audit /></Gate>} />
        <Route path="admin/users" element={<Gate action="manage_roles"><AdminUsers /></Gate>} />
        <Route path="admin/sla" element={<Gate action="set_sla"><AdminSLA /></Gate>} />
        <Route path="*" element={<p className="notice">Page not found.</p>} />
      </Routes>
    </Layout>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />
        <Route path="/report" element={<Report />} />
        <Route path="/app/*" element={<Staff />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AuthProvider>
  )
}
