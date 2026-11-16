import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { finding, json, mockApi, renderAt, users } from '../test-utils'
import AdminSLA from './AdminSLA'
import AdminUsers from './AdminUsers'
import Audit from './Audit'
import Dashboard from './Dashboard'
import Import from './Import'

afterEach(() => vi.unstubAllGlobals())

const dash = {
  openBySeverity: { Critical: 2, High: 5, Medium: 1, Low: 0 },
  byStatus: { New: 3, Triaged: 2, 'In Progress': 3, Remediated: 4, 'Accepted Risk': 1 },
  overdue: 1, openWithDue: 3, slaCompliancePct: 66.7,
  overdueList: [finding({ id: 9, title: 'Late one', dueDate: '2026-11-10T00:00:00Z', status: 'Triaged' })],
}
const trend = [
  { weekStart: '2026-11-09T00:00:00Z', opened: 1, closed: 0 },
  { weekStart: '2026-11-16T00:00:00Z', opened: 5, closed: 2 },
]

describe('Dashboard', () => {
  it('renders counts and SLA compliance', async () => {
    mockApi({ 'GET /api/me': users.auditor, 'GET /api/dashboard': dash, 'GET /api/dashboard/trend': trend })
    renderAt(<Dashboard />, '/')
    const sev = await screen.findByRole('figure', { name: /open findings by severity/i })
    expect(within(sev).getByText('Critical: 2')).toBeInTheDocument()
    expect(within(sev).getByText('High: 5')).toBeInTheDocument()
    expect(screen.getByRole('figure', { name: /findings by status/i })).toHaveTextContent('Remediated: 4')
    expect(screen.getByText('66.7%')).toBeInTheDocument()
    expect(screen.getByText(/1 of 3 open findings with a due date are overdue/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Late one' })).toHaveAttribute('href', '/findings/9')
    expect(screen.getByRole('figure', { name: /opened vs closed/i })).toHaveTextContent('week of')
  })

  it('offers the signed report to roles that may export', async () => {
    const calls = mockApi({
      'GET /api/me': users.pm, 'GET /api/dashboard': dash, 'GET /api/dashboard/trend': trend,
      'GET /api/reports': { reportJson: '{"total":0}', signature: 'vault:v1:abc', keyVersion: 1 },
    })
    renderAt(<Dashboard />, '/')
    const u = userEvent.setup()
    await u.type(await screen.findByLabelText(/^from/i), '2026-11-01')
    await u.type(screen.getByLabelText(/^to/i), '2026-12-01')
    await u.click(screen.getByRole('button', { name: /download signed report/i }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/reports?from=2026-11-01&to=2026-12-01')).toBe(true))
    expect(await screen.findByText(/signed with key version 1/i)).toBeInTheDocument()
  })

  it('hides the report form from developers', async () => {
    mockApi({ 'GET /api/me': users.developer, 'GET /api/dashboard': dash, 'GET /api/dashboard/trend': trend })
    renderAt(<Dashboard />, '/')
    await screen.findByRole('figure', { name: /open findings by severity/i })
    expect(screen.queryByRole('button', { name: /download signed report/i })).toBeNull()
  })
})

describe('Audit', () => {
  it('verify shows the first tampered id', async () => {
    mockApi({ 'GET /api/me': users.auditor, 'GET /api/audit': [], 'GET /api/audit/verify': { ok: false, checked: 12, firstBadId: 5 } })
    renderAt(<Audit />, '/audit')
    await userEvent.click(await screen.findByRole('button', { name: /verify integrity/i }))
    expect(await screen.findByText(/tampering detected at entry #5/i)).toBeInTheDocument()
  })

  it('verify reports a clean chain', async () => {
    mockApi({ 'GET /api/me': users.pm, 'GET /api/audit': [], 'GET /api/audit/verify': { ok: true, checked: 12 } })
    renderAt(<Audit />, '/audit')
    await userEvent.click(await screen.findByRole('button', { name: /verify integrity/i }))
    expect(await screen.findByText(/all 12 entries verified/i)).toBeInTheDocument()
  })

  it('lists entries with actor and action', async () => {
    mockApi({
      'GET /api/me': users.pm,
      'GET /api/audit': [{ id: 3, at: '2026-11-16T10:00:00Z', actorSub: 'an-1', actorRole: 'analyst', action: 'finding.status', findingId: 7, details: { to: 'Triaged' }, hash: 'ab' }],
      'GET /api/audit/verify': { ok: true, checked: 1 },
    })
    renderAt(<Audit />, '/audit')
    expect(await screen.findByText('finding.status')).toBeInTheDocument()
    expect(screen.getByText(/an-1/)).toBeInTheDocument()
  })
})

describe('AdminUsers', () => {
  it('role select is disabled for yourself and changes others', async () => {
    const calls = mockApi({
      'GET /api/me': users.pm,
      'GET /api/users': [
        { sub: 'pm-1', name: 'Pat PM', role: 'pm' },
        { sub: 'dev-1', name: 'Dee Dev', role: 'developer' },
      ],
      'PUT /api/users/:sub/role': { sub: 'dev-1', name: 'Dee Dev', role: 'analyst' },
    })
    renderAt(<AdminUsers />, '/admin/users')
    expect(await screen.findByLabelText('Role for Pat PM')).toBeDisabled()
    await userEvent.selectOptions(screen.getByLabelText('Role for Dee Dev'), 'analyst')
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.url === '/api/users/dev-1/role')).toBe(true))
    expect(await screen.findByText(/Dee Dev is now analyst/i)).toBeInTheDocument()
  })
})

describe('AdminSLA', () => {
  it('rejects 0 days client-side and shows a server error', async () => {
    const calls = mockApi({
      'GET /api/me': users.pm,
      'GET /api/sla': { Critical: 7, High: 30, Medium: 90, Low: 180 },
      'PUT /api/sla': json({ error: 'Low: days must be 1-365' }, 400),
    })
    renderAt(<AdminSLA />, '/admin/sla')
    const u = userEvent.setup()
    const crit = await screen.findByLabelText(/critical/i)
    await u.clear(crit)
    await u.type(crit, '0')
    await u.click(screen.getByRole('button', { name: /save/i }))
    expect(calls.some((c) => c.method === 'PUT')).toBe(false)
    await u.clear(crit)
    await u.type(crit, '5')
    await u.click(screen.getByRole('button', { name: /save/i }))
    expect(await screen.findByText('Low: days must be 1-365')).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'PUT')!.body).toEqual({ Critical: 5, High: 30, Medium: 90, Low: 180 })
  })
})

describe('Import', () => {
  it('uploads a Trivy report and shows the result', async () => {
    const calls = mockApi({ 'GET /api/me': users.analyst, 'POST /api/import/trivy': { created: 12, skipped: 2 } })
    renderAt(<Import />, '/import')
    const u = userEvent.setup()
    await u.upload(await screen.findByLabelText(/trivy json report/i), new File(['{"SchemaVersion":2}'], 'scan.json', { type: 'application/json' }))
    await u.type(screen.getByLabelText(/asset name/i), 'billing')
    await u.click(screen.getByRole('button', { name: /import scan/i }))
    expect(await screen.findByText(/12 findings created, 2 already tracked/i)).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'POST')!.url).toBe('/api/import/trivy?asset=billing')
  })

  it('shows the CSV row error', async () => {
    mockApi({ 'GET /api/me': users.analyst, 'POST /api/import/csv': json({ error: 'row 3: severity must be Critical, High, Medium or Low' }, 400) })
    renderAt(<Import />, '/import')
    const u = userEvent.setup()
    await u.upload(await screen.findByLabelText(/csv file/i), new File(['title,asset,severity'], 'f.csv', { type: 'text/csv' }))
    await u.click(screen.getByRole('button', { name: /import csv/i }))
    expect(await screen.findByText(/row 3: severity must be/)).toBeInTheDocument()
  })
})
