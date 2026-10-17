package gui

import (
	"strings"
	"testing"
)

func TestParseWorldInputs(t *testing.T) {
	minPx, maxPx, minPy, maxPy, ps, err := parseWorldInputs("0", "99", "0", "99", "8")
	if err != nil {
		t.Fatal(err)
	}
	if minPx != 0 || maxPx != 99 || minPy != 0 || maxPy != 99 || ps != 8 {
		t.Fatalf("got %d %d %d %d %v", minPx, maxPx, minPy, maxPy, ps)
	}

	if _, _, _, _, _, err := parseWorldInputs(" 1 ", "10", "0", "10", "8"); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("origin: %v", err)
	}
	if _, _, _, _, _, err := parseWorldInputs("0", "300", "0", "10", "8"); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("side: %v", err)
	}
	if _, _, _, _, _, err := parseWorldInputs("0", "10", "0", "10", "0"); err == nil || !strings.Contains(err.Error(), "patch size") {
		t.Fatalf("patch: %v", err)
	}
	if _, _, _, _, _, err := parseWorldInputs("0", "10", "0", "10", "101"); err == nil {
		t.Fatal("patch size 101 accepted")
	}
	if _, _, _, _, _, err := parseWorldInputs("x", "10", "0", "10", "8"); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Fatalf("coord: %v", err)
	}
	if _, _, _, _, _, err := parseWorldInputs("-16", "16", "-16", "16", "13.5"); err != nil {
		t.Fatal(err)
	}
}

func TestViewPixels(t *testing.T) {
	w, h := viewPixels(100, 100, 12)
	if w < 519 || w > 521 || h < 519 || h > 521 {
		t.Fatalf("100×100 patch 12 -> %v×%v, want about 520", w, h)
	}
	w, h = viewPixels(33, 33, 13)
	if w < 428 || w > 430 || h < 428 || h > 430 {
		t.Fatalf("33×33 patch 13 -> %v×%v, want 429", w, h)
	}
	w, h = viewPixels(10, 10, 1)
	if w < 279 || w > 281 || h < 279 || h > 281 {
		t.Fatalf("tiny world -> %v×%v, want 280", w, h)
	}
	w, h = viewPixels(200, 50, 2)
	if w < 399 || w > 401 || h < 99 || h > 101 {
		t.Fatalf("wide world -> %v×%v, want 400×100", w, h)
	}
	w, h = viewPixels(256, 32, 4)
	if w < 519 || w > 521 || h < 64 || h > 66 {
		t.Fatalf("capped wide world -> %v×%v, want 520×65", w, h)
	}
}
