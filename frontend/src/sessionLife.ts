// The web UI keeps its session id in sessionStorage, not localStorage
// and not a cookie. sessionStorage survives a reload of this tab and is
// discarded when the tab closes, so a reload can attach to the same
// interpreter and a new tab starts a new one.

export const sessionStorageKey = 'gonetlogo.session'

const sessionIDRe = /^[A-Za-z0-9_-]{8,64}$/

export type KeyValueStore = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

function browserSessionStore(): KeyValueStore {
  return sessionStorage
}

export function readStoredSession(store: KeyValueStore = browserSessionStore()): string | null {
  try {
    const id = store.getItem(sessionStorageKey)
    if (!id || !sessionIDRe.test(id)) return null
    return id
  } catch {
    return null
  }
}

export function writeStoredSession(id: string, store: KeyValueStore = browserSessionStore()): void {
  if (!sessionIDRe.test(id)) return
  try {
    store.setItem(sessionStorageKey, id)
  } catch {
    // Private mode still runs; the id just will not survive a reload.
  }
}

export function clearStoredSession(store: KeyValueStore = browserSessionStore()): void {
  try {
    store.removeItem(sessionStorageKey)
  } catch {
    // Ignore a store that is blocked.
  }
}

export const sessionEndPath = '/session/end'
export const sessionHeartbeatPath = '/session/heartbeat'

// The page pings about this often. The server's default heartbeat timeout
// is three of these beats (45s).
export const heartbeatIntervalMs = 15_000

// sessionEndBody is the text/plain payload navigator.sendBeacon posts.
// text/plain is a simple request type, so the beacon does not need a
// preflight while the page is going away.
export function sessionEndBody(id: string): Blob {
  return new Blob([id], { type: 'text/plain' })
}

type Beacon = (url: string, body: BodyInit) => boolean

function navigatorBeacon(url: string, body: BodyInit): boolean {
  if (typeof navigator === 'undefined' || typeof navigator.sendBeacon !== 'function') return false
  return navigator.sendBeacon(url, body)
}

// postSessionEnd asks the server to end this session after its grace period.
// A reload that subscribes again before the grace period keeps the session.
// sendBeacon is same-origin, which is what /session/end accepts.
export function postSessionEnd(id: string, beacon: Beacon = navigatorBeacon): boolean {
  if (!sessionIDRe.test(id)) return false
  return beacon(sessionEndPath, sessionEndBody(id))
}

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>

// postSessionHeartbeat tells the server this tab is still open. It is a
// same-origin POST, like the end beacon.
export async function postSessionHeartbeat(id: string, fetchImpl: FetchLike = fetch): Promise<boolean> {
  if (!sessionIDRe.test(id)) return false
  try {
    const res = await fetchImpl(sessionHeartbeatPath, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: id,
      credentials: 'same-origin',
      keepalive: true,
    })
    return res.ok
  } catch {
    return false
  }
}

export type IntervalClock = {
  setInterval(fn: () => void, ms: number): number
  clearInterval(id: number): void
}

const browserClock: IntervalClock = {
  setInterval: (fn, ms) => window.setInterval(fn, ms),
  clearInterval: (id) => window.clearInterval(id),
}

// bindSessionHeartbeat pings while this tab has a session id.
// The interval is not paused when the tab is hidden. Browsers already
// throttle timers in background tabs (often to 1s, and in Chrome's
// intensive throttle to about 1 minute). Pausing on top of that would
// make a tab that is only in the background look gone. When the tab
// becomes visible again, a beat is sent immediately so a throttled gap
// does not have to run out. The server allows about three nominal beats
// of silence before it reaps a detached session.
export function bindSessionHeartbeat(
  getId: () => string | null,
  send: (id: string) => void,
  options?: {
    intervalMs?: number
    clock?: IntervalClock
    target?: EventTarget | null
    isHidden?: () => boolean
  },
): () => void {
  const intervalMs = options?.intervalMs ?? heartbeatIntervalMs
  const clock = options?.clock ?? browserClock
  const isHidden = options?.isHidden ?? (() => typeof document !== 'undefined' && document.visibilityState === 'hidden')
  const target = options?.target === undefined ? (typeof document !== 'undefined' ? document : null) : options.target
  const beat = () => {
    const id = getId()
    if (id) send(id)
  }
  beat()
  const timer = clock.setInterval(beat, intervalMs)
  const onVisible = () => {
    if (!isHidden()) beat()
  }
  target?.addEventListener('visibilitychange', onVisible)
  return () => {
    clock.clearInterval(timer)
    target?.removeEventListener('visibilitychange', onVisible)
  }
}

// bindSessionEnd sends the beacon on pagehide and beforeunload.
// A pagehide that is only parking the page in the back/forward cache
// (event.persisted) does not send, so coming back does not race the grace timer.
export function bindSessionEnd(
  target: EventTarget,
  getId: () => string | null,
  send: (id: string) => void,
): () => void {
  const fire = (event: Event) => {
    if ('persisted' in event && Boolean((event as Event & { persisted?: boolean }).persisted)) return
    const id = getId()
    if (id) send(id)
  }
  target.addEventListener('pagehide', fire)
  target.addEventListener('beforeunload', fire)
  return () => {
    target.removeEventListener('pagehide', fire)
    target.removeEventListener('beforeunload', fire)
  }
}
