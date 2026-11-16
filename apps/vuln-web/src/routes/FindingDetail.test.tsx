import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { finding, json, mockApi, renderAt, users } from '../test-utils'
import FindingDetail from './FindingDetail'

afterEach(() => vi.unstubAllGlobals())

function api(user: keyof typeof users, f = finding(), extra: Record<string, unknown> = {}) {
  return mockApi({
    'GET /api/me': users[user],
    'GET /api/findings/:id': f,
    'GET /api/findings/:id/links': [],
    'GET /api/findings/:id/attachments': [],
    'GET /api/findings/:id/history': [],
    'GET /api/assignees': [{ sub: 'dev-1', name: 'Dee Dev', role: 'developer' }],
    ...extra,
  })
}

describe('FindingDetail', () => {
  it('developer sees no edit controls on a finding assigned to someone else', async () => {
    api('developer', finding({ assigneeSub: 'dev-2', assigneeName: 'Dan', status: 'Triaged' }))
    renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    expect(await screen.findByText('SQL injection in login')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /edit/i })).toBeNull()
    expect(screen.queryByLabelText(/new status/i)).toBeNull()
    expect(screen.queryByLabelText(/assign to/i)).toBeNull()
    expect(screen.queryByLabelText(/attach a file/i)).toBeNull()
  })

  it('the assigned developer can move it on, but only to legal states', async () => {
    api('developer', finding({ assigneeSub: 'dev-1', status: 'Triaged' }))
    renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    const select = await screen.findByLabelText(/new status/i)
    const options = Array.from((select as HTMLSelectElement).options).map((o) => o.value).filter(Boolean)
    expect(options).toEqual(['In Progress'])
    expect(screen.queryByLabelText(/assign to/i)).toBeNull()
  })

  it('status control lists only legal next states for an analyst', async () => {
    const calls = api('analyst', finding({ status: 'New' }), {
      'POST /api/findings/:id/status': finding({ status: 'Triaged' }),
    })
    renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    const select = (await screen.findByLabelText(/new status/i)) as HTMLSelectElement
    expect(Array.from(select.options).map((o) => o.value).filter(Boolean)).toEqual(['Triaged', 'Accepted Risk'])
    await userEvent.selectOptions(select, 'Triaged')
    await userEvent.click(screen.getByRole('button', { name: /change status/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/findings/7/status')).toBe(true))
    expect(await screen.findByText('Triaged', { selector: '.status' })).toBeInTheDocument()
  })

  it('upload shows the server 400 message', async () => {
    api('analyst', finding(), {
      'POST /api/findings/:id/attachments': json({ error: 'file type text/html is not allowed (png, jpeg, pdf, plain text, json)' }, 400),
    })
    renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    const input = (await screen.findByLabelText(/attach a file/i)) as HTMLInputElement
    await userEvent.upload(input, new File(['<html>'], 'evil.png', { type: 'image/png' }))
    await userEvent.click(screen.getByRole('button', { name: /upload/i }))
    expect(await screen.findByText(/file type text\/html is not allowed/)).toBeInTheDocument()
  })

  it('auditor sees the history tab; analyst does not', async () => {
    api('auditor', finding(), {
      'GET /api/findings/:id/history': [{ id: 1, at: '2026-11-16T10:00:00Z', actorSub: 'an-1', actorRole: 'analyst', action: 'finding.create', details: {}, hash: 'ab' }],
    })
    renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    expect(await screen.findByText('finding.create')).toBeInTheDocument()
  })

  it('shows the description as text, never HTML', async () => {
    api('analyst', finding({ description: '<img src=x onerror=alert(1)>' }))
    const { container } = renderAt(<FindingDetail />, '/findings/7', '/findings/:id')
    expect(await screen.findByText('<img src=x onerror=alert(1)>')).toBeInTheDocument()
    expect(container.querySelector('img')).toBeNull()
  })
})
