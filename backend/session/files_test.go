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
