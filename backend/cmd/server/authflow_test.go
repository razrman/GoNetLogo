package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gonetlogo/backend/session"
	"gonetlogo/internal/auth"
)

func TestNonAdminSignupAndLoginOwnTheModelTheyRun(t *testing.T) {
	srv, jar := authFlowServer(t)
	token := flowCSRF(t, srv)
	user := flowPost(t, srv, "/auth/signup", token, `{"email":"pat@example.com","password":"correct-horse","displayName":"Pat"}`, "")
	if user["role"] != auth.RoleUser {
		t.Fatalf("role %#v", user["role"])
	}
	created := flowGraphQL(t, srv, flowCookie(t, srv), `mutation { createSession(model: "fire.nlogo") { id modelName } }`)
	sessionID := created["id"].(string)
	if created["modelName"] != "fire.nlogo" || sessionID == "" {
		t.Fatalf("create %#v", created)
	}
	if !flowHasWorkspace(t, srv, sessionID, "fire.nlogo") {
		t.Fatal("signup then run did not list the model")
	}

	token = flowCSRF(t, srv)
	flowPost(t, srv, "/auth/logout", token, `{}`, flowCookie(t, srv))
	jar.SetCookies(flowURL(t, srv.URL), nil)
	token = flowCSRF(t, srv)
	again := flowPost(t, srv, "/auth/login", token, `{"email":"pat@example.com","password":"correct-horse"}`, "")
	if again["role"] != auth.RoleUser {
		t.Fatalf("login role %#v", again["role"])
	}
	second := flowGraphQL(t, srv, flowCookie(t, srv), `mutation { createSession(model: "fire.nlogo") { id modelName } }`)
	if !flowHasWorkspace(t, srv, second["id"].(string), "fire.nlogo") {
		t.Fatal("login then run did not list the model")
	}
}

func TestModelOpenedBeforeSignupIsClaimed(t *testing.T) {
	srv, _ := authFlowServer(t)
	opened := flowGraphQL(t, srv, "", `mutation { createSession(model: "fire.nlogo") { id } }`)
	id := opened["id"].(string)
	token := flowCSRF(t, srv)
	user := flowPost(t, srv, "/auth/signup", token, `{"email":"sam@example.com","password":"correct-horse","displayName":"Sam"}`, "")
	if user["role"] != auth.RoleUser {
		t.Fatalf("role %#v", user["role"])
	}
	// The model was already running. The next model operation attaches it.
	flowGraphQL(t, srv, flowCookie(t, srv), `query ($id: ID!) { session(id: $id) { id } }`, id)
	if !flowHasWorkspace(t, srv, id, "fire.nlogo") {
		t.Fatal("the session that was open at sign-up was not attached")
	}
}

func TestAccountCountIsOnlyThatUsersSessions(t *testing.T) {
	srv, _ := authFlowServer(t)
	pat := &http.Client{Jar: mustJar(t)}
	ada := &http.Client{Jar: mustJar(t)}

	signupAs(t, srv, pat, "pat@example.com", "Pat")
	patList := ensureAs(t, srv, pat, "")
	if patList.OpenCount != 1 || len(patList.Workspaces) != 1 || patList.MaxSessions != 4 {
		t.Fatalf("pat after ensure %+v", patList)
	}
	again := ensureAs(t, srv, pat, patList.Workspaces[0].ID)
	if again.OpenCount != 1 || again.Workspaces[0].ID != patList.Workspaces[0].ID {
		t.Fatalf("second ensure created another session %+v", again)
	}

	// An anonymous model session is not part of anyone's count.
	anon := flowGraphQL(t, srv, "", `mutation { createSession(model: "fire.nlogo") { id } }`)
	anonID := anon["id"].(string)
	if ensureAs(t, srv, pat, "").OpenCount != 1 {
		t.Fatal("anonymous session changed pat's count")
	}

	signupAs(t, srv, ada, "ada@example.com", "Ada")
	adaList := ensureAs(t, srv, ada, "")
	if adaList.OpenCount != 1 || adaList.MaxSessions != 4 {
		t.Fatalf("ada %+v", adaList)
	}
	if adaList.Workspaces[0].ID == patList.Workspaces[0].ID || adaList.Workspaces[0].ID == anonID {
		t.Fatal("ada was given someone else's session")
	}
	if ensureAs(t, srv, pat, "").OpenCount != 1 {
		t.Fatal("ada's session was counted for pat")
	}

	ended := endAs(t, srv, pat, patList.Workspaces[0].ID)
	if !ended {
		t.Fatal("ending the last session did not log pat out")
	}
	if user := statusAs(t, srv, pat); user != nil {
		t.Fatalf("pat still signed in %#v", user)
	}
	if ensureAs(t, srv, ada, "").OpenCount != 1 {
		t.Fatal("logging pat out changed ada's sessions")
	}
}

