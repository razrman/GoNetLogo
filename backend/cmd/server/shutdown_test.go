package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"gonetlogo/backend/session"
)

func TestResolveShutdownTimeout(t *testing.T) {
	d, err := resolveShutdownTimeout("", func(string) string { return "" })
	if err != nil || d != defaultShutdownTimeout {
		t.Fatalf("default %s %v", d, err)
	}
	d, err = resolveShutdownTimeout("", func(key string) string {
		if key == envShutdownTimeout {
			return " 5s "
		}
		return ""
	})
	if err != nil || d != 5*time.Second {
		t.Fatalf("env %s %v", d, err)
	}
	d, err = resolveShutdownTimeout("8s", func(string) string { return "5s" })
	if err != nil || d != 8*time.Second {
		t.Fatalf("flag %s %v", d, err)
	}
	if _, err := resolveShutdownTimeout("0s", nil); err == nil {
		t.Fatal("zero duration accepted")
	}
	if _, err := resolveShutdownTimeout("", func(string) string { return "nope" }); err == nil || !strings.Contains(err.Error(), envShutdownTimeout) {
		t.Fatalf("bad env err = %v", err)
	}
}

func TestShutdownDrainsThenClosesStore(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)
	s, err := store.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	id := s.ID()

	entered := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
			return
		}
	})
	srv, addr := serveLocal(t, mux)

	code := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/hold")
		if err != nil {
			code <- 0
			return
		}
		resp.Body.Close()
		code <- resp.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach the handler")
	}
	go func() {
		time.Sleep(40 * time.Millisecond)
		close(release)
	}()

	if err := shutdownHTTP(srv, 2*time.Second, store.Close); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, ok := store.Get(id); ok {
		t.Fatal("session still open after shutdown")
	}
	select {
	case got := <-code:
		if got != http.StatusNoContent {
			t.Fatalf("drained status %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not finish")
	}
}

func TestShutdownTimeoutThenClosesStore(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)
	s, err := store.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	id := s.ID()

	entered := make(chan struct{})
	finished := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(finished)
	})
	srv, addr := serveLocal(t, mux)
	go func() {
		resp, err := http.Get("http://" + addr + "/hold")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach the handler")
	}

	start := time.Now()
	err = shutdownHTTP(srv, 150*time.Millisecond, store.Close)
	if err == nil {
		t.Fatal("expected the drain to hit its deadline")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("shutdown took %s", time.Since(start))
	}
	if _, ok := store.Get(id); ok {
		t.Fatal("session still open after a timed-out shutdown")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("request was not canceled after the drain deadline")
	}
}

func serveLocal(t *testing.T, h http.Handler) (*http.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() { _ = srv.Close() })
	return srv, ln.Addr().String()
}
