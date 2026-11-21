import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { json, mockApi } from '../test-utils'
import Report from './Report'

afterEach(() => vi.unstubAllGlobals())

async function fill(title = 'IDOR in order history') {
  const u = userEvent.setup()
  await u.type(screen.getByLabelText(/title/i), title)
  await u.type(screen.getByLabelText(/affected system/i), 'shop')
  await u.type(screen.getByLabelText(/what did you find/i), 'Changing the id shows other orders.')
  await u.type(screen.getByLabelText(/your e-mail/i), 'r@example.org')
  await u.click(screen.getByRole('button', { name: /submit report/i }))
}

describe('Report (public)', () => {
  it('submits and shows the reference', async () => {
    const calls = mockApi({ 'POST /api/public/reports': json({ reference: 'VM-ABC123DEF4', message: 'Thank you.' }, 201) })
    render(<MemoryRouter><Report /></MemoryRouter>)
    await fill()
    expect(await screen.findByText('VM-ABC123DEF4')).toBeInTheDocument()
    expect(calls[0].body).toEqual({
      title: 'IDOR in order history', asset: 'shop', description: 'Changing the id shows other orders.',
      contactEmail: 'r@example.org', website: '',
    })
  })

  it('shows the rate-limit message on 429', async () => {
    mockApi({ 'POST /api/public/reports': json({ error: 'too many' }, 429) })
    render(<MemoryRouter><Report /></MemoryRouter>)
    await fill()
    expect(await screen.findByText(/too many reports from your network/i)).toBeInTheDocument()
  })

  it('shows a server validation error', async () => {
    mockApi({ 'POST /api/public/reports': json({ error: 'contactEmail must be a plain e-mail address' }, 400) })
    render(<MemoryRouter><Report /></MemoryRouter>)
    await fill()
    expect(await screen.findByText('contactEmail must be a plain e-mail address')).toBeInTheDocument()
  })

  it('renders user input as text', async () => {
    mockApi({ 'POST /api/public/reports': json({ reference: 'VM-XSSXSSXSS0', message: 'Thank you.' }, 201) })
    const { container } = render(<MemoryRouter><Report /></MemoryRouter>)
    await fill('<img src=x onerror=alert(1)>')
    await screen.findByText('VM-XSSXSSXSS0')
    // The only image allowed is the logo; the injected <img src=x> must stay text.
    expect(container.querySelector('img:not(.brand-mark)')).toBeNull()
    expect(screen.getByText(/<img src=x onerror=alert\(1\)>/)).toBeInTheDocument()
  })

  it('keeps the honeypot out of sight and out of the tab order', () => {
    mockApi({})
    render(<MemoryRouter><Report /></MemoryRouter>)
    const hp = document.querySelector('input[name="website"]') as HTMLInputElement
    expect(hp).not.toBeNull()
    expect(hp.tabIndex).toBe(-1)
    expect(hp.autocomplete).toBe('off')
    expect(hp.closest('.visually-hidden')).not.toBeNull()
  })
})
