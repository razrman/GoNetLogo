package nl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedWorldImport(t *testing.T) {
	vm := NewVM(mustParse(t, "to go end"), nil)
	for _, line := range []string{
		`import-world "w.csv"`,
		`export-world "w.csv"`,
		`import-pcolors "p.png"`,
		`import-pcolors-rgb "p.png"`,
	} {
		_, err := vm.ExecuteLine(line)
		if err == nil || !strings.Contains(err.Error(), "not supported yet") {
			t.Fatalf("%s: %v", line, err)
		}
	}
}

func TestFileReadConstantsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	body := "1 -2.5 \"hello\" true false nobody [2 nobody [3 \"a\\\"b\"]]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	vm := NewVM(mustParse(t, `
globals [secret got]
to read-one
  set got file-read
end
`), nil)
	vm.ModelDir = dir
	mustEval(t, vm, `set secret 99`)
	mustEval(t, vm, `file-open "data.txt"`)
	want := []string{"1", "-2.5", "hello", "true", "false", "nobody", `[2 nobody [3 "a\"b"]]`}
	for _, w := range want {
		mustEval(t, vm, "read-one")
		if got := mustEval(t, vm, "got"); got != w {
			t.Fatalf("file-read got %q, want %q", got, w)
		}
	}
	mustEval(t, vm, "file-close")

	bad := filepath.Join(dir, "code.txt")
	if err := os.WriteFile(bad, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustEval(t, vm, `file-open "code.txt"`)
	_, err := vm.ExecuteLine("read-one")
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Fatalf("file-read of a name: %v", err)
	}
	if got := mustEval(t, vm, "secret"); got != "99" {
		t.Fatalf("file-read changed a global to %s", got)
	}
}

func TestFileReadCharacters(t *testing.T) {
	dir := t.TempDir()
	vm := NewVM(mustParse(t, `
globals [got]
to write-chars
  file-open "chars.txt"
  file-type "abcdef"
  file-close
end
to take [n]
  set got file-read-characters n
end
`), nil)
	vm.ModelDir = dir
	mustEval(t, vm, "write-chars")
	mustEval(t, vm, `file-open "chars.txt"`)
	mustEval(t, vm, "take 4")
	if got := mustEval(t, vm, "got"); got != "abcd" {
		t.Fatalf("first read %q", got)
	}
	mustEval(t, vm, "take 2")
	if got := mustEval(t, vm, "got"); got != "ef" {
		t.Fatalf("second read %q", got)
	}
	_, err := vm.ExecuteLine("take 1")
	if err == nil || !strings.Contains(err.Error(), "end of file") {
		t.Fatalf("short read: %v", err)
	}
}

func TestUserFileHook(t *testing.T) {
	vm := NewVM(mustParse(t, "to go end"), nil)
	if got := mustEval(t, vm, "user-file"); got != "false" {
		t.Fatalf("headless user-file %s", got)
	}
	if got := mustEval(t, vm, "user-new-file"); got != "false" {
		t.Fatalf("headless user-new-file %s", got)
	}
	if got := mustEval(t, vm, "user-directory"); got != "false" {
		t.Fatalf("headless user-directory %s", got)
	}
	vm.UserFile = func() (string, bool) { return "chosen.txt", true }
	if got := mustEval(t, vm, "user-file"); got != "chosen.txt" {
		t.Fatalf("hook user-file %s", got)
	}
	vm.UserNewFile = func() (string, bool) { return "", false }
	if got := mustEval(t, vm, "user-new-file"); got != "false" {
		t.Fatalf("cancel user-new-file %s", got)
	}
	vm.UserDirectory = func() (string, bool) { return "plots", true }
	if got := mustEval(t, vm, "user-directory"); got != "plots" {
		t.Fatalf("hook user-directory %s", got)
	}
}

func TestHatchThenDie(t *testing.T) {
	// Wolf Sheep style: a sheep hatches, then dies, in the same ask.
	// The offspring stays. Commands after die do not run. Calling die
	// with self already set must not panic out of Call or ExecuteLine.
	vm := NewVM(mustParse(t, `
globals [flag babies]
breed [sheep a-sheep]
sheep-own [energy]
to go
  create-sheep 1 [ set energy 10 ]
  ask sheep [
    if energy > 0 [
      hatch 1 [
        set energy 5
        set babies babies + 1
      ]
      die
      set flag 1
    ]
  ]
end
to die-first
  create-sheep 1
  ask sheep [
    die
    hatch 1 [ set babies babies + 1 ]
    set flag 1
  ]
end
to die-now
  die
  set flag 1
