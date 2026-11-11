package session

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Lifecycle is how long a web session outlives its client.
// Grace is how long after the last frames subscription drops, or after
// the end-session beacon, before the server looks again. A new
// subscription in that window cancels the removal, so a reload or a
// short network blip keeps the same interpreter and world.
// Heartbeat is how long a detached session may go without a UI heartbeat
// or websocket activity before it is removed. The default is three missed
// beats of the 15s page heartbeat. A live heartbeat keeps the session
// through a websocket reconnect that lasts longer than Grace.
// Idle is the longer safety net, measured from the same last-seen clock.
// Sweep is how often that check runs.
// Neither the grace timer nor the sweep removes a session that still has
// an attached frames subscription.
type Lifecycle struct {
	Grace     time.Duration
	Idle      time.Duration
	Sweep     time.Duration
	Heartbeat time.Duration
}

const (
	// EnvGrace, EnvIdle, EnvSweep, and EnvHeartbeat override the defaults
	// when the matching server flag is omitted.
	EnvGrace     = "GONETLOGO_SESSION_GRACE"
	EnvIdle      = "GONETLOGO_SESSION_IDLE"
	EnvSweep     = "GONETLOGO_SESSION_SWEEP"
	EnvHeartbeat = "GONETLOGO_SESSION_HEARTBEAT"
)

// DefaultLifecycle keeps a disconnected client for 15 seconds, allows 45
// seconds without a heartbeat (three 15s beats), and sweeps every 30
// seconds. The 5 minute idle TTL is the longer backstop.
func DefaultLifecycle() Lifecycle {
	return Lifecycle{
		Grace:     15 * time.Second,
		Idle:      5 * time.Minute,
		Sweep:     30 * time.Second,
		Heartbeat: 45 * time.Second,
	}
}

// ResolveLifecycle builds a Lifecycle. A non-empty flag wins over the
// environment, and an empty environment value keeps the default. Idle and
// Heartbeat must be at least Grace so a session inside its reconnect
// window is not removed.
func ResolveLifecycle(graceFlag, idleFlag, sweepFlag, heartbeatFlag string, getenv func(string) string) (Lifecycle, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	base := DefaultLifecycle()
	var err error
	if base.Grace, err = pickDuration(graceFlag, getenv(EnvGrace), base.Grace, EnvGrace); err != nil {
		return Lifecycle{}, err
	}
	if base.Idle, err = pickDuration(idleFlag, getenv(EnvIdle), base.Idle, EnvIdle); err != nil {
		return Lifecycle{}, err
	}
	if base.Sweep, err = pickDuration(sweepFlag, getenv(EnvSweep), base.Sweep, EnvSweep); err != nil {
		return Lifecycle{}, err
	}
	if base.Heartbeat, err = pickDuration(heartbeatFlag, getenv(EnvHeartbeat), base.Heartbeat, EnvHeartbeat); err != nil {
		return Lifecycle{}, err
	}
	if base.Idle < base.Grace {
		return Lifecycle{}, fmt.Errorf("session idle TTL %s is shorter than the grace period %s", base.Idle, base.Grace)
	}
	if base.Heartbeat < base.Grace {
		return Lifecycle{}, fmt.Errorf("session heartbeat timeout %s is shorter than the grace period %s", base.Heartbeat, base.Grace)
	}
	return base, nil
}

func pickDuration(flagVal, envVal string, fallback time.Duration, name string) (time.Duration, error) {
	raw := strings.TrimSpace(flagVal)
	source := "-" + durationFlag(name)
	if raw == "" {
		raw = strings.TrimSpace(envVal)
		source = name
	}
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: want a positive duration, got %q", source, raw)
	}
	return d, nil
}

func durationFlag(env string) string {
	switch env {
	case EnvGrace:
		return "session-grace"
	case EnvIdle:
		return "session-idle"
	case EnvSweep:
		return "session-sweep"
	case EnvHeartbeat:
		return "session-heartbeat"
	default:
		return env
	}
}

