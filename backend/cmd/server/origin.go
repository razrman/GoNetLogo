package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"gonetlogo/backend/session"
)

const (
	// envProduction turns on production mode when -production is omitted.
	envProduction = "GONETLOGO_PRODUCTION"
	// envAllowedOrigins is the comma-separated origin list when
	// -allowed-origins is omitted.
	envAllowedOrigins = "GONETLOGO_ALLOWED_ORIGINS"
)

// originPolicy is who may call the API from a browser.
// Production mode disables the wildcard. Same-origin requests are always
// allowed. An empty allow list in production mode means same-origin only.
// Outside production mode every origin is allowed, which is what the Vite
// dev proxy needs.
type originPolicy struct {
	production bool
	allowed    map[string]struct{}
}

func newOriginPolicy(production bool, origins []string) originPolicy {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return originPolicy{production: production, allowed: allowed}
}

// allows reports whether r may use a browser feature that checks Origin:
// CORS, and the websocket handshake. A request with no Origin header is
// allowed, matching non-browser clients and gorilla/websocket's default.
func (p originPolicy) allows(r *http.Request) bool {
	if r == nil || !p.production {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if sameOrigin(r, origin) {
		return true
	}
	norm, err := normalizeOrigin(origin)
	if err != nil {
		return false
	}
	_, ok := p.allowed[norm]
	return ok
}

// sameOrigin compares the Origin host (including port) with the request
// Host. The scheme is not compared: a TLS-terminating load balancer
// forwards https browser requests to the task as http, and the Host
// header is what the browser used.
func sameOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// resolveProduction applies flag-over-environment precedence.
// Recognized environment values are 1, true, yes, on, and the false forms.
func resolveProduction(explicit bool, flagVal bool, getenv func(string) string) (bool, error) {
	if explicit {
		return flagVal, nil
	}
	raw := ""
	if getenv != nil {
		raw = strings.TrimSpace(getenv(envProduction))
	}
	if raw == "" {
		return false, nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s: want true or false, got %q", envProduction, raw)
	}
}

// resolveAllowedOrigins applies flag-over-environment precedence.
// An explicit flag, including an empty one, wins over the environment.
func resolveAllowedOrigins(explicit bool, flagVal string, getenv func(string) string) ([]string, error) {
	raw := flagVal
	if !explicit {
		raw = ""
		if getenv != nil {
			raw = getenv(envAllowedOrigins)
		}
	}
	return parseAllowedOrigins(raw)
}

// parseAllowedOrigins splits a comma-separated list of http and https
// origins. The empty string is an empty list. Hosts are compared
// case-insensitively, and the default port for the scheme is dropped so
// https://app.example:443 matches a browser Origin of https://app.example.
func parseAllowedOrigins(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		norm, err := normalizeOrigin(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out, nil
}

func normalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" {
		return "", fmt.Errorf("invalid origin %q", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("invalid origin %q", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid origin %q", raw)
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return u.Scheme + "://" + host, nil
}

func cors(next http.Handler) http.Handler {
	return withCORS(originPolicy{}, next)
}

func withCORS(pol originPolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The end beacon and the heartbeat are same-origin only. A
		// wildcard, or a reflected allow-origin, would invite other
		// sites to post them.
		if !session.BrowserSessionPath(r.URL.Path) {
			writeCORS(w, r, pol)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeCORS(w http.ResponseWriter, r *http.Request, pol originPolicy) {
	if !pol.production {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || !pol.allows(r) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
}
