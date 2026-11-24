package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPUsesTheLastForwardedHop(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "198.51.100.4, 203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("client %q", got)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.10:443"
	if got := clientIP(r); got != "192.0.2.10" {
		t.Fatalf("client %q", got)
	}
}
