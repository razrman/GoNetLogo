import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  bindSessionEnd,
  clearStoredSession,
  postSessionEnd,
  readStoredSession,
  sessionEndBody,
  sessionEndPath,
  sessionStorageKey,
  writeStoredSession,
  type KeyValueStore,
} from './sessionLife.ts'

function memory(): KeyValueStore {
  const data = new Map<string, string>()
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
    removeItem: (key) => {
      data.delete(key)
    },
  }
}

test('a session id round-trips in session storage and rejects garbage', () => {
  const store = memory()
  assert.equal(readStoredSession(store), null)
  writeStoredSession('abc', store)
  assert.equal(readStoredSession(store), null)
  writeStoredSession('0123456789abcdef', store)
  assert.equal(readStoredSession(store), '0123456789abcdef')
  assert.equal(store.getItem(sessionStorageKey), '0123456789abcdef')
  store.setItem(sessionStorageKey, '../etc/passwd')
  assert.equal(readStoredSession(store), null)
  store.setItem(sessionStorageKey, '0123456789abcdef')
  clearStoredSession(store)
  assert.equal(readStoredSession(store), null)
})

test('the beacon body is the session id posted to /session/end', async () => {
  const body = sessionEndBody('0123456789abcdef')
  assert.equal(body.type, 'text/plain')
  assert.equal(await body.text(), '0123456789abcdef')

  let url = ''
  let posted = ''
  const ok = postSessionEnd('0123456789abcdef', (next, data) => {
    url = next
    posted = data instanceof Blob ? 'blob' : String(data)
    return true
  })
  assert.equal(ok, true)
  assert.equal(url, sessionEndPath)
  assert.equal(posted, 'blob')
  assert.equal(postSessionEnd('short', () => true), false)
})

test('pagehide and beforeunload send the current id, and a cached page does not', () => {
  const target = new EventTarget()
  const sent: string[] = []
  let id: string | null = '0123456789abcdef'
  const stop = bindSessionEnd(target, () => id, (next) => {
    sent.push(next)
  })
  target.dispatchEvent(new Event('pagehide'))
  target.dispatchEvent(new Event('beforeunload'))
  const parked = new Event('pagehide')
  Object.defineProperty(parked, 'persisted', { value: true })
  target.dispatchEvent(parked)
  id = null
  target.dispatchEvent(new Event('pagehide'))
  stop()
  target.dispatchEvent(new Event('beforeunload'))
  assert.deepEqual(sent, ['0123456789abcdef', '0123456789abcdef'])
})
