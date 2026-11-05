package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gonetlogo/models"
)

func TestWalkLibrary(t *testing.T) {
	root := modelsRoot(t)
	files, err := Walk(filepath.Join(root, "library"))
	if err != nil {
		t.Fatal(err)
	}
	var nlogo, nlogox, n3d, nls int
	for _, path := range files {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".nlogo":
			nlogo++
		case ".nlogox":
			nlogox++
		case ".nlogox3d":
			n3d++
		default:
			t.Errorf("unexpected model extension: %s", path)
		}
		if strings.EqualFold(filepath.Ext(path), ".nls") {
			nls++
		}
	}
	if nlogox < 500 || n3d < 30 {
		t.Fatalf("library walk nlogox=%d nlogox3d=%d nlogo=%d, want the bundled library", nlogox, n3d, nlogo)
	}
	if nls != 0 {
		t.Fatalf("includes were treated as models: %d", nls)
	}
}

func TestRobbyCompiles(t *testing.T) {
	root := modelsRoot(t)
	res := Check(filepath.Join(root, "Robby the Robot.nlogo"), Options{SkipRun: true, Seed: 1})
	if !res.CompileOK {
		t.Fatalf("stage %s: %s", res.Stage, res.Error)
	}
	if !res.HasSetup || !res.HasGo {
		t.Fatalf("procedures setup=%v go=%v", res.HasSetup, res.HasGo)
	}
	if res.Setup != "not-run" || res.Go != "not-run" {
		t.Fatalf("skip-run still ran: setup %s go %s", res.Setup, res.Go)
	}
}

func TestFireRuns(t *testing.T) {
	root := modelsRoot(t)
	path := filepath.Join(root, "library", "Sample Models", "Earth Science", "Fire.nlogox")
	res := Check(path, Options{Timeout: 20 * time.Second, Ticks: 3, Seed: 1})
	if !res.CompileOK {
		t.Fatalf("stage %s: %s", res.Stage, res.Error)
	}
	if res.Setup != "ok" {
		t.Fatalf("setup %s: %s", res.Setup, res.Error)
	}
	if res.Go != "ok" && res.Go != "stopped" {
		t.Fatalf("go %s ticks %d: %s", res.Go, res.GoTicks, res.Error)
	}
	if res.GoTicks < 1 {
		t.Fatalf("go ticks %d", res.GoTicks)
	}
}

func TestMissingCommand(t *testing.T) {
	path := writeModel(t, "bad.nlogo", "to setup\n  no-such-command\nend\n")
	res := Check(path, Options{SkipRun: true})
	if res.CompileOK || res.Stage != "compile" {
		t.Fatalf("stage %s compile %v err %s", res.Stage, res.CompileOK, res.Error)
	}
	if !strings.Contains(res.Error, "NO-SUCH-COMMAND") {
		t.Fatalf("error %s", res.Error)
	}
	if Signature(res) != "missing no-such-command" {
		t.Fatalf("signature %s", Signature(res))
	}
}

func TestExtensionColon(t *testing.T) {
	src := "extensions [ table ]\nto setup\n  clear-all\nend\nto go\n  table:make\nend\n"
	path := writeModel(t, "ext.nlogo", src)
	res := Check(path, Options{SkipRun: true})
	if res.CodeOK || res.CompileOK {
		t.Fatalf("extension model compiled: %+v", res)
	}
	if !contains(res.Extensions, "table") {
		t.Fatalf("extensions %+v", res.Extensions)
	}
	if Signature(res) != "extension table" {
		t.Fatalf("signature %q snippet %q err %q", Signature(res), res.Snippet, res.Error)
	}
	src = "extensions [ import-a ]\nto setup\n  import-a:pcolors \"x\"\nend\n"
	res = Check(writeModel(t, "impa.nlogo", src), Options{SkipRun: true})
	if Signature(res) != "extension import-a" {
		t.Fatalf("signature %q snippet %q", Signature(res), res.Snippet)
	}
}

