import type { ReactNode } from 'react'
import { NavLink } from 'react-router'
import { useAuth } from '../auth'
import Icon from './Icons'

function Item({ to, icon, label, end }: { to: string; icon: string; label: string; end?: boolean }) {
  return (
    <NavLink to={to} end={end} className={({ isActive }) => 'nav-item' + (isActive ? ' active' : '')}>
      <Icon name={icon} /> <span>{label}</span>
    </NavLink>
  )
}

const roleLabel: Record<string, string> = { pm: 'Project manager', analyst: 'Analyst', developer: 'Developer', auditor: 'Auditor', guest: 'Guest' }

export default function Layout({ children }: { children: ReactNode }) {
  const { user, can } = useAuth()
  const initials = (user?.name ?? '?').split(' ').map((p) => p[0]).join('').slice(0, 2).toUpperCase()
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><img className="brand-mark" src="/favicon.svg" alt="" width={30} height={30} /> Cerberoast</div>
        <nav aria-label="Main">
          {can('view') && <Item to="/app" end icon="grid" label="Dashboard" />}
          {can('view') && <Item to="/app/findings" end icon="list" label="Findings" />}
          {can('create') && <Item to="/app/findings/new" icon="plus" label="New finding" />}
          {can('import') && <Item to="/app/import" icon="upload" label="Import" />}
          {can('view_audit') && <Item to="/app/audit" icon="history" label="Audit trail" />}
          {(can('manage_roles') || can('set_sla')) && <p className="nav-section">Administration</p>}
          {can('manage_roles') && <Item to="/app/admin/users" icon="users" label="Users" />}
          {can('set_sla') && <Item to="/app/admin/sla" icon="clock" label="SLA targets" />}
        </nav>
        <div className="sidebar-user">
          <span className="avatar">{initials}</span>
          <div className="who">
            <strong>{user?.name}</strong>
            <small>{roleLabel[user?.role ?? ''] ?? user?.role}</small>
          </div>
          {/* A real form POST: the server answers with a redirect to Keycloak's logout. */}
          <form method="post" action="/auth/logout">
            <button type="submit" className="icon-button" title="Sign out" aria-label="Sign out"><Icon name="logout" /></button>
          </form>
        </div>
      </aside>
      <main className="content">{children}</main>
    </div>
  )
}
