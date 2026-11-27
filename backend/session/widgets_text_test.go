package session

import (
	"strings"
	"testing"

	"gonetlogo/pkg/nl"
)

func TestSetWidgetsKeepsWorldAndOtherSections(t *testing.T) {
	s := newSession()
	defer s.Close()
	src := classicWidgetModel("go", "100")
	if err := s.LoadModelSource("view.nlogo", src); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("crt 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("set density 7"); err != nil {
		t.Fatal(err)
	}
	section := nl.ExtractWidgetSection(src)
	edited := strings.Replace(section.Text, "\ngo\n", "\nRun it\n", 1)
	edited = strings.Replace(edited, "\n100\n", "\n250\n", 1)
	edited += "\n" + plotBlock("herd") + "\n"
	if err := s.SetWidgets(edited); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Frame().World.Turtles); n != 1 {
		t.Fatalf("turtles %d", n)
	}
	got, err := s.RunCommand("density")
	if err != nil || got != "7" {
		t.Fatalf("density %q %v", got, err)
	}
	if !widgetShown(s, "button", "Run it") {
		t.Fatal("button label did not update")
	}
	if sliderMaxOf(s, "density") != 250 {
		t.Fatalf("slider max %v", sliderMaxOf(s, "density"))
	}
	views := s.vm.PlotViews()
	if len(views) != 1 || views[0].Name != "herd" {
		t.Fatalf("plots %+v", views)
	}
	stored := s.ModelSource()
	if !strings.Contains(stored, "to go") || !strings.Contains(stored, "KEEP-INFO") || !strings.Contains(stored, "KEEP-SHAPES") {
		t.Fatalf("stored model lost a section:\n%s", stored)
	}
	if !strings.Contains(stored, "\nRun it\n") || !strings.Contains(stored, "\n250\n") {
		t.Fatal("stored model did not keep the edited interface")
	}

	s.mu.Lock()
	s.running = 0
	s.mu.Unlock()
	if err := s.SetWidgets(edited); err == nil || !strings.Contains(err.Error(), "Stop the model before changing the widgets.") {
		t.Fatalf("running apply %v", err)
	}
	s.mu.Lock()
	s.running = -1
	s.mu.Unlock()

	bad := strings.Replace(edited, "\n-2\n2\n-2\n2\n", "\n5\n1\n-2\n2\n", 1)
	if err := s.SetWidgets(bad); err == nil || !strings.Contains(err.Error(), "line ") {
		t.Fatalf("bad widgets %v", err)
	}
	if !widgetShown(s, "button", "Run it") {
		t.Fatal("parse error replaced the widgets")
	}
	if n := len(s.Frame().World.Turtles); n != 1 {
		t.Fatalf("parse error dropped turtles: %d", n)
	}
}

func TestSetWidgetsNLogoX(t *testing.T) {
	s := newSession()
	defer s.Close()
	src := xmlWidgetModel("go", "100")
	if err := s.LoadModelSource("view.nlogox", src); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("crt 1"); err != nil {
		t.Fatal(err)
	}
	section := nl.ExtractWidgetSection(src)
	edited := strings.Replace(section.Text, `display="go"`, `display="Run it"`, 1)
	edited = strings.Replace(edited, `max="100"`, `max="250"`, 1)
	if err := s.SetWidgets(edited); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Frame().World.Turtles); n != 1 {
		t.Fatalf("turtles %d", n)
	}
	if !widgetShown(s, "button", "Run it") || sliderMaxOf(s, "density") != 250 {
		t.Fatal("nlogox widgets did not update")
	}
	stored := s.ModelSource()
	if !strings.Contains(stored, "KEEP-INFO") || !strings.Contains(stored, "to go") || !strings.Contains(stored, `display="Run it"`) {
		t.Fatalf("stored xml:\n%s", stored)
	}
	if err := s.SetWidgets("<widgets><view"); err == nil || !strings.Contains(err.Error(), "line ") {
		t.Fatalf("bad xml %v", err)
	}
	if !widgetShown(s, "button", "Run it") {
		t.Fatal("xml parse error replaced the widgets")
	}
}

