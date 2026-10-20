package nl

import (
	"strings"
	"testing"
)

// TestRobbyTheRobotCompiles loads the unmodified Models Library sample.
// Plot commands and non-wrapping movement are both present on this branch,
// so the file compiles and builds a VM.
func TestRobbyTheRobotCompiles(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	if m.View.MinPxcor != 0 || m.View.MaxPxcor != 9 || m.View.MinPycor != 0 || m.View.MaxPycor != 9 || m.View.WrapX || m.View.WrapY || m.View.PatchSize != 30 {
		t.Fatalf("view %+v", m.View)
	}
	if !strings.Contains(m.Code, "plot-pen-up") || !strings.Contains(m.Code, "Public Domain") {
		t.Fatal("model file was rewritten; the test expects the official source")
	}
	for _, note := range m.Notes {
		if strings.Contains(note, "plot not implemented") {
			t.Fatalf("plot widget was skipped: %v", m.Notes)
		}
	}
	var plot *Widget
	for i := range m.Widgets {
		if m.Widgets[i].Kind == "plot" {
			plot = &m.Widgets[i]
		}
	}
	if plot == nil || plot.Display != "Best Fitness" || !plot.AutoPlot {
		t.Fatalf("Best Fitness plot %+v", plot)
	}
	if len(plot.Pens) != 1 || plot.Pens[0].Update != "plot best-fitness" {
		t.Fatalf("pen %+v", plot.Pens)
	}
	if _, err := m.Compile(); err != nil {
		t.Fatalf("compile unmodified model: %v", err)
	}
	if _, err := m.NewVM(1); err != nil {
		t.Fatal(err)
	}
}

// TestRobbyTheRobotRuns drives setup, one generation, and the robot trial
// on both interpreters. fd must stop at the box edge, and the Best Fitness
// pen must record the generation.
func TestRobbyTheRobotRuns(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	var first string
	for _, walk := range []bool{false, true} {
		mode := "closure compiler"
		vm, err := m.NewVM(1)
		if err != nil {
			t.Fatal(err)
		}
		if walk {
			mode = "tree walker"
			forceTreeWalker(vm.Prog)
		}
		if _, err := vm.Call("setup"); err != nil {
			t.Fatalf("%s setup: %v", mode, err)
		}
		ticks, err := vm.World.Ticks()
		if err != nil || ticks != 0 {
			t.Fatalf("%s ticks after setup %v %v", mode, ticks, err)
		}
		if got := mustEval(t, vm, "count individuals"); got != "100" {
			t.Fatalf("%s individuals %s", mode, got)
		}
		fit := mustEval(t, vm, "best-fitness")
		if _, err := vm.Call("go"); err != nil {
			t.Fatalf("%s go: %v", mode, err)
		}
		if got := mustEval(t, vm, "count individuals"); got != "100" {
			t.Fatalf("%s individuals after go %s", mode, got)
		}
		ticks, err = vm.World.Ticks()
		if err != nil || ticks != 1 {
			t.Fatalf("%s ticks after go %v %v", mode, ticks, err)
		}
		next := mustEval(t, vm, "best-fitness")
		if next == fit {
			t.Fatalf("%s best fitness did not change (%s)", mode, fit)
		}
		pts, ok := vm.PlotPoints("Best Fitness", "best")
		if !ok || len(pts) < 2 {
			t.Fatalf("%s plot points %+v", mode, pts)
		}
		last := pts[len(pts)-1]
		if !last.Down || last.X != 1 {
			t.Fatalf("%s last plot point %+v", mode, last)
		}
		views := vm.Plots()
		if len(views) != 1 || views[0].Error != "" {
			t.Fatalf("%s plot view %+v", mode, views)
		}
		if _, err := vm.Call("setup-robot-visuals"); err != nil {
			t.Fatalf("%s visuals: %v", mode, err)
		}
		if !strings.Contains(vm.OutputText(), "Setting up a new random can distribution") {
			t.Fatalf("%s visuals output %q", mode, vm.OutputText())
		}
		if _, err := vm.Call("run-trial-step"); err != nil {
			t.Fatalf("%s trial: %v", mode, err)
		}
		if !strings.Contains(vm.OutputText(), "Stepping through the best strategy") {
			t.Fatalf("%s trial output %q", mode, vm.OutputText())
		}
		if _, err := vm.Call("setup-individual-visuals"); err != nil {
			t.Fatalf("%s strategies: %v", mode, err)
		}
		if got := mustEval(t, vm, "count individuals with [not hidden?]"); got != "100" {
			t.Fatalf("%s shown individuals %s", mode, got)
		}
		sig := fit + " -> " + next
		if first == "" {
			first = sig
		} else if sig != first {
			t.Fatalf("interpreters diverged: %s vs %s", first, sig)
		}
	}
}
