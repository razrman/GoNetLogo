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
	all, err := FormatWidgetsYAML(m.View, m.Widgets)
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
	doc, err := FormatWidgetsYAML(m.View, m.Widgets)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(doc, "\n- ")
	if len(parts) != 3 || !strings.Contains(parts[0], "kind: view") {
		t.Fatalf("entries %d\n%s", len(parts), doc)
	}
	// parts[0] is "- kind: view...", then the button and the slider.
	viewEntry := parts[0]
	button := parts[1]
	slider := parts[2]
	reordered := viewEntry + "\n- " + slider + "- " + button
	_, next, from, err := ApplyWidgetsYAML(m.View, m.Widgets, reordered)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 2 || next[0].Kind != "slider" || next[1].Kind != "button" || from[0] != 1 || from[1] != 0 {
		t.Fatalf("reorder %+v from %v", next, from)
	}
	added := doc + "- kind: switch\n  display: flag\n  variable: flag\n  on: true\n  x: 0\n  y: 0\n  width: 80\n  height: 30\n  description: \"\"\n"
	_, withSwitch, _, err := ApplyWidgetsYAML(m.View, m.Widgets, added)
	if err != nil || len(withSwitch) != 3 || withSwitch[2].Kind != "switch" || withSwitch[2].Variable != "flag" {
		t.Fatalf("add %+v %v", withSwitch, err)
	}
	onlyButton := viewEntry + "\n- " + button
	_, deleted, _, err := ApplyWidgetsYAML(m.View, m.Widgets, onlyButton)
	if err != nil || len(deleted) != 1 || deleted[0].Kind != "button" {
		t.Fatalf("delete %+v %v", deleted, err)
	}
	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, "- "+button); err == nil || !strings.Contains(err.Error(), "view cannot be removed") {
		t.Fatalf("removed view %v", err)
	}
	changed := strings.Replace(doc, "kind: button", "kind: note", 1)
	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, changed); err == nil || !strings.Contains(err.Error(), "kind cannot be changed") {
		t.Fatalf("kind change %v", err)
	}
	bad := strings.Replace(doc, "max: 100", "max: wide", 1)
	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, bad); err == nil || !strings.Contains(err.Error(), "widget") || !strings.Contains(err.Error(), "max") {
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

func TestViewYAMLRoundTrip(t *testing.T) {
	m, err := ParseModel("t.nlogo", classicWidgetModel("setup", "100"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := FormatWidgetsYAML(m.View, m.Widgets)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "kind: view") || !strings.Contains(doc, "patchSize: 12") || !strings.Contains(doc, "updateMode: continuous") || !strings.Contains(doc, "ticksLabel: ticks") {
		t.Fatalf("view yaml:\n%s", doc)
	}
	if strings.Contains(doc, "minPzcor:") {
		t.Fatalf("2D view gained z:\n%s", doc)
	}
	edited := strings.Replace(doc, "x: 0\n", "x: 40\n", 1)
	edited = strings.Replace(edited, "minPxcor: -2", "minPxcor: -4", 1)
	edited = strings.Replace(edited, "patchSize: 12", "patchSize: 9", 1)
	edited = strings.Replace(edited, `description: ""`, `description: The world`, 1)
	view, widgets, _, err := ApplyWidgetsYAML(m.View, m.Widgets, edited)
	if err != nil {
		t.Fatal(err)
	}
	if view.X != 40 || view.MinPxcor != -4 || view.PatchSize != 9 || view.Description != "The world" || len(widgets) != 2 {
		t.Fatalf("applied view %+v widgets %d", view, len(widgets))
	}
	src, err := WriteWidgets(classicWidgetModel("setup", "100"), view, widgets, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseModel("t.nlogo", src)
	if err != nil {
		t.Fatal(err)
	}
	if again.View.X != 40 || again.View.MinPxcor != -4 || again.View.PatchSize != 9 || again.View.Description != "The world" || again.View.TickLabel != "ticks" {
		t.Fatalf("classic round trip %+v", again.View)
	}
	if !strings.Contains(again.Code, "to go") {
		t.Fatal("code changed")
	}

	xmlSrc := xmlWidgetModel("setup", "100")
	xm, err := ParseModel("t.nlogox", xmlSrc)
	if err != nil {
		t.Fatal(err)
	}
	xdoc, err := FormatWidgetsYAML(xm.View, xm.Widgets)
	if err != nil {
		t.Fatal(err)
	}
	xedited := strings.Replace(xdoc, "width: 0", "width: 200", 1)
	xedited = strings.Replace(xedited, "maxPxcor: 2", "maxPxcor: 3", 1)
	xview, xwidgets, _, err := ApplyWidgetsYAML(xm.View, xm.Widgets, xedited)
	if err != nil {
		t.Fatal(err)
	}
	xsrc, err := WriteWidgets(xmlSrc, xview, xwidgets, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	xagain, err := ParseModel("t.nlogox", xsrc)
	if err != nil {
		t.Fatal(err)
	}
	if xagain.View.W != 200 || xagain.View.MaxPxcor != 3 || xagain.View.PatchSize != 12 {
		t.Fatalf("xml round trip %+v\n%s", xagain.View, xsrc)
	}

	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, strings.Replace(doc, "patchSize: 12", "patchSize: 0", 1)); err == nil || !strings.Contains(err.Error(), "widget 0 (view)") || !strings.Contains(err.Error(), "patchSize") {
		t.Fatalf("patch size %v", err)
	}
	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, strings.Replace(doc, "minPxcor: -2", "minPxcor: 1", 1)); err == nil || !strings.Contains(err.Error(), "widget 0 (view)") || !strings.Contains(err.Error(), "minPxcor") || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("origin %v", err)
	}
	dup := strings.Replace(doc, "kind: view", "kind: view\n- kind: view", 1)
	if _, _, _, err := ApplyWidgetsYAML(m.View, m.Widgets, dup); err == nil || !strings.Contains(err.Error(), "duplicates the view") {
		t.Fatalf("duplicate %v", err)
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
