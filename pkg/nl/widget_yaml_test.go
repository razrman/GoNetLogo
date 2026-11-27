package nl

import (
	"strings"
	"testing"
)

func TestWidgetDescriptionRoundTrip(t *testing.T) {
	m, err := ParseModel("t.nlogo", classicWidgetModel("go", "100"))
	if err != nil {
		t.Fatal(err)
	}
	slider := findWidget(t, m, "slider")
	if slider.Description != "" {
		t.Fatalf("missing description should be empty, got %q", slider.Description)
	}
	doc, err := FormatWidgetYAML(slider)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "kind: slider") || !strings.Contains(doc, "variable: density") || !strings.Contains(doc, "description:") {
		t.Fatalf("slider yaml:\n%s", doc)
	}
	if strings.Contains(doc, `"y"`) {
		t.Fatalf("y key was quoted:\n%s", doc)
	}
	all, err := FormatWidgetsYAML(m.Widgets)
	if err != nil || !strings.HasPrefix(all, "- kind:") || !strings.Contains(all, "kind: button") {
		t.Fatalf("list:\n%s %v", all, err)
	}
	edited := strings.Replace(doc, "max: 100", "max: 250", 1)
	edited = strings.Replace(edited, `description: ""`, `description: How dense the forest is`, 1)
	next, err := ApplyWidgetYAML(slider, edited)
	if err != nil {
		t.Fatal(err)
	}
	if next.Max != 250 || next.Description != "How dense the forest is" {
		t.Fatalf("applied %+v", next)
	}
	src, err := WriteWidget(classicWidgetModel("go", "100"), widgetIndex(m, "slider"), next)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseModel("t.nlogo", src)
	if err != nil {
		t.Fatal(err)
	}
	got := findWidget(t, again, "slider")
	if got.Max != 250 || got.Description != "How dense the forest is" {
		t.Fatalf("round trip %+v", got)
	}
	if !strings.Contains(again.Code, "to go") || !strings.Contains(again.Info, "KEEP-INFO") {
		t.Fatal("other sections changed")
	}

	xmlSrc := xmlWidgetModel("setup", "100")
	xm, err := ParseModel("t.nlogox", xmlSrc)
	if err != nil {
		t.Fatal(err)
	}
	button := findWidget(t, xm, "button")
	bdoc, err := FormatWidgetYAML(button)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bdoc, "kind: button") || !strings.Contains(bdoc, "source: go") {
		t.Fatalf("button yaml:\n%s", bdoc)
	}
	bedit := strings.Replace(bdoc, `description: ""`, `description: Reset the run`, 1)
	bedit = strings.Replace(bedit, "forever: false", "forever: true", 1)
	bnext, err := ApplyWidgetYAML(button, bedit)
	if err != nil {
		t.Fatal(err)
	}
	xsrc, err := WriteWidget(xmlSrc, widgetIndex(xm, "button"), bnext)
	if err != nil {
		t.Fatal(err)
	}
	xagain, err := ParseModel("t.nlogox", xsrc)
	if err != nil {
		t.Fatal(err)
	}
	xbutton := findWidget(t, xagain, "button")
	if xbutton.Description != "Reset the run" || !xbutton.Forever {
		t.Fatalf("xml button %+v\n%s", xbutton, xsrc)
	}
}

func TestWidgetsYAMLListEditsReorderAndKind(t *testing.T) {
	m, err := ParseModel("t.nlogo", classicWidgetModel("setup", "100"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := FormatWidgetsYAML(m.Widgets)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(doc, "\n- ")
	if len(parts) != 2 {
		t.Fatalf("entries %d\n%s", len(parts), doc)
	}
	// parts[0] is "- kind: button...", parts[1] is "kind: slider..." without the dash.
	button := strings.TrimPrefix(parts[0], "- ")
	slider := parts[1]
	reordered := "- " + slider + "- " + button
	next, from, err := ApplyWidgetsYAML(m.Widgets, reordered)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 2 || next[0].Kind != "slider" || next[1].Kind != "button" || from[0] != 1 || from[1] != 0 {
		t.Fatalf("reorder %+v from %v", next, from)
	}
	added := doc + "- kind: switch\n  display: flag\n  variable: flag\n  on: true\n  x: 0\n  y: 0\n  width: 80\n  height: 30\n  description: \"\"\n"
	withSwitch, _, err := ApplyWidgetsYAML(m.Widgets, added)
	if err != nil || len(withSwitch) != 3 || withSwitch[2].Kind != "switch" || withSwitch[2].Variable != "flag" {
		t.Fatalf("add %+v %v", withSwitch, err)
	}
	onlyButton := "- " + button
	deleted, _, err := ApplyWidgetsYAML(m.Widgets, onlyButton)
	if err != nil || len(deleted) != 1 || deleted[0].Kind != "button" {
		t.Fatalf("delete %+v %v", deleted, err)
	}
	changed := strings.Replace(doc, "kind: button", "kind: note", 1)
	if _, _, err := ApplyWidgetsYAML(m.Widgets, changed); err == nil || !strings.Contains(err.Error(), "kind cannot be changed") {
		t.Fatalf("kind change %v", err)
	}
	bad := strings.Replace(doc, "max: 100", "max: wide", 1)
	if _, _, err := ApplyWidgetsYAML(m.Widgets, bad); err == nil || !strings.Contains(err.Error(), "widget") || !strings.Contains(err.Error(), "max") {
		t.Fatalf("type %v", err)
	}
}

func TestWidgetYAMLRejectsBadDocuments(t *testing.T) {
	m, err := ParseModel("t.nlogo", classicWidgetModel("go", "100"))
	if err != nil {
		t.Fatal(err)
	}
	slider := findWidget(t, m, "slider")
	doc, err := FormatWidgetYAML(slider)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		text string
		want string
	}{
		{"syntax", "kind: [\n", "YAML"},
		{"unknown", doc + "color: red\n", "unknown key"},
		{"type", strings.Replace(doc, "max: 100", "max: wide", 1), "wrong type"},
		{"kind", strings.Replace(doc, "kind: slider", "kind: button", 1), "cannot be changed"},
		{"empty", "   ", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyWidgetYAML(slider, tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v", err)
			}
			if got.Max != slider.Max || got.Description != slider.Description {
				t.Fatal("failed apply changed the widget")
			}
		})
	}
}

func findWidget(t *testing.T, m *FileModel, kind string) Widget {
	t.Helper()
	for _, w := range m.Widgets {
		if w.Kind == kind {
			return w
		}
	}
	t.Fatalf("no %s in %+v", kind, m.Widgets)
	return Widget{}
}

func widgetIndex(m *FileModel, kind string) int {
	for i, w := range m.Widgets {
		if w.Kind == kind {
			return i
		}
	}
	return -1
}
