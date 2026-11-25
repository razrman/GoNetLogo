import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  bindSessionEnd,
  bindSessionHeartbeat,
  clearStoredSession,
  heartbeatIntervalMs,
  postSessionEnd,
  postSessionHeartbeat,
  presentCreateSessionError,
  readStoredSession,
  sessionEndBody,
  sessionEndPath,
  sessionMark,
  sessionHeartbeatPath,
  sessionStorageKey,
  writeStoredSession,
  type IntervalClock,
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

test('the interface mark is the last five characters and the tooltip is the full id', () => {
  assert.equal(sessionMark(null), null)
  assert.equal(sessionMark(''), null)
  assert.equal(sessionMark('  '), null)
  assert.deepEqual(sessionMark('0123456789abcdef'), { label: '· …bcdef', title: '0123456789abcdef' })
  assert.deepEqual(sessionMark('a1b2c'), { label: '· a1b2c', title: 'a1b2c' })
  assert.deepEqual(sessionMark('zzzzza1b2c'), { label: '· …a1b2c', title: 'zzzzza1b2c' })
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

test('the heartbeat posts the session id to /session/heartbeat', async () => {
  let url = ''
  let body = ''
  let credentials = ''
  const ok = await postSessionHeartbeat('0123456789abcdef', async (input, init) => {
    url = String(input)
    body = String(init?.body ?? '')
    credentials = init?.credentials ?? ''
    return new Response(null, { status: 204 })
  })
  assert.equal(ok, true)
  assert.equal(url, sessionHeartbeatPath)
  assert.equal(body, '0123456789abcdef')
  assert.equal(credentials, 'same-origin')
  assert.equal(await postSessionHeartbeat('short', async () => new Response(null, { status: 204 })), false)
  assert.equal(await postSessionHeartbeat('0123456789abcdef', async () => {
    throw new Error('offline')
  }), false)
})

function manualClock(): IntervalClock & { fire(): void; delay: number } {
  let next = 1
  let current: (() => void) | null = null
  let delay = 0
  return {
    get delay() {
      return delay
    },
    setInterval(fn, ms) {
      current = fn
      delay = ms
      return next++
    },
    clearInterval() {
      current = null
    },
    fire() {
      current?.()
    },
  }
}

test('the heartbeat keeps beating while the tab is hidden and pings when it is shown', () => {
  const clock = manualClock()
  const target = new EventTarget()
  const sent: string[] = []
  let hidden = true
  let id: string | null = '0123456789abcdef'
  const stop = bindSessionHeartbeat((() => id), (next) => sent.push(next), {
    clock,
    target,
    isHidden: () => hidden,
  })
  assert.equal(clock.delay, heartbeatIntervalMs)
  assert.deepEqual(sent, ['0123456789abcdef'])
  clock.fire()
  assert.deepEqual(sent, ['0123456789abcdef', '0123456789abcdef'])
  target.dispatchEvent(new Event('visibilitychange'))
  assert.equal(sent.length, 2)
  hidden = false
  target.dispatchEvent(new Event('visibilitychange'))
  assert.equal(sent.length, 3)
  id = null
  clock.fire()
  assert.equal(sent.length, 3)
  stop()
  id = '0123456789abcdef'
  clock.fire()
  target.dispatchEvent(new Event('visibilitychange'))
  assert.equal(sent.length, 3)
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

test('a full server is explained without the GraphQL prefix', () => {
  const friendly = 'GoNetLogo is already hosting as many sessions as it can. Wait a moment and reload the page.'
  assert.equal(
    presentCreateSessionError('[GraphQL] too many sessions: this server is hosting as many as it can'),
    friendly,
  )
  assert.equal(presentCreateSessionError('too many sessions'), friendly)
  assert.equal(presentCreateSessionError('[GraphQL] no such model'), 'no such model')
  assert.match(
    presentCreateSessionError('[GraphQL] you have 4 open model sessions, which is your limit of 4. End one on the Account page before starting another'),
    /End one on the Account page/,
  )
  assert.equal(presentCreateSessionError(''), 'Could not open a session')
  assert.equal(presentCreateSessionError(null), 'Could not open a session')
  assert.match(presentCreateSessionError('[Network] failed to fetch'), /failed to fetch/)
})