end
`), nil)
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "count sheep"); got != "1" {
		t.Fatalf("count sheep %s, want the hatchling only", got)
	}
	if got := mustEval(t, vm, "babies"); got != "1" {
		t.Fatalf("babies %s", got)
	}
	if got := mustEval(t, vm, "flag"); got != "0" {
		t.Fatalf("commands after die ran, flag %s", got)
	}
	if got := mustEval(t, vm, "[energy] of one-of sheep"); got != "5" {
		t.Fatalf("hatchling energy %s", got)
	}

	vm = NewVM(vm.Prog, nil)
	if _, err := vm.Call("die-first"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "count sheep"); got != "0" {
		t.Fatalf("hatch after die created %s sheep", got)
	}
	if got := mustEval(t, vm, "babies"); got != "0" {
		t.Fatalf("hatch after die ran, babies %s", got)
	}

	vm = NewVM(vm.Prog, nil)
	mustEval(t, vm, "create-sheep 1")
	turtle := vm.World.Turtle(0)
	vm.self = turtle
	if _, err := vm.Call("die-now"); err != nil {
		t.Fatal(err)
	}
	if !turtle.IsDead() {
		t.Fatal("turtle survived die-now")
	}
	if got := mustEval(t, vm, "flag"); got != "0" {
		t.Fatalf("die-now continued, flag %s", got)
	}

	vm = NewVM(vm.Prog, nil)
	mustEval(t, vm, "create-sheep 1")
	vm.self = vm.World.Turtle(0)
	if _, err := vm.ExecuteLine("die"); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine("die set flag 1"); err != nil {
		t.Fatal(err)
	}
}

func TestEveryTimersDropDeadAgents(t *testing.T) {
	vm := NewVM(mustParse(t, `
globals [n]
to bump
  every 0 [ set n n + 1 ]
end
to go
  crt 2
  ask turtles [ bump ]
  ask turtle 0 [ die ]
end
`), nil)
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	alive := 0
	for k := range vm.everyTimers {
		if k.agent != nil && !k.agent.IsDead() {
			alive++
		}
		if k.agent != nil && k.agent.IsDead() {
			t.Fatal("timer kept for a dead turtle")
		}
	}
	if alive != 1 {
		t.Fatalf("live timers %d, want 1", alive)
	}
	mustEval(t, vm, "clear-all")
	for k := range vm.everyTimers {
		if k.agent != nil {
			t.Fatal("clear-all left an agent timer")
		}
	}
}

func TestBreedAgentSetPrint(t *testing.T) {
	vm := NewVM(mustParse(t, `
breed [wolves wolf]
to go
  create-wolves 3
end
`), nil)
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "wolves"); got != "wolves" {
		t.Fatalf("full breed prints %q", got)
	}
	if got := mustEval(t, vm, "up-to-n-of 2 wolves"); got != "(agentset, 2 turtles)" {
		t.Fatalf("subset prints %q", got)
	}
}

func TestTieFollowerCannotLeaveWorld(t *testing.T) {
	vm := NewVM(mustParse(t, `
to go
  crt 1 [ setxy 0 4.6 set heading 0 ]
  crt 1 [ setxy 0 5.2 set heading 0 ]
  ask turtle 0 [
    create-link-to turtle 1 [ tie ]
    fd 0.5
  ]
end
`), NewWorld(-5, 5, -5, 5, false, false))
	_, err := vm.Call("go")
	if err == nil || !strings.Contains(err.Error(), "tied") {
		t.Fatalf("tied move past the edge: %v", err)
	}
}

func TestShapesFromModel(t *testing.T) {
	vm := NewVM(mustParse(t, "to go end"), nil)
	got := mustEval(t, vm, "shapes")
	var names []string
	for _, sh := range vm.World.Shapes.All() {
		names = append(names, `"`+sh.Name+`"`)
	}
	want := "[" + strings.Join(names, " ") + "]"
	if got != want {
		t.Fatalf("shapes %s, want the model library", got)
	}
	if !strings.Contains(got, `"airplane"`) {
		t.Fatalf("shapes missing airplane: %s", got)
	}
}

func TestIncludesMergeAndRun(t *testing.T) {
	dir := t.TempDir()
	nls := "globals [from-include]\nto helper\n  set from-include 7\nend\n"
	if err := os.WriteFile(filepath.Join(dir, "actions.nls"), []byte(nls), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &FileModel{
		Name: "m.nlogo",
		Path: filepath.Join(dir, "m.nlogo"),
		Code: "__includes [ \"actions.nls\" ]\nto setup\n  helper\nend\n",
		View: DefaultView(),
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "from-include"); got != "7" {
		t.Fatalf("included procedure left %s", got)
	}

	missing := &FileModel{
		Path: filepath.Join(dir, "missing.nlogo"),
		Code: `__includes [ "no-such.nls" ]` + "\nto setup\nend\n",
		View: DefaultView(),
	}
	if _, err := missing.Compile(); err == nil || !strings.Contains(err.Error(), "__includes") {
		t.Fatalf("missing include: %v", err)
	}
}

func TestLayoutSpringStaysInside(t *testing.T) {
	vm := NewVM(mustParse(t, `
to go
  crt 4 [ setxy 2 2 ]
  ask turtle 0 [ create-link-with turtle 1 ]
  ask turtle 1 [ create-link-with turtle 2 ]
  ask turtle 2 [ create-link-with turtle 3 ]
  layout-spring turtles links 1 0.2 8
end
`), NewWorld(-2, 2, -2, 2, false, false))
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	for _, a := range vm.World.Turtles().Agents {
		turt := a.(*Turtle)
		if turt.X < -2.5 || turt.X >= 2.5 || turt.Y < -2.5 || turt.Y >= 2.5 {
			t.Fatalf("turtle %d left the world at %v %v", turt.Who, turt.X, turt.Y)
		}
	}
}
