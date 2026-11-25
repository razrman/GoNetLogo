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

// modelLocked is true when the model UI must wait for a signed-in user.
export function modelLocked(status: AuthStatus | null): boolean {
  return !!status?.requireLogin && !status.user
}

export function nextAuthView(status: AuthStatus | null, asked: AuthView): AuthView {
  if (modelLocked(status) && (asked === 'app' || asked === 'admin' && status?.user?.role !== 'admin')) {
    return 'login'
  }
  if (asked === 'admin' && status?.user?.role !== 'admin') return 'app'
  if ((asked === 'account' || asked === 'admin') && !status?.user) return 'login'
  return asked
}

export function passwordProblem(password: string, min = 8): string | null {
  if (password.length < min) return `Use at least ${min} characters.`
  if (password.length > 200) return 'That password is too long.'
  return null
}
