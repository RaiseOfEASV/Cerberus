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
