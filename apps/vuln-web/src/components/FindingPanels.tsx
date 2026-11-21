import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { api } from '../api'
import { useAuth } from '../auth'
import { nextStatuses } from '../permissions'
import type { Attachment, AuditEntry, Finding, Link as FLink, LinkKind, Person, Status } from '../types'
import ErrorText, { message } from './ErrorText'
import { fmtTime } from './SeverityBadge'

type OnChange = (f: Finding) => void

export function StatusControl({ f, onChange }: { f: Finding; onChange: OnChange }) {
  const { user, canOn } = useAuth()
  const options = user && canOn('change_status', f.assigneeSub) ? nextStatuses(user.role, f.status) : []
  const [to, setTo] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  if (options.length === 0) return null
  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      onChange(await api.changeStatus(f.id, to as Status, note))
      setTo('')
      setNote('')
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={submit}>
      <h2>Status</h2>
      <div className="row">
        <div>
          <label htmlFor="s-to">New status</label>
          <select id="s-to" required value={to} onChange={(e) => setTo(e.target.value)}>
            <option value="">Choose…</option>
            {options.map((s) => <option key={s}>{s}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="s-note">Note (optional)</label>
          <input id="s-note" maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} />
        </div>
      </div>
      <ErrorText error={error} />
      <p><button type="submit" disabled={!to}>Change status</button></p>
    </form>
  )
}

export function AssignControl({ f, onChange }: { f: Finding; onChange: OnChange }) {
  const { can } = useAuth()
  const [people, setPeople] = useState<Person[]>([])
  const [sub, setSub] = useState(f.assigneeSub ?? '')
  const [due, setDue] = useState(f.dueDate ? f.dueDate.slice(0, 10) : '')
  const [error, setError] = useState('')
  useEffect(() => {
    if (can('assign')) api.assignees().then(setPeople, () => setPeople([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  if (!can('assign')) return null
  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      const iso = due && due !== (f.dueDate ?? '').slice(0, 10) ? new Date(due + 'T23:59:59Z').toISOString() : undefined
      onChange(await api.assign(f.id, sub, iso))
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={submit}>
      <h2>Assignment and SLA</h2>
      <div className="row">
        <div>
          <label htmlFor="a-sub">Assign to</label>
          <select id="a-sub" value={sub} onChange={(e) => setSub(e.target.value)}>
            <option value="">Unassigned</option>
            {people.map((p) => <option key={p.sub} value={p.sub}>{p.name} ({p.role})</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="a-due">Due date</label>
          <input id="a-due" type="date" value={due} onChange={(e) => setDue(e.target.value)} />
        </div>
      </div>
      <ErrorText error={error} />
      <p><button type="submit">Assign</button></p>
    </form>
  )
}

export function Links({ f }: { f: Finding }) {
  const { canOn } = useAuth()
  const [links, setLinks] = useState<FLink[]>([])
  const [ref, setRef] = useState('')
  const [kind, setKind] = useState<LinkKind>('related')
  const [error, setError] = useState('')
  const load = () => api.links(f.id).then(setLinks, (e) => setError(message(e)))
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [f.id])
  const editable = canOn('link', f.assigneeSub)
  async function add(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      await api.addLink(f.id, ref.trim(), kind)
      setRef('')
      await load()
    } catch (err) {
      setError(message(err))
    }
  }
  async function remove(other: number) {
    try {
      await api.removeLink(f.id, other)
      await load()
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <section className="panel">
      <h2>Related findings</h2>
      {links.length === 0 ? <p>None.</p> : (
        <ul>
          {links.map((l) => (
            <li key={l.findingId}>
              {l.kind.replaceAll('_', ' ')}: <Link to={`/app/findings/${l.findingId}`}>{l.ref} - {l.title}</Link>{' '}
              {editable && <button className="link" onClick={() => remove(l.findingId)}>remove</button>}
            </li>
          ))}
        </ul>
      )}
      {editable && (
        <form className="row" onSubmit={add}>
          <div>
            <label htmlFor="l-ref">Link to (VM- reference)</label>
            <input id="l-ref" required value={ref} onChange={(e) => setRef(e.target.value)} />
          </div>
          <div>
            <label htmlFor="l-kind">Relation</label>
            <select id="l-kind" value={kind} onChange={(e) => setKind(e.target.value as LinkKind)}>
              <option value="related">related</option>
              <option value="duplicate">duplicate</option>
              <option value="same_root_cause">same root cause</option>
            </select>
          </div>
          <div><button type="submit">Add link</button></div>
        </form>
      )}
      <ErrorText error={error} />
    </section>
  )
}

export function Attachments({ f }: { f: Finding }) {
  const { canOn } = useAuth()
  const [list, setList] = useState<Attachment[]>([])
  const [file, setFile] = useState<File | null>(null)
  const [error, setError] = useState('')
  const load = () => api.attachments(f.id).then(setList, (e) => setError(message(e)))
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [f.id])
  async function upload(e: FormEvent) {
    e.preventDefault()
    if (!file) return
    setError('')
    try {
      await api.upload(f.id, file)
      setFile(null)
      await load()
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <section className="panel">
      <h2>Evidence</h2>
      {list.length === 0 ? <p>No attachments.</p> : (
        <ul>
          {list.map((a) => (
            <li key={a.id}>
              <a href={api.attachmentUrl(f.id, a.id)}>{a.name}</a> ({Math.ceil(a.size / 1024)} KiB, {a.contentType}, by {a.createdBy})
            </li>
          ))}
        </ul>
      )}
      {canOn('attach', f.assigneeSub) && (
        <form className="row" onSubmit={upload}>
          <div>
            <label htmlFor="att-file">Attach a file (png, jpeg, pdf, text, json - max 10 MiB)</label>
            <input id="att-file" type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
          </div>
          <div><button type="submit" disabled={!file}>Upload</button></div>
        </form>
      )}
      <ErrorText error={error} />
    </section>
  )
}

export function History({ f }: { f: Finding }) {
  const { can } = useAuth()
  const [rows, setRows] = useState<AuditEntry[]>([])
  useEffect(() => {
    if (can('view_audit')) api.history(f.id).then(setRows, () => setRows([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [f.id, f.updatedAt])
  if (!can('view_audit')) return null
  return (
    <section className="panel">
      <h2>History</h2>
      <table>
        <thead><tr><th>When</th><th>Who</th><th>Action</th><th>Details</th></tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id}>
              <td>{fmtTime(r.at)}</td>
              <td>{r.actorSub} ({r.actorRole})</td>
              <td>{r.action}</td>
              <td><code>{JSON.stringify(r.details)}</code></td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
