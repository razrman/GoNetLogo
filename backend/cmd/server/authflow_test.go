package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
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
		Models:   modelBridge{store},
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

func flowURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