// NewStoreWith starts an empty store that ends disconnected sessions on lc.
func NewStoreWith(lc Lifecycle) *Store {
	if lc.Grace <= 0 || lc.Idle <= 0 || lc.Sweep <= 0 || lc.Heartbeat <= 0 {
		panic("session lifecycle durations must be positive")
	}
	st := &Store{
		sessions:   map[string]*Session{},
		grace:      lc.Grace,
		idleTTL:    lc.Idle,
		sweepEvery: lc.Sweep,
		heartbeat:  lc.Heartbeat,
		stop:       make(chan struct{}),
		sweepDone:  make(chan struct{}),
	}
	go st.sweepLoop()
	return st
}

// Close stops the idle sweep and every session. It is safe to call more
// than once, and a second call waits until the first has finished.
func (st *Store) Close() {
	st.closeOnce.Do(func() {
		st.mu.Lock()
		st.closed = true
		close(st.stop)
		all := make([]*Session, 0, len(st.sessions))
		for id, s := range st.sessions {
			st.disarmLocked(s)
			delete(st.sessions, id)
			all = append(all, s)
		}
		st.mu.Unlock()
		for _, s := range all {
			s.Close()
		}
		<-st.sweepDone
	})
}

func (st *Store) sweepLoop() {
	defer close(st.sweepDone)
	tick := time.NewTicker(st.sweepEvery)
	defer tick.Stop()
	for {
		select {
		case <-st.stop:
			return
		case now := <-tick.C:
			st.sweep(now)
		}
	}
}

// sweep removes detached sessions whose last heartbeat or websocket
// activity is older than the heartbeat timeout, or older than the idle
// TTL. The client count is read under Store.mu. A session with clients
// greater than zero is left alone, including one whose beacon already
// fired and whose clocks are stale.
func (st *Store) sweep(now time.Time) {
	st.mu.Lock()
	var dead []*Session
	reasons := make([]string, 0)
	for id, s := range st.sessions {
		if s.clients > 0 {
			continue
		}
		quiet := now.Sub(s.lastSeenLocked())
		if quiet < st.heartbeat && quiet < st.idleTTL {
			continue
		}
		reason := "idle"
		if quiet < st.idleTTL {
			reason = "heartbeat"
		}
		st.disarmLocked(s)
		delete(st.sessions, id)
		dead = append(dead, s)
		reasons = append(reasons, reason)
	}
	st.mu.Unlock()
	for i, s := range dead {
		st.finish(s, reasons[i])
	}
}

// Attach records one live frames subscription and cancels a pending end.
// The session stays in the registry. ok is false when id is unknown or
// the store is closed.
func (st *Store) Attach(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return nil, false
	}
	s, ok := st.sessions[id]
	if !ok {
		return nil, false
	}
	s.clients++
	s.closing = false
	s.lastSocket = time.Now()
	st.disarmLocked(s)
	return s, true
}

// Detach records that one frames subscription ended. The last one arms
// the grace timer. A subscription that reconnects before it fires calls
// Attach, which disarms the timer.
func (st *Store) Detach(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return
	}
	s, ok := st.sessions[id]
	if !ok || s.clients == 0 {
		return
	}
	s.clients--
	s.lastSocket = time.Now()
	if s.clients == 0 {
		st.armLocked(s)
	}
}

// Heartbeat records that the open tab pinged. It does not attach a
// websocket and it does not cancel a grace timer by itself: the timer
// reads this timestamp when it fires and waits out the heartbeat window
// when the ping is fresh. Unknown ids report false.
func (st *Store) Heartbeat(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return false
	}
	s, ok := st.sessions[id]
	if !ok {
		return false
	}
	s.lastHeartbeat = time.Now()
	return true
}

// Release is the end-session beacon. It arms the grace timer. The timer
// does not remove the session while a frames subscription is still
// attached; a reload that subscribes again disarms it. Unknown ids
// report false and leave the registry unchanged.
func (st *Store) Release(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return false
	}
	s, ok := st.sessions[id]
	if !ok {
		return false
	}
	s.closing = true
	if s.clients == 0 {
		st.armLocked(s)
	}
	if debugEnabled() {
		slog.Debug("session end requested", "session", id, "grace", st.grace, "clients", s.clients)
	}
	return true
}

func (st *Store) disarmLocked(s *Session) {
	s.leaseGen++
	if s.leaseTimer != nil {
		s.leaseTimer.Stop()
		s.leaseTimer = nil
	}
}

