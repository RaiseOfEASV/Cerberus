import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { api } from '../api'
import { useAuth } from '../auth'
import { BarChart, TrendChart } from '../components/Charts'
import ErrorText, { message } from '../components/ErrorText'
import { fmtDate } from '../components/SeverityBadge'
import { severities, statuses, type Dashboard as D, type WeekCount } from '../types'

// FR-RPT-01 (open by severity, by status), FR-RPT-04 (SLA compliance),
// FR-RPT-05 (trend), FR-RPT-02/03 (signed report export).
export default function Dashboard() {
  const { can } = useAuth()
  const [d, setD] = useState<D | null>(null)
  const [weeks, setWeeks] = useState<WeekCount[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    api.dashboard().then(setD, (e) => setError(message(e)))
    api.trend(12).then(setWeeks, () => setWeeks([]))
  }, [])
  if (error) return <ErrorText error={error} />
  if (!d) return <p className="notice">Loading…</p>
  return (
    <>
      <h1>Dashboard</h1>
      <div className="grid">
        <BarChart title="Open findings by severity" data={severities.map((s) => [s, d.openBySeverity[s] ?? 0])}
          colors={Object.fromEntries(severities.map((s) => [s, `sev-${s}`]))} />
        <BarChart title="Findings by status" data={statuses.map((s) => [s, d.byStatus[s] ?? 0])} />
        <section className="panel">
          <h2>SLA compliance</h2>
          <p className="stat">{d.slaCompliancePct}%</p>
          <p>{d.overdue} of {d.openWithDue} open findings with a due date are overdue.</p>
          {d.overdueList.length > 0 && (
            <ul>
              {d.overdueList.map((f) => (
                <li key={f.id}><Link to={`/findings/${f.id}`}>{f.title}</Link> - due {fmtDate(f.dueDate)}</li>
              ))}
            </ul>
          )}
          <p><Link to="/findings?overdue=true">All overdue findings</Link></p>
        </section>
      </div>
      {weeks.length > 0 && <TrendChart title="Opened vs closed per week" weeks={weeks} />}
      {can('export') && <ReportExport />}
    </>
  )
}

function ReportExport() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [asset, setAsset] = useState('')
  const [done, setDone] = useState('')
  const [error, setError] = useState('')
  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setDone('')
    try {
      const r = await api.report(from, to, asset)
      if (typeof URL.createObjectURL === 'function') {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(new Blob([JSON.stringify(r)], { type: 'application/json' }))
        a.download = `report-${from}-${to}.json`
        a.click()
        URL.revokeObjectURL(a.href)
      }
      setDone(`Report downloaded - signed with key version ${r.keyVersion}.`)
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={submit}>
      <h2>Export a signed report</h2>
      <p>Findings discovered in a period, as JSON signed with Ed25519 (Vault Transit). Check it later with
        <code> vuln-verify-report report.json public-key.b64</code>; any change after export is detected.</p>
      <div className="row">
        <div><label htmlFor="r-from">From</label><input id="r-from" type="date" required value={from} onChange={(e) => setFrom(e.target.value)} /></div>
        <div><label htmlFor="r-to">To (exclusive)</label><input id="r-to" type="date" required value={to} onChange={(e) => setTo(e.target.value)} /></div>
        <div><label htmlFor="r-asset">Asset (optional)</label><input id="r-asset" value={asset} onChange={(e) => setAsset(e.target.value)} /></div>
        <div><button type="submit">Download signed report</button></div>
      </div>
      {done && <p className="success">{done}</p>}
      <ErrorText error={error} />
    </form>
  )
}
