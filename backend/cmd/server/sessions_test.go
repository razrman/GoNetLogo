package main

import (
	"strings"
	"testing"
)

func TestResolveMaxSessions(t *testing.T) {
	n, err := resolveMaxSessions(false, 0, func(string) string { return "" })
	if err != nil || n != 0 {
		t.Fatalf("default %d %v", n, err)
	}
	n, err = resolveMaxSessions(false, 9, func(key string) string {
		if key == envMaxSessions {
			return " 32 "
		}
		return ""
	})
	if err != nil || n != 32 {
		t.Fatalf("env %d %v", n, err)
	}
	n, err = resolveMaxSessions(true, 0, func(string) string { return "32" })
	if err != nil || n != 0 {
		t.Fatalf("explicit zero %d %v, want the flag to win", n, err)
	}
	n, err = resolveMaxSessions(true, 4, func(string) string { return "32" })
	if err != nil || n != 4 {
		t.Fatalf("flag %d %v", n, err)
	}
	if _, err := resolveMaxSessions(true, -1, nil); err == nil || !strings.Contains(err.Error(), "-max-sessions") {
		t.Fatalf("negative flag err = %v", err)
	}
	if _, err := resolveMaxSessions(false, 0, func(string) string { return "-2" }); err == nil || !strings.Contains(err.Error(), envMaxSessions) {
		t.Fatalf("negative env err = %v", err)
	}
	if _, err := resolveMaxSessions(false, 0, func(string) string { return "many" }); err == nil {
		t.Fatal("non-integer accepted")
	}
}
