package session

import (
	"strings"
	"testing"
)

func TestFrameCarriesInterfaceBoxes(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.LoadModelSource("placed.nlogo", placedWidgetModel()); err != nil {
		t.Fatal(err)
	}
	frame := s.Frame()
	if frame.World.ViewX != 280 || frame.World.ViewY != 10 || frame.World.ViewWidth != 312 || frame.World.ViewHeight != 312 {
		t.Fatalf("view box %+v", frame.World)
	}
	var buttonX, buttonW int
	for _, w := range frame.Widgets {
		if w.Kind == "button" && w.Display == "setup" {
			buttonX, buttonW = w.X, w.Width
		}
	}
	if buttonX != 16 || buttonW != 94 {
		t.Fatalf("button box %d %d", buttonX, buttonW)
	}
	spec, err := s.WidgetYAML()
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(spec, "x: 16\n", "x: 40\n", 1)
	if _, err := s.SetWidgetYAML(edited); err != nil {
		t.Fatal(err)
	}
	moved := 0
	for _, w := range s.Frame().Widgets {
		if w.Kind == "button" && w.Display == "setup" {
			moved = w.X
		}
	}
	if moved != 40 {
		t.Fatalf("button x after yaml %d", moved)
	}
}

func TestViewYAMLMovesAndResizesTheWorld(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.LoadModelSource("placed.nlogo", placedWidgetModel()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("crt 1"); err != nil {
		t.Fatal(err)
	}
	spec, err := s.WidgetYAML()
	if err != nil || !strings.Contains(spec, "kind: view") || !strings.HasPrefix(spec, "- kind: view\n") {
		t.Fatalf("spec %v\n%s", err, spec)
	}
	moved := strings.Replace(spec, "x: 280\n", "x: 300\n", 1)
	if _, err := s.SetWidgetYAML(moved); err != nil {
		t.Fatal(err)
	}
	frame := s.Frame()
	if frame.World.ViewX != 300 {
		t.Fatalf("view x %d", frame.World.ViewX)
	}
	if len(frame.World.Turtles) != 1 {
		t.Fatal("moving the view removed turtles")
	}
	sized := strings.Replace(spec, "patchSize: 12", "patchSize: 8", 1)
	if _, err := s.SetWidgetYAML(sized); err != nil {
		t.Fatal(err)
	}
	frame = s.Frame()
	if frame.World.PatchSize != 8 || len(frame.World.Turtles) != 1 {
		t.Fatalf("patch size %v turtles %d", frame.World.PatchSize, len(frame.World.Turtles))
	}
	// The saved document still has x: 280 because sized was built from the
	// original spec. Bounds below start from the current document.
	current, err := s.WidgetYAML()
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(current, "minPxcor: -12", "minPxcor: 2", 1)
	if _, err := s.SetWidgetYAML(bad); err == nil || !strings.Contains(err.Error(), "view") || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("origin %v", err)
	}
	if len(s.Frame().World.Turtles) != 1 {
		t.Fatal("rejected bounds removed turtles")
	}
	resized := strings.Replace(current, "maxPxcor: 12", "maxPxcor: 8", 1)
	if _, err := s.SetWidgetYAML(resized); err != nil {
		t.Fatal(err)
	}
	frame = s.Frame()
	if frame.World.MaxPxcor != 8 || len(frame.World.Turtles) != 0 {
		t.Fatalf("resized max %d turtles %d", frame.World.MaxPxcor, len(frame.World.Turtles))
	}
}

func TestSetWidgetYAMLAfterModelSwitchKeepsTheNewWidgets(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.LoadModel("fire.nlogo"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadModelFile("library/3D/Sample Models/Flocking 3D Alternate.nlogox3d"); err != nil {
		t.Fatal(err)
	}
	spec, err := s.WidgetYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spec, "build-cubes") || strings.Contains(spec, "burned trees") {
		t.Fatalf("yaml is not the flocking model:\n%s", spec)
	}
	edited := strings.Replace(spec, "x: 0\n", "x: 40\n", 1)
	got, err := s.SetWidgetYAML(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "build-cubes") || strings.Contains(got, "burned trees") || strings.Contains(got, "density") {
		t.Fatalf("apply kept the previous model's widgets:\n%s", got)
	}
	frame := s.Frame()
	if !frame.World.ThreeD || frame.World.ViewX != 40 {
		t.Fatalf("view %+v threeD=%v", frame.World.ViewX, frame.World.ThreeD)
	}
	buttons := map[string]bool{}
	for _, w := range frame.Widgets {
		if w.Kind == "button" {
			buttons[w.Display] = true
		}
	}
	if !buttons["build-cubes"] || !buttons["build-wall"] || buttons["setup"] && len(frame.Widgets) < 5 {
		t.Fatalf("buttons %+v count %d", buttons, len(frame.Widgets))
	}
	if buttons["go"] && !buttons["follow turtle 0"] {
		t.Fatalf("widgets look like the fire model: %+v", buttons)
	}
}

func placedWidgetModel() string {
	view := strings.Join([]string{
		"GRAPHICS-WINDOW",
		"280", "10", "592", "322",
		"-1", "-1", "12.0", "1", "10", "1", "1", "1", "0", "0", "0", "1",
		"-12", "12", "-12", "12",
		"0", "1", "1", "ticks", "30.0",
	}, "\n")
	button := strings.Join([]string{
		"BUTTON", "16", "20", "110", "53", "setup", "setup", "NIL", "1", "T", "OBSERVER", "NIL", "NIL", "NIL", "NIL", "1",
	}, "\n")
	return strings.Join([]string{
		"to setup\n  clear-all\nend\n",
		"\n" + view + "\n\n" + button + "\n",
		"\n",
		"\n",
		"\nNetLogo 6.4.0\n",
		"\n",
	}, "@#$#@#$#@")
}
