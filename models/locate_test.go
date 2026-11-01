package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverModelsRootFromRootBinAndApp(t *testing.T) {
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	lib := filepath.Join(modelsDir, "library", "Sample Models")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "fire.nlogo"), []byte("to setup\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	macOS := filepath.Join(root, "bin", "GoNetLogo.app", "Contents", "MacOS")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		t.Fatal(err)
	}
	resources := filepath.Join(root, "bin", "GoNetLogo.app", "Contents", "Resources")

	checks := []string{
		root,
		filepath.Join(root, "bin"),
		macOS,
		resources,
		modelsDir,
	}
	for _, start := range checks {
		got, ok := discoverModelsRoot([]string{start})
		if !ok {
			t.Errorf("discover from %s found nothing", start)
			continue
		}
		want, err := filepath.EvalSymlinks(modelsDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Clean(want) {
			t.Errorf("discover from %s = %s, want %s", start, got, want)
		}
	}
	if _, ok := discoverModelsRoot([]string{t.TempDir()}); ok {
		t.Fatal("an unrelated directory was treated as the models root")
	}
}

func TestOpenDirOverrideAndFallback(t *testing.T) {
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	lib := filepath.Join(modelsDir, "library")
	if err := os.MkdirAll(filepath.Join(lib, "Sample Models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "fire.nlogo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODELS_DIR", "")
	t.Setenv("GONETLOGO_MODELS_DIR", "")
	chdir(t, root)

	got, ok := OpenDir()
	if !ok {
		t.Fatal("OpenDir not found")
	}
	want, err := filepath.EvalSymlinks(lib)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("OpenDir = %s, want library %s", got, want)
	}

	custom := t.TempDir()
	t.Setenv("GONETLOGO_MODELS_DIR", custom)
	got, ok = OpenDir()
	if !ok {
		t.Fatal("override not found")
	}
	want, err = filepath.EvalSymlinks(custom)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("override = %s, want %s", got, want)
	}

	t.Setenv("GONETLOGO_MODELS_DIR", filepath.Join(custom, "missing"))
	got, ok = OpenDir()
	if !ok {
		t.Fatal("missing override should fall back")
	}
	want, err = filepath.EvalSymlinks(lib)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("fallback = %s, want %s", got, want)
	}
}

func TestModelsRootHonorsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	got, err := ModelsRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("ModelsRoot = %s, want %s", got, want)
	}
	t.Setenv("MODELS_DIR", filepath.Join(dir, "nope"))
	if _, err := ModelsRoot(); err == nil {
		t.Fatal("missing MODELS_DIR was accepted")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
}
