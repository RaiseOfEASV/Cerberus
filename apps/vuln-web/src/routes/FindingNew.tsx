import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { api } from '../api'
import ErrorText, { message } from '../components/ErrorText'
import { severities } from '../types'

// FIND-01: analysts and PMs record a finding by hand.
export default function FindingNew() {
  const nav = useNavigate()
  const [f, setF] = useState({ title: '', asset: '', severity: 'High', cvss: '', cve: '', description: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const created = await api.createFinding({
        title: f.title, asset: f.asset, severity: f.severity, description: f.description,
        ...(f.cve ? { cve: f.cve } : {}),
        ...(f.cvss ? { cvss: Number(f.cvss) } : {}),
      })
      nav(`/findings/${created.id}`)
    } catch (err) {
      setError(message(err))
      setBusy(false)
    }
  }

  return (
    <form className="panel" onSubmit={submit}>
      <h1>New finding</h1>
      <label htmlFor="n-title">Title</label>
      <input id="n-title" required minLength={3} maxLength={200} value={f.title} onChange={set('title')} />
      <div className="row">
        <div>
          <label htmlFor="n-asset">Asset / component</label>
          <input id="n-asset" required maxLength={200} value={f.asset} onChange={set('asset')} />
        </div>
        <div>
          <label htmlFor="n-severity">Severity</label>
          <select id="n-severity" value={f.severity} onChange={set('severity')}>
            {severities.map((s) => <option key={s}>{s}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="n-cvss">CVSS score (optional)</label>
          <input id="n-cvss" type="number" min={0} max={10} step={0.1} value={f.cvss} onChange={set('cvss')} />
        </div>
        <div>
          <label htmlFor="n-cve">CVE (optional)</label>
          <input id="n-cve" placeholder="CVE-2026-12345" value={f.cve} onChange={set('cve')} />
        </div>
      </div>
      <label htmlFor="n-desc">Description (stored encrypted)</label>
      <textarea id="n-desc" maxLength={20000} value={f.description} onChange={set('description')} />
      <ErrorText error={error} />
      <button type="submit" disabled={busy}>Create finding</button>
    </form>
  )
}
