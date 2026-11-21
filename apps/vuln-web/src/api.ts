// Same-origin client for vuln-api: the session is an HttpOnly cookie the
// browser sends by itself; no token ever touches JavaScript.
import type {
  Attachment, AuditEntry, Dashboard, Finding, FindingFilter, FindingList, ImportResult, Link, LinkKind, Person,
  SLA, SignedReport, Status, User, VerifyResult, WeekCount,
} from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

let onUnauthorized = (loginUrl: string) => window.location.assign(loginUrl)
export function setUnauthorizedHandler(fn: (loginUrl: string) => void) {
  onUnauthorized = fn
}

export function loginUrl(returnPath = window.location.pathname + window.location.search) {
  return '/login?return=' + encodeURIComponent(returnPath)
}

export async function request<T>(method: string, path: string, body?: unknown, opts: { quiet401?: boolean } = {}): Promise<T> {
  const headers: Record<string, string> = {}
  let payload: BodyInit | undefined
  if (body instanceof FormData || body instanceof Blob || typeof body === 'string') {
    payload = body as BodyInit
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const res = await fetch(path, { method, headers, body: payload, credentials: 'same-origin' })
  if (res.status === 401 && !opts.quiet401) {
    onUnauthorized(loginUrl())
    throw new ApiError(401, 'Your session has ended - signing you in again.')
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let data: unknown = undefined
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    data = undefined
  }
  if (!res.ok) {
    const msg = (data as { error?: string } | undefined)?.error ?? `Request failed (HTTP ${res.status})`
    throw new ApiError(res.status, msg)
  }
  return data as T
}

function query(f: FindingFilter): string {
  const p = new URLSearchParams()
  f.status?.forEach((s) => p.append('status', s))
  f.severity?.forEach((s) => p.append('severity', s))
  if (f.assignee) p.set('assignee', f.assignee)
  if (f.asset) p.set('asset', f.asset)
  if (f.q) p.set('q', f.q)
  if (f.overdue) p.set('overdue', 'true')
  if (f.limit) p.set('limit', String(f.limit))
  if (f.offset) p.set('offset', String(f.offset))
  const s = p.toString()
  return s ? '?' + s : ''
}

export interface NewFinding { title: string; description: string; asset: string; severity: string; cvss?: number; cve?: string }
export type FindingPatch = Partial<{ title: string; description: string; asset: string; severity: string; cve: string; cvss: number | null }>

export const api = {
  // quiet401: "not signed in" is a normal answer here, not a reason to redirect.
  me: () => request<User>('GET', '/api/me', undefined, { quiet401: true }),
  // VulnMan's own login form: vuln-api checks the credentials with Keycloak.
  signIn: (username: string, password: string) =>
    request<User>('POST', '/auth/password', { username, password }, { quiet401: true }),
  logout: () => request<void>('POST', '/auth/logout'),
  listFindings: (f: FindingFilter = {}) => request<FindingList>('GET', '/api/findings' + query(f)),
  exportCsvUrl: (f: FindingFilter = {}) => '/api/export/csv' + query(f),
  getFinding: (id: number) => request<Finding>('GET', `/api/findings/${id}`),
  createFinding: (nf: NewFinding) => request<Finding>('POST', '/api/findings', nf),
  patchFinding: (id: number, p: FindingPatch) => request<Finding>('PATCH', `/api/findings/${id}`, p),
  changeStatus: (id: number, status: Status, note = '') => request<Finding>('POST', `/api/findings/${id}/status`, { status, note }),
  assign: (id: number, assigneeSub: string, dueDate?: string) =>
    request<Finding>('POST', `/api/findings/${id}/assignment`, dueDate ? { assigneeSub, dueDate } : { assigneeSub }),
  assignees: () => request<Person[]>('GET', '/api/assignees'),
  links: (id: number) => request<Link[]>('GET', `/api/findings/${id}/links`),
  addLink: (id: number, otherRef: string, kind: LinkKind) => request<unknown>('POST', `/api/findings/${id}/links`, { otherRef, kind }),
  removeLink: (id: number, other: number) => request<void>('DELETE', `/api/findings/${id}/links/${other}`),
  attachments: (id: number) => request<Attachment[]>('GET', `/api/findings/${id}/attachments`),
  attachmentUrl: (id: number, att: number) => `/api/findings/${id}/attachments/${att}`,
  upload: (id: number, file: File) => {
    const fd = new FormData()
    fd.append('file', file, file.name)
    return request<Attachment>('POST', `/api/findings/${id}/attachments`, fd)
  },
  history: (id: number) => request<AuditEntry[]>('GET', `/api/findings/${id}/history`),
  dashboard: () => request<Dashboard>('GET', '/api/dashboard'),
  trend: (weeks = 12) => request<WeekCount[]>('GET', `/api/dashboard/trend?weeks=${weeks}`),
  sla: () => request<SLA>('GET', '/api/sla'),
  setSla: (sla: SLA) => request<SLA>('PUT', '/api/sla', sla),
  report: (from: string, to: string, asset = '') =>
    request<SignedReport>('GET', `/api/reports?from=${from}&to=${to}` + (asset ? '&asset=' + encodeURIComponent(asset) : '')),
  importTrivy: (body: Blob, asset = '') =>
    request<ImportResult>('POST', '/api/import/trivy' + (asset ? '?asset=' + encodeURIComponent(asset) : ''), body),
  importCsv: (body: Blob) => request<ImportResult>('POST', '/api/import/csv', body),
  audit: (p: { finding?: string; actor?: string; from?: string; to?: string } = {}) => {
    const q = new URLSearchParams(Object.entries(p).filter(([, v]) => v) as [string, string][]).toString()
    return request<AuditEntry[]>('GET', '/api/audit' + (q ? '?' + q : ''))
  },
  verifyAudit: () => request<VerifyResult>('GET', '/api/audit/verify'),
  users: () => request<Person[]>('GET', '/api/users'),
  setRole: (sub: string, role: string) => request<Person>('PUT', `/api/users/${encodeURIComponent(sub)}/role`, { role }),
  publicReport: (r: { title: string; description: string; asset: string; contactEmail: string; website: string }) =>
    request<{ reference: string; message: string }>('POST', '/api/public/reports', r),
}
