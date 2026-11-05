package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nlogoPace = `globals [pace]
to setup
  clear-all
  reset-ticks
end
to go
  tick
end
@#$#@#$#@
GRAPHICS-WINDOW
0
0
0
0
0
0
12
0
0
0
0
0
0
1
1
0
-1
1
-1
1

SLIDER
0
0
0
0
pace
pace
0
10
4
1
1
NIL
HORIZONTAL

@#$#@#$#@
`

const nlogoxPace = `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[
globals [pace]
to setup
  clear-all
  reset-ticks
end
to go
  tick
end
]]></code>
  <widgets>
    <view minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" patchSize="10" wrappingAllowedX="false" wrappingAllowedY="true"/>
    <slider variable="pace" min="0" max="9" default="3" step="1"/>
  </widgets>
</model>
`

const nlogoBroken = `to setup
  clear-all
end
fd 1
`

func TestLoadModelSourceBothFormats(t *testing.T) {
	s := newSession()
	t.Cleanup(s.Close)

	if err := s.LoadModelSource("pace.nlogo", nlogoPace); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "pace.nlogo" {
		t.Fatalf("name %q", s.modelName)
	}
	if s.vm.World.MinPxcor != -1 || s.vm.World.MaxPxcor != 1 || s.vm.World.PatchSize != 12 {
		t.Fatalf("nlogo world %+v", s.vm.World)
	}
	if g, ok := s.vm.World.Global("pace"); !ok || g.N != 4 {
		t.Fatalf("pace %v ok %v", g, ok)
	}

	if err := s.LoadModelSource(`..\library\pace.nlogox`, nlogoxPace); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "pace.nlogox" {
		t.Fatalf("name %q, want the base name only", s.modelName)
	}
	if s.vm.World.MinPxcor != -2 || s.vm.World.MaxPxcor != 2 || s.vm.World.WrapX || !s.vm.World.WrapY {
		t.Fatalf("nlogox world min %d max %d wrap %v %v", s.vm.World.MinPxcor, s.vm.World.MaxPxcor, s.vm.World.WrapX, s.vm.World.WrapY)
	}
	if g, ok := s.vm.World.Global("pace"); !ok || g.N != 3 {
		t.Fatalf("pace %v ok %v", g, ok)
	}
}

func TestLoadModelSourceCompileErrorKeepsModel(t *testing.T) {
	s := newSession()
	t.Cleanup(s.Close)
	if err := s.LoadModelSource("pace.nlogo", nlogoPace); err != nil {
		t.Fatal(err)
	}
	err := s.LoadModelSource("broken.nlogo", nlogoBroken)
	if err == nil || !strings.Contains(err.Error(), "line 4:") {
		t.Fatalf("error %v, want line 4", err)
	}
	if s.modelName != "pace.nlogo" {
		t.Fatalf("compile error replaced the model with %q", s.modelName)
	}
	if err := s.LoadModelSource("empty.nlogo", "   \n"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty file: %v", err)
	}
}

func TestLoadModelSourceContentOverridesExtension(t *testing.T) {
	s := newSession()
	t.Cleanup(s.Close)
	// XML text with a .nlogo name is still the XML model.
	if err := s.LoadModelSource("mislabeled.nlogo", nlogoxPace); err != nil {
		t.Fatal(err)
	}
	if s.vm.World.MaxPxcor != 2 {
		t.Fatalf("xml content parsed as nlogo, maxPxcor %d", s.vm.World.MaxPxcor)
	}
	// A classic section file named .nlogox is still the classic model.
	if err := s.LoadModelSource("mislabeled.nlogox", nlogoPace); err != nil {
		t.Fatal(err)
	}
	if s.vm.World.MaxPxcor != 1 || s.vm.World.PatchSize != 12 {
		t.Fatalf("section content parsed as xml, max %d size %v", s.vm.World.MaxPxcor, s.vm.World.PatchSize)
	}
}

