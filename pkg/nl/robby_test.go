package nl

import (
	"fmt"
	"strings"
	"testing"
)

// robbyPlotPrimitives are the plot commands in the official Robby the Robot
// setup procedure. The file under models/ stays unmodified.
//
// TODO(#12): plot primitives are still missing on main until the
// plot-commands pull request merges. The unmodified model is expected to
// fail compilation only on these five names. Do not widen this list. After
// that pull request merges, compile the file as shipped and delete this skip.
var robbyPlotPrimitives = []string{
	"plot-pen-up",
	"plotxy",
	"set-plot-y-range",
	"plot",
	"plot-pen-down",
}

// TestRobbyTheRobotCompiles parses the Models Library sample. It tolerates
// only the plot primitives in robbyPlotPrimitives being undefined, then
// compiles that copy. See the TODO on robbyPlotPrimitives.
func TestRobbyTheRobotCompiles(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	if m.View.MinPxcor != 0 || m.View.MaxPxcor != 9 || m.View.MinPycor != 0 || m.View.MaxPycor != 9 || m.View.WrapX || m.View.WrapY || m.View.PatchSize != 30 {
		t.Fatalf("view %+v", m.View)
	}
	if !strings.Contains(m.Code, "plot-pen-up") || !strings.Contains(m.Code, "Public Domain") {
		t.Fatal("model file was rewritten; the test expects the official source")
	}
	foundPlot := false
	for _, note := range m.Notes {
		if note == "plot not implemented: Best Fitness" {
			foundPlot = true
		}
	}
	if !foundPlot {
		t.Fatalf("notes %v, want the Best Fitness plot recorded and not run", m.Notes)
	}
	if _, err := m.Compile(); err == nil {
		t.Fatal("unmodified model compiled; plot commands were expected to be unsupported")
	}
	skipped := commentKnownPlotCommands(t, m)
	if strings.Join(skipped, ",") != strings.Join(robbyPlotPrimitives, ",") {
		t.Fatalf("skipped %v, want %v", skipped, robbyPlotPrimitives)
	}
	if _, err := m.Compile(); err != nil {
		t.Fatalf("compile after skipping known plot commands: %v", err)
	}
	if _, err := m.NewVM(1); err != nil {
		t.Fatal(err)
	}
}

// TestRobbyTheRobotSetupAndGo runs setup and one go tick after the known
// plot commands are commented out of a copy of the source. fd that reaches
// the non-wrapping edge stops inside the world, so those calls succeed.
// The dual-interpreter trial from test/robby-combined stays out until the
// unmodified model compiles.
func TestRobbyTheRobotSetupAndGo(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	commentKnownPlotCommands(t, m)
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := mustEval(t, vm, "count individuals"); got != "100" {
		t.Fatalf("individuals %s", got)
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatalf("go: %v", err)
	}
	if got := mustEval(t, vm, "ticks"); got != "1" {
		t.Fatalf("ticks %s", got)
	}
	if got := mustEval(t, vm, "count individuals"); got != "100" {
		t.Fatalf("individuals after go %s", got)
	}
}

// commentKnownPlotCommands comments out each plot command the compiler
// rejects, in the order the compiler reports them. Any other missing name
// fails the test.
func commentKnownPlotCommands(t *testing.T, m *FileModel) []string {
	t.Helper()
	known := map[string]bool{}
	for _, name := range robbyPlotPrimitives {
		known[name] = true
	}
	code := m.Code
	var skipped []string
	for n := 0; n <= len(robbyPlotPrimitives); n++ {
		m.Code = code
		_, err := m.Compile()
		if err == nil {
			return skipped
		}
		line, name, ok := robbyMissingPrimitive(err.Error())
		if !ok {
			t.Fatalf("compile: %v", err)
		}
		if !known[name] {
			t.Fatalf("unsupported primitive %s is outside the known plot set: %v", name, err)
		}
		skipped = append(skipped, name)
		lines := strings.Split(code, "\n")
		if line < 1 || line > len(lines) {
			t.Fatalf("bad line %d in %v", line, err)
		}
		lines[line-1] = "; known-unsupported plot primitive (feature/plot-commands): " + lines[line-1]
		code = strings.Join(lines, "\n")
	}
	t.Fatal("model still does not compile after skipping known plot commands")
	return nil
}

func robbyMissingPrimitive(msg string) (int, string, bool) {
	var line int
	var name string
	if _, err := fmt.Sscanf(msg, "line %d: nothing named %s has been defined", &line, &name); err != nil {
		return 0, "", false
	}
	return line, strings.ToLower(name), true
}
