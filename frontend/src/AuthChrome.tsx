import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { passwordProblem, type AuthStatus, type AuthUser, type AuthView } from './auth'

type SessionRow = {
  id: string
  userAgent: string
  created: string
  lastSeen: string
  current?: boolean
}

type WorkspaceRow = {
  id: string
  modelName: string
  created: string
  running: boolean
}

type AdminUser = {
  id: string
  email: string
  displayName: string
  role: string
  disabled: boolean
  lastLogin?: string
}

async function csrfToken(): Promise<string> {
  const res = await fetch('/auth/csrf')
  if (!res.ok) throw new Error('Could not start the form.')
  const body = (await res.json()) as { token?: string }
  if (!body.token) throw new Error('Could not start the form.')
  return body.token
}

async function send(path: string, method: string, body?: unknown): Promise<unknown> {
  const headers: Record<string, string> = {}
  let payload: string | undefined
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    headers['X-CSRF-Token'] = await csrfToken()
    payload = JSON.stringify(body)
  }
  const res = await fetch(path, { method, headers, body: payload })
  const data = (await res.json().catch(() => ({}))) as { error?: string }
  if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`)
  return data
}

export function claimWorkspace(id: string): Promise<unknown> {
  return send('/auth/workspaces/claim', 'POST', { id })
}

export function AuthMenu({
  status,
  view,
  onView,
}: {
  status: AuthStatus
  view: AuthView
  onView: (view: AuthView) => void
}) {
  const user = status.user
  if (!user) {
    return (
      <div className="auth-menu">
        <button type="button" className={view === 'login' ? 'auth-link on' : 'auth-link'} onClick={() => onView('login')}>
          Log in
        </button>
        <button type="button" className={view === 'signup' ? 'auth-link on' : 'auth-link'} onClick={() => onView('signup')}>
          Sign up
        </button>
      </div>
    )
  }
  return (
    <div className="auth-menu">
      <span className="auth-who" title={user.email}>{user.displayName || user.email}</span>
      <button type="button" className={view === 'account' ? 'auth-link on' : 'auth-link'} onClick={() => onView('account')}>
        Account
      </button>
      {user.role === 'admin' && (
        <button type="button" className={view === 'admin' ? 'auth-link on' : 'auth-link'} onClick={() => onView('admin')}>
          Admin
        </button>
      )}
      <button
        type="button"
        className="auth-link"
        onClick={() => {
          void send('/auth/logout', 'POST', {}).then(() => {
            window.location.reload()
          }).catch((err: Error) => {
            window.alert(err.message)
          })
        }}
      >
        Log out
      </button>
    </div>
  )
}

export function AuthScreens({
  view,
  onView,
  onSignedIn,
}: {
  view: 'login' | 'signup' | 'account' | 'admin'
  onView: (view: AuthView) => void
  onSignedIn: (user: AuthUser) => void
}) {
  return (
    <section className="auth-page" aria-label={view === 'admin' ? 'Admin' : view === 'account' ? 'Account' : 'Sign in'}>
      {view === 'login' && <LoginForm onView={onView} onSignedIn={onSignedIn} />}
      {view === 'signup' && <SignupForm onView={onView} onSignedIn={onSignedIn} />}
      {view === 'account' && <AccountPage onView={onView} />}
      {view === 'admin' && <AdminPage />}
    </section>
  )
}

function LoginForm({ onView, onSignedIn }: { onView: (view: AuthView) => void; onSignedIn: (user: AuthUser) => void }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    void send('/auth/login', 'POST', { email, password })
      .then((body) => {
        const user = (body as { user: AuthUser }).user
        onSignedIn(user)
        onView('app')
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(false))
  }
  return (
    <form className="auth-card" onSubmit={submit}>
      <h1>Log in</h1>
      <label>Email<input type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required /></label>
      <label>Password<input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
      {error && <p className="auth-error">{error}</p>}
      <button className="act" type="submit" disabled={busy}>Log in</button>
      <button className="auth-text" type="button" onClick={() => onView('signup')}>Need an account? Sign up</button>
    </form>
  )
}

function SignupForm({ onView, onSignedIn }: { onView: (view: AuthView) => void; onSignedIn: (user: AuthUser) => void }) {
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const problem = passwordProblem(password)
    if (problem) {
      setError(problem)
      return
    }
    setBusy(true)
    setError(null)
    void send('/auth/signup', 'POST', { email, password, displayName })
      .then((body) => {
        const user = (body as { user: AuthUser }).user
        onSignedIn(user)
        onView('app')
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(false))
  }
  return (
    <form className="auth-card" onSubmit={submit}>
      <h1>Sign up</h1>
      <label>Email<input type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required /></label>
      <label>Display name<input type="text" autoComplete="nickname" value={displayName} onChange={(e) => setDisplayName(e.target.value)} /></label>
      <label>Password<input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
      {error && <p className="auth-error">{error}</p>}
      <button className="act" type="submit" disabled={busy}>Create account</button>
      <button className="auth-text" type="button" onClick={() => onView('login')}>Already have an account? Log in</button>
    </form>
  )
}

function AccountPage({ onView }: { onView: (view: AuthView) => void }) {
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [workspaces, setWorkspaces] = useState<WorkspaceRow[]>([])
  const [error, setError] = useState<string | null>(null)
  const load = () => {
    void Promise.all([
      fetch('/auth/sessions').then((res) => res.json()) as Promise<{ sessions?: SessionRow[]; error?: string }>,
      fetch('/auth/workspaces').then((res) => res.json()) as Promise<{ workspaces?: WorkspaceRow[]; error?: string }>,
    ]).then(([s, w]) => {
      if (s.error || w.error) setError(s.error || w.error || 'Could not load the account.')
      setSessions(s.sessions ?? [])
      setWorkspaces(w.workspaces ?? [])
    }).catch((err: Error) => setError(err.message))
  }
  useEffect(load, [])
  const revoke = (body: { id?: string; allOthers?: boolean }) => {
    void send('/auth/sessions/revoke', 'POST', body).then(() => {
      if (body.id && sessions.some((row) => row.id === body.id && row.current)) window.location.reload()
      else load()
    }).catch((err: Error) => setError(err.message))
  }
  return (
    <div className="auth-card wide">
      <h1>Account</h1>
      {error && <p className="auth-error">{error}</p>}
      <h2>Signed-in browsers</h2>
      <SessionTable rows={sessions} onRevoke={(id) => revoke({ id })} />
      <button className="auth-text" type="button" onClick={() => revoke({ allOthers: true })}>Sign out other browsers</button>
      <h2>Model workspaces</h2>
      {workspaces.length === 0 && <p className="auth-quiet">No model is open under this account.</p>}
      <ul className="auth-list">
        {workspaces.map((row) => (
          <li key={row.id}>
            <span>{row.modelName || 'Untitled'}{row.running ? ' · running' : ''}</span>
            <button type="button" onClick={() => {
              void send('/auth/workspaces/end', 'POST', { id: row.id }).then(load).catch((err: Error) => setError(err.message))
            }}>End</button>
          </li>
        ))}
      </ul>
      <button className="auth-text" type="button" onClick={() => onView('app')}>Back to the model</button>
    </div>
  )
}

function SessionTable({ rows, onRevoke }: { rows: SessionRow[]; onRevoke: (id: string) => void }) {
  if (rows.length === 0) return <p className="auth-quiet">No other browsers are signed in.</p>
  return (
    <table className="auth-table">
      <thead>
        <tr><th>Browser</th><th>Signed in</th><th>Last seen</th><th></th></tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.id}>
            <td>{row.userAgent || 'Unknown browser'}{row.current ? ' (this browser)' : ''}</td>
            <td>{stamp(row.created)}</td>
            <td>{stamp(row.lastSeen)}</td>
            <td><button type="button" onClick={() => onRevoke(row.id)}>Sign out</button></td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function AdminPage() {
  const [query, setQuery] = useState('')
  const [users, setUsers] = useState<AdminUser[]>([])
  const [sessions, setSessions] = useState<Record<string, SessionRow[]>>({})
  const [error, setError] = useState<string | null>(null)
  const load = (q = query) => {
    const path = q.trim() ? `/admin/users?q=${encodeURIComponent(q.trim())}` : '/admin/users'
    void fetch(path).then(async (res) => {
      const body = (await res.json()) as { users?: AdminUser[]; error?: string }
      if (!res.ok) throw new Error(body.error || 'Admin only')
      setUsers(body.users ?? [])
      setError(null)
    }).catch((err: Error) => setError(err.message))
  }
  useEffect(() => { load('') }, [])
  const act = (method: string, path: string, body: unknown = {}) => {
    void send(path, method, body).then(() => load()).catch((err: Error) => setError(err.message))
  }
  return (
    <div className="auth-card wide">
      <h1>Admin</h1>
      {error && <p className="auth-error">{error}</p>}
      <form className="auth-search" onSubmit={(e) => { e.preventDefault(); load() }}>
        <input aria-label="Search users" placeholder="Search email or name" value={query} onChange={(e) => setQuery(e.target.value)} />
        <button type="submit">Search</button>
      </form>
      <table className="auth-table">
        <thead>
          <tr><th>Name</th><th>Email</th><th>Role</th><th>Status</th><th></th></tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id}>
              <td>{user.displayName}</td>
              <td>{user.email}</td>
              <td>{user.role}</td>
              <td>{user.disabled ? 'disabled' : 'active'}</td>
              <td className="auth-actions">
                <button type="button" onClick={() => act('POST', `/admin/users/${user.id}/role`, { role: user.role === 'admin' ? 'user' : 'admin' })}>
                  {user.role === 'admin' ? 'Demote' : 'Promote'}
                </button>
                <button type="button" onClick={() => act('POST', `/admin/users/${user.id}/disabled`, { disabled: !user.disabled })}>
                  {user.disabled ? 'Enable' : 'Disable'}
                </button>
                <button type="button" onClick={() => {
                  if (window.confirm(`Delete ${user.email}?`)) act('DELETE', `/admin/users/${user.id}`)
                }}>Delete</button>
                <button type="button" onClick={() => {
                  void fetch(`/admin/users/${user.id}/sessions`).then((res) => res.json()).then((body: { sessions?: SessionRow[] }) => {
                    setSessions((prev) => ({ ...prev, [user.id]: body.sessions ?? [] }))
                  }).catch((err: Error) => setError(err.message))
                }}>Sessions</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {Object.entries(sessions).map(([id, rows]) => (
        <div key={id}>
          <h2>Sessions</h2>
          <SessionTable rows={rows} onRevoke={(sessionId) => {
            void send('/admin/sessions/revoke', 'POST', { id: sessionId }).then(() => {
              setSessions((prev) => ({ ...prev, [id]: prev[id].filter((row) => row.id !== sessionId) }))
            }).catch((err: Error) => setError(err.message))
          }} />
        </div>
      ))}
    </div>
  )
}

function stamp(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
