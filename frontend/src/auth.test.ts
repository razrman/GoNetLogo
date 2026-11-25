import assert from 'node:assert/strict'
import { test } from 'node:test'
import { addUserControl, anonymousUseAllowed, atSessionLimit, authControls, clampMaxSessions, freshSessionNavigation, modelLocked, nextAuthView, passwordProblem, sessionLimitText, workspaceLabel, workspaceOpen, workspaceTarget, type AuthStatus } from './auth.ts'

const anon: AuthStatus = { provider: 'none', requireLogin: false, user: null }
const localOpen: AuthStatus = { provider: 'local', requireLogin: false, user: null }
const user: AuthStatus = {
  provider: 'local',
  requireLogin: true,
  user: { userId: 'u', provider: 'local', subject: 'a@b.co', email: 'a@b.co', displayName: 'A', role: 'user' },
}
const admin: AuthStatus = {
  ...user,
  user: { ...user.user!, role: 'admin' },
}

test('provider none hides sign-in and leaves the model open', () => {
  assert.deepEqual(authControls(anon), { showAuth: false, showAdmin: false, signedIn: false })
  assert.equal(modelLocked(anon), false)
  assert.equal(nextAuthView(anon, 'app'), 'app')
})

test('accounts on show sign-in until allow_anonymous is set', () => {
  assert.equal(anonymousUseAllowed(anon), true)
  assert.equal(anonymousUseAllowed(localOpen), false)
  assert.equal(modelLocked(localOpen), true)
  assert.equal(nextAuthView(localOpen, 'app'), 'login')
  assert.equal(modelLocked({ ...localOpen, allowAnonymous: true }), false)
  assert.equal(nextAuthView({ ...localOpen, allowAnonymous: true }, 'app'), 'app')
  assert.equal(modelLocked({ ...localOpen, requireLogin: true, allowAnonymous: true }), true)
  assert.equal(nextAuthView(user, 'app'), 'app')
})

test('only an admin opens the admin page', () => {
  assert.equal(authControls(user).showAdmin, false)
  assert.equal(authControls(admin).showAdmin, true)
  assert.equal(nextAuthView(user, 'admin'), 'app')
  assert.equal(nextAuthView(admin, 'admin'), 'admin')
  assert.equal(nextAuthView(localOpen, 'account'), 'login')
})

test('a workspace row shows the server status and only opens a live session', () => {
  assert.equal(workspaceLabel('running'), 'Running')
  assert.equal(workspaceLabel('stopped'), 'Stopped')
  assert.equal(workspaceLabel('ending'), 'Ending')
  assert.deepEqual(workspaceOpen('running'), { enabled: true, reason: null })
  assert.deepEqual(workspaceOpen('stopped'), { enabled: true, reason: null })
  assert.equal(workspaceOpen('ending').enabled, false)
  assert.match(workspaceOpen('ending').reason ?? '', /ending/)
  assert.match(workspaceOpen('gone').reason ?? '', /ended/)
  assert.deepEqual(workspaceTarget('aaa', 'bbb', 'stopped'), { action: 'open' })
  assert.deepEqual(workspaceTarget('aaa', 'aaa', 'running'), { action: 'stay' })
  assert.equal(workspaceTarget('aaa', 'bbb', 'ending').action, 'blocked')
})

test('a new session opens that session id and a running row shows its id', () => {
  assert.deepEqual(freshSessionNavigation('0123456789abcdef'), { action: 'open', id: '0123456789abcdef' })
  assert.equal(freshSessionNavigation('  ').action, 'error')
  assert.equal(freshSessionNavigation(null).action, 'error')
  assert.equal(workspaceLabel('running'), 'Running')
  assert.equal(workspaceTarget(null, '0123456789abcdef', 'running').action, 'open')
})

test('the session cap stays between 1 and 50 and the account shows the count', () => {
  assert.equal(clampMaxSessions(0), 1)
  assert.equal(clampMaxSessions(4.2), 4)
  assert.equal(clampMaxSessions(80), 50)
  assert.equal(clampMaxSessions(Number.NaN), 4)
  assert.equal(sessionLimitText(2, 4), '2 of 4 open sessions')
  assert.equal(atSessionLimit(4, 4), true)
  assert.equal(atSessionLimit(3, 4), false)
})

test('add user is available only when the provider can create accounts', () => {
  assert.deepEqual(addUserControl(true), { enabled: true, reason: null })
  assert.equal(addUserControl(false).enabled, false)
  assert.match(addUserControl(false).reason ?? '', /cannot create users/)
})

test('short passwords are rejected before they are sent', () => {
  assert.match(passwordProblem('short') ?? '', /at least 8/)
  assert.equal(passwordProblem('long-enough'), null)
})
