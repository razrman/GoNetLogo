package nl

import (
	"strings"
	"testing"
)

func TestWidgetSectionRoundTrip(t *testing.T) {
	classic := classicWidgetModel("go", "100")
	got := ExtractWidgetSection(classic)
	if got.XML || !strings.Contains(got.Text, "GRAPHICS-WINDOW") || !strings.Contains(got.Text, "BUTTON") {
		t.Fatalf("classic section xml=%v text=%q", got.XML, got.Text)
	}
	edited := strings.Replace(got.Text, "\ngo\n", "\nRun it\n", 1)
	edited = strings.Replace(edited, "\n100\n", "\n250\n", 1)
	next, err := ReplaceWidgetSection(classic, edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "to go") || !strings.Contains(next, "KEEP-INFO") || !strings.Contains(next, "KEEP-SHAPES") {
		t.Fatalf("other sections changed:\n%s", next)
	}
	m, err := ParseModel("t.nlogo", next)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Code, "to go") || !strings.Contains(m.Info, "KEEP-INFO") {
		t.Fatalf("parsed code %q info %q", m.Code, m.Info)
	}
	if !widgetDisplay(m, "button", "Run it") {
		t.Fatalf("button %+v", m.Widgets)
	}
	if sliderMax(m, "density") != 250 {
		t.Fatalf("slider max %v", sliderMax(m, "density"))
	}

	xml := xmlWidgetModel("go", "100")
	xgot := ExtractWidgetSection(xml)
	if !xgot.XML || !strings.Contains(xgot.Text, "<widgets>") {
		t.Fatalf("xml section %+v", xgot)
	}
	xedited := strings.Replace(xgot.Text, `display="go"`, `display="Run it"`, 1)
	xedited = strings.Replace(xedited, `max="100"`, `max="250"`, 1)
	xnext, err := ReplaceWidgetSection(xml, xedited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xnext, "KEEP-INFO") || !strings.Contains(xnext, "to go") {
		t.Fatalf("xml other sections changed:\n%s", xnext)
	}
	xm, err := ParseModel("t.nlogox", xnext)
	if err != nil {
		t.Fatal(err)
	}
	if !widgetDisplay(xm, "button", "Run it") || sliderMax(xm, "density") != 250 {
		t.Fatalf("xml widgets %+v", xm.Widgets)
	}
}

func TestWidgetSectionParseErrorKeepsALine(t *testing.T) {
	section := ExtractWidgetSection(classicWidgetModel("go", "100")).Text
	bad := strings.Replace(section, "\n-2\n2\n-2\n2\n", "\n5\n1\n-2\n2\n", 1)
	_, _, _, err := ParseWidgetSection(bad, false, false)
	if err == nil || !strings.Contains(err.Error(), "line ") || !strings.Contains(err.Error(), "empty world") {
		t.Fatalf("classic err %v", err)
	}
	_, _, _, err = ParseWidgetSection("<widgets><view minPxcor=\"0\"", true, false)
	if err == nil || !strings.Contains(err.Error(), "line ") {
		t.Fatalf("xml err %v", err)
	}
}

func classicWidgetModel(label, max string) string {
	return strings.Join([]string{
		"to go\n  tick\nend\n",
		"\n" + graphicsWindow() + "\n\n" + buttonBlock(label) + "\n\n" + sliderBlock(max) + "\n",
		"\nKEEP-INFO\n",
		"\nKEEP-SHAPES\n",
		"\nNetLogo 6.4.0\n",
		"\n",
	}, sectionSep)
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

func widgetDisplay(m *FileModel, kind, display string) bool {
	for _, w := range m.Widgets {
		if w.Kind == kind && w.Display == display {
			return true
		}
	}
	return false
}

func sliderMax(m *FileModel, variable string) float64 {
	for _, w := range m.Widgets {
		if w.Kind == "slider" && w.Variable == variable {
			return w.Max
		}
	}
	return -1
}