func (st *Store) armLocked(s *Session) {
	st.armAfterLocked(s, st.grace)
}

func (st *Store) armAfterLocked(s *Session, delay time.Duration) {
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	s.leaseGen++
	gen := s.leaseGen
	id := s.id
	if s.leaseTimer != nil {
		s.leaseTimer.Stop()
	}
	s.leaseTimer = time.AfterFunc(delay, func() {
		st.expire(id, gen)
	})
}

// lastSeenLocked is the later of the last UI heartbeat and the last
// websocket attach, detach, or create. Caller holds Store.mu.
func (s *Session) lastSeenLocked() time.Time {
	if s.lastHeartbeat.After(s.lastSocket) {
		return s.lastHeartbeat
	}
	return s.lastSocket
}

func (st *Store) expire(id string, gen uint64) {
	st.mu.Lock()
	s, ok := st.sessions[id]
	if !ok || st.closed || s.leaseGen != gen {
		st.mu.Unlock()
		return
	}
	// Re-check under the lock. A client that re-attached, or that was
	// still attached when a beacon armed this timer, keeps the session.
	// The generation bump in Attach already ignores this callback; the
	// client count is the second check for a timer that was not disarmed.
	if s.clients > 0 {
		s.leaseTimer = nil
		st.mu.Unlock()
		return
	}
	silence := time.Since(s.lastSeenLocked())
	if silence < st.heartbeat {
		st.armAfterLocked(s, st.heartbeat-silence)
		st.mu.Unlock()
		return
	}
	reason := "grace"
	if s.closing {
		reason = "beacon"
	}
	delete(st.sessions, id)
	s.leaseTimer = nil
	st.mu.Unlock()
	st.finish(s, reason)
}

func (st *Store) finish(s *Session, reason string) {
	slog.Info("session ended", "session", s.ID(), "reason", reason)
	s.Close()
}

// BrowserSessionPath reports the routes that must not carry
// Access-Control-Allow-Origin: *. They are same-origin POSTs from the
// web UI (the end beacon and the heartbeat).
func BrowserSessionPath(path string) bool {
	switch path {
	case "/session/end", "/session/heartbeat":
		return true
	default:
		return false
	}
}

// EndHandler is POST /session/end. The body is the session id as
// text/plain (what navigator.sendBeacon sends), a JSON object with id
// or sessionId, or a form field id. A query parameter id is used when
// the body is empty. The handler schedules the grace period; it does
// not stop the session on this request, so a reload can subscribe again
// and keep it. An unknown id is still 204. A cross-origin request is 403
// and does not schedule anything.
func EndHandler(st *Store) http.Handler {
	return sessionPost(func(id string) { st.Release(id) })
}

// HeartbeatHandler is POST /session/heartbeat. The body is the session
// id, in the same shapes as /session/end. A same-origin ping records
// last-seen. A cross-origin request is 403 and does not.
func HeartbeatHandler(st *Store) http.Handler {
	return sessionPost(func(id string) { st.Heartbeat(id) })
}

func sessionPost(fn func(string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		id, err := endSessionID(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fn(id)
		w.WriteHeader(http.StatusNoContent)
	})
}

// sameOrigin accepts a request whose Origin host equals the request
// host. When Origin is absent, Referer's host is used instead. A request
// with neither header is rejected. The comparison is the host as sent,
// including a non-default port.
func sameOrigin(r *http.Request) bool {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return false
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return hostEqual(originHost(origin), host)
	}
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
		return hostEqual(originHost(ref), host)
	}
	return false
}

func originHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func hostEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}

func endSessionID(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("invalid session id")
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	id := ""
	switch media {
	case "application/json":
		var payload struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionId"`
		}
		if len(bytesTrim(raw)) == 0 {
			break
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", fmt.Errorf("invalid session id")
		}
		id = payload.ID
		if id == "" {
			id = payload.SessionID
		}
	case "application/x-www-form-urlencoded":
		vals, err := url.ParseQuery(string(raw))
		if err != nil {
			return "", fmt.Errorf("invalid session id")
		}
		id = vals.Get("id")
	default:
		id = string(raw)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if !validSessionID(id) {
		return "", fmt.Errorf("invalid session id")
	}
	return id, nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func validSessionID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}
