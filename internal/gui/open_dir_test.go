package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModelOpenURIUsesLibrary(t *testing.T) {
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	lib := filepath.Join(modelsDir, "library", "Sample Models")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "fire.nlogo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODELS_DIR", "")
	t.Setenv("GONETLOGO_MODELS_DIR", "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})

	uri, ok := modelOpenURI()
	if !ok {
		t.Fatal("model open folder was not found")
	}
	got := uri.Path()
	want, err := filepath.EvalSymlinks(filepath.Join(modelsDir, "library"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("open uri path %s, want %s", got, want)
	}
}

func TestIsOpenModelName(t *testing.T) {
	for _, name := range []string{"Fire.nlogo", "Fire.nlogox", "Maze.nlogox3d", "a.NLOGO"} {
		if !isOpenModelName(name) {
			t.Errorf("%s should be a model", name)
		}
	}
	for _, name := range []string{"notes.txt", "Fire.png", "Fire.nls"} {
		if isOpenModelName(name) {
			t.Errorf("%s should not be a model", name)
		}
	}
}
