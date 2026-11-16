import { useEffect, useState } from 'react'
import { api } from '../api'
import { useAuth } from '../auth'
import ErrorText, { message } from '../components/ErrorText'
import type { Person } from '../types'

const roles = ['analyst', 'developer', 'pm', 'auditor'] as const

// AUTH-04: a PM changes other users' roles (in Keycloak). The change ends
// that user's sessions, so it applies on their next request.
export default function AdminUsers() {
  const { user } = useAuth()
  const [people, setPeople] = useState<Person[]>([])
  const [done, setDone] = useState('')
  const [error, setError] = useState('')
  useEffect(() => {
    api.users().then(setPeople, (e) => setError(message(e)))
  }, [])
  async function change(p: Person, role: string) {
    setError('')
    setDone('')
    try {
      const updated = await api.setRole(p.sub, role)
      setPeople(people.map((x) => (x.sub === p.sub ? { ...x, role: updated.role } : x)))
      setDone(`${p.name} is now ${updated.role}. Their sessions were ended.`)
    } catch (err) {
      setError(message(err))
    }
  }
  return (
    <>
      <h1>Users and roles</h1>
      {done && <p className="success" role="status">{done}</p>}
      <ErrorText error={error} />
      <table>
        <thead><tr><th>Name</th><th>Username</th><th>E-mail</th><th>Role</th></tr></thead>
        <tbody>
          {people.map((p) => (
            <tr key={p.sub}>
              <td>{p.name}</td>
              <td>{p.username}</td>
              <td>{p.email}</td>
              <td>
                <select aria-label={`Role for ${p.name}`} value={p.role} disabled={p.sub === user?.sub}
                  onChange={(e) => change(p, e.target.value)}>
                  {p.role === '' && <option value="" disabled>(no role)</option>}
                  {roles.map((r) => <option key={r} value={r}>{r}</option>)}
                </select>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
