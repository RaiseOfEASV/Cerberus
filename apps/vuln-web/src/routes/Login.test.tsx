import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { json, mockApi } from '../test-utils'
import Login from './Login'

const assign = vi.fn()
beforeEach(() => {
  vi.stubGlobal('location', { ...window.location, assign })
})
afterEach(() => {
  vi.unstubAllGlobals()
  assign.mockReset()
})

function renderLogin(path = '/login?return=%2Fapp%2Ffindings%2F7') {
  return render(<MemoryRouter initialEntries={[path]}><Login /></MemoryRouter>)
}

async function signIn(user = 'amir', pass = 'secret-pass') {
  const u = userEvent.setup()
  await u.type(screen.getByLabelText(/username/i), user)
  await u.type(screen.getByLabelText(/password/i), pass)
  await u.click(screen.getByRole('button', { name: /^sign in$/i }))
}

describe('Login', () => {
  it('posts the credentials to vuln-api and continues to the return path', async () => {
    const calls = mockApi({ 'POST /auth/password': { sub: 'u1', name: 'Amir', email: '', role: 'pm' } })
    renderLogin()
    await signIn()
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/app/findings/7'))
    expect(calls[0]).toMatchObject({ method: 'POST', url: '/auth/password', body: { username: 'amir', password: 'secret-pass' } })
  })

  it('never redirects off-site', async () => {
    mockApi({ 'POST /auth/password': { sub: 'u1', name: 'A', email: '', role: 'pm' } })
    renderLogin('/login?return=%2F%2Fevil.example')
    await signIn()
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/app'))
  })

  it('shows a plain message for wrong credentials and stays on the page', async () => {
    mockApi({ 'POST /auth/password': json({ error: 'invalid username or password' }, 401) })
    renderLogin()
    await signIn('amir', 'nope')
    expect(await screen.findByText('Wrong username or password.')).toBeInTheDocument()
    expect(assign).not.toHaveBeenCalled()
  })

  it('explains rate limiting and a missing role', async () => {
    mockApi({ 'POST /auth/password': json({ error: 'x' }, 429) })
    renderLogin()
    await signIn()
    expect(await screen.findByText(/too many attempts/i)).toBeInTheDocument()
  })

  it('offers single sign-on and the public report form', () => {
    mockApi({})
    renderLogin()
    expect(screen.getByRole('link', { name: /single sign-on/i })).toHaveAttribute('href', '/auth/login?return=%2Fapp%2Ffindings%2F7')
    expect(screen.getByRole('link', { name: /report a vulnerability/i })).toHaveAttribute('href', '/report')
  })
})
