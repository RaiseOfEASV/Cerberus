import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api, ApiError } from '../api'
import Icon from '../components/Icons'

// Same-site paths only - never send someone off-site after signing in.
function safeReturn(p: string | null) {
  return p && p.startsWith('/') && !p.startsWith('//') && !p.startsWith('/\\') ? p : '/app'
}

// VulnMan's own sign-in page. The password goes to vuln-api (same origin),
// which checks it with Keycloak and answers with a session cookie.
export default function Login() {
  const [params] = useSearchParams()
  const ret = safeReturn(params.get('return'))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.signIn(username.trim(), password)
      // A full navigation, so the app starts with the new session.
      window.location.assign(ret)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setError('Wrong username or password.')
      else if (err instanceof ApiError && err.status === 429) setError('Too many attempts. Wait a while and try again.')
      else if (err instanceof ApiError && err.status === 403) setError('Your account has no VulnMan role yet. Ask a project manager to assign one.')
      else setError(err instanceof Error ? err.message : 'Sign-in failed.')
      setBusy(false)
    }
  }

  return (
    <div className="auth-page">
      <aside className="auth-aside">
        <Link to="/" className="brand light"><span className="brand-mark"><Icon name="shield" size={18} /></span> VulnMan</Link>
        <p className="auth-quote">Every finding, tracked to the fix.</p>
        <small>Secure Software Development, EASV 2026</small>
      </aside>
      <main className="auth-main">
        <form className="auth-card" onSubmit={submit}>
          <h1>Sign in</h1>
          <p className="muted">Use the username and password from your team lead.</p>
          <label htmlFor="l-user">Username</label>
          <input id="l-user" autoComplete="username" required autoFocus value={username} onChange={(e) => setUsername(e.target.value)} />
          <label htmlFor="l-pass">Password</label>
          <input id="l-pass" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
          {error && <p className="error" role="alert">{error}</p>}
          <button type="submit" className="block" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
          <p className="auth-alt">
            <a href={'/auth/login?return=' + encodeURIComponent(ret)}>Use single sign-on instead</a>
            <Link to="/report">Report a vulnerability</Link>
          </p>
        </form>
      </main>
    </div>
  )
}
