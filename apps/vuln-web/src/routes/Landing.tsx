import { Link } from 'react-router'

// The lifecycle a finding really moves through (the app's own statuses).
const lifecycle = [
  { status: 'New', text: 'Reported by a researcher, a scanner or your own team.' },
  { status: 'Triaged', text: 'An analyst confirms severity and scope. The SLA clock starts.' },
  { status: 'In Progress', text: 'The assigned developer works on the fix.' },
  { status: 'Remediated', text: 'Fixed, recorded, and visible to auditors.' },
]

// Cerberus had three heads and let nothing past the gate. Each head keeps
// one watch; the copy groups the app's real features under them.
const heads = [
  {
    title: 'The first head watches the gate',
    text: 'Every report comes through one queue: findings your team enters, the public disclosure form, Trivy image and SBOM scans, and CSV imports. Duplicates are merged on the way in.',
  },
  {
    title: 'The second head watches the clock',
    text: 'Assign an owner and the due date follows from the SLA for that severity. The dashboard shows what is open, what is overdue and how compliance is trending.',
  },
  {
    title: 'The third head watches the record',
    text: 'Sensitive fields are encrypted with keys held in Vault. Every change goes into a hash-chained audit log, and exported reports are signed so anyone can verify them.',
  },
]

function FindingSpecimen() {
  return (
    <figure className="specimen" aria-label="An example finding record">
      <div className="specimen-head">
        <code>VM-0142</code>
        <span className="badge sev-Critical">Critical</span>
      </div>
      <p className="specimen-title">SQL injection in invoice search</p>
      <dl className="specimen-fields">
        <div><dt>Asset</dt><dd>billing-api</dd></div>
        <div><dt>CVSS</dt><dd><code>9.1</code></dd></div>
        <div><dt>Assignee</dt><dd>Developer</dd></div>
        <div><dt>Status</dt><dd>In Progress</dd></div>
      </dl>
      <div className="sla">
        <div className="sla-label"><span>SLA, 7 days for critical</span><strong>2 days left</strong></div>
        <div className="sla-track"><span className="sla-fill" /></div>
      </div>
      <ol className="chain">
        <li><code>3f9a…c21e</code> Reported through the public form</li>
        <li><code>b07d…4a90</code> Severity set to Critical by an analyst</li>
        <li><code>e51c…08f3</code> Assigned to a developer</li>
      </ol>
    </figure>
  )
}

export default function Landing() {
  return (
    <div className="site">
      <header className="site-nav">
        <Link to="/" className="brand"><img className="brand-mark" src="/favicon.svg" alt="" width={30} height={30} /> Cerberoast</Link>
        <nav aria-label="Site">
          <Link to="/report">Report a vulnerability</Link>
          <Link to="/login" className="button small">Sign in</Link>
        </nav>
      </header>

      <section className="hero">
        <div className="hero-copy">
          <h1>Nothing gets past the gate.</h1>
          <p className="lead">
            Cerberus kept the gates of the underworld, and nothing slipped by him. Cerberoast keeps
            watch over your software the same way: every vulnerability is reported, triaged, assigned with
            a deadline and fixed, with a record of each step that nobody can quietly edit.
          </p>
          <div className="cta">
            <Link to="/login" className="button large">Sign in</Link>
            <Link to="/report" className="button large ghost">Report a vulnerability</Link>
          </div>
        </div>
        <FindingSpecimen />
      </section>

      <section className="band" aria-labelledby="lifecycle-h">
        <h2 id="lifecycle-h">How a finding moves</h2>
        <ol className="lifecycle">
          {lifecycle.map((s) => (
            <li key={s.status}>
              <span className="lifecycle-dot" />
              <h3>{s.status}</h3>
              <p>{s.text}</p>
            </li>
          ))}
        </ol>
      </section>

      <section className="band" aria-labelledby="cap-h">
        <h2 id="cap-h">Three heads, three watches</h2>
        <div className="capabilities">
          {heads.map((c) => (
            <div key={c.title}>
              <h3>{c.title}</h3>
              <p>{c.text}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="disclose">
        <div>
          <h2>Found a security issue?</h2>
          <p>You don't need an account. Describe what you found and you'll get a reference number to follow up with.</p>
        </div>
        <Link to="/report" className="button large signal">Report a vulnerability</Link>
      </section>

      <footer className="site-footer">
        <span>Cerberoast, Secure Software Development, EASV 2026</span>
        <Link to="/login">Sign in</Link>
      </footer>
    </div>
  )
}
