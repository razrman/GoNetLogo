package main

import (
	"strings"
	"testing"
)

func TestResolveAddr(t *testing.T) {
	const def = "0.0.0.0:43131"
	got, err := resolveAddr(false, def, func(string) string { return "" })
	if err != nil || got != def {
		t.Fatalf("default %q %v", got, err)
	}
	got, err = resolveAddr(true, "127.0.0.1:8080", func(key string) string {
		switch key {
		case envAddr:
			return "10.0.0.1:9"
		case envPort:
			return "9090"
		default:
			return ""
		}
	})
	if err != nil || got != "127.0.0.1:8080" {
		t.Fatalf("explicit flag %q %v", got, err)
	}
	got, err = resolveAddr(false, def, func(key string) string {
		if key == envAddr {
			return " 127.0.0.1:9090 "
		}
		if key == envPort {
			return "8080"
		}
		return ""
	})
	if err != nil || got != "127.0.0.1:9090" {
		t.Fatalf("ADDR should win over PORT, got %q %v", got, err)
	}
	got, err = resolveAddr(false, def, func(key string) string {
		if key == envPort {
			return "8080"
		}
		return ""
	})
	if err != nil || got != "0.0.0.0:8080" {
		t.Fatalf("PORT %q %v", got, err)
	}
	got, err = resolveAddr(false, "[::]:43131", func(key string) string {
		if key == envPort {
			return "9090"
		}
		return ""
	})
	if err != nil || got != "[::]:9090" {
		t.Fatalf("ipv6 PORT %q %v", got, err)
	}
	if _, err := resolveAddr(false, def, func(string) string { return "nope" }); err == nil || !strings.Contains(err.Error(), envAddr) {
		t.Fatalf("bad ADDR err = %v", err)
	}
	if _, err := resolveAddr(false, def, func(key string) string {
		if key == envPort {
			return "0"
		}
		return ""
	}); err == nil || !strings.Contains(err.Error(), envPort) {
		t.Fatalf("port 0 err = %v", err)
	}
	if _, err := resolveAddr(false, def, func(key string) string {
		if key == envPort {
			return "65536"
		}
		return ""
	}); err == nil {
		t.Fatal("port 65536 accepted")
	}
	if _, err := resolveAddr(true, "8080", nil); err == nil || !strings.Contains(err.Error(), "-addr") {
		t.Fatalf("bare port flag err = %v", err)
	}
	got, err = resolveAddr(true, ":8080", func(string) string { return "1.2.3.4:1" })
	if err != nil || got != ":8080" {
		t.Fatalf("empty host %q %v", got, err)
	}
}
