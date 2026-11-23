// Mirrors vuln-api's JSON (internal/store, internal/domain).
export type Role = 'analyst' | 'developer' | 'pm' | 'auditor' | 'guest'
export type Severity = 'Critical' | 'High' | 'Medium' | 'Low'
export type Status = 'New' | 'Triaged' | 'In Progress' | 'Remediated' | 'Accepted Risk'
export const severities: Severity[] = ['Critical', 'High', 'Medium', 'Low']
export const statuses: Status[] = ['New', 'Triaged', 'In Progress', 'Remediated', 'Accepted Risk']

export interface User { sub: string; name: string; email: string; role: Role }
export interface Person { sub: string; username?: string; name: string; email?: string; role: Role | '' }

export interface Finding {
  id: number
  ref: string
  title: string
  description?: string
  asset: string
  severity: Severity
  cvss?: number
  cve?: string
  status: Status
  source: 'internal' | 'external' | 'trivy' | 'csv'
  reporterEmail?: string
  assigneeSub?: string
  assigneeName?: string
  dueDate?: string
  discoveredAt: string
  triagedAt?: string
  createdAt: string
  updatedAt: string
}

export interface FindingList { items: Finding[]; total: number }
export type LinkKind = 'duplicate' | 'related' | 'same_root_cause'
export interface Link { findingId: number; ref: string; title: string; kind: LinkKind }
export interface Attachment { id: number; findingId: number; name: string; contentType: string; size: number; createdBy: string; createdAt: string }
export interface AuditEntry { id: number; at: string; actorSub: string; actorRole: string; action: string; findingId?: number; details: Record<string, unknown>; hash: string }
export interface VerifyResult { ok: boolean; checked: number; firstBadId?: number }
export interface Dashboard {
  openBySeverity: Record<Severity, number>
  byStatus: Record<Status, number>
  overdue: number
  openWithDue: number
  slaCompliancePct: number
  overdueList: Finding[]
}
export interface WeekCount { weekStart: string; opened: number; closed: number }
export type SLA = Record<Severity, number>
export interface ImportResult { created: number; skipped: number }
export interface SignedReport { reportJson: string; signature: string; keyVersion: number }

export interface FindingFilter {
  status?: Status[]
  severity?: Severity[]
  assignee?: string
  asset?: string
  q?: string
  overdue?: boolean
  limit?: number
  offset?: number
}
