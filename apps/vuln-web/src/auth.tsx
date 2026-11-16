import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, ApiError } from './api'
import { can as canRole, canOnFinding, type Action } from './permissions'
import type { User } from './types'

interface AuthState {
  user: User | null
  loading: boolean
  error: string
  can: (a: Action) => boolean
  canOn: (a: Action, assigneeSub?: string) => boolean
}

const Ctx = createContext<AuthState>({ user: null, loading: true, error: '', can: () => false, canOn: () => false })

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => {
    api
      .me()
      .then(setUser)
      .catch((e: unknown) => setError(e instanceof ApiError && e.status !== 401 ? e.message : ''))
      .finally(() => setLoading(false))
  }, [])
  const value: AuthState = {
    user,
    loading,
    error,
    can: (a) => (user ? canRole(user.role, a) : false),
    canOn: (a, assigneeSub = '') => (user ? canOnFinding(user.role, a, user.sub, assigneeSub) : false),
  }
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export const useAuth = () => useContext(Ctx)
