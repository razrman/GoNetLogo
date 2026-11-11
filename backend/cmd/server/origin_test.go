package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"gonetlogo/backend/session"
)

func TestResolveProductionAndOrigins(t *testing.T) {
	on, err := resolveProduction(false, false, func(string) string { return "" })
	if err != nil || on {
		t.Fatalf("default %v %v", on, err)
	}
	on, err = resolveProduction(false, false, func(key string) string {
		if key == envProduction {
			return " YES "
		}
		return ""
	})
	if err != nil || !on {
		t.Fatalf("env %v %v", on, err)
	}
	on, err = resolveProduction(true, false, func(string) string { return "true" })
	if err != nil || on {
		t.Fatalf("explicit false %v %v, want the flag to win", on, err)
	}
	if _, err := resolveProduction(false, false, func(string) string { return "maybe" }); err == nil || !strings.Contains(err.Error(), envProduction) {
		t.Fatalf("bad env err = %v", err)
	}

	got, err := parseAllowedOrigins(" https://App.Example:443 , http://localhost:43132, https://app.example ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://app.example", "http://localhost:43132"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("origins %v", got)
	}
	if _, err := parseAllowedOrigins("https://app.example, nope"); err == nil {
		t.Fatal("invalid origin accepted")
	}
	if _, err := parseAllowedOrigins("https://app.example/graphql"); err == nil {
		t.Fatal("origin with a path accepted")
	}
	empty, err := resolveAllowedOrigins(true, "", func(string) string { return "https://app.example" })
	if err != nil || len(empty) != 0 {
		t.Fatalf("explicit empty %v %v", empty, err)
	}
	fromEnv, err := resolveAllowedOrigins(false, "", func(key string) string {
		if key == envAllowedOrigins {
			return "http://127.0.0.1:8080"
		}
		return ""
	})
	if err != nil || len(fromEnv) != 1 || fromEnv[0] != "http://127.0.0.1:8080" {
		t.Fatalf("env origins %v %v", fromEnv, err)
	}
}

func TestProductionCORSReflectsTheAllowList(t *testing.T) {
	pol := newOriginPolicy(true, []string{"https://app.example"})
	h := withCORS(pol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	allowed.Host = "api.internal"
	allowed.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, allowed)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("allowed status %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allowed origin %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("vary %q", got)
	}

	cased := httptest.NewRequest(http.MethodOptions, "/graphql", nil)
	cased.Host = "api.internal"
	cased.Header.Set("Origin", "https://APP.Example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, cased)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://APP.Example" {
		t.Fatalf("cased origin %q", got)
	}

	evil := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	evil.Host = "api.internal"
	evil.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, evil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("evil origin %q", got)
	}

	same := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	same.Host = "app.example:8080"
	same.Header.Set("Origin", "http://app.example:8080")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, same)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://app.example:8080" {
		t.Fatalf("same-origin %q", got)
	}

	plain := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	plain.Host = "app.example"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, plain)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no-origin header %q", got)
	}
}

func TestProductionWithoutAllowListIsSameOriginOnly(t *testing.T) {
	h := withCORS(newOriginPolicy(true, nil), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	cross := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	cross.Host = "app.example"
	cross.Header.Set("Origin", "https://other.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, cross)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("cross origin %q", got)
	}
	same := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	same.Host = "app.example"
	same.Header.Set("Origin", "https://app.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, same)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("same origin %q", got)
	}
}

func TestDevCORSStaysWildcard(t *testing.T) {
	called := false
	h := withCORS(newOriginPolicy(false, []string{"https://app.example"}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Origin", "http://127.0.0.1:43132")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("dev request was not forwarded")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("dev origin %q", got)
	}

	store := session.NewStore()
	t.Cleanup(store.Close)
	mux := http.NewServeMux()
	mux.Handle("/session/end", session.EndHandler(store))
	prod := withCORS(newOriginPolicy(true, []string{"https://app.example"}), mux)
	beacon := httptest.NewRequest(http.MethodPost, "/session/end", strings.NewReader("0123456789abcdef"))
	beacon.Host = "app.example"
	beacon.Header.Set("Origin", "https://app.example")
	rec = httptest.NewRecorder()
	prod.ServeHTTP(rec, beacon)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("production beacon origin %q", got)
	}
}

func TestIntrospectionFollowsProductionMode(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)
	query := `{"query":"{ __schema { queryType { name } } }"}`

	off := postJSON(newHandler(store, newOriginPolicy(true, nil)), query)
	if !strings.Contains(off, "introspection disabled") {
		t.Fatalf("production introspection %s", off)
	}
	on := postJSON(newHandler(store, originPolicy{}), query)
	if strings.Contains(on, "introspection disabled") || !strings.Contains(on, "Query") {
		t.Fatalf("dev introspection %s", on)
	}
}

