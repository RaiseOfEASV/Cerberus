import { useEffect, useState, type FormEvent } from 'react'
import { useParams } from 'react-router'
import { api, type FindingPatch } from '../api'
import { useAuth } from '../auth'
import ErrorText, { message } from '../components/ErrorText'
import { AssignControl, Attachments, History, Links, StatusControl } from '../components/FindingPanels'
import SeverityBadge, { fmtDate, fmtTime } from '../components/SeverityBadge'
import { severities, type Finding } from '../types'

export default function FindingDetail() {
  const id = Number(useParams().id)
  const { canOn } = useAuth()
  const [f, setF] = useState<Finding | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  useEffect(() => {
    api.getFinding(id).then(setF, (e) => setError(message(e)))
  }, [id])
  if (error && !f) return <ErrorText error={error} />
  if (!f) return <p className="notice">Loading…</p>

  return (
    <>
      <p><code>{f.ref}</code> · <SeverityBadge severity={f.severity} /> · <span className="status">{f.status}</span></p>
      <h1>{f.title}</h1>
      {editing ? (
        <EditForm f={f} onDone={(nf) => { if (nf) setF(nf); setEditing(false) }} />
      ) : (
        <section className="panel">
          <dl className="grid">
            <div><dt>Asset</dt><dd>{f.asset}</dd></div>
            <div><dt>CVSS</dt><dd>{f.cvss ?? '-'}</dd></div>
            <div><dt>CVE</dt><dd>{f.cve || '-'}</dd></div>
            <div><dt>Source</dt><dd>{f.source}</dd></div>
            <div><dt>Discovered</dt><dd>{fmtDate(f.discoveredAt)}</dd></div>
            <div><dt>Assignee</dt><dd>{f.assigneeName || 'Unassigned'}</dd></div>
            <div><dt>Due</dt><dd>{fmtDate(f.dueDate)}</dd></div>
            <div><dt>Last change</dt><dd>{fmtTime(f.updatedAt)}</dd></div>
            {f.reporterEmail && <div><dt>Reporter contact</dt><dd>{f.reporterEmail}</dd></div>}
          </dl>
          <h2>Description</h2>
          <pre className="wrap">{f.description || '(none)'}</pre>
          {canOn('edit', f.assigneeSub) && <button className="secondary" onClick={() => setEditing(true)}>Edit details</button>}
        </section>
      )}
      <StatusControl f={f} onChange={setF} />
      <AssignControl f={f} onChange={setF} />
      <Links f={f} />
      <Attachments f={f} />
      <History f={f} />
    </>
  )
}

function EditForm({ f, onDone }: { f: Finding; onDone: (f?: Finding) => void }) {
  const [v, setV] = useState({
    title: f.title, asset: f.asset, severity: f.severity as string, cvss: f.cvss?.toString() ?? '', cve: f.cve ?? '', description: f.description ?? '',
  })
  const [error, setError] = useState('')
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV({ ...v, [k]: e.target.value })
  async function save(e: FormEvent) {
    e.preventDefault()
    const p: FindingPatch = {}
    if (v.title !== f.title) p.title = v.title
    if (v.asset !== f.asset) p.asset = v.asset
    if (v.severity !== f.severity) p.severity = v.severity
    if (v.cve !== (f.cve ?? '')) p.cve = v.cve
    if (v.description !== (f.description ?? '')) p.description = v.description
    if (v.cvss !== (f.cvss?.toString() ?? '')) p.cvss = v.cvss === '' ? null : Number(v.cvss)
    try {
      onDone(Object.keys(p).length ? await api.patchFinding(f.id, p) : undefined)
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={save}>
      <label htmlFor="e-title">Title</label>
      <input id="e-title" required value={v.title} onChange={set('title')} />
      <div className="row">
        <div><label htmlFor="e-asset">Asset</label><input id="e-asset" required value={v.asset} onChange={set('asset')} /></div>
        <div>
          <label htmlFor="e-sev">Severity</label>
          <select id="e-sev" value={v.severity} onChange={set('severity')}>{severities.map((s) => <option key={s}>{s}</option>)}</select>
        </div>
        <div><label htmlFor="e-cvss">CVSS</label><input id="e-cvss" type="number" min={0} max={10} step={0.1} value={v.cvss} onChange={set('cvss')} /></div>
        <div><label htmlFor="e-cve">CVE</label><input id="e-cve" value={v.cve} onChange={set('cve')} /></div>
      </div>
      <label htmlFor="e-desc">Description</label>
      <textarea id="e-desc" value={v.description} onChange={set('description')} />
      <ErrorText error={error} />
      <p className="row"><button type="submit">Save</button><button type="button" className="secondary" onClick={() => onDone()}>Cancel</button></p>
    </form>
  )
}
