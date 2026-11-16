import type { ReactNode } from 'react'
import { NavLink } from 'react-router'
import { useAuth } from '../auth'

export default function Layout({ children }: { children: ReactNode }) {
  const { user, can } = useAuth()
  return (
    <div className="shell">
      <header className="topbar">
        <span className="brand">VulnMan</span>
        <nav aria-label="Main">
          <NavLink to="/" end>Dashboard</NavLink>
          <NavLink to="/findings" end>Findings</NavLink>
          {can('create') && <NavLink to="/findings/new">New finding</NavLink>}
          {can('import') && <NavLink to="/import">Import</NavLink>}
          {can('view_audit') && <NavLink to="/audit">Audit trail</NavLink>}
          {can('manage_roles') && <NavLink to="/admin/users">Users</NavLink>}
          {can('set_sla') && <NavLink to="/admin/sla">SLA</NavLink>}
        </nav>
        <div className="who">
          <span>{user?.name} <small>({user?.role})</small></span>
          {/* A real form POST: the server answers with a redirect to Keycloak's logout. */}
          <form method="post" action="/auth/logout">
            <button type="submit" className="link">Sign out</button>
          </form>
        </div>
      </header>
      <main>{children}</main>
    </div>
  )
}
