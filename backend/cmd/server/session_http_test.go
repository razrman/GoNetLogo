package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gonetlogo/backend/session"
)

func TestSessionRoutesOmitWildcardCORS(t *testing.T) {
	store := session.NewStoreWith(session.Lifecycle{
		Grace:     40 * time.Millisecond,
		Idle:      time.Hour,
		Sweep:     time.Hour,
		Heartbeat: 40 * time.Millisecond,
	})
	t.Cleanup(store.Close)
	s, err := store.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/graphql", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("/session/end", session.EndHandler(store))
	mux.Handle("/session/heartbeat", session.HeartbeatHandler(store))
	h := cors(mux)

	graphql := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, graphql)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("graphql allow-origin %q", got)
	}

	beat := httptest.NewRequest(http.MethodPost, "/session/heartbeat", strings.NewReader(s.ID()))
	beat.Host = "app.example:43131"
	beat.Header.Set("Origin", "http://app.example:43131")
	beat.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, beat)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("heartbeat %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("heartbeat allow-origin %q", got)
	}

	cross := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader(s.ID()))
	cross.Host = "app.example:43131"
	cross.Header.Set("Origin", "http://evil.example")
	cross.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, cross)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin end %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("cross-origin end allow-origin %q", got)
	}
	time.Sleep(150 * time.Millisecond)
	if _, ok := store.Get(s.ID()); !ok {
		t.Fatal("cross-origin end removed the session")
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/session/end", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, preflight)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("preflight allow-origin %q", got)
	}
}
