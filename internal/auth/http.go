package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

const csrfCookie = "gonetlogo_csrf"
const csrfHeader = "X-CSRF-Token"

// ModelWorkspace is one GoNetLogo model session owned by a user.
// Status is running, stopped, or ending.
type ModelWorkspace struct {
	ID        string `json:"id"`
	ModelName string `json:"modelName"`
	Created   string `json:"created"`
	Status    string `json:"status"`
}

// Workspaces lists and ends model sessions for a user. The session
// package implements this so auth does not import it. Claim attaches
// a model the browser opened before sign-in, when that model has no owner.
type Workspaces interface {
	List(userID string) []ModelWorkspace
	End(userID, id string) bool
	Claim(userID, id string) bool
}

// Handler serves /auth and /admin.
type Handler struct {
	Config   Config
	Provider AuthProvider
	Sessions *SessionManager
	Users    UserStore
	Models   Workspaces
}

// Mount registers the auth and admin routes on mux.
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/status", h.status)
	mux.HandleFunc("GET /auth/csrf", h.csrf)
	mux.HandleFunc("POST /auth/signup", h.signup)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /auth/sessions", h.sessions)
	mux.HandleFunc("POST /auth/sessions/revoke", h.revoke)
	mux.HandleFunc("GET /auth/workspaces", h.workspaces)
	mux.HandleFunc("POST /auth/workspaces/claim", h.claimWorkspace)
	mux.HandleFunc("POST /auth/workspaces/end", h.endWorkspace)
	mux.Handle("GET /admin/users", RequireAdmin(http.HandlerFunc(h.adminUsers)))
	mux.Handle("POST /admin/users", RequireAdmin(http.HandlerFunc(h.adminCreate)))
	mux.Handle("POST /admin/users/{id}/role", RequireAdmin(http.HandlerFunc(h.adminRole)))
	mux.Handle("POST /admin/users/{id}/disabled", RequireAdmin(http.HandlerFunc(h.adminDisabled)))
	mux.Handle("DELETE /admin/users/{id}", RequireAdmin(http.HandlerFunc(h.adminDelete)))
	mux.Handle("GET /admin/users/{id}/sessions", RequireAdmin(http.HandlerFunc(h.adminSessions)))
	mux.Handle("POST /admin/sessions/revoke", RequireAdmin(http.HandlerFunc(h.adminRevoke)))
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"provider":     h.Config.Provider,
		"requireLogin": h.Config.RequireLogin,
		"user":         nil,
	}
	if ident, ok := FromContext(r.Context()); ok {
		body["user"] = ident
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) {
	token := csrfFrom(r)
	if token == "" {
		var err error
		token, err = randomID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not start a form token")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     h.Config.Cookie.Path,
		Secure:   h.Config.Cookie.Secure,
		SameSite: sameSite(h.Config.Cookie.SameSite),
		MaxAge:   int(h.Config.SessionLifetime.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "could not read the sign-up form")
		return
	}
	ident, err := h.Provider.Signup(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.start(w, r, ident)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "could not read the sign-in form")
		return
	}
	ident, err := h.Provider.Login(r.Context(), req)
	if err != nil {
		status := http.StatusUnauthorized
		if err == ErrDisabled {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	h.start(w, r, ident)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request, ident Identity) {
	item, err := h.Sessions.Start(ident.UserID, r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	h.Sessions.SetCookie(w, item.ID)
	writeJSON(w, http.StatusOK, map[string]any{"user": ident})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	id := h.Sessions.IDFromRequest(r)
	_ = h.Provider.Logout(r.Context(), id)
	h.Sessions.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) sessions(w http.ResponseWriter, r *http.Request) {
	ident, ok := FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": h.Sessions.ForUser(ident.UserID, h.Sessions.IDFromRequest(r)),
	})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	ident, ok := FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	var body struct {
		ID        string `json:"id"`
		AllOthers bool   `json:"allOthers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "could not read that request")
		return
	}
	current := h.Sessions.IDFromRequest(r)
	if body.AllOthers {
		h.Sessions.RevokeUser(ident.UserID, current, true)
	} else if body.ID == "" || !h.Sessions.Revoke(body.ID, ident.UserID) {
		writeError(w, http.StatusNotFound, "that session is not yours")
		return
	} else if body.ID == current {
		h.Sessions.ClearCookie(w)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) workspaces(w http.ResponseWriter, r *http.Request) {
	ident, ok := FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	var items []ModelWorkspace
	if h.Models != nil {
		items = h.Models.List(ident.UserID)
	}
	if items == nil {
		items = []ModelWorkspace{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": items})
}

func (h *Handler) claimWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	ident, ok := FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "name the workspace to keep")
		return
	}
	if h.Models == nil || !h.Models.Claim(ident.UserID, body.ID) {
		writeError(w, http.StatusNotFound, "that workspace cannot be kept")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) endWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	ident, ok := FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "name the workspace to end")
		return
	}
	if h.Models == nil || !h.Models.End(ident.UserID, body.ID) {
		writeError(w, http.StatusNotFound, "that workspace is not yours")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) adminUsers(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"users":             []User{},
		"canCreateUsers":    h.creator() != nil,
		"minPasswordLength": h.Config.MinPasswordLength,
	}
	if h.Users == nil {
		writeJSON(w, http.StatusOK, body)
		return
	}
	users, err := h.Users.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	out := make([]User, 0, len(users))
	for _, user := range users {
		if q != "" && !strings.Contains(strings.ToLower(user.Email), q) && !strings.Contains(strings.ToLower(user.DisplayName), q) {
			continue
		}
		out = append(out, user.Public())
	}
	body["users"] = out
	writeJSON(w, http.StatusOK, body)
}

func (h *Handler) adminCreate(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	creator := h.creator()
	if creator == nil {
		writeError(w, http.StatusNotFound, "this provider cannot create users")
		return
	}
	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "could not read that request")
		return
	}
	ident, err := creator.CreateUser(r.Context(), SignupRequest{
		Email:       body.Email,
		DisplayName: body.DisplayName,
		Password:    body.Password,
	}, body.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Users == nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": ident})
		return
	}
	user, ok, err := h.Users.ByID(ident.UserID)
	if err != nil || !ok {
		writeJSON(w, http.StatusOK, map[string]any{"user": ident})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user.Public()})
}

func (h *Handler) creator() UserCreator {
	creator, _ := h.Provider.(UserCreator)
	return creator
}

func (h *Handler) adminRole(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	user, ok := h.userParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "could not read that request")
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if role != RoleUser && role != RoleAdmin {
		writeError(w, http.StatusBadRequest, "role must be user or admin")
		return
	}
	if user.Role == RoleAdmin && role != RoleAdmin && h.lastAdmin(user.ID) {
		writeError(w, http.StatusBadRequest, "the last admin cannot be demoted")
		return
	}
	user.Role = role
	if err := h.Users.Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user.Public()})
}

func (h *Handler) adminDisabled(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	actor, _ := FromContext(r.Context())
	user, ok := h.userParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Disabled bool `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "could not read that request")
		return
	}
	if user.ID == actor.UserID && body.Disabled {
		writeError(w, http.StatusBadRequest, "you cannot disable your own account")
		return
	}
	user.Disabled = body.Disabled
	if err := h.Users.Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.Disabled {
		h.Sessions.RevokeUser(user.ID, "", false)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user.Public()})
}

