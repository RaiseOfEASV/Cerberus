import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { api, ApiError } from '../api'
import Icon from '../components/Icons'

// AUTH-03 / FIND-02: anyone may report a vulnerability - no account needed.
// The acknowledgement shows the reference the reporter can quote later.
export default function Report() {
  const [f, setF] = useState({ title: '', asset: '', description: '', contactEmail: '', website: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState<{ reference: string; title: string } | null>(null)
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const r = await api.publicReport(f)
      setDone({ reference: r.reference, title: f.title })
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError('Too many reports from your network, try again later.')
      else setError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="site">
    <header className="site-nav">
      <Link to="/" className="brand"><span className="brand-mark"><Icon name="shield" size={18} /></span> VulnMan</Link>
      <nav><Link to="/login" className="button small">Sign in</Link></nav>
    </header>
    <main className="public">
      <h1>Report a security vulnerability</h1>
      {done ? (
        <section className="panel" aria-live="polite">
          <h2>Thank you - your report has been received</h2>
          <p>Your reference is <strong>{done.reference}</strong>. Please quote it in any follow-up.</p>
          <p>Report: <em>{done.title}</em></p>
          <p>Our security team will triage it and contact you at the address you gave.</p>
        </section>
      ) : (
        <form className="panel" onSubmit={submit}>
          <p>Found a vulnerability in one of our systems? Tell us here. Please don't include real customer data.</p>
          <label htmlFor="title">Title</label>
          <input id="title" required minLength={3} maxLength={200} value={f.title} onChange={set('title')} />
          <label htmlFor="asset">Affected system</label>
          <input id="asset" required maxLength={200} value={f.asset} onChange={set('asset')} placeholder="e.g. cerberoast.site" />
          <label htmlFor="description">What did you find, and how can we reproduce it?</label>
          <textarea id="description" maxLength={20000} value={f.description} onChange={set('description')} />
          <label htmlFor="contactEmail">Your e-mail (only used to follow up)</label>
          <input id="contactEmail" type="email" required maxLength={254} value={f.contactEmail} onChange={set('contactEmail')} />
          <div className="visually-hidden" aria-hidden="true">
            <label htmlFor="website">Leave this field empty</label>
            <input id="website" name="website" tabIndex={-1} autoComplete="off" value={f.website} onChange={set('website')} />
          </div>
          {error && <p className="error" role="alert">{error}</p>}
          <button type="submit" disabled={busy}>{busy ? 'Sending…' : 'Submit report'}</button>
        </form>
      )}
    </main>
    </div>
  )
}
