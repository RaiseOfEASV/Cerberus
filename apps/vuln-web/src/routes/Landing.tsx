import { Link } from 'react-router'
import Icon from '../components/Icons'

const features = [
  { icon: 'inbox', title: 'Every intake channel', text: 'Manual findings, a public disclosure form, Trivy image and SBOM scans, and CSV imports land in one queue - de-duplicated automatically.' },
  { icon: 'list', title: 'Triage that sticks', text: 'Severity, CVSS, CVE and affected asset on every finding. Assign an owner and the SLA due date follows from the severity.' },
  { icon: 'clock', title: 'SLAs you can see', text: 'A live dashboard shows what is open, what is overdue and how compliance is trending week over week.' },
  { icon: 'lock', title: 'Encrypted by default', text: 'Descriptions, reporter contacts and evidence are encrypted per finding with keys held in Vault - never in the database.' },
  { icon: 'history', title: 'Tamper-evident audit trail', text: 'Every change is recorded in a hash-chained log. One click verifies that nobody rewrote history.' },
  { icon: 'file', title: 'Signed reports', text: 'Export a report for any period or asset, signed with Ed25519, so auditors can verify it was not altered.' },
]

const steps = [
  { title: 'Report', text: 'A researcher, a scanner or your own team reports an issue.' },
  { title: 'Triage', text: 'Analysts confirm severity and scope, and the SLA clock starts.' },
  { title: 'Fix', text: 'The assigned developer moves it to In Progress, then Remediated.' },
  { title: 'Prove it', text: 'Auditors read the trail and verify signed reports.' },
]

export default function Landing() {
  return (
    <div className="site">
      <header className="site-nav">
        <div className="brand"><span className="brand-mark"><Icon name="shield" size={20} /></span> VulnMan</div>
        <nav>
          <a href="#features">Features</a>
          <a href="#how">How it works</a>
          <Link to="/report">Report a vulnerability</Link>
          <Link to="/login" className="button small">Sign in</Link>
        </nav>
      </header>

      <section className="hero">
        <p className="eyebrow">Vulnerability management</p>
        <h1>From the first report to the verified fix.</h1>
        <p className="lead">
          VulnMan collects vulnerabilities from every source, routes them to the right people with clear
          deadlines, and keeps a tamper-evident record of everything that happened.
        </p>
        <div className="cta">
          <Link to="/login" className="button large">Sign in <Icon name="arrow" /></Link>
          <Link to="/report" className="button large ghost">Report a vulnerability</Link>
        </div>
        <ul className="hero-points">
          <li><Icon name="check" /> Role-based access for analysts, developers, managers and auditors</li>
          <li><Icon name="check" /> Single sign-on through Keycloak</li>
          <li><Icon name="check" /> Built and deployed with signed, scanned images</li>
        </ul>
      </section>

      <section id="features" className="section">
        <h2>Everything a security team needs, nothing it doesn't</h2>
        <div className="feature-grid">
          {features.map((f) => (
            <article key={f.title} className="feature">
              <span className="feature-icon"><Icon name={f.icon} size={22} /></span>
              <h3>{f.title}</h3>
              <p>{f.text}</p>
            </article>
          ))}
        </div>
      </section>

      <section id="how" className="section">
        <h2>How it works</h2>
        <ol className="steps">
          {steps.map((s, i) => (
            <li key={s.title}>
              <span className="step-no">{i + 1}</span>
              <h3>{s.title}</h3>
              <p>{s.text}</p>
            </li>
          ))}
        </ol>
      </section>

      <section className="section disclose">
        <div>
          <h2>Found a security issue?</h2>
          <p>You don't need an account. Tell us what you found and you'll get a reference number to follow up with.</p>
        </div>
        <Link to="/report" className="button large">Report a vulnerability</Link>
      </section>

      <footer className="site-footer">
        <span>VulnMan - Secure Software Development, EASV 2026</span>
        <Link to="/login">Sign in</Link>
      </footer>
    </div>
  )
}
