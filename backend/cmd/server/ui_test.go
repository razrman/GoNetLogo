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

func TestSPAServesUI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>GoNetLogo</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	ui, err := openUI(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(spaHandler(ui))
	t.Cleanup(srv.Close)

	body := get(t, srv.URL+"/")
	if !strings.Contains(body, "GoNetLogo") {
		t.Fatalf("index body = %q", body)
	}
	body = get(t, srv.URL+"/assets/app.js")
	if !strings.Contains(body, "console.log") {
		t.Fatalf("asset body = %q", body)
	}
	body = get(t, srv.URL+"/some/client/route")
	if !strings.Contains(body, "GoNetLogo") {
		t.Fatalf("spa fallback = %q", body)
	}

	res, err := http.Get(srv.URL + "/assets/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", res.StatusCode)
	}
}

func TestOpenUIRejectsEmptyDir(t *testing.T) {
	if _, err := openUI(t.TempDir()); err == nil {
		t.Fatal("expected error for a directory without index.html")
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d", url, res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
