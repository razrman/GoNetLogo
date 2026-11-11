package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzIsOutsideCORS(t *testing.T) {
	graphqlCalled := false
	mux := http.NewServeMux()
	mux.Handle("/graphql", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphqlCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	h := rootHandler(cors(mux))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if graphqlCalled {
		t.Fatal("healthz touched /graphql")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Body.String() != "ok\n" {
		t.Fatalf("body %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content-type %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("healthz allow-origin %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control %q", got)
	}

	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status %d", post.Code)
	}
	if got := post.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow %q", got)
	}
	if got := post.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("POST allow-origin %q", got)
	}
	if graphqlCalled {
		t.Fatal("rejected healthz touched /graphql")
	}

	gql := httptest.NewRecorder()
	h.ServeHTTP(gql, httptest.NewRequest(http.MethodPost, "/graphql", nil))
	if !graphqlCalled {
		t.Fatal("graphql was not forwarded")
	}
	if got := gql.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("graphql allow-origin %q", got)
	}
}