func (h *Handler) adminDelete(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	actor, _ := FromContext(r.Context())
	id := r.PathValue("id")
	if id == actor.UserID {
		writeError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	if h.lastAdmin(id) {
		writeError(w, http.StatusBadRequest, "the last admin cannot be deleted")
		return
	}
	if err := h.Users.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	h.Sessions.RevokeUser(id, "", false)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) adminSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": h.Sessions.ForUser(user.ID, ""),
	})
}

func (h *Handler) adminRevoke(w http.ResponseWriter, r *http.Request) {
	if !h.allowChange(w, r) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "name the session to revoke")
		return
	}
	if !h.Sessions.Revoke(body.ID, "") {
		writeError(w, http.StatusNotFound, "no such session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) userParam(w http.ResponseWriter, r *http.Request) (User, bool) {
	if h.Users == nil {
		writeError(w, http.StatusNotFound, "no user store")
		return User{}, false
	}
	user, ok, err := h.Users.ByID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return User{}, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no such user")
		return User{}, false
	}
	return user, true
}

func (h *Handler) lastAdmin(id string) bool {
	if h.Users == nil {
		return false
	}
	users, err := h.Users.List()
	if err != nil {
		return false
	}
	admins := 0
	targetAdmin := false
	for _, user := range users {
		if user.Role == RoleAdmin && !user.Disabled {
			admins++
			if user.ID == id {
				targetAdmin = true
			}
		}
	}
	return targetAdmin && admins <= 1
}

func (h *Handler) allowChange(w http.ResponseWriter, r *http.Request) bool {
	if h.Config.Provider == "none" && (r.URL.Path == "/auth/signup" || r.URL.Path == "/auth/login") {
		writeError(w, http.StatusNotFound, ErrDisabled.Error())
		return false
	}
	if !csrfOK(r) {
		writeError(w, http.StatusForbidden, "the form token does not match; reload and try again")
		return false
	}
	return true
}

func csrfOK(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	return subtleConstant(c.Value, r.Header.Get(csrfHeader))
}

func csrfFrom(r *http.Request) string {
	c, err := r.Cookie(csrfCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func subtleConstant(a, b string) bool {
	if len(a) != len(b) || a == "" {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
