package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"gonetlogo/models"
)

func TestPanesAreDraggableSplits(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	if s.split == nil || !s.split.Horizontal {
		t.Fatal("world and widgets should sit in an HSplit")
	}
	if s.vsplit == nil || s.vsplit.Horizontal {
		t.Fatal("tabs and command center should sit in a VSplit")
	}
	if s.split.Offset <= 0 || s.split.Offset >= 1 {
		t.Fatalf("horizontal offset %v", s.split.Offset)
	}
	if s.vsplit.Offset <= 0 || s.vsplit.Offset >= 1 {
		t.Fatalf("vertical offset %v", s.vsplit.Offset)
	}
	if s.tabs == nil || len(s.tabs.Items) != 3 {
		t.Fatalf("tabs %#v", s.tabs)
	}
	want := []string{"Interface", "World", "Code"}
	for i, name := range want {
		if s.tabs.Items[i].Text != name {
			t.Fatalf("tab %d = %q", i, s.tabs.Items[i].Text)
		}
	}
	// Dragging the dividers has to be able to move them. Offsets are the
	// public Fyne control for that.
	s.split.SetOffset(0.55)
	s.vsplit.SetOffset(0.6)
	if s.split.Offset != 0.55 || s.vsplit.Offset != 0.6 {
		t.Fatalf("offsets after drag h=%v v=%v", s.split.Offset, s.vsplit.Offset)
	}
}