func TestLogoutKeepsTheModelForTheNextLogin(t *testing.T) {
	srv, _ := authFlowServer(t)
	pat := &http.Client{Jar: mustJar(t)}
	ada := &http.Client{Jar: mustJar(t)}
	signupAs(t, srv, pat, "pat@example.com", "Pat")
	signupAs(t, srv, ada, "ada@example.com", "Ada")
	patList := ensureAs(t, srv, pat, "")
	adaList := ensureAs(t, srv, ada, "")
	patID := patList.Workspaces[0].ID
	adaID := adaList.Workspaces[0].ID
	if patID == adaID {
		t.Fatal("two browser contexts shared one model session")
	}
	logoutAs(t, srv, pat)
	if user := statusAs(t, srv, pat); user != nil {
		t.Fatalf("pat still signed in %#v", user)
	}
	if ensureAs(t, srv, ada, "").Workspaces[0].ID != adaID {
		t.Fatal("pat logging out changed ada's session")
	}
	loginAs(t, srv, pat, "pat@example.com")
	again := ensureAs(t, srv, pat, "")
	if again.OpenCount != 1 || again.Workspaces[0].ID != patID || again.Workspaces[0].Status == "ending" {
		t.Fatalf("logout did not leave pat's model resumable %+v", again)
	}
}

func TestProductionStackLoginListsTheRunningModel(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)
	cfg := auth.DefaultConfig()
	cfg.Provider = "local"
	cfg.AllowAnonymous = true
	cfg.Cookie.Secure = true
	cfg.UsersFile = filepath.Join(t.TempDir(), "users.json")
	cfg.InitialAdminEmails = []string{"admin@example.com"}
	sessions := auth.NewSessionManager(cfg)
	provider, err := auth.NewProvider(cfg, sessions)
	if err != nil {
		t.Fatal(err)
	}
	users := provider.(interface{ Users() auth.UserStore }).Users()
	h := &auth.Handler{
		Config: cfg, Provider: provider, Sessions: sessions, Users: users, Models: &modelBridge{store: store},
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>ui</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	ui, err := openUI(dir)
	if err != nil {
		t.Fatal(err)
	}
	pol := originPolicy{production: true}
	mux := http.NewServeMux()
	mux.Handle("/graphql", newHandler(store, pol))
	h.Mount(mux)
	mountUI(mux, ui, true)
	var root http.Handler = auth.Middleware(provider, sessions)(withCORS(pol, mux))
	root = logHTTP(root)
	srv := httptest.NewServer(root)
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar

	page, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("index cache %q", page.Header.Get("Cache-Control"))
	}
	html, _ := io.ReadAll(page.Body)
	if !strings.Contains(string(html), "<title>ui</title>") {
		t.Fatalf("embedded ui route returned %q", html)
	}

	opened := flowGraphQL(t, srv, "", `mutation { createSession(model: "fire.nlogo") { id modelName } }`)
	modelID := opened["id"].(string)

	token := flowCSRF(t, srv)
	res := flowPostRaw(t, srv, "/auth/login", token, `{"email":"nope@example.com","password":"correct-horse"}`)
	res.Body.Close()
	// No such user yet. Sign up the non-admin and the admin through the same stack.
	token = flowCSRF(t, srv)
	pat := flowPost(t, srv, "/auth/signup", token, `{"email":"pat@example.com","password":"correct-horse","displayName":"Pat"}`, "")
	if pat["role"] != auth.RoleUser {
		t.Fatalf("pat role %#v", pat["role"])
	}
	login := flowPostRaw(t, srv, "/auth/login", flowCSRF(t, srv), `{"email":"pat@example.com","password":"correct-horse"}`)
	setCookie := login.Header.Get("Set-Cookie")
	login.Body.Close()
	if strings.Contains(setCookie, "Secure") {
		t.Fatalf("http login set a Secure cookie the browser would drop: %s", setCookie)
	}
	// The model was already running before this login. Asking for it with
	// the new cookie attaches it, which is what the page does on resume.
	flowGraphQL(t, srv, flowCookie(t, srv), `query ($id: ID!) { session(id: $id) { id modelName } }`, modelID)
	if !flowHasWorkspace(t, srv, modelID, "fire.nlogo") {
		t.Fatal("logged-in non-admin did not see the running model")
	}

	jar.SetCookies(flowURL(t, srv.URL), nil)
	token = flowCSRF(t, srv)
	admin := flowPost(t, srv, "/auth/signup", token, `{"email":"admin@example.com","password":"correct-horse","displayName":"Ada"}`, "")
	if admin["role"] != auth.RoleAdmin {
		t.Fatalf("admin role %#v", admin["role"])
	}
	created := flowGraphQL(t, srv, flowCookie(t, srv), `mutation { createSession(model: "fire.nlogo") { id modelName } }`)
	if !flowHasWorkspace(t, srv, created["id"].(string), "fire.nlogo") {
		t.Fatal("logged-in admin did not see the model started after sign-in")
	}
}

