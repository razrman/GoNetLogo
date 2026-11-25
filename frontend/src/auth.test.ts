import assert from 'node:assert/strict'
import { test } from 'node:test'
import { authControls, modelLocked, nextAuthView, passwordProblem, type AuthStatus } from './auth.ts'

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

test('require_login sends an anonymous visitor to the sign-in page', () => {
  assert.equal(modelLocked(localOpen), false)
  assert.equal(modelLocked({ ...localOpen, requireLogin: true }), true)
  assert.equal(nextAuthView({ ...localOpen, requireLogin: true }, 'app'), 'login')
  assert.equal(nextAuthView(user, 'app'), 'app')
})

test('only an admin opens the admin page', () => {
  assert.equal(authControls(user).showAdmin, false)
  assert.equal(authControls(admin).showAdmin, true)
  assert.equal(nextAuthView(user, 'admin'), 'app')
  assert.equal(nextAuthView(admin, 'admin'), 'admin')
  assert.equal(nextAuthView(localOpen, 'account'), 'login')
})

test('short passwords are rejected before they are sent', () => {
  assert.match(passwordProblem('short') ?? '', /at least 8/)
  assert.equal(passwordProblem('long-enough'), null)
})
