import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { finding, json, mockApi, renderAt, users } from '../test-utils'
import FindingNew from './FindingNew'

afterEach(() => vi.unstubAllGlobals())

it('creates a finding and opens it', async () => {
  const calls = mockApi({ 'GET /api/me': users.analyst, 'POST /api/findings': json(finding({ id: 42 }), 201) })
  renderAt(<FindingNew />, '/findings/new')
  const u = userEvent.setup()
  await u.type(await screen.findByLabelText(/^title/i), 'Open redirect')
  await u.type(screen.getByLabelText(/asset/i), 'login')
  await u.selectOptions(screen.getByLabelText(/severity/i), 'Medium')
  await u.type(screen.getByLabelText(/cvss/i), '6.1')
  await u.click(screen.getByRole('button', { name: /create/i }))
  await waitFor(() => expect(screen.getByTestId('location').textContent).toBe('/app/findings/42'))
  expect(calls.find((c) => c.method === 'POST')!.body).toMatchObject({ title: 'Open redirect', asset: 'login', severity: 'Medium', cvss: 6.1 })
})

it('shows the server validation error', async () => {
  mockApi({ 'GET /api/me': users.analyst, 'POST /api/findings': json({ error: 'cve must look like CVE-2026-12345' }, 400) })
  renderAt(<FindingNew />, '/findings/new')
  const u = userEvent.setup()
  await u.type(await screen.findByLabelText(/^title/i), 'Bad cve')
  await u.type(screen.getByLabelText(/asset/i), 'x')
  await u.type(screen.getByLabelText(/^cve/i), 'CVE-1')
  await u.click(screen.getByRole('button', { name: /create/i }))
  expect(await screen.findByText('cve must look like CVE-2026-12345')).toBeInTheDocument()
})
