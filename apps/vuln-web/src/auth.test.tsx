import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider, useAuth } from './auth'

function Probe() {
  const { user, loading, can, canOn } = useAuth()
  if (loading) return <p>loading</p>
  return (
    <ul>
      <li>user:{user?.name ?? 'none'}</li>
      <li>assign:{String(can('assign'))}</li>
      <li>audit:{String(can('view_audit'))}</li>
      <li>edit-own:{String(canOn('edit', 'dev-1'))}</li>
      <li>edit-other:{String(canOn('edit', 'dev-2'))}</li>
    </ul>
  )
}

afterEach(() => vi.unstubAllGlobals())

function me(user: object | null, status = 200) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(user ? JSON.stringify(user) : '{"error":"x"}', { status })))
}

describe('AuthProvider', () => {
  it('exposes the role-based can() for a developer', async () => {
    me({ sub: 'dev-1', name: 'Dee', email: '', role: 'developer' })
    render(<AuthProvider><Probe /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('user:Dee')).toBeInTheDocument())
    expect(screen.getByText('assign:false')).toBeInTheDocument()
    expect(screen.getByText('audit:false')).toBeInTheDocument()
    expect(screen.getByText('edit-own:true')).toBeInTheDocument()
    expect(screen.getByText('edit-other:false')).toBeInTheDocument()
  })

  it('a PM may assign and read the audit trail', async () => {
    me({ sub: 'pm-1', name: 'Pat', email: '', role: 'pm' })
    render(<AuthProvider><Probe /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('assign:true')).toBeInTheDocument())
    expect(screen.getByText('audit:true')).toBeInTheDocument()
    expect(screen.getByText('edit-other:true')).toBeInTheDocument()
  })
})
