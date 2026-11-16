import type { Severity } from '../types'

export default function SeverityBadge({ severity }: { severity: Severity }) {
  return <span className={`badge sev-${severity}`}>{severity}</span>
}

export function fmtDate(iso?: string) {
  return iso ? new Date(iso).toLocaleDateString() : '-'
}

export function fmtTime(iso?: string) {
  return iso ? new Date(iso).toLocaleString() : '-'
}
