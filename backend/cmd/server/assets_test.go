package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelAssetServesImagesOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	sub := filepath.Join(dir, "Sample Models")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "pic.png"), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("<script>alert(1)</script>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(modelAssetHandler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/model-asset/Sample%20Models/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type %q", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff %q", got)
	}
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("csp %q", res.Header.Get("Content-Security-Policy"))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "png-bytes" {
		t.Fatalf("body %q", body)
	}

	missing, err := http.Get(srv.URL + "/model-asset/page.html")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("html status %d", missing.StatusCode)
	}
}
