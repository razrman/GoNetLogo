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
// Grace is the quiet period after the last frames subscription drops,
// or after the end-session beacon, before the session is stopped and
// removed. A new subscription in that window cancels the removal, so a
// reload or a short network blip keeps the same interpreter and world.
// Idle is the safety net for a session that never connected, or whose
// disconnect was missed: with no frames subscription and no request for
// this long, the sweep removes it. Sweep is how often that check runs.
type Lifecycle struct {
	Grace time.Duration
	Idle  time.Duration
	Sweep time.Duration
}

const (
	// EnvGrace, EnvIdle, and EnvSweep override the defaults when the
	// matching server flag is omitted.
	EnvGrace = "GONETLOGO_SESSION_GRACE"
	EnvIdle  = "GONETLOGO_SESSION_IDLE"
	EnvSweep = "GONETLOGO_SESSION_SWEEP"
)

// DefaultLifecycle keeps a disconnected client for 15 seconds and sweeps
// sessions that have been idle for 5 minutes every 30 seconds.
func DefaultLifecycle() Lifecycle {
	return Lifecycle{
		Grace: 15 * time.Second,
		Idle:  5 * time.Minute,
		Sweep: 30 * time.Second,
	}
}

// ResolveLifecycle builds a Lifecycle. A non-empty flag wins over the
// environment, and an empty environment value keeps the default. Idle
// must be at least Grace so the sweep cannot remove a session that is
// still inside its reconnect window.
func ResolveLifecycle(graceFlag, idleFlag, sweepFlag string, getenv func(string) string) (Lifecycle, error) {
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
	if base.Idle < base.Grace {
		return Lifecycle{}, fmt.Errorf("session idle TTL %s is shorter than the grace period %s", base.Idle, base.Grace)
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
	default:
		return env
	}
}

// NewStoreWith starts an empty store that ends disconnected sessions on lc.
func NewStoreWith(lc Lifecycle) *Store {
	if lc.Grace <= 0 || lc.Idle <= 0 || lc.Sweep <= 0 {
		panic("session lifecycle durations must be positive")
	}
	st := &Store{
		sessions:   map[string]*Session{},
		grace:      lc.Grace,
		idleTTL:    lc.Idle,
		sweepEvery: lc.Sweep,
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

// sweep removes sessions that have no frames subscription and have not
// been touched for the idle TTL. A connected client is left alone. A
// beacon already scheduled its own timer; this is the backstop.
func (st *Store) sweep(now time.Time) {
	st.mu.Lock()
	var dead []*Session
	for id, s := range st.sessions {
		if s.clients > 0 && !s.closing {
			continue
		}
		if now.Sub(s.lastTouch) < st.idleTTL {
			continue
		}
		st.disarmLocked(s)
		delete(st.sessions, id)
		dead = append(dead, s)
	}
	st.mu.Unlock()
	for _, s := range dead {
		st.finish(s, "idle")
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
	s.lastTouch = time.Now()
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
	s.lastTouch = time.Now()
	if s.clients == 0 {
		st.armLocked(s)
	}
}

// Release is the end-session beacon. The session is stopped after the
// grace period even if the websocket has not closed yet, unless Attach
// runs first and cancels it. A page reload fires the beacon and then
// subscribes again; the subscribe wins when it arrives inside the window.
// Unknown ids report false and leave the registry unchanged.
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
	s.lastTouch = time.Now()
	st.armLocked(s)
	if debugEnabled() {
		slog.Debug("session end requested", "session", id, "grace", st.grace)
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
	s.leaseGen++
	gen := s.leaseGen
	id := s.id
	if s.leaseTimer != nil {
		s.leaseTimer.Stop()
	}
	s.leaseTimer = time.AfterFunc(st.grace, func() {
		st.expire(id, gen)
	})
}

func (st *Store) expire(id string, gen uint64) {
	st.mu.Lock()
	s, ok := st.sessions[id]
	if !ok || st.closed || s.leaseGen != gen || (s.clients > 0 && !s.closing) {
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

// EndHandler is POST /session/end. The body is the session id as
// text/plain (what navigator.sendBeacon sends), a JSON object with id
// or sessionId, or a form field id. A query parameter id is used when
// the body is empty. The handler schedules the grace period; it does
// not stop the session on this request, so a reload can subscribe again
// and keep it. An unknown id is still 204.
func EndHandler(st *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id, err := endSessionID(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		st.Release(id)
		w.WriteHeader(http.StatusNoContent)
	})
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