func TestLoadModelFileFormatsAndTraversal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "pace.nlogo"), []byte(nlogoPace), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "pace.nlogox"), []byte(nlogoxPace), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "pace.nlogo"), filepath.Join(dir, "alias.nlogo")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.nlogo")
	if err := os.WriteFile(secret, []byte("to setup\n  clear-all\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "linked.nlogo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSession()
	t.Cleanup(s.Close)

	if err := s.LoadModelFile("pace.nlogo"); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "pace.nlogo" || s.vm.World.MaxPxcor != 1 {
		t.Fatalf("nlogo file name %q world %d", s.modelName, s.vm.World.MaxPxcor)
	}
	if err := s.LoadModelFile("nested/pace.nlogox"); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "pace.nlogox" || s.vm.World.MaxPxcor != 2 {
		t.Fatalf("nlogox file name %q world %d", s.modelName, s.vm.World.MaxPxcor)
	}
	if err := s.LoadModelFile("./alias.nlogo"); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "alias.nlogo" {
		t.Fatalf("alias name %q", s.modelName)
	}

	rejected := []string{
		"../secret.nlogo",
		`..\secret.nlogo`,
		"nested/../../secret.nlogo",
		"nested/../pace.nlogo",
		"/etc/passwd",
		secret,
		`C:\models\pace.nlogo`,
		"linked.nlogo",
		"notes.txt",
		"",
		"missing.nlogo",
		"pace.nlogo\x00.nlogo",
	}
	for _, path := range rejected {
		err := s.LoadModelFile(path)
		if err == nil {
			t.Errorf("accepted %q", path)
			continue
		}
		if path == "linked.nlogo" && !strings.Contains(err.Error(), "escapes") {
			t.Errorf("symlink %q: %v, want an escape error", path, err)
		}
		if strings.Contains(path, "..") && !strings.Contains(err.Error(), "escapes") {
			t.Errorf("traversal %q: %v, want an escape error", path, err)
		}
	}
	if s.modelName != "alias.nlogo" {
		t.Fatalf("a rejected path replaced the model with %q", s.modelName)
	}
}

func TestLoadModelStillEmbeddedOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "pace.nlogo"), []byte(nlogoPace), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newSession()
	t.Cleanup(s.Close)
	if err := s.LoadModel("fire.nlogo"); err != nil {
		t.Fatal(err)
	}
	if s.modelName != "fire.nlogo" {
		t.Fatalf("name %q", s.modelName)
	}
	// A file that exists only in MODELS_DIR is not an embedded demo.
	if err := s.LoadModel("pace.nlogo"); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("MODELS_DIR file via loadModel: %v", err)
	}
	for _, name := range []string{"../fire.nlogo", `..\fire.nlogo`, "/etc/passwd", "nested/fire.nlogo"} {
		err := s.LoadModel(name)
		if err == nil || !strings.Contains(err.Error(), "unknown model") {
			t.Errorf("loadModel %q: %v", name, err)
		}
	}
	if s.modelName != "fire.nlogo" {
		t.Fatalf("demo replaced by %q", s.modelName)
	}
}

func TestListFolderLibraryAndTraversal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	t.Setenv("GONETLOGO_MODELS_DIR", "")
	lib := filepath.Join(dir, "library", "Sample Models")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pace.nlogo"), []byte(nlogoPace), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Fire.nlogox"), []byte(nlogoxPace), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "notes.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "linked-dir")); err != nil {
		t.Fatal(err)
	}

	open, ok := OpenPath()
	if !ok || open != "library" {
		t.Fatalf("OpenPath %q ok %v", open, ok)
	}
	root, err := ListFolder("")
	if err != nil || !root.Available {
		t.Fatalf("root list %+v %v", root, err)
	}
	if root.Parent != nil {
		t.Fatalf("root parent %#v", root.Parent)
	}
	got := map[string]LibraryEntry{}
	for _, e := range root.Entries {
		got[e.Name] = e
	}
	if dir, ok := got["library"]; !ok || !dir.Directory {
		t.Fatalf("library dir missing: %+v", root.Entries)
	}
	if file, ok := got["pace.nlogo"]; !ok || file.Directory {
		t.Fatalf("pace.nlogo missing: %+v", root.Entries)
	}
	if _, ok := got["linked-dir"]; ok {
		t.Fatalf("symlink outside the models directory was listed: %+v", root.Entries)
	}

	sub, err := ListFolder("library/Sample Models")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Path != "library/Sample Models" || sub.Parent == nil || *sub.Parent != "library" {
		t.Fatalf("sub %+v", sub)
	}
	if len(sub.Entries) != 1 || sub.Entries[0].Name != "Fire.nlogox" || sub.Entries[0].Directory {
		t.Fatalf("sample entries %+v", sub.Entries)
	}

	for _, path := range []string{"../secret", "library/../../etc", "/etc", `C:\models`} {
		if _, err := ListFolder(path); err == nil {
			t.Errorf("listed %q", path)
		}
	}
}

func TestOpenModelImage(t *testing.T) {
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
	f, ctype, err := OpenModelImage("Sample Models/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if ctype != "image/png" {
		t.Fatalf("content type %q", ctype)
	}
	body, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "png-bytes" {
		t.Fatalf("body %q", body)
	}
	for _, rel := range []string{"../pic.png", "page.html", "/etc/passwd", "Sample Models/../../page.html", "note.svg"} {
		if _, _, err := OpenModelImage(rel); err == nil {
			t.Errorf("opened %q", rel)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(dir, "secret.png")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenModelImage("secret.png"); err == nil {
		t.Fatal("symlink outside the models directory was opened")
	}
}
