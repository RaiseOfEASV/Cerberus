import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api } from '../api'
import { useAuth } from '../auth'
import ErrorText, { message } from '../components/ErrorText'
import SeverityBadge, { fmtDate } from '../components/SeverityBadge'
import { severities, statuses, type FindingFilter, type FindingList, type Person, type Severity, type Status } from '../types'

const PAGE = 50

// FIND-05: filter and search by status, severity, assignee and asset. The
// filters live in the URL, so a filtered view can be bookmarked or shared.
export default function Findings() {
  const { can } = useAuth()
  const [params, setParams] = useSearchParams()
  const [data, setData] = useState<FindingList | null>(null)
  const [people, setPeople] = useState<Person[]>([])
  const [error, setError] = useState('')
  const filter: FindingFilter = {
    status: params.getAll('status') as Status[],
    severity: params.getAll('severity') as Severity[],
    assignee: params.get('assignee') ?? '',
    asset: params.get('asset') ?? '',
    q: params.get('q') ?? '',
    overdue: params.get('overdue') === 'true',
    limit: PAGE,
    offset: Number(params.get('offset') ?? 0) || 0,
  }
  const key = params.toString()

  useEffect(() => {
    api.listFindings(filter).then(setData, (e) => setError(message(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  useEffect(() => {
    api.assignees().then(setPeople, () => setPeople([]))
  }, [])

  function update(name: string, value: string) {
    const next = new URLSearchParams(params)
    next.delete(name)
    next.delete('offset')
    if (value) next.set(name, value)
    setParams(next)
  }

  return (
    <>
      <h1>Findings</h1>
      <section className="panel row" aria-label="Filters">
        <div>
          <label htmlFor="f-q">Search</label>
          <input id="f-q" type="search" value={filter.q} placeholder="Title, asset or VM- reference" onChange={(e) => update('q', e.target.value)} />
        </div>
        <div>
          <label htmlFor="f-status">Status</label>
          <select id="f-status" value={filter.status?.[0] ?? ''} onChange={(e) => update('status', e.target.value)}>
            <option value="">Any</option>
            {statuses.map((s) => <option key={s}>{s}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="f-severity">Severity</label>
          <select id="f-severity" value={filter.severity?.[0] ?? ''} onChange={(e) => update('severity', e.target.value)}>
            <option value="">Any</option>
            {severities.map((s) => <option key={s}>{s}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="f-assignee">Assignee</label>
          <select id="f-assignee" value={filter.assignee} onChange={(e) => update('assignee', e.target.value)}>
            <option value="">Anyone</option>
            {people.map((p) => <option key={p.sub} value={p.sub}>{p.name}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="f-asset">Asset</label>
          <input id="f-asset" value={filter.asset} onChange={(e) => update('asset', e.target.value)} />
        </div>
        <div>
          <label htmlFor="f-overdue">
            <input id="f-overdue" type="checkbox" style={{ width: 'auto' }} checked={filter.overdue} onChange={(e) => update('overdue', e.target.checked ? 'true' : '')} /> Overdue only
          </label>
        </div>
      </section>
      <ErrorText error={error} />
      {can('export') && <p><a href={api.exportCsvUrl({ ...filter, limit: undefined, offset: undefined })}>Export these findings as CSV</a></p>}
      {data && data.items.length === 0 && <p className="notice">No findings match these filters.</p>}
      {data && data.items.length > 0 && (
        <>
          <table>
            <thead>
              <tr><th>Ref</th><th>Title</th><th>Asset</th><th>Severity</th><th>Status</th><th>Assignee</th><th>Due</th></tr>
            </thead>
            <tbody>
              {data.items.map((f) => (
                <tr key={f.id}>
                  <td><code>{f.ref}</code></td>
                  <td><Link to={`/app/findings/${f.id}`}>{f.title}</Link></td>
                  <td>{f.asset}</td>
                  <td><SeverityBadge severity={f.severity} /></td>
                  <td>{f.status}</td>
                  <td>{f.assigneeName || '-'}</td>
                  <td>{fmtDate(f.dueDate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="row" style={{ alignItems: 'center' }}>
            <span>{data.total} finding{data.total === 1 ? '' : 's'}</span>
            <button className="secondary" disabled={filter.offset === 0} onClick={() => update('offset', String(Math.max(0, (filter.offset ?? 0) - PAGE)))}>Previous</button>
            <button className="secondary" disabled={(filter.offset ?? 0) + PAGE >= data.total} onClick={() => update('offset', String((filter.offset ?? 0) + PAGE))}>Next</button>
          </p>
        </>
      )}
    </>
  )
}
