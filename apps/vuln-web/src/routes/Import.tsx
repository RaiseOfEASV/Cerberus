import { useState, type FormEvent } from 'react'
import { api } from '../api'
import ErrorText, { message } from '../components/ErrorText'
import type { ImportResult } from '../types'

// Intake from scanners (FIND-09: Trivy image or SBOM scan JSON) and from
// spreadsheets (FIND-08: CSV). Re-importing never duplicates open findings.
export default function Import() {
  return (
    <>
      <h1>Import findings</h1>
      <TrivyForm />
      <CsvForm />
    </>
  )
}

function Result({ r }: { r: ImportResult | null }) {
  return r ? <p className="success" role="status">{r.created} findings created, {r.skipped} already tracked (skipped).</p> : null
}

function TrivyForm() {
  const [file, setFile] = useState<File | null>(null)
  const [asset, setAsset] = useState('')
  const [res, setRes] = useState<ImportResult | null>(null)
  const [error, setError] = useState('')
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!file) return
    setError('')
    setRes(null)
    try {
      setRes(await api.importTrivy(file, asset.trim()))
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={submit}>
      <h2>Vulnerability scan (Trivy)</h2>
      <p>Upload the output of <code>trivy image --format json</code> or <code>trivy sbom --format json</code> (max 5 MiB).</p>
      <label htmlFor="t-file">Trivy JSON report</label>
      <input id="t-file" type="file" accept="application/json,.json" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
      <label htmlFor="t-asset">Asset name (optional - overrides the scan's artifact name)</label>
      <input id="t-asset" value={asset} onChange={(e) => setAsset(e.target.value)} />
      <ErrorText error={error} />
      <Result r={res} />
      <p><button type="submit" disabled={!file}>Import scan</button></p>
    </form>
  )
}

function CsvForm() {
  const [file, setFile] = useState<File | null>(null)
  const [res, setRes] = useState<ImportResult | null>(null)
  const [error, setError] = useState('')
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!file) return
    setError('')
    setRes(null)
    try {
      setRes(await api.importCsv(file))
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <form className="panel" onSubmit={submit}>
      <h2>Spreadsheet (CSV)</h2>
      <p>Header row with <code>title,asset,severity</code> and optionally <code>cvss,cve,description,discovered_at</code>. Nothing is imported if any row is invalid.</p>
      <label htmlFor="c-file">CSV file</label>
      <input id="c-file" type="file" accept="text/csv,.csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
      <ErrorText error={error} />
      <Result r={res} />
      <p><button type="submit" disabled={!file}>Import CSV</button></p>
    </form>
  )
}
