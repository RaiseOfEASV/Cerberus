import { vi } from 'vitest'

type Handler = (url: string, init: RequestInit) => Response | Promise<Response>

/** Stubs fetch with a router: route key "METHOD /path" (path without query). */
export function mockApi(routes: Record<string, Handler | unknown>) {
  const calls: { method: string; url: string; body: unknown }[] = []
  const fn = vi.fn(async (url: string, init: RequestInit = {}) => {
    const method = (init.method ?? 'GET').toUpperCase()
    const path = url.split('?')[0]
    let body: unknown = init.body
    if (typeof body === 'string') {
      try {
        body = JSON.parse(body)
      } catch {
        /* raw */
      }
    }
    calls.push({ method, url, body })
    const key = `${method} ${path}`
    const h = routes[key] ?? Object.entries(routes).find(([k]) => new RegExp('^' + k.replace(/:\w+/g, '[^/]+') + '$').test(key))?.[1]
    if (h === undefined) return new Response(JSON.stringify({ error: 'no mock for ' + key }), { status: 500 })
    if (typeof h === 'function') return (h as Handler)(url, init)
    if (h instanceof Response) return h.clone()
    return json(h)
  })
  vi.stubGlobal('fetch', fn)
  return calls
}

export function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { AuthProvider } from './auth'
import type { Finding, User } from './types'

export const users: Record<string, User> = {
  analyst: { sub: 'an-1', name: 'Ana Analyst', email: '', role: 'analyst' },
  developer: { sub: 'dev-1', name: 'Dee Dev', email: '', role: 'developer' },
  pm: { sub: 'pm-1', name: 'Pat PM', email: '', role: 'pm' },
  auditor: { sub: 'au-1', name: 'Aud Auditor', email: '', role: 'auditor' },
  guest: { sub: 'gu-1', name: 'Pen Tester', email: '', role: 'guest' },
}

export function LocationProbe() {
  const l = useLocation()
  return <output data-testid="location">{l.pathname + l.search}</output>
}

/** Renders ui at path (matching routePattern) inside the real AuthProvider. */
export function renderAt(ui: ReactNode, path: string, routePattern = path.split('?')[0]) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <Routes>
          <Route path={routePattern} element={ui} />
          <Route path="*" element={<p>elsewhere</p>} />
        </Routes>
        <LocationProbe />
      </AuthProvider>
    </MemoryRouter>,
  )
}

export function finding(over: Partial<Finding> = {}): Finding {
  return {
    id: 7, ref: 'VM-AAAAAAAAAA', title: 'SQL injection in login', description: 'details', asset: 'quote-api',
    severity: 'High', status: 'New', source: 'internal', discoveredAt: '2026-11-16T10:00:00Z',
    createdAt: '2026-11-16T10:00:00Z', updatedAt: '2026-11-16T10:00:00Z', ...over,
  }
}
