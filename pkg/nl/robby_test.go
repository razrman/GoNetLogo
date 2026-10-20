package nl

import (
	"fmt"
	"strings"
	"testing"
)

// robbyPlotPrimitives are the plot commands in the official Robby the Robot
// setup procedure. They are not implemented in this tree; feature/plot-commands
// is the branch that should add them. Tests below comment these out of a
// copy of the source so the rest of the model can compile. The file under
// models/ stays unmodified.
var robbyPlotPrimitives = []string{
	"plot-pen-up",
	"plotxy",
	"set-plot-y-range",
	"plot",
	"plot-pen-down",
}

// TestRobbyTheRobotCompiles parses the Models Library sample and compiles
// it after skipping the known-unsupported plot primitives listed above.
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

// TestRobbyTheRobotSetupHitsWorldEdge runs setup and go on the compiled
// model. Both stop because fd tries to leave the non-wrapping world.
// feature/fd-world-edge is the branch that should cover this; it is not
// implemented here. Plot commands are skipped first, as in
// TestRobbyTheRobotCompiles.
func TestRobbyTheRobotSetupHitsWorldEdge(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	commentKnownPlotCommands(t, m)
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = vm.Call("setup")
	if err == nil || !strings.Contains(err.Error(), "cannot move beyond the world's edge") {
		t.Fatalf("setup error %v, want the non-wrapping world edge", err)
	}
	// Fitness is computed before display-fitness moves the individuals.
	if got := mustEval(t, vm, "count individuals"); got == "0" {
		t.Fatal("setup died before the population existed")
	}
	_, err = vm.Call("go")
	if err == nil || !strings.Contains(err.Error(), "cannot move beyond the world's edge") {
		t.Fatalf("go error %v, want the non-wrapping world edge", err)
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
