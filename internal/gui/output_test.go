package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"gonetlogo/models"
)

func TestDesktopOutputPanel(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	w.SetContent(s.layout)
	w.Resize(fyne.NewSize(1200, 800))

	b, err := models.FS.ReadFile("output.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.load("output.nlogo", string(b))
	if s.model == nil || !s.model.HasOutput() || len(s.areas) != 1 {
		t.Fatalf("areas %d model %+v", len(s.areas), s.model)
	}
	area := s.areas[0]
	if area.fontSize != 13 {
		t.Fatalf("font size %v", area.fontSize)
	}
	s.pressNamed("setup")
	if !strings.Contains(area.body, "Output area ready.") || !strings.Contains(area.body, `"widget"`) {
		t.Fatalf("output panel %q", area.body)
	}
	if strings.Contains(s.output.Text, "Output area ready.") {
		t.Fatalf("command center %q", s.output.Text)
	}
	s.mu.Lock()
	stepped := false
	for i := range s.ctls {
		c := &s.ctls[i]
		if c.widget.Kind == "button" && strings.EqualFold(strings.TrimSpace(c.widget.Source), "go") {
			stepped = s.stepLocked(c)
			break
		}
	}
	s.refreshOutputLocked()
	s.mu.Unlock()
	if !stepped {
		t.Fatal("go did not run")
	}
	if !strings.Contains(area.body, "Output area ready.") || !strings.HasSuffix(strings.TrimRight(area.body, "\n"), "0") {
		t.Fatalf("after go %q", area.body)
	}
}
