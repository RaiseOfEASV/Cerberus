import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { finding, mockApi, renderAt, users } from '../test-utils'
import Findings from './Findings'

afterEach(() => vi.unstubAllGlobals())

describe('Findings list', () => {
  it('filters round-trip via the URL', async () => {
    const calls = mockApi({
      'GET /api/me': users.analyst,
      'GET /api/assignees': [],
      'GET /api/findings': { items: [finding()], total: 1 },
    })
    renderAt(<Findings />, '/findings?status=Triaged&q=sql')
    expect(await screen.findByText('SQL injection in login')).toBeInTheDocument()
    const first = calls.find((c) => c.url.startsWith('/api/findings'))!
    expect(first.url).toContain('status=Triaged')
    expect(first.url).toContain('q=sql')
    await userEvent.selectOptions(screen.getByLabelText(/severity/i), 'Critical')
    await waitFor(() => expect(screen.getByTestId('location').textContent).toContain('severity=Critical'))
    await waitFor(() => expect(calls.some((c) => c.url.includes('severity=Critical') && c.url.includes('status=Triaged'))).toBe(true))
  })

  it('links each row to its detail page and shows severity', async () => {
    mockApi({ 'GET /api/me': users.auditor, 'GET /api/assignees': [], 'GET /api/findings': { items: [finding({ severity: 'Critical' })], total: 1 } })
    renderAt(<Findings />, '/findings')
    const link = await screen.findByRole('link', { name: 'SQL injection in login' })
    expect(link.getAttribute('href')).toBe('/findings/7')
    expect(screen.getByText('Critical', { selector: '.badge' })).toHaveClass('sev-Critical')
  })

  it('shows an empty state', async () => {
    mockApi({ 'GET /api/me': users.auditor, 'GET /api/assignees': [], 'GET /api/findings': { items: [], total: 0 } })
    renderAt(<Findings />, '/findings')
    expect(await screen.findByText(/no findings match/i)).toBeInTheDocument()
  })
})