func TestIncludesDirective(t *testing.T) {
	path := writeModel(t, "inc.nlogo", "__includes [ \"actions.nls\" ]\nto setup\nend\n")
	res := Check(path, Options{SkipRun: true})
	if res.CompileOK || !res.Includes {
		t.Fatalf("includes %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.Error), "__includes") {
		t.Fatalf("signature %s err %s", Signature(res), res.Error)
	}
}

func TestIncludesLoaded(t *testing.T) {
	dir := t.TempDir()
	nls := "to helper\nend\n"
	if err := os.WriteFile(filepath.Join(dir, "actions.nls"), []byte(nls), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "__includes [ \"actions.nls\" ]\nto setup\n  helper\nend\n"
	path := filepath.Join(dir, "inc.nlogo")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Check(path, Options{SkipRun: true})
	if !res.CompileOK {
		t.Fatalf("stage %s: %s", res.Stage, res.Error)
	}
}

func TestSetupRuntimeError(t *testing.T) {
	path := writeModel(t, "fd.nlogo", "to setup\n  fd 1\nend\n")
	res := Check(path, Options{Timeout: 5 * time.Second, Ticks: 1, Seed: 1})
	if !res.CompileOK {
		t.Fatalf("compile: %s", res.Error)
	}
	if res.Setup != "error" {
		t.Fatalf("setup %s: %s", res.Setup, res.Error)
	}
	if res.Go != "skipped" {
		t.Fatalf("go %s", res.Go)
	}
}

func TestGoTicks(t *testing.T) {
	src := "to setup\n  clear-all\n  reset-ticks\nend\nto go\n  tick\nend\n"
	path := writeModel(t, "tick.nlogo", src)
	res := Check(path, Options{Timeout: 5 * time.Second, Ticks: 2, Seed: 1})
	if res.Setup != "ok" || res.Go != "ok" || res.GoTicks != 2 {
		t.Fatalf("setup %s go %s ticks %d err %s", res.Setup, res.Go, res.GoTicks, res.Error)
	}
}

func TestOversizedWorldSkipsRun(t *testing.T) {
	src := `<?xml version="1.0"?>
<model>
  <code><![CDATA[to setup
  clear-all
end
]]></code>
  <widgets>
    <view minPxcor="-400" maxPxcor="400" minPycor="-2" maxPycor="2" patchSize="1" wrappingAllowedX="true" wrappingAllowedY="true"/>
  </widgets>
</model>`
	path := writeModel(t, "big.nlogox", src)
	res := Check(path, Options{Timeout: 5 * time.Second, Ticks: 1, Seed: 1})
	if !res.CompileOK || res.Stage != "world" || res.Setup != "skipped" {
		t.Fatalf("%+v", res)
	}
}

func TestWidgetCommand(t *testing.T) {
	src := `<?xml version="1.0"?>
<model>
  <code><![CDATA[to setup
  clear-all
end
]]></code>
  <widgets>
    <view minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" patchSize="8" wrappingAllowedX="true" wrappingAllowedY="true"/>
    <monitor>no-such-reporter</monitor>
    <button kind="Observer">no-such-command</button>
  </widgets>
</model>`
	// The monitor is first. An unknown reporter is still a name, so the
	// button command is the first error.
	path := writeModel(t, "widgets.nlogox", src)
	res := Check(path, Options{SkipRun: true})
	if !res.CodeOK || res.CompileOK || res.Stage != "widget" {
		t.Fatalf("stage %s code %v compile %v err %s", res.Stage, res.CodeOK, res.CompileOK, res.Error)
	}
	if !strings.Contains(res.Error, "button") || !strings.Contains(res.Error, "NO-SUCH-COMMAND") {
		t.Fatalf("error %s", res.Error)
	}
}

func TestSkipRunDoesNotExecute(t *testing.T) {
	path := writeModel(t, "loop.nlogo", "to setup\n  loop [ ]\nend\n")
	start := time.Now()
	res := Check(path, Options{SkipRun: true, Timeout: time.Second, Ticks: 1, Seed: 1})
	if time.Since(start) > 2*time.Second {
		t.Fatal("skip-run executed setup")
	}
	if !res.CompileOK || res.Setup != "not-run" {
		t.Fatalf("%+v", res)
	}
}

func TestLibraryWhenEnabled(t *testing.T) {
	if os.Getenv("MODELCHECK") == "" {
		t.Skip("set MODELCHECK=1 to compile every model under models/library")
	}
	root := modelsRoot(t)
	files, err := Walk(filepath.Join(root, "library"))
	if err != nil {
		t.Fatal(err)
	}
	robby := filepath.Join(root, "Robby the Robot.nlogo")
	files = append(files, robby)
	var rs []Result
	for _, path := range files {
		rs = append(rs, Check(path, Options{SkipRun: true, Seed: 1}))
	}
	sum := Summarize(rs)
	t.Logf("total %d compile_ok %d compile_fail %d", sum.Total, sum.CompileOK, sum.CompileFail)
	if sum.Total < 600 {
		t.Fatalf("only found %d models", sum.Total)
	}
}

func contains(xs []string, want string) bool {
	for _, s := range xs {
		if s == want {
			return true
		}
	}
	return false
}

func modelsRoot(t *testing.T) string {
	t.Helper()
	root, err := models.ModelsRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeModel(t *testing.T, name, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
