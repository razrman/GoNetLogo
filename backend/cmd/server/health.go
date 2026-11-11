package main

import (
	"io"
	"net/http"
)

// healthz is a liveness and readiness check for a load balancer.
// It does not touch GraphQL or session state.
func healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

// rootHandler serves /healthz itself and forwards every other request to
// next. The check stays outside the CORS wrapper and the request log so a
// load balancer probe does not depend on either.
func rootHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			healthz(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