func TestPlaygroundDisabledInProduction(t *testing.T) {
	ui := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>app shell</html>")}}
	prod := http.NewServeMux()
	mountUI(prod, ui, true)
	if body, code := getBody(prod, "/playground"); code != http.StatusNotFound || strings.Contains(body, "GoNetLogo GraphQL") {
		t.Fatalf("production playground %d %s", code, body)
	}
	if body, code := getBody(prod, "/"); code != http.StatusOK || !strings.Contains(body, "app shell") {
		t.Fatalf("production app %d %s", code, body)
	}

	bare := http.NewServeMux()
	mountUI(bare, nil, true)
	if _, code := getBody(bare, "/"); code != http.StatusNotFound {
		t.Fatalf("production / without a UI: %d", code)
	}
	if _, code := getBody(bare, "/playground"); code != http.StatusNotFound {
		t.Fatalf("production playground without a UI: %d", code)
	}

	dev := http.NewServeMux()
	mountUI(dev, nil, false)
	if body, code := getBody(dev, "/"); code != http.StatusOK || !strings.Contains(body, "GoNetLogo GraphQL") {
		t.Fatalf("dev playground %d %s", code, body)
	}
	withUI := http.NewServeMux()
	mountUI(withUI, ui, false)
	if body, code := getBody(withUI, "/playground"); code != http.StatusOK || !strings.Contains(body, "GoNetLogo GraphQL") {
		t.Fatalf("dev playground beside the app %d %s", code, body)
	}
}

func TestWebsocketOriginFollowsProductionMode(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)

	prod := httptest.NewServer(newHandler(store, newOriginPolicy(true, []string{"https://app.example"})))
	t.Cleanup(prod.Close)
	if got := websocketStatus(t, prod.URL, "https://evil.example"); got != http.StatusForbidden {
		t.Fatalf("production evil origin %d", got)
	}
	if got := websocketStatus(t, prod.URL, "https://app.example"); got == http.StatusForbidden {
		t.Fatal("production allow-list origin was rejected")
	}
	if got := websocketStatus(t, prod.URL, prod.URL); got == http.StatusForbidden {
		t.Fatal("production same-origin websocket was rejected")
	}

	sameOnly := httptest.NewServer(newHandler(store, newOriginPolicy(true, nil)))
	t.Cleanup(sameOnly.Close)
	if got := websocketStatus(t, sameOnly.URL, "https://app.example"); got != http.StatusForbidden {
		t.Fatalf("same-origin-only foreign origin %d", got)
	}
	if got := websocketStatus(t, sameOnly.URL, sameOnly.URL); got == http.StatusForbidden {
		t.Fatal("same-origin-only matching origin was rejected")
	}

	dev := httptest.NewServer(newHandler(store, originPolicy{}))
	t.Cleanup(dev.Close)
	if got := websocketStatus(t, dev.URL, "https://evil.example"); got == http.StatusForbidden {
		t.Fatal("dev mode rejected a foreign websocket origin")
	}
}

func postJSON(h http.Handler, body string) string {
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String()
}

func getBody(h http.Handler, path string) (string, int) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Body.String(), rec.Code
}

func websocketStatus(t *testing.T, rawURL, origin string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")
	req.Header.Set("Origin", origin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// A successful upgrade has no finite body. Reading it waits forever.
	resp.Body.Close()
	return resp.StatusCode
}
