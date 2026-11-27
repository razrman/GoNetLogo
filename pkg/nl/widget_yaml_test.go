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
