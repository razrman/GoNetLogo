// Sign-in state for the web UI. provider "none" keeps the anonymous app.

export type AuthUser = {
  userId: string
  provider: string
  subject: string
  email: string
  displayName: string
  role: string
}

export type AuthStatus = {
  provider: string
  requireLogin: boolean
  allowAnonymous?: boolean
  user: AuthUser | null
}

export type AuthView = 'app' | 'login' | 'signup' | 'account' | 'admin'

export function authControls(status: AuthStatus | null): {
  showAuth: boolean
  showAdmin: boolean
  signedIn: boolean
} {
  const provider = status?.provider ?? 'none'
  const user = status?.user ?? null
  return {
    showAuth: provider !== 'none',
    showAdmin: user?.role === 'admin',
    signedIn: !!user,
  }
}

// anonymousUseAllowed is true for provider none, and for other providers
// only when allow_anonymous is explicitly on and login is not required.
export function anonymousUseAllowed(status: AuthStatus | null): boolean {
  if (!status || status.provider === 'none') return true
  if (status.requireLogin) return false
  return status.allowAnonymous === true
}

// modelLocked is true when the model UI must wait for a signed-in user.
export function modelLocked(status: AuthStatus | null): boolean {
  if (!status || status.user) return false
  return !anonymousUseAllowed(status)
}

export function nextAuthView(status: AuthStatus | null, asked: AuthView): AuthView {
  if (modelLocked(status) && (asked === 'app' || asked === 'admin' && status?.user?.role !== 'admin')) {
    return 'login'
  }
  if (asked === 'admin' && status?.user?.role !== 'admin') return 'app'
  if ((asked === 'account' || asked === 'admin') && !status?.user) return 'login'
  return asked
}

export type WorkspaceStatus = 'running' | 'stopped' | 'ending'

export function workspaceLabel(status: string): string {
  if (status === 'running') return 'Running'
  if (status === 'ending') return 'Ending'
  return 'Stopped'
}

// A running or stopped session can be shown again. An ending session is
// in its grace period, and anything else is already gone.
export function workspaceOpen(status: string): { enabled: boolean; reason: string | null } {
  if (status === 'running' || status === 'stopped') return { enabled: true, reason: null }
  if (status === 'ending') return { enabled: false, reason: 'This session is ending.' }
  return { enabled: false, reason: 'That session has ended.' }
}

// workspaceTarget is what Open does for one row. The current workspace
// only switches the page back. Another openable session is loaded.
export function workspaceTarget(currentId: string | null, workspaceId: string, status: string):
  | { action: 'open' }
  | { action: 'stay' }
  | { action: 'blocked'; reason: string } {
  const choice = workspaceOpen(status)
  if (!choice.enabled) return { action: 'blocked', reason: choice.reason ?? 'That session has ended.' }
  if (currentId && currentId === workspaceId) return { action: 'stay' }
  return { action: 'open' }
}

// freshSessionNavigation opens the session id returned by createSession.
export function freshSessionNavigation(id: string | null | undefined):
  | { action: 'open'; id: string }
  | { action: 'error'; reason: string } {
  const trimmed = (id ?? '').trim()
  if (!trimmed) return { action: 'error', reason: 'Could not start a session.' }
  return { action: 'open', id: trimmed }
}

export const minUserSessions = 1
export const maxUserSessions = 50

export function clampMaxSessions(n: number): number {
  if (!Number.isFinite(n)) return 4
  const rounded = Math.round(n)
  return Math.min(maxUserSessions, Math.max(minUserSessions, rounded))
}

// ownSessionSummary is this account's open model sessions and this
// account's limit. The limit is not a count of sessions on the server.
export function ownSessionSummary(open: number, limit: number): { openSessions: string; limit: string } {
  const used = Number.isFinite(open) && open > 0 ? Math.floor(open) : 0
  const cap = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : 4
  return {
    openSessions: `Open sessions: ${used}`,
    limit: `Limit: ${cap}`,
  }
}

export function atSessionLimit(open: number, max: number): boolean {
  return max > 0 && open >= max
}

export function addUserControl(canCreate: boolean): { enabled: boolean; reason: string | null } {
  if (canCreate) return { enabled: true, reason: null }
  return { enabled: false, reason: 'This sign-in provider cannot create users.' }
}

export function passwordProblem(password: string, min = 8): string | null {
  if (password.length < min) return `Use at least ${min} characters.`
  if (password.length > 200) return 'That password is too long.'
  return null
}
