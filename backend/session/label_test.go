package session

import (
	"testing"

	"gonetlogo/pkg/nl"
)

func TestFrameFormatsNumericLabels(t *testing.T) {
	w := nl.NewWorld(0, 1, 0, 1, false, false)
	tu := w.CreateTurtle("", 0, 0, 5, 0)
	tu.Label = nl.Num(0)
	if tu.Label.K != nl.KindNum {
		t.Fatalf("stored label kind %s", tu.Label.TypeName())
	}
	w.GetPatch(1, 0).Label = nl.Num(7)
	a := w.CreateTurtle("", 1, 1, 5, 0)
	b := w.CreateTurtle("", 0, 1, 5, 0)
	link, _, err := w.CreateLink(a, b, false, "")
	if err != nil {
		t.Fatal(err)
	}
	link.Label = nl.Num(3)

	frame := worldLocked(w)
	if len(frame.Turtles) < 1 || frame.Turtles[0].Label != "0" {
		t.Fatalf("turtle payload labels: %+v", frame.Turtles)
	}
	if tu.Label.K != nl.KindNum || tu.Label.N != 0 {
		t.Fatalf("payload formatting changed the stored label %#v", tu.Label)
	}
	foundPatch := false
	for _, p := range frame.PatchLabels {
		if p.X == 1 && p.Y == 0 && p.Label == "7" {
			foundPatch = true
		}
		if p.Label == "" {
			t.Fatalf("empty patch label included: %+v", p)
		}
	}
	if !foundPatch {
		t.Fatalf("patch labels %+v", frame.PatchLabels)
	}
	if len(frame.Links) != 1 || frame.Links[0].Label != "3" {
		t.Fatalf("link payload %+v", frame.Links)
	}
}
