import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, request, setUnauthorizedHandler } from './api'

const fetchMock = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState({}, '', '/findings?status=New')
})
afterEach(() => {
  vi.unstubAllGlobals()
  fetchMock.mockReset()
})

function reply(status: number, body?: unknown, contentType = 'application/json') {
  fetchMock.mockResolvedValueOnce(
    new Response(body === undefined ? null : typeof body === 'string' ? body : JSON.stringify(body), {
      status,
      headers: { 'Content-Type': contentType },
    }),
  )
}

describe('request', () => {
  it('sends same-origin JSON and parses the reply', async () => {
    reply(200, { ok: true })
    const out = await request<{ ok: boolean }>('POST', '/api/findings', { title: 'x' })
    expect(out).toEqual({ ok: true })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/findings')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('same-origin')
    expect(init.headers['Content-Type']).toBe('application/json')
    expect(init.body).toBe('{"title":"x"}')
  })

  it('redirects to login with the encoded current path on 401', async () => {
    const onUnauth = vi.fn()
    setUnauthorizedHandler(onUnauth)
    reply(401, { error: 'not logged in' })
    await expect(request('GET', '/api/me')).rejects.toBeInstanceOf(ApiError)
    expect(onUnauth).toHaveBeenCalledWith('/login?return=%2Ffindings%3Fstatus%3DNew')
  })

  it('turns an error reply into ApiError with the server message', async () => {
    reply(409, { error: 'illegal status transition: New -> Remediated' })
    const err = await request<never>('POST', '/api/findings/1/status', { status: 'Remediated' }).catch((e: ApiError) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(409)
    expect(err.message).toBe('illegal status transition: New -> Remediated')
  })

  it('copes with a non-JSON error body', async () => {
    reply(502, '<html>Bad gateway</html>', 'text/html')
    const err = await request<never>('GET', '/api/dashboard').catch((e: ApiError) => e)
    expect(err.status).toBe(502)
    expect(err.message).toMatch(/502/)
  })

  it('returns undefined for 204', async () => {
    reply(204)
    await expect(request('DELETE', '/api/findings/1/links/2')).resolves.toBeUndefined()
  })

  it('sends FormData without forcing a JSON content type', async () => {
    reply(201, { id: 1 })
    const fd = new FormData()
    fd.append('file', new Blob(['x']), 'x.txt')
    await request('POST', '/api/findings/1/attachments', fd)
    const init = fetchMock.mock.calls[0][1]
    expect(init.body).toBe(fd)
    expect(init.headers['Content-Type']).toBeUndefined()
  })
})
