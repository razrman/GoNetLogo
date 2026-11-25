import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { addUserControl, atSessionLimit, clampMaxSessions, passwordProblem, sessionLimitText, workspaceLabel, workspaceOpen, type AuthStatus, type AuthUser, type AuthView } from './auth'

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
  status: string
}

type AdminUser = {
  id: string
  email: string
  displayName: string
  role: string
  disabled: boolean
  maxSessions?: number
  lastLogin?: string
}

type UserDetail = {
  sessions: SessionRow[]
  workspaces: WorkspaceRow[]
}

async function csrfToken(): Promise<string> {
  const res = await fetch('/auth/csrf', { credentials: 'same-origin' })
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
  const res = await fetch(path, { method, headers, body: payload, credentials: 'same-origin' })
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
  onOpenWorkspace,
  onNewSession,
  claimCurrent,
}: {
  view: 'login' | 'signup' | 'account' | 'admin'
  onView: (view: AuthView) => void
  onSignedIn: (user: AuthUser) => void
  onOpenWorkspace: (id: string) => Promise<string | null>
  onNewSession: () => Promise<string | null>
  claimCurrent: () => Promise<void>
}) {
  return (
    <section className="auth-page" aria-label={view === 'admin' ? 'Admin' : view === 'account' ? 'Account' : 'Sign in'}>
      {view === 'login' && <LoginForm onView={onView} onSignedIn={onSignedIn} />}
      {view === 'signup' && <SignupForm onView={onView} onSignedIn={onSignedIn} />}
      {view === 'account' && <AccountPage onOpen={onOpenWorkspace} onNew={onNewSession} claimCurrent={claimCurrent} />}
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

function AccountPage({
  onOpen,
  onNew,
  claimCurrent,
}: {
  onOpen: (id: string) => Promise<string | null>
  onNew: () => Promise<string | null>
  claimCurrent: () => Promise<void>
}) {
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [workspaces, setWorkspaces] = useState<WorkspaceRow[]>([])
  const [openCount, setOpenCount] = useState(0)
  const [maxSessions, setMaxSessions] = useState(4)
  const [error, setError] = useState<string | null>(null)
  const [rowNote, setRowNote] = useState<Record<string, string>>({})
  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)
  const load = () => {
    void Promise.all([
      fetch('/auth/sessions').then((res) => res.json()) as Promise<{ sessions?: SessionRow[]; error?: string }>,
      fetch('/auth/workspaces').then((res) => res.json()) as Promise<{ workspaces?: WorkspaceRow[]; openCount?: number; maxSessions?: number; error?: string }>,
    ]).then(([s, w]) => {
      if (s.error || w.error) setError(s.error || w.error || 'Could not load the account.')
      setSessions(s.sessions ?? [])
      setWorkspaces(w.workspaces ?? [])
      setOpenCount(w.openCount ?? 0)
      setMaxSessions(w.maxSessions && w.maxSessions > 0 ? w.maxSessions : 4)
    }).catch((err: Error) => setError(err.message))
  }
  useEffect(() => {
    let cancel = false
    void claimCurrent().finally(() => {
      if (!cancel) load()
    })
    return () => {
      cancel = true
    }
  }, [])
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
      <div className="auth-section-head">
        <h2>Model workspaces</h2>
        <button
          type="button"
          disabled={starting || atSessionLimit(openCount, maxSessions)}
          title={atSessionLimit(openCount, maxSessions) ? 'End one before starting another.' : undefined}
          onClick={() => {
            setStarting(true)
            setStartError(null)
            void onNew().then((problem) => {
              setStarting(false)
              if (problem) setStartError(problem)
            })
          }}
        >New session</button>
      </div>
      <p className="auth-quiet">{sessionLimitText(openCount, maxSessions)}</p>
      {startError && <p className="auth-error">{startError}</p>}
      {workspaces.length === 0 && <p className="auth-quiet">No model is open under this account.</p>}
      {workspaces.length > 0 && (
        <table className="auth-table">
          <thead>
            <tr><th>Model</th><th>Session</th><th>Status</th><th></th></tr>
          </thead>
          <tbody>
            {workspaces.map((row) => {
              const choice = workspaceOpen(row.status)
              const note = rowNote[row.id] || (!choice.enabled ? choice.reason : null)
              return (
                <tr key={row.id}>
                  <td>{row.modelName || 'Untitled'}</td>
                  <td><code className="auth-id">{row.id}</code></td>
                  <td>{workspaceLabel(row.status)}</td>
                  <td className="auth-actions">
                    <button
                      type="button"
                      disabled={!choice.enabled}
                      title={choice.reason ?? 'Open this model'}
                      onClick={() => {
                        void onOpen(row.id).then((problem) => {
                          if (problem) setRowNote((prev) => ({ ...prev, [row.id]: problem }))
                        })
                      }}
                    >Open</button>
                    <button type="button" onClick={() => {
                      void send('/auth/workspaces/end', 'POST', { id: row.id }).then(load).catch((err: Error) => setError(err.message))
                    }}>End</button>
                    {note && <span className="auth-quiet">{note}</span>}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
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
  const [details, setDetails] = useState<Record<string, UserDetail>>({})
  const [error, setError] = useState<string | null>(null)
  const [canCreate, setCanCreate] = useState(false)
  const [minPass, setMinPass] = useState(8)
  const [adding, setAdding] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState({ email: '', displayName: '', password: '', role: 'user' })
  const load = (q = query) => {
    const path = q.trim() ? `/admin/users?q=${encodeURIComponent(q.trim())}` : '/admin/users'
    void fetch(path).then(async (res) => {
      const body = (await res.json()) as {
        users?: AdminUser[]
        error?: string
        canCreateUsers?: boolean
        minPasswordLength?: number
      }
      if (!res.ok) throw new Error(body.error || 'Admin only')
      setUsers(body.users ?? [])
      setCanCreate(!!body.canCreateUsers)
      if (body.minPasswordLength && body.minPasswordLength >= 8) setMinPass(body.minPasswordLength)
      setError(null)
    }).catch((err: Error) => setError(err.message))
  }
  useEffect(() => { load('') }, [])
  const createControl = addUserControl(canCreate)
  const createUser = (e: FormEvent) => {
    e.preventDefault()
    const problem = passwordProblem(draft.password, minPass)
    if (!draft.email.trim()) {
      setFormError('Enter an email address.')
      return
    }
    if (problem) {
      setFormError(problem)
      return
    }
    setBusy(true)
    setFormError(null)
    void send('/admin/users', 'POST', draft).then(() => {
      setAdding(false)
      setDraft({ email: '', displayName: '', password: '', role: 'user' })
      load()
    }).catch((err: Error) => setFormError(err.message))
      .finally(() => setBusy(false))
  }
  const act = (method: string, path: string, body: unknown = {}) => {
    void send(path, method, body).then(() => load()).catch((err: Error) => setError(err.message))
  }
  const loadDetail = (id: string) => {
    void fetch(`/admin/users/${id}/sessions`).then(async (res) => {
      const body = (await res.json()) as { sessions?: SessionRow[]; workspaces?: WorkspaceRow[]; error?: string }
      if (!res.ok) throw new Error(body.error || 'Could not load sessions')
      setDetails((prev) => ({
        ...prev,
        [id]: { sessions: body.sessions ?? [], workspaces: body.workspaces ?? [] },
      }))
    }).catch((err: Error) => setError(err.message))
  }
  return (
    <div className="auth-card wide">
      <h1>Admin</h1>
      {error && <p className="auth-error">{error}</p>}
      <div className="auth-actions">
        <button type="button" disabled={!createControl.enabled} title={createControl.reason ?? undefined} onClick={() => { setAdding((open) => !open); setFormError(null) }}>
          Add user
        </button>
        {!createControl.enabled && <span className="auth-quiet">{createControl.reason}</span>}
      </div>
      {adding && createControl.enabled && (
        <form className="auth-create" onSubmit={createUser}>
          <label>Email<input type="email" required value={draft.email} onChange={(e) => setDraft({ ...draft, email: e.target.value })} /></label>
          <label>Display name<input type="text" value={draft.displayName} onChange={(e) => setDraft({ ...draft, displayName: e.target.value })} /></label>
          <label>Password<input type="password" required value={draft.password} onChange={(e) => setDraft({ ...draft, password: e.target.value })} /></label>
          <label>Role
            <select value={draft.role} onChange={(e) => setDraft({ ...draft, role: e.target.value })}>
              <option value="user">user</option>
              <option value="admin">admin</option>
            </select>
          </label>
          {formError && <p className="auth-error">{formError}</p>}
          <button className="act" type="submit" disabled={busy}>Create user</button>
        </form>
      )}
      <form className="auth-search" onSubmit={(e) => { e.preventDefault(); load() }}>
        <input aria-label="Search users" placeholder="Search email or name" value={query} onChange={(e) => setQuery(e.target.value)} />
        <button type="submit">Search</button>
      </form>
      <table className="auth-table">
        <thead>
          <tr><th>Name</th><th>Email</th><th>Role</th><th>Status</th><th>Max sessions</th><th></th></tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id}>
              <td>{user.displayName}</td>
              <td>{user.email}</td>
              <td>{user.role}</td>
              <td>{user.disabled ? 'disabled' : 'active'}</td>
              <td>
                <MaxSessionsControl
                  key={`${user.id}-${user.maxSessions || 4}`}
                  value={user.maxSessions || 4}
                  onChange={(n) => {
                    if (n !== (user.maxSessions || 4)) act('POST', `/admin/users/${user.id}/max-sessions`, { maxSessions: n })
                  }}
                />
              </td>
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
                <button type="button" onClick={() => loadDetail(user.id)}>Sessions</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {Object.entries(details).map(([id, detail]) => (
        <div key={id}>
          <h2>Login sessions</h2>
          <SessionTable rows={detail.sessions} onRevoke={(sessionId) => {
            void send('/admin/sessions/revoke', 'POST', { id: sessionId }).then(() => loadDetail(id)).catch((err: Error) => setError(err.message))
          }} />
          <h2>Model workspaces</h2>
          {detail.workspaces.length === 0 && <p className="auth-quiet">No model is open for this account.</p>}
          {detail.workspaces.length > 0 && (
            <table className="auth-table">
              <thead>
                <tr><th>Model</th><th>Session</th><th>Status</th><th></th></tr>
              </thead>
              <tbody>
                {detail.workspaces.map((row) => (
                  <tr key={row.id}>
                    <td>{row.modelName || 'Untitled'}</td>
                    <td><code className="auth-id">{row.id}</code></td>
                    <td>{workspaceLabel(row.status)}</td>
                    <td>
                      <button type="button" onClick={() => {
                        void send('/admin/workspaces/end', 'POST', { userId: id, id: row.id }).then(() => loadDetail(id)).catch((err: Error) => setError(err.message))
                      }}>End</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      ))}
    </div>
  )
}

function MaxSessionsControl({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  const shown = clampMaxSessions(value)
  const [text, setText] = useState(String(shown))
  const commit = (n: number) => {
    const next = clampMaxSessions(n)
    setText(String(next))
    if (next !== shown) onChange(next)
  }
  return (
    <span className="session-stepper">
      <button type="button" aria-label="Decrease max sessions" disabled={shown <= 1} onClick={() => commit(shown - 1)}>▼</button>
      <input
        aria-label="Max sessions"
        type="number"
        min={1}
        max={50}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={() => commit(Number(text))}
      />
      <button type="button" aria-label="Increase max sessions" disabled={shown >= 50} onClick={() => commit(shown + 1)}>▲</button>
    </span>
  )
}

function stamp(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
