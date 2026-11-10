package session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func shortLife(grace time.Duration) Lifecycle {
	return Lifecycle{Grace: grace, Idle: time.Hour, Sweep: time.Hour}
}

func waitGone(t *testing.T, st *Store, id string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := st.Get(id); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %s still registered", id)
}

func waitLoops(t *testing.T, s *Session) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		s.loops.Wait()
		close(done)
	}()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatal("session goroutines still running")
	}
}

func waitQuiet(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var n int
	for {
		runtime.GC()
		n = runtime.NumGoroutine()
		if n <= before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines %d, want <= %d", n, before)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDisconnectKillsAfterGrace(t *testing.T) {
	grace := 80 * time.Millisecond
	st := NewStoreWith(shortLife(grace))
	t.Cleanup(st.Close)
	s, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Attach(s.ID()); !ok {
		t.Fatal("attach")
	}
	time.Sleep(grace + 40*time.Millisecond)
	if _, ok := st.Get(s.ID()); !ok {
		t.Fatal("connected session ended at the grace deadline")
	}

	// A second subscriber keeps the session when the first one leaves.
	if _, ok := st.Attach(s.ID()); !ok {
		t.Fatal("second attach")
	}
	st.Detach(s.ID())
	time.Sleep(grace + 40*time.Millisecond)
	if _, ok := st.Get(s.ID()); !ok {
		t.Fatal("session ended while another subscriber was connected")
	}

	st.Detach(s.ID())
	if _, ok := st.Get(s.ID()); !ok {
		t.Fatal("session ended before the grace period")
	}
	waitGone(t, st, s.ID())
}

func TestReconnectWithinGraceKeepsSession(t *testing.T) {
	grace := 80 * time.Millisecond
	st := NewStoreWith(shortLife(grace))
	t.Cleanup(st.Close)

	t.Run("subscription", func(t *testing.T) {
		s, err := st.Create("fire.nlogo")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := st.Attach(s.ID()); !ok {
			t.Fatal("attach")
		}
		st.Detach(s.ID())
		if _, ok := st.Attach(s.ID()); !ok {
			t.Fatal("reconnect")
		}
		time.Sleep(grace + 50*time.Millisecond)
		if _, ok := st.Get(s.ID()); !ok {
			t.Fatal("reconnect did not cancel the grace timer")
		}
		st.Detach(s.ID())
	})

	t.Run("beacon", func(t *testing.T) {
		s, err := st.Create("fire.nlogo")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := st.Attach(s.ID()); !ok {
			t.Fatal("attach")
		}
		if !st.Release(s.ID()) {
			t.Fatal("beacon")
		}
		if _, ok := st.Attach(s.ID()); !ok {
			t.Fatal("reconnect after beacon")
		}
		time.Sleep(grace + 50*time.Millisecond)
		if _, ok := st.Get(s.ID()); !ok {
			t.Fatal("reconnect did not cancel the beacon")
		}
		st.Detach(s.ID())
	})
}

func TestDisconnectStopsRunningGo(t *testing.T) {
	before := runtime.NumGoroutine()
	grace := 60 * time.Millisecond
	st := NewStoreWith(shortLife(grace))
	t.Cleanup(st.Close)
	s, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.keepUpload("dropped.nlogo", "to setup\n  clear-all\nend\n")
	s.mu.Lock()
	scratch := s.scratch
	s.mu.Unlock()
	uploaded := filepath.Join(scratch, "dropped.nlogo")
	if _, err := os.Stat(uploaded); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	note := filepath.Join(dir, "note.txt")
	s.mu.Lock()
	s.vm.ModelDir = dir
	s.mu.Unlock()
	if _, err := s.RunCommand(`file-open "note.txt"`); err != nil {
		t.Fatal(err)
	}
	if n := openFDs(note); n < 1 {
		t.Fatalf("open files %d, want the note held open", n)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for s.Session().Ticks == nil || *s.Session().Ticks <= 0 {
		if time.Now().After(deadline) {
			t.Fatal("go did not advance ticks")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !s.Running() {
		t.Fatal("go was not running")
	}
	if _, ok := st.Attach(s.ID()); !ok {
		t.Fatal("attach")
	}
	st.Detach(s.ID())
	waitGone(t, st, s.ID())
	if s.Running() {
		t.Fatal("go still running after the session ended")
	}
	waitLoops(t, s)
	held := s.Session().Ticks
	time.Sleep(80 * time.Millisecond)
	if got := s.Session().Ticks; held == nil || got == nil || *got != *held {
		t.Fatalf("ticks moved after stop, %v -> %v", held, got)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		_, statErr := os.Stat(scratch)
		fds := openFDs(note)
		if os.IsNotExist(statErr) && fds == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resources remain: scratch %v open files %d", statErr, fds)
		}
		time.Sleep(10 * time.Millisecond)
	}
	st.Close()
	waitQuiet(t, before)
}

func TestBeaconEndsSession(t *testing.T) {
	grace := 60 * time.Millisecond
	st := NewStoreWith(shortLife(grace))
	t.Cleanup(st.Close)
	s, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.keepUpload("pace.nlogo", nlogoPace)
	s.mu.Lock()
	scratch := s.scratch
	s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(scratch, "pace.nlogo")); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	handler := EndHandler(st)
	req := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader(s.ID()))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if _, ok := st.Get(s.ID()); !ok {
		t.Fatal("beacon ended the session before the grace period")
	}
	waitGone(t, st, s.ID())
	if s.Running() {
		t.Fatal("go still running after the beacon")
	}
	waitLoops(t, s)
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("uploaded model storage %v", err)
	}

	// A second call for a session that is already gone is still success.
	// The first request's body has been consumed, so this is a new POST.
	again := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader(s.ID()))
	again.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, again)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("missing session status %d body %s", rec.Code, rec.Body.String())
	}

	jsonReq := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader(`{"sessionId":"0123456789abcdef"}`))
	jsonReq.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, jsonReq)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("json status %d", rec.Code)
	}

	bad := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader("nope"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id status %d", rec.Code)
	}
	get := httptest.NewRequest(http.MethodGet, "/session/end", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("get status %d", rec.Code)
	}
}

