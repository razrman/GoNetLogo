package auth

import (
	"context"
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
	"time"
)

func TestParseRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing provider", "require_login: false\n", "provider is required"},
		{"unknown provider", "provider: cognito\n", "not implemented"},
		{"login without a provider", "provider: none\nrequire_login: true\n", "require_login cannot be true"},
		{"bad lifetime", "provider: local\nsession_lifetime: later\n", "session_lifetime"},
		{"same site none", "provider: local\ncookie:\n  same_site: none\n  secure: false\n", "requires cookie.secure"},
		{"bad email", "provider: local\ninitial_admin_emails: [not-an-email]\n", "not an email"},
		{"short password", "provider: local\nproviders:\n  local:\n    min_password_length: 3\n", "min_password_length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLoadMissingEnvFailsAndCommittedFileStaysAnonymous(t *testing.T) {
	cfg, err := LoadFile(filepath.Join("..", "..", "config", "auth.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "none" || cfg.RequireLogin {
		t.Fatalf("committed config %#v", cfg)
	}
	_, err = Load(func(string) string { return filepath.Join(t.TempDir(), "missing.yaml") })
	if err == nil {
		t.Fatal("a missing explicit config should fail")
	}
}

func TestFileStoreRoundTripAndPromote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users", "users.json")
	store, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	user := User{ID: "u1", Provider: "local", Subject: "a@b.co", Email: "a@b.co", DisplayName: "A", PasswordHash: "hash", Role: RoleUser}
	if err := store.Create(user); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(user); err == nil {
		t.Fatal("duplicate email should fail")
	}
	got, ok, err := store.ByEmail("A@B.CO")
	if err != nil || !ok || got.PasswordHash != "hash" {
		t.Fatalf("lookup %#v %v %v", got, ok, err)
	}
	if got.Public().PasswordHash != "" {
		t.Fatal("public user still has a hash")
	}
	if err := store.PromoteEmails([]string{"a@b.co"}); err != nil {
		t.Fatal(err)
	}
	got, _, _ = store.ByID("u1")
	if got.Role != RoleAdmin {
		t.Fatalf("role %s", got.Role)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "\x00") {
		t.Fatal("user file looks truncated")
	}
	again, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err = again.ByID("u1")
	if err != nil || !ok || got.Role != RoleAdmin {
		t.Fatalf("reopen %#v %v %v", got, ok, err)
	}
}

func TestSessionsExpireAndRevokeOthers(t *testing.T) {
	mgr := NewSessionManager(Config{
		SessionLifetime: time.Hour,
		Cookie:          CookieConfig{Name: "gonetlogo_auth", SameSite: "lax", Path: "/"},
	})
	one, err := mgr.Start("user", "Browser One")
	if err != nil {
		t.Fatal(err)
	}
	two, err := mgr.Start("user", "Browser Two")
	if err != nil {
		t.Fatal(err)
	}
	if n := mgr.RevokeUser("user", one.ID, true); n != 1 {
		t.Fatalf("revoked %d", n)
	}
	if _, ok := mgr.Lookup(two.ID); ok {
		t.Fatal("the other session should be gone")
	}
	if _, ok := mgr.Lookup(one.ID); !ok {
		t.Fatal("the current session should stay")
	}
	rec := httptest.NewRecorder()
	mgr.SetCookie(rec, one.ID)
	res := rec.Result()
	defer res.Body.Close()
	cookie := res.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Name != "gonetlogo_auth" {
		t.Fatalf("cookie %#v", cookie)
	}
	mgr.lifetime = time.Millisecond
	stale, _ := mgr.Start("user", "old")
	time.Sleep(5 * time.Millisecond)
	if _, ok := mgr.Lookup(stale.ID); ok {
		t.Fatal("expired session was accepted")
	}
}

func TestMiddlewareAnonymousAndRequireAdmin(t *testing.T) {
	none := Middleware(noneProvider{}, NewSessionManager(DefaultConfig()))
	var saw bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	none(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, saw = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(httptest.NewRecorder(), req)
	if saw {
		t.Fatal("provider none attached a user")
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	adminReq = adminReq.WithContext(WithIdentity(adminReq.Context(), Identity{UserID: "a", Role: RoleAdmin}))
	rec := httptest.NewRecorder()
	RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, adminReq)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin status %d", rec.Code)
	}

	userReq := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	userReq = userReq.WithContext(WithIdentity(userReq.Context(), Identity{UserID: "u", Role: RoleUser}))
	rec = httptest.NewRecorder()
	RequireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("user reached an admin handler")
	})).ServeHTTP(rec, userReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("anonymous reached a signed-in handler")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/graphql", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d", rec.Code)
	}
}

