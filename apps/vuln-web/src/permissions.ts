// Client copy of vuln-api's internal/domain rules, used ONLY to hide
// controls a user can't use. The server enforces every rule regardless.
import type { Role, Status } from './types'
export type { Role }

export type Action =
  | 'view' | 'create' | 'import' | 'edit' | 'change_status' | 'assign' | 'link' | 'attach'
  | 'export' | 'view_audit' | 'set_sla' | 'manage_roles'

const matrix: Record<Role, Action[]> = {
  analyst: ['view', 'create', 'import', 'edit', 'change_status', 'assign', 'link', 'attach', 'export'],
  developer: ['view', 'edit', 'change_status', 'link', 'attach'],
  pm: ['view', 'create', 'import', 'edit', 'change_status', 'assign', 'link', 'attach', 'export', 'view_audit', 'set_sla', 'manage_roles'],
  auditor: ['view', 'export', 'view_audit'],
}

export function can(role: Role, a: Action): boolean {
  return matrix[role]?.includes(a) ?? false
}

const owned: Action[] = ['edit', 'change_status', 'link', 'attach']

export function canOnFinding(role: Role, a: Action, actorSub: string, assigneeSub = ''): boolean {
  if (!can(role, a)) return false
  if (role === 'developer' && owned.includes(a)) return assigneeSub !== '' && actorSub === assigneeSub
  return true
}

const transitions: Record<Status, Status[]> = {
  New: ['Triaged', 'Accepted Risk'],
  Triaged: ['In Progress', 'Accepted Risk'],
  'In Progress': ['Remediated', 'Accepted Risk'],
  Remediated: ['In Progress'],
  'Accepted Risk': ['Triaged'],
}

export function nextStatuses(role: Role, from: Status): Status[] {
  if (!can(role, 'change_status')) return []
  const all = transitions[from] ?? []
  return role === 'developer' ? all.filter((s) => s === 'In Progress' || s === 'Remediated') : all
}
