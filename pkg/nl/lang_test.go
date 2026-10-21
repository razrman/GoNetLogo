package nl

import (
	"math"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Program {
	t.Helper()
	p, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	return p
}

func newTestVM(t *testing.T, src string, min, max int, wrap bool, seed int64) *VM {
	t.Helper()
	w := NewWorld(min, max, min, max, wrap, wrap)
	w.Seed(seed)
	return NewVM(mustParse(t, src), w)
}

func mustCall(t *testing.T, vm *VM, name string) bool {
	t.Helper()
	stopped, err := vm.Call(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return stopped
}

func mustEval(t *testing.T, vm *VM, expr string) string {
	t.Helper()
	out, err := vm.ExecuteLine(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return out
}

func TestArithmeticAndLists(t *testing.T) {
	vm := newTestVM(t, `
to setup clear-all reset-ticks end
to-report double [n] report n * 2 end
`, -2, 2, true, 1)
	if got := mustEval(t, vm, "2 + 3 * 4"); got != "14" {
		t.Fatalf("precedence: %s", got)
	}
	if got := mustEval(t, vm, "2 ^ 3 ^ 2"); got != "512" {
		t.Fatalf("pow: %s", got)
	}
	if got := mustEval(t, vm, "1 / 2"); got != "0.5" {
		t.Fatalf("div: %s", got)
	}
	if got := mustEval(t, vm, "-7 mod 4"); got != "1" {
		t.Fatalf("mod: %s", got)
	}
	if got := mustEval(t, vm, `"AbC" = "abc"`); got != "true" {
		t.Fatalf("string eq: %s", got)
	}
	if got := mustEval(t, vm, "map [ x -> x + 1 ] [ 1 2 3 ]"); got != "[2 3 4]" {
		t.Fatalf("map: %s", got)
	}
	if got := mustEval(t, vm, "filter [ x -> x > 1 ] [ 1 2 3 ]"); got != "[2 3]" {
		t.Fatalf("filter: %s", got)
	}
	if got := mustEval(t, vm, "reduce [ [a b] -> a + b ] [ 1 2 3 4 ]"); got != "10" {
		t.Fatalf("reduce: %s", got)
	}
	if got := mustEval(t, vm, "double 5"); got != "10" {
		t.Fatalf("reporter: %s", got)
	}
	if got := mustEval(t, vm, "scale-color red 50 0 100"); got != "15" {
		t.Fatalf("scale-color: %s", got)
	}
	if got := mustEval(t, vm, "but-first [ 10 20 30 ]"); got != "[20 30]" {
		t.Fatalf("bf: %s", got)
	}
	if got := mustEval(t, vm, `word "a" "b"`); got != "ab" {
		t.Fatalf("word: %s", got)
	}
}

func TestControlAndRNG(t *testing.T) {
	vm := newTestVM(t, `
globals [ acc ]
to setup
  clear-all
  set acc 0
  repeat 4 [ set acc acc + 1 ]
  reset-ticks
end
to go
  while [ acc < 10 ] [ set acc acc + 1 ]
  foreach [ 1 2 3 ] [ n -> set acc acc + n ]
  ifelse acc > 0 [ set acc acc ] [ set acc 0 ]
  tick
end
`, -2, 2, true, 1)
	mustCall(t, vm, "setup")
	if mustEval(t, vm, "acc") != "4" {
		t.Fatalf("repeat: %s", mustEval(t, vm, "acc"))
	}
	mustCall(t, vm, "go")
	if mustEval(t, vm, "acc") != "16" {
		t.Fatalf("while/foreach: %s", mustEval(t, vm, "acc"))
	}
	if mustEval(t, vm, "ticks") != "1" {
		t.Fatalf("ticks: %s", mustEval(t, vm, "ticks"))
	}

	vm.World.Seed(99)
	// variadic reporters need parentheses; confirm the parser says so
	if _, err := vm.ExecuteLine("list random 100 random 100 random-float 1"); err == nil {
		t.Fatal("bare list of 3 args should not parse")
	}
	a := mustEval(t, vm, "(list random 100 random 100 random-float 1)")
	vm.World.Seed(99)
	b := mustEval(t, vm, "(list random 100 random 100 random-float 1)")
	if a != b {
		t.Fatalf("seed mismatch %s vs %s", a, b)
	}
	vm.World.Seed(100)
	c := mustEval(t, vm, "(list random 100 random 100)")
	if a == c {
		t.Fatalf("different seeds matched: %s", a)
	}
}

func TestMovementWrapAndAgents(t *testing.T) {
	vm := newTestVM(t, `
to setup
  clear-all
  create-turtles 1 [
    setxy 2 0
    set heading 90
    set color red
  ]
  reset-ticks
end
to step
  ask turtles [ fd 1 ]
end
to edge
  clear-all
  create-turtles 1 [ setxy 2 0 set heading 90 ]
end
`, -2, 2, true, 1)
	mustCall(t, vm, "setup")
	if mustEval(t, vm, "[ xcor ] of one-of turtles") != "2" {
		t.Fatalf("xcor %s", mustEval(t, vm, "[ xcor ] of one-of turtles"))
	}
	mustCall(t, vm, "step")
	if mustEval(t, vm, "[ xcor ] of one-of turtles") != "-2" {
		t.Fatalf("wrap xcor %s", mustEval(t, vm, "[ xcor ] of one-of turtles"))
	}
	if mustEval(t, vm, "count turtles with [ color = red ]") != "1" {
		t.Fatalf("with: %s", mustEval(t, vm, "count turtles with [ color = red ]"))
	}
	// corner of a non-wrapping world has 3 neighbors
	box := newTestVM(t, `
to setup
  clear-all
  ask patch -2 -2 [ sprout 1 ]
  reset-ticks
end
`, -2, 2, false, 1)
	mustCall(t, box, "setup")
	if got := mustEval(t, box, "[ count neighbors ] of patch -2 -2"); got != "3" {
		t.Fatalf("neighbors corner: %s", got)
	}
	if got := mustEval(t, box, "[ count neighbors4 ] of patch 0 0"); got != "4" {
		t.Fatalf("neighbors4: %s", got)
	}
	// fd stops inside a non-wrapping world instead of raising an error.
	if _, err := box.ExecuteLine("ask turtles [ fd 10 ]"); err != nil {
		t.Fatal(err)
	}
	for _, a := range box.World.Turtles().Alive() {
		tu := a.(*Turtle)
		if tu.X < -2.5 || tu.X >= 2.5 || tu.Y < -2.5 || tu.Y >= 2.5 {
			t.Fatalf("fd left the box at %v %v", tu.X, tu.Y)
		}
	}
	if _, err := box.ExecuteLine("ask turtles [ setxy 3 0 ]"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected setxy outside error, got %v", err)
	}
}

func TestAskCreateDieAndLinks(t *testing.T) {
	vm := newTestVM(t, `
breed [ mice mouse ]
directed-link-breed [ arcs arc ]
turtles-own [ tag ]
to setup
  clear-all
  create-mice 3 [
    set tag who
  ]
  ask turtle 0 [ create-arc-to turtle 1 ]
  reset-ticks
end
to cull
  ask mice with [ who = 0 ] [ die ]
end
`, -3, 3, true, 2)
	mustCall(t, vm, "setup")
	if mustEval(t, vm, "count mice") != "3" {
		t.Fatal(mustEval(t, vm, "count mice"))
	}
	if mustEval(t, vm, "count arcs") != "1" {
		t.Fatal(mustEval(t, vm, "count arcs"))
	}
	if mustEval(t, vm, "sum [ tag ] of mice") != "3" {
		t.Fatalf("sum of: %s", mustEval(t, vm, "sum [ tag ] of mice"))
	}
	mustCall(t, vm, "cull")
	if mustEval(t, vm, "count mice") != "2" {
		t.Fatal(mustEval(t, vm, "count mice"))
	}
	if mustEval(t, vm, "count links") != "0" {
		t.Fatalf("links should die with the turtle: %s", mustEval(t, vm, "count links"))
	}
}

func TestDiffuseAndHatch(t *testing.T) {
	vm := newTestVM(t, `
patches-own [ chemical ]
to setup
  clear-all
  ask patch 0 0 [ set chemical 80 ]
  diffuse chemical 0.5
  create-turtles 1 [ set color blue hatch 2 [ set color red ] ]
  reset-ticks
end
`, -2, 2, true, 1)
	mustCall(t, vm, "setup")
	center := mustEval(t, vm, "[ chemical ] of patch 0 0")
	if center != "40" {
		t.Fatalf("center chemical %s", center)
	}
	neigh := mustEval(t, vm, "[ chemical ] of patch 1 0")
	if neigh != "5" {
		t.Fatalf("neighbor chemical %s", neigh)
	}
	if mustEval(t, vm, "count turtles with [ color = red ]") != "2" {
		t.Fatal(mustEval(t, vm, "count turtles with [ color = red ]"))
	}
	if mustEval(t, vm, "count turtles") != "3" {
		t.Fatal(mustEval(t, vm, "count turtles"))
	}
}

func TestStopAndInterfaceGlobal(t *testing.T) {
	prog := mustParse(t, `
globals [ counter ]
to go
  set counter counter + 1
  if counter >= 3 [ stop ]
  tick
end
to setup
  clear-all
  reset-ticks
end
`)
	prog.InterfaceVars["density"] = true
	w := NewWorld(-1, 1, -1, 1, true, true)
	w.Seed(1)
	w.SetGlobal("density", Num(70))
	vm := NewVM(prog, w)
	mustCall(t, vm, "setup")
	if mustEval(t, vm, "density") != "70" {
		t.Fatalf("clear-all cleared a slider: %s", mustEval(t, vm, "density"))
	}
	var stopped bool
	for i := 0; i < 5; i++ {
		stopped = mustCall(t, vm, "go")
		if stopped {
			break
		}
	}
	if !stopped {
		t.Fatal("go should stop")
	}
	if mustEval(t, vm, "counter") != "3" {
		t.Fatalf("counter %s", mustEval(t, vm, "counter"))
	}
}

func TestButtonStopAndTurtleButton(t *testing.T) {
	vm := newTestVM(t, `
to go
  tick
  stop
end
to wander
  fd 1
end
to setup
  clear-all
  create-turtles 4
  reset-ticks
end
`, -4, 4, true, 1)
	mustCall(t, vm, "setup")
	stmts, err := CompileButton(vm.Prog, "go", "observer")
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := vm.RunCompiled(stmts)
	if err != nil || !stopped {
		t.Fatalf("stopped %v err %v", stopped, err)
	}
	if mustEval(t, vm, "ticks") != "1" {
		t.Fatal(mustEval(t, vm, "ticks"))
	}
	before := mustEval(t, vm, "sum [ xcor ] of turtles")
	walk, err := CompileButton(vm.Prog, "wander", "turtle")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunCompiled(walk); err != nil {
		t.Fatal(err)
	}
	after := mustEval(t, vm, "sum [ xcor ] of turtles")
	if before == after {
		t.Fatalf("turtle button did not move anyone: %s", after)
	}
	_ = math.NaN
}

func TestResizeWorld100(t *testing.T) {
	vm := newTestVM(t, "", -2, 2, true, 1)
	if _, err := vm.ExecuteLine("crt 5"); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine("ask patches [ set pcolor red ]"); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine("resize-world 0 99 0 99"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "count patches"); got != "10000" {
		t.Fatalf("count patches = %s, want 10000", got)
	}
	if got := mustEval(t, vm, "count turtles"); got != "0" {
		t.Fatalf("count turtles = %s, want 0", got)
	}
	if got := mustEval(t, vm, "count links"); got != "0" {
		t.Fatalf("count links = %s, want 0", got)
	}
	if got := mustEval(t, vm, "count patches with [ pcolor = black ]"); got != "10000" {
		t.Fatalf("patches were not reset: %s", got)
	}
	checks := map[string]string{
		"min-pxcor": "0", "max-pxcor": "99", "min-pycor": "0", "max-pycor": "99",
		"world-width": "100", "world-height": "100",
	}
	for expr, want := range checks {
		if got := mustEval(t, vm, expr); got != want {
			t.Fatalf("%s = %s, want %s", expr, got, want)
		}
	}
	if _, err := vm.ExecuteLine("set-patch-size 4.5"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "patch-size"); got != "4.5" {
		t.Fatalf("patch-size = %s", got)
	}
	if _, err := vm.ExecuteLine("resize-world 1 10 0 10"); err == nil {
		t.Fatal("resize-world accepted a world that excludes the origin")
	}
}

func TestApplyView(t *testing.T) {
	w := NewWorld(-2, 2, -2, 2, true, true)
	w.CreateTurtle("turtles", 0, 0, 0, 0)
	if err := w.ApplyView(0, 99, 0, 99, 8, false, true); err != nil {
		t.Fatal(err)
	}
	if w.Width() != 100 || w.Height() != 100 || len(w.Turtles().Agents) != 0 {
		t.Fatalf("size %dx%d turtles %d", w.Width(), w.Height(), len(w.Turtles().Agents))
	}
	if w.PatchSize != 8 || w.WrapX || !w.WrapY {
		t.Fatalf("view size %v wrap %v %v", w.PatchSize, w.WrapX, w.WrapY)
	}
	if err := w.ApplyView(1, 10, 0, 10, 8, false, false); err == nil {
		t.Fatal("ApplyView accepted a world that excludes the origin")
	}
	if err := w.ApplyView(0, 99, 0, 99, 0, false, false); err == nil {
		t.Fatal("ApplyView accepted a zero patch size")
	}
	if err := w.ApplyView(0, 300, 0, 10, 8, false, false); err == nil {
		t.Fatal("ApplyView accepted a side longer than 256")
	}
}