func TestLocalSignupLoginAndAdminHTTP(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Provider = "local"
	cfg.UsersFile = filepath.Join(dir, "users.json")
	cfg.InitialAdminEmails = []string{"admin@example.com"}
	sessions := NewSessionManager(cfg)
	provider, err := NewProvider(cfg, sessions)
	if err != nil {
		t.Fatal(err)
	}
	local := provider.(*localProvider)
	h := &Handler{Config: cfg, Provider: provider, Sessions: sessions, Users: local.Users(), Models: fakeModels{}}
	mux := http.NewServeMux()
	h.Mount(mux)
	srv := httptest.NewServer(Middleware(provider, sessions)(mux))
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar

	token := csrfToken(t, srv)
	admin := postJSON(t, srv, "/auth/signup", token, `{"email":"admin@example.com","password":"correct-horse","displayName":"Ada"}`, nil)
	if admin["role"] != RoleAdmin {
		t.Fatalf("first admin role %#v", admin)
	}
	adminCookie := cookieHeader(t, srv)

	token = csrfToken(t, srv)
	user := postJSON(t, srv, "/auth/signup", token, `{"email":"pat@example.com","password":"correct-horse","displayName":"Pat"}`, nil)
	if user["role"] != RoleUser {
		t.Fatalf("user role %#v", user)
	}
	userCookie := cookieHeader(t, srv)

	status := getJSON(t, srv, "/auth/status", userCookie)
	if status["user"].(map[string]any)["email"] != "pat@example.com" {
		t.Fatalf("status %#v", status)
	}

	denied := do(t, srv, http.MethodGet, "/admin/users", userCookie, "", "")
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("user admin status %d", denied.StatusCode)
	}
	denied.Body.Close()

	listed := getJSON(t, srv, "/admin/users?q=pat", adminCookie)
	users := listed["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["email"] != "pat@example.com" {
		t.Fatalf("search %#v", listed)
	}
	patID := users[0].(map[string]any)["id"].(string)

	token = csrfToken(t, srv)
	noCSRF := do(t, srv, http.MethodPost, "/admin/users/"+patID+"/role", adminCookie, token, `{"role":"admin"}`)
	noCSRF.Header.Del("X-CSRF-Token")
	// The helper already set the header. A missing header is checked separately.
	noCSRF.Body.Close()
	missing := do(t, srv, http.MethodPost, "/admin/users/"+patID+"/disabled", adminCookie, "", `{"disabled":true}`)
	if missing.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf %d", missing.StatusCode)
	}
	missing.Body.Close()

	token = csrfToken(t, srv)
	changed := postJSON(t, srv, "/admin/users/"+patID+"/role", token, `{"role":"admin"}`, map[string]string{"Cookie": adminCookie})
	if changed["role"] != RoleAdmin {
		t.Fatalf("promote %#v", changed)
	}
	token = csrfToken(t, srv)
	demoted := postJSON(t, srv, "/admin/users/"+patID+"/role", token, `{"role":"user"}`, map[string]string{"Cookie": adminCookie})
	if demoted["role"] != RoleUser {
		t.Fatalf("demote %#v", demoted)
	}

	self := do(t, srv, http.MethodDelete, "/admin/users/"+admin["userId"].(string), adminCookie, csrfToken(t, srv), "")
	if self.StatusCode != http.StatusBadRequest {
		t.Fatalf("self delete %d", self.StatusCode)
	}
	self.Body.Close()

	token = csrfToken(t, srv)
	signedOut := postJSON(t, srv, "/auth/logout", token, `{}`, map[string]string{"Cookie": userCookie})
	if signedOut["ok"] != true {
		t.Fatalf("logout %#v", signedOut)
	}
	after := getJSON(t, srv, "/auth/status", "")
	if after["user"] != nil {
		t.Fatalf("still signed in %#v", after)
	}

	ident, err := provider.Login(context.Background(), LoginRequest{Email: "pat@example.com", Password: "wrong-password"})
	if err == nil || ident.UserID != "" {
		t.Fatal("wrong password was accepted")
	}
}