func TestIdleSweepRemovesOrphan(t *testing.T) {
	st := NewStoreWith(Lifecycle{Grace: time.Hour, Idle: time.Hour, Sweep: time.Hour})
	t.Cleanup(st.Close)
	s, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Attach(s.ID()); !ok {
		t.Fatal("attach")
	}
	st.mu.Lock()
	s.lastTouch = time.Now().Add(-2 * time.Hour)
	st.mu.Unlock()
	st.sweep(time.Now())
	if _, ok := st.Get(s.ID()); !ok {
		t.Fatal("sweep removed a connected session")
	}
	st.Detach(s.ID())
	// Detach refreshed lastTouch and armed a long grace timer. An orphan
	// is one that nobody is subscribed to and that has been quiet.
	st.mu.Lock()
	s.lastTouch = time.Now().Add(-2 * time.Hour)
	st.mu.Unlock()
	st.sweep(time.Now())
	if _, ok := st.Get(s.ID()); ok {
		t.Fatal("idle session was not removed")
	}
	waitLoops(t, s)

	live := NewStoreWith(Lifecycle{Grace: time.Hour, Idle: 40 * time.Millisecond, Sweep: 10 * time.Millisecond})
	t.Cleanup(live.Close)
	orphan, err := live.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	waitGone(t, live, orphan.ID())
	waitLoops(t, orphan)
}

func TestResolveLifecycle(t *testing.T) {
	got, err := ResolveLifecycle("", "", "", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultLifecycle()
	if got != want {
		t.Fatalf("defaults %+v, want %+v", got, want)
	}
	env := map[string]string{EnvGrace: "12s", EnvIdle: "2m"}
	got, err = ResolveLifecycle("", "", "", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if got.Grace != 12*time.Second || got.Idle != 2*time.Minute || got.Sweep != want.Sweep {
		t.Fatalf("env %+v", got)
	}
	got, err = ResolveLifecycle("20s", "", "5s", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if got.Grace != 20*time.Second || got.Idle != 2*time.Minute || got.Sweep != 5*time.Second {
		t.Fatalf("flag %+v", got)
	}
	if _, err := ResolveLifecycle("nope", "", "", func(string) string { return "" }); err == nil {
		t.Fatal("invalid duration accepted")
	}
	if _, err := ResolveLifecycle("30s", "10s", "", func(string) string { return "" }); err == nil {
		t.Fatal("idle shorter than grace accepted")
	}
}

func openFDs(path string) int {
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		want = path
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	n := 0
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil {
			continue
		}
		got, err := filepath.EvalSymlinks(target)
		if err != nil {
			got = target
		}
		if got == want {
			n++
		}
	}
	return n
}
