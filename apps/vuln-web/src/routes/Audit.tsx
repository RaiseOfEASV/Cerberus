import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { api } from '../api'
import ErrorText, { message } from '../components/ErrorText'
import { fmtTime } from '../components/SeverityBadge'
import type { AuditEntry, VerifyResult } from '../types'

// AUDIT-02 (read the trail) and AUDIT-03 (verify its hash chain).
export default function Audit() {
  const [rows, setRows] = useState<AuditEntry[]>([])
  const [q, setQ] = useState({ finding: '', actor: '', from: '', to: '' })
  const [verify, setVerify] = useState<VerifyResult | null>(null)
  const [error, setError] = useState('')
  const load = (p = q) =>
    api.audit({
      finding: p.finding, actor: p.actor,
      from: p.from ? new Date(p.from + 'T00:00:00Z').toISOString() : '',
      to: p.to ? new Date(p.to + 'T00:00:00Z').toISOString() : '',
    }).then(setRows, (e) => setError(message(e)))
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    load()
  }
  async function check() {
    setError('')
    try {
      setVerify(await api.verifyAudit())
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <>
      <h1>Audit trail</h1>
      <section className="panel">
        <p>Every change is chained with SHA-256 to the entry before it, so editing or deleting history breaks the chain.</p>
        <button onClick={check}>Verify integrity</button>
        {verify && (verify.ok
          ? <p className="success">All {verify.checked} entries verified - the trail is intact.</p>
          : <p className="error" role="alert">Tampering detected at entry #{verify.firstBadId} ({verify.checked} checked).</p>)}
      </section>
      <form className="panel row" onSubmit={submit}>
        <div><label htmlFor="q-f">Finding id</label><input id="q-f" inputMode="numeric" value={q.finding} onChange={(e) => setQ({ ...q, finding: e.target.value })} /></div>
        <div><label htmlFor="q-a">Actor (user id)</label><input id="q-a" value={q.actor} onChange={(e) => setQ({ ...q, actor: e.target.value })} /></div>
        <div><label htmlFor="q-from">From</label><input id="q-from" type="date" value={q.from} onChange={(e) => setQ({ ...q, from: e.target.value })} /></div>
        <div><label htmlFor="q-to">To</label><input id="q-to" type="date" value={q.to} onChange={(e) => setQ({ ...q, to: e.target.value })} /></div>
        <div><button type="submit">Filter</button></div>
      </form>
      <ErrorText error={error} />
      <table>
        <thead><tr><th>#</th><th>When</th><th>Who</th><th>Action</th><th>Finding</th><th>Details</th></tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id}>
              <td>{r.id}</td>
              <td>{fmtTime(r.at)}</td>
              <td>{r.actorSub} ({r.actorRole})</td>
              <td>{r.action}</td>
              <td>{r.findingId ? <Link to={`/app/findings/${r.findingId}`}>{r.findingId}</Link> : '-'}</td>
              <td><code>{JSON.stringify(r.details)}</code></td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
