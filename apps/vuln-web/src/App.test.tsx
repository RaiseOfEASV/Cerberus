import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { json, LocationProbe, mockApi, users } from './test-utils'

afterEach(() => vi.unstubAllGlobals())

function at(path: string) {
  return render(<MemoryRouter initialEntries={[path]}><App /><LocationProbe /></MemoryRouter>)
}

describe('routing', () => {
  it('shows the product page to visitors', async () => {
    mockApi({ 'GET /api/me': json({ error: 'not logged in' }, 401) })
    at('/')
    expect(await screen.findByRole('heading', { level: 1, name: /nothing gets past the gate/i })).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /sign in/i })[0]).toHaveAttribute('href', '/login')
    expect(screen.getAllByRole('link', { name: /report a vulnerability/i })[0]).toHaveAttribute('href', '/report')
  })

  it('sends signed-in users from / straight to the app', async () => {
    mockApi({ 'GET /api/me': users.pm, 'GET /api/dashboard': { openBySeverity: {}, byStatus: {}, overdue: 0, openWithDue: 0, slaCompliancePct: 100, overdueList: [] }, 'GET /api/dashboard/trend': [] })
    at('/')
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByTestId('location').textContent).toBe('/app')
  })

  it('sends a visitor who opens an app page to the login page, keeping the destination', async () => {
    mockApi({ 'GET /api/me': json({ error: 'not logged in' }, 401) })
    at('/app/findings/7')
    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
    expect(screen.getByTestId('location').textContent).toBe('/login?return=%2Fapp%2Ffindings%2F7')
  })

  it('keeps the public report form reachable without an account', async () => {
    mockApi({ 'GET /api/me': json({ error: 'not logged in' }, 401) })
    at('/report')
    expect(await screen.findByRole('heading', { name: /report a security vulnerability/i })).toBeInTheDocument()
  })
})
