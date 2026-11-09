package session

import (
	"testing"

	"gonetlogo/pkg/nl"
)

func TestDrawingFrameOmitsPenUpMotion(t *testing.T) {
	w := nl.NewWorld(-5, 5, -5, 5, true, true)
	turt := w.CreateTurtle("", 0, 0, 0, 0)
	turt.Color = 15
	if err := w.MoveTo(turt, 0, 3); err != nil {
		t.Fatal(err)
	}
	f := worldLocked(w)
	if len(f.Drawing) != 0 {
		t.Fatalf("pen-up motion produced drawing marks: %+v", f.Drawing)
	}
	turt.PenDown = true
	if err := w.MoveTo(turt, 0, 5); err != nil {
		t.Fatal(err)
	}
	f = worldLocked(w)
	if len(f.Drawing) == 0 {
		t.Fatal("pen-down motion produced no drawing marks")
	}
	for _, m := range f.Drawing {
		if !m.Pen {
			t.Fatalf("pen stroke has pen=false: %+v", m)
		}
		if m.Erase {
			t.Fatalf("pen stroke has erase=true: %+v", m)
		}
	}
}
