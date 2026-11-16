import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import ErrorText, { message } from '../components/ErrorText'
import { severities, type SLA } from '../types'

// FIND-04 / PM role: SLA targets in days per severity. A finding's due date
// is set from these when it is triaged.
export default function AdminSLA() {
  const [sla, setSla] = useState<Record<string, string> | null>(null)
  const [error, setError] = useState('')
  const [done, setDone] = useState('')
  useEffect(() => {
    api.sla().then((s) => setSla(Object.fromEntries(severities.map((k) => [k, String(s[k])]))), (e) => setError(message(e)))
  }, [])
  async function save(e: FormEvent) {
    e.preventDefault()
    setError('')
    setDone('')
    const out = {} as SLA
    for (const k of severities) {
      const n = Number(sla![k])
      if (!Number.isInteger(n) || n < 1 || n > 365) {
        setError(`${k}: days must be a whole number from 1 to 365`)
        return
      }
      out[k] = n
    }
    try {
      await api.setSla(out)
      setDone('SLA targets saved. They apply to findings triaged from now on.')
    } catch (err) {
      setError(message(err))
    }
  }
  if (!sla) return <ErrorText error={error} />
  return (
    <form className="panel" onSubmit={save} noValidate>
      <h1>SLA targets</h1>
      <div className="row">
        {severities.map((k) => (
          <div key={k}>
            <label htmlFor={`sla-${k}`}>{k} (days)</label>
            <input id={`sla-${k}`} type="number" min={1} max={365} value={sla[k]} onChange={(e) => setSla({ ...sla, [k]: e.target.value })} />
          </div>
        ))}
      </div>
      <ErrorText error={error} />
      {done && <p className="success">{done}</p>}
      <button type="submit">Save</button>
    </form>
  )
}