func flowPostRaw(t *testing.T, srv *httptest.Server, path, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: "gonetlogo_csrf", Value: token})
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func authFlowServer(t *testing.T) (*httptest.Server, http.CookieJar) {
	t.Helper()
	store := session.NewStore()
	t.Cleanup(store.Close)
	cfg := auth.DefaultConfig()
	cfg.Provider = "local"
	cfg.AllowAnonymous = true
	cfg.UsersFile = filepath.Join(t.TempDir(), "users.json")
	sessions := auth.NewSessionManager(cfg)
	provider, err := auth.NewProvider(cfg, sessions)
	if err != nil {
		t.Fatal(err)
	}
	h := &auth.Handler{
		Config:   cfg,
		Provider: provider,
		Sessions: sessions,
		Users:    provider.(interface{ Users() auth.UserStore }).Users(),
		Models:   &modelBridge{store: store},
	}
	mux := http.NewServeMux()
	mux.Handle("/graphql", newHandler(store, originPolicy{}))
	h.Mount(mux)
	srv := httptest.NewServer(auth.Middleware(provider, sessions)(mux))
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar
	return srv, jar
}

func flowCSRF(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	res, err := srv.Client().Get(srv.URL + "/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body["token"]
}

func flowCookie(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	var parts []string
	for _, c := range srv.Client().Jar.Cookies(flowURL(t, srv.URL)) {
		if c.Name == "gonetlogo_auth" || c.Name == "gonetlogo_csrf" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func flowPost(t *testing.T, srv *httptest.Server, path, token, body, cookie string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	if cookie == "" {
		req.AddCookie(&http.Cookie{Name: "gonetlogo_csrf", Value: token})
	} else if !strings.Contains(cookie, "gonetlogo_csrf=") {
		req.Header.Set("Cookie", cookie+"; gonetlogo_csrf="+token)
	} else {
		req.Header.Set("Cookie", cookie)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s %s: %s", path, res.Status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if user, ok := out["user"].(map[string]any); ok {
		return user
	}
	return out
}

func flowGraphQL(t *testing.T, srv *httptest.Server, cookie, query string, id ...string) map[string]any {
	t.Helper()
	payload := map[string]any{"query": query}
	if len(id) > 0 {
		payload["variables"] = map[string]string{"id": id[0]}
	}
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/graphql", strings.NewReader(string(rawBody)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Data   map[string]map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("graphql: %s", out.Errors[0].Message)
	}
	for _, value := range out.Data {
		return value
	}
	t.Fatal("graphql response had no data")
	return nil
}

func flowHasWorkspace(t *testing.T, srv *httptest.Server, id, model string) bool {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/auth/workspaces", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", flowCookie(t, srv))
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Workspaces []struct {
			ID        string `json:"id"`
			ModelName string `json:"modelName"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, item := range body.Workspaces {
		if item.ID == id && item.ModelName == model {
			return true
		}
	}
	t.Logf("workspaces %#v", body.Workspaces)
	return false
}

type workspaceList struct {
	Workspaces []struct {
		ID        string `json:"id"`
		ModelName string `json:"modelName"`
		Status    string `json:"status"`
	} `json:"workspaces"`
	OpenCount   int `json:"openCount"`
	MaxSessions int `json:"maxSessions"`
}

func mustJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func csrfAs(t *testing.T, srv *httptest.Server, client *http.Client) string {
	t.Helper()
	res, err := client.Get(srv.URL + "/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body["token"]
}

func signupAs(t *testing.T, srv *httptest.Server, client *http.Client, email, name string) {
	t.Helper()
	token := csrfAs(t, srv, client)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/signup", strings.NewReader(`{"email":"`+email+`","password":"correct-horse","displayName":"`+name+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("signup %s: %s", res.Status, raw)
	}
}

func ensureAs(t *testing.T, srv *httptest.Server, client *http.Client, id string) workspaceList {
	t.Helper()
	token := csrfAs(t, srv, client)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/workspaces/ensure", strings.NewReader(`{"id":"`+id+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("ensure %s: %s", res.Status, raw)
	}
	var body workspaceList
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OpenCount != countOwn(body) {
		t.Fatalf("openCount %d does not match this user's open rows %+v", body.OpenCount, body.Workspaces)
	}
	return body
}

func countOwn(body workspaceList) int {
	n := 0
	for _, item := range body.Workspaces {
		if item.Status != "ending" {
			n++
		}
	}
	return n
}

func logoutAs(t *testing.T, srv *httptest.Server, client *http.Client) {
	t.Helper()
	token := csrfAs(t, srv, client)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/logout", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("logout %s: %s", res.Status, raw)
	}
}

func loginAs(t *testing.T, srv *httptest.Server, client *http.Client, email string) {
	t.Helper()
	token := csrfAs(t, srv, client)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/login", strings.NewReader(`{"email":"`+email+`","password":"correct-horse"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("login %s: %s", res.Status, raw)
	}
}

func endAs(t *testing.T, srv *httptest.Server, client *http.Client, id string) bool {
	t.Helper()
	token := csrfAs(t, srv, client)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/workspaces/end", strings.NewReader(`{"id":"`+id+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("end %s: %s", res.Status, raw)
	}
	var body struct {
		LoggedOut bool `json:"loggedOut"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.LoggedOut
}

func statusAs(t *testing.T, srv *httptest.Server, client *http.Client) any {
	t.Helper()
	res, err := client.Get(srv.URL + "/auth/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		User any `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.User
}

func flowURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