func TestSetWidgetYAMLKeepsTheDraftOnABadDocument(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.LoadModelSource("view.nlogo", classicWidgetModel("go", "100")); err != nil {
		t.Fatal(err)
	}
	spec, err := s.WidgetYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spec, "kind: slider") || !strings.Contains(spec, "kind: button") || !strings.Contains(spec, "y: ") {
		t.Fatalf("spec:\n%s", spec)
	}
	if strings.Contains(spec, `"y"`) {
		t.Fatalf("y key was quoted:\n%s", spec)
	}
	bad := strings.Replace(spec, "kind: slider", "kind: slider\n  color: red", 1)
	if _, err := s.SetWidgetYAML(bad); err == nil || !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "density") {
		t.Fatalf("bad yaml %v", err)
	}
	if sliderMaxOf(s, "density") != 100 {
		t.Fatal("bad yaml changed the slider")
	}
	edited := strings.Replace(spec, "max: 100", "max: 80", 1)
	edited = strings.Replace(edited, "description: \"\"", "description: Forest density", 1)
	got, err := s.SetWidgetYAML(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "description: Forest density") || !strings.Contains(got, "max: 80") {
		t.Fatalf("canonical:\n%s", got)
	}
	if sliderMaxOf(s, "density") != 80 {
		t.Fatal("max was not saved")
	}
}

func widgetShown(s *Session, kind, display string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.model.Widgets {
		if w.Kind == kind && w.Display == display {
			return true
		}
	}
	return false
}

func sliderMaxOf(s *Session, variable string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.model.Widgets {
		if w.Kind == "slider" && w.Variable == variable {
			return w.Max
		}
	}
	return -1
}

func classicWidgetModel(label, max string) string {
	return strings.Join([]string{
		"to go\n  tick\nend\n",
		"\n" + graphicsWindow() + "\n\n" + buttonBlock(label) + "\n\n" + sliderBlock(max) + "\n",
		"\nKEEP-INFO\n",
		"\nKEEP-SHAPES\n",
		"\nNetLogo 6.4.0\n",
		"\n",
	}, "@#$#@#$#@")
}

func graphicsWindow() string {
	return strings.Join([]string{
		"GRAPHICS-WINDOW",
		"0", "0", "0", "0", "-1", "-1", "12.0", "1", "10", "1", "1", "1", "0", "0", "0", "1",
		"-2", "2", "-2", "2",
		"0", "1", "1", "ticks", "30.0",
	}, "\n")
}

func buttonBlock(label string) string {
	return strings.Join([]string{
		"BUTTON", "16", "20", "110", "53", label, "go", "NIL", "1", "T", "OBSERVER", "NIL", "NIL", "NIL", "NIL", "1",
	}, "\n")
}

func sliderBlock(max string) string {
	return strings.Join([]string{
		"SLIDER", "16", "70", "200", "110", "density", "density", "0", max, "40", "1", "1", "NIL", "HORIZONTAL",
	}, "\n")
}

func xmlWidgetModel(label, max string) string {
	return `<?xml version="1.0"?>
<model version="NetLogo 6.4.0">
  <code><![CDATA[to go
  tick
end
]]></code>
  <widgets>
    <view minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" patchSize="12" wrappingAllowedX="false" wrappingAllowedY="false" updateMode="0"/>
    <button display="` + label + `" forever="false" kind="Observer">go</button>
    <slider variable="density" min="0" max="` + max + `" default="40" step="1"/>
  </widgets>
  <info>KEEP-INFO</info>
</model>`
}

func plotBlock(name string) string {
	return strings.Join([]string{
		"PLOT", "16", "130", "250", "300", name, "ticks", "turtles",
		"0.0", "20.0", "0.0", "40.0", "true", "true", `"" ""`,
		"PENS",
		`"turtles" 1.0 0 -16711936 true "" "plot count turtles"`,
	}, "\n")
}
