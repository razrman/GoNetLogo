package nl

import "testing"

// New turtles start pen-up. Movement must not ink the drawing layer until
// pen-down (or set pen-mode "down"). Hatch copies the parent's pen-mode.
func TestTurtlePenModeDefaultsUp(t *testing.T) {
	vm := newTestVM(t, "", -5, 5, true, 1)
	mustRun(t, vm, `crt 2 [ fd 3 ]`)
	if n := len(vm.World.Marks()); n != 0 {
		t.Fatalf("create-turtles left %d drawing marks without pen-down: %+v", n, vm.World.Marks())
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "up" {
		t.Fatalf("pen-mode of new turtle = %q, want up", got)
	}
	if got := mustEval(t, vm, "[pen-size] of turtle 0"); got != "1" {
		t.Fatalf("pen-size of new turtle = %q, want 1", got)
	}

	mustRun(t, vm, `ask turtle 0 [ hatch 1 [ fd 2 ] ]`)
	if n := len(vm.World.Marks()); n != 0 {
		t.Fatalf("hatch from pen-up parent left %d marks: %+v", n, vm.World.Marks())
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 2"); got != "up" {
		t.Fatalf("hatched pen-mode = %q, want up", got)
	}

	mustRun(t, vm, `ask patch 1 1 [ sprout 1 [ fd 2 ] ]`)
	if n := len(vm.World.Marks()); n != 0 {
		t.Fatalf("sprout left %d marks: %+v", n, vm.World.Marks())
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 3"); got != "up" {
		t.Fatalf("sprouted pen-mode = %q, want up", got)
	}

	// pen-down still draws in 2D
	mustRun(t, vm, `ask turtle 0 [ pen-down fd 2 ]`)
	marks := vm.World.Marks()
	if len(marks) == 0 {
		t.Fatal("pen-down turtle did not draw")
	}
	for _, m := range marks {
		if !m.Pen || m.Erase {
			t.Fatalf("expected pen stroke, got %+v", m)
		}
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "down" {
		t.Fatalf("pen-mode after pen-down = %q", got)
	}

	// hatch from pen-down parent inherits down and draws
	before := len(vm.World.Marks())
	mustRun(t, vm, `ask turtle 0 [ hatch 1 [ fd 1 ] ]`)
	if got := mustEval(t, vm, "[pen-mode] of turtle 4"); got != "down" {
		t.Fatalf("hatch from pen-down parent = %q, want down", got)
	}
	if len(vm.World.Marks()) <= before {
		t.Fatal("hatch from pen-down parent did not draw while moving")
	}

	// pen-up and pen-erase still work
	mustRun(t, vm, `ask turtle 0 [ pen-up ]`)
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "up" {
		t.Fatalf("pen-mode after pen-up = %q", got)
	}
	mustRun(t, vm, `ask turtle 0 [ pen-erase ]`)
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "erase" {
		t.Fatalf("pen-mode after pen-erase = %q", got)
	}
}

func TestTurtlePenModeDefaultsUp3D(t *testing.T) {
	vm := newTestVM3D(t, "", -5, 5, 1)
	mustRun(t, vm, `crt 1 [ set pitch 90 fd 2 ]`)
	if n := len(vm.World.Marks()); n != 0 {
		t.Fatalf("3D create-turtles left %d marks without pen-down: %+v", n, vm.World.Marks())
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "up" {
		t.Fatalf("3D pen-mode = %q, want up", got)
	}
	mustRun(t, vm, `ask turtle 0 [ pen-down fd 1 ]`)
	marks := vm.World.Marks()
	if len(marks) == 0 {
		t.Fatal("3D pen-down turtle did not draw")
	}
	if !marks[0].Pen || marks[0].Erase {
		t.Fatalf("3D stroke %+v", marks[0])
	}
	if marks[0].Z == marks[0].Z2 && marks[0].X == marks[0].X2 && marks[0].Y == marks[0].Y2 {
		t.Fatalf("3D stroke has no length: %+v", marks[0])
	}
}
