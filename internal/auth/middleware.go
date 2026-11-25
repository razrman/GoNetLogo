package auth

import "net/http"

// Middleware puts the signed-in user on the request context. Provider
// none does nothing, so anonymous requests stay anonymous.
func Middleware(provider AuthProvider, sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if provider == nil || provider.Name() == "none" || sessions == nil {
				next.ServeHTTP(w, r)
				return
			}
			id := sessions.IDFromRequest(r)
			if id != "" {
				if ident, err := provider.Verify(r.Context(), id); err == nil && ident.UserID != "" {
					r = r.WithContext(WithIdentity(r.Context(), ident))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser rejects a request that has no signed-in user.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "sign in to continue")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects a request that is not an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ident, ok := FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in to continue")
			return
		}
		if ident.Role != RoleAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}