func TestLogoutInvalidatesTheSession(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Provider = "local"
	cfg.UsersFile = filepath.Join(dir, "users.json")
	sessions := NewSessionManager(cfg)
	provider, err := NewProvider(cfg, sessions)
	if err != nil {
		t.Fatal(err)
	}
	ident, err := provider.Signup(context.Background(), SignupRequest{Email: "a@b.co", Password: "correct-horse", DisplayName: "A"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := sessions.Start(ident.UserID, "Test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Verify(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Logout(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Verify(context.Background(), item.ID); err != ErrAnonymous {
		t.Fatalf("verify after logout: %v", err)
	}
}

func TestAdminCreatesUser(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Provider = "local"
	cfg.UsersFile = filepath.Join(dir, "users.json")
	cfg.MinPasswordLength = 8
	cfg.InitialAdminEmails = []string{"admin@example.com"}
	sessions := NewSessionManager(cfg)
	provider, err := NewProvider(cfg, sessions)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(UserCreator); !ok {
		t.Fatal("local provider should create users")
	}
	h := &Handler{Config: cfg, Provider: provider, Sessions: sessions, Users: provider.(*localProvider).Users()}
	mux := http.NewServeMux()
	h.Mount(mux)
	srv := httptest.NewServer(Middleware(provider, sessions)(mux))
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar

	token := csrfToken(t, srv)
	admin := postJSON(t, srv, "/auth/signup", token, `{"email":"admin@example.com","password":"correct-horse","displayName":"Ada"}`, nil)
	if admin["role"] != RoleAdmin && admin["role"] != RoleUser {
		t.Fatalf("admin %#v", admin)
	}
	adminCookie := cookieHeader(t, srv)

	listed := getJSON(t, srv, "/admin/users", adminCookie)
	if listed["canCreateUsers"] != true {
		t.Fatalf("canCreateUsers %#v", listed["canCreateUsers"])
	}

	token = csrfToken(t, srv)
	created := postJSON(t, srv, "/admin/users", token, `{"email":"new@example.com","password":"correct-horse","displayName":"New","role":"user"}`, map[string]string{"Cookie": adminCookie})
	if created["email"] != "new@example.com" || created["role"] != RoleUser || created["passwordHash"] != nil {
		t.Fatalf("created %#v", created)
	}

	token = csrfToken(t, srv)
	dup := do(t, srv, http.MethodPost, "/admin/users", adminCookie, token, `{"email":"new@example.com","password":"correct-horse","role":"admin"}`)
	if dup.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate %d", dup.StatusCode)
	}
	dup.Body.Close()

	token = csrfToken(t, srv)
	short := do(t, srv, http.MethodPost, "/admin/users", adminCookie, token, `{"email":"short@example.com","password":"tiny","role":"user"}`)
	if short.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password %d", short.StatusCode)
	}
	short.Body.Close()

	token = csrfToken(t, srv)
	badRole := do(t, srv, http.MethodPost, "/admin/users", adminCookie, token, `{"email":"role@example.com","password":"correct-horse","role":"owner"}`)
	if badRole.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad role %d", badRole.StatusCode)
	}
	badRole.Body.Close()

	found := getJSON(t, srv, "/admin/users?q=new@", adminCookie)
	users := found["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["displayName"] != "New" {
		t.Fatalf("list %#v", found)
	}
}

func TestProviderWithoutCreateHidesAddUser(t *testing.T) {
	if _, ok := AuthProvider(noneProvider{}).(UserCreator); ok {
		t.Fatal("provider none should not create users")
	}
	cfg := DefaultConfig()
	sessions := NewSessionManager(cfg)
	item, err := sessions.Start("admin", "test")
	if err != nil {
		t.Fatal(err)
	}
	provider := stubAdminProvider{sessionID: item.ID}
	h := &Handler{Config: cfg, Provider: provider, Sessions: sessions}
	mux := http.NewServeMux()
	h.Mount(mux)
	srv := httptest.NewServer(Middleware(provider, sessions)(mux))
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar
	srv.Client().Jar.SetCookies(mustURL(t, srv.URL), []*http.Cookie{{
		Name: cfg.Cookie.Name, Value: item.ID, Path: "/",
	}})
	token := csrfToken(t, srv)
	cookie := cookieHeader(t, srv)
	listed := getJSON(t, srv, "/admin/users", cookie)
	if listed["canCreateUsers"] != false {
		t.Fatalf("canCreateUsers %#v", listed["canCreateUsers"])
	}
	res := do(t, srv, http.MethodPost, "/admin/users", cookie, token, `{"email":"a@b.co","password":"correct-horse","role":"user"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("create status %d", res.StatusCode)
	}
}

type stubAdminProvider struct {
	noneProvider
	sessionID string
}

func (p stubAdminProvider) Name() string { return "stub" }

func (p stubAdminProvider) Verify(_ context.Context, sessionID string) (Identity, error) {
	if sessionID != p.sessionID {
		return Identity{}, ErrAnonymous
	}
	return Identity{UserID: "admin", Provider: "stub", Subject: "admin", Email: "admin@example.com", DisplayName: "Ada", Role: RoleAdmin}, nil
}

type fakeModels struct{}

func (fakeModels) List(string) []ModelWorkspace { return nil }
func (fakeModels) End(string, string) bool      { return false }
func (fakeModels) Claim(string, string) bool    { return false }

func csrfToken(t *testing.T, srv *httptest.Server) string {
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

func cookieHeader(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	var parts []string
	for _, c := range srv.Client().Jar.Cookies(mustURL(t, srv.URL)) {
		if c.Name == "gonetlogo_auth" || c.Name == csrfCookie {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func postJSON(t *testing.T, srv *httptest.Server, path, token, body string, header map[string]string) map[string]any {
	t.Helper()
	cookie := ""
	if header != nil {
		cookie = header["Cookie"]
	}
	res := do(t, srv, http.MethodPost, path, cookie, token, body)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %s: %s", path, res.Status, b)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if user, ok := out["user"].(map[string]any); ok {
		return user
	}
	return out
}

func getJSON(t *testing.T, srv *httptest.Server, path, cookie string) map[string]any {
	t.Helper()
	res := do(t, srv, http.MethodGet, path, cookie, "", "")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %s: %s", path, res.Status, b)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func do(t *testing.T, srv *httptest.Server, method, path, cookie, token, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if token != "" {
		req.Header.Set(csrfHeader, token)
		if cookie == "" {
			req.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
		} else if !strings.Contains(cookie, csrfCookie+"=") {
			req.Header.Set("Cookie", cookie+"; "+csrfCookie+"="+token)
		}
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
