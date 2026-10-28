package session

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWorldPublishesShapesAndFont(t *testing.T) {
	raw, err := os.ReadFile("../../models/Robby the Robot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("Robby the Robot.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := s.FrameSince(-1, 0, nil)
	if frame.World.LabelFont != 10 {
		t.Fatalf("label font %d, want 10", frame.World.LabelFont)
	}
	var square, dot, person bool
	for _, sh := range frame.World.Shapes {
		switch sh.Name {
		case "square":
			square = !sh.Rotatable && len(sh.Elements) == 1 && sh.Elements[0].Kind == "rectangle" && sh.Elements[0].Marked
		case "dot":
			dot = !sh.Rotatable && len(sh.Elements) == 1 && sh.Elements[0].Kind == "circle" && sh.Elements[0].W == 120
		case "person":
			person = !sh.Rotatable && len(sh.Elements) > 1
		}
	}
	if !square || !dot || !person {
		t.Fatalf("published shapes square=%v dot=%v person=%v (%d total)", square, dot, person, len(frame.World.Shapes))
	}
	body, err := json.Marshal(frame.World.Shapes)
	if err != nil {
		t.Fatal(err)
	}
	// The library rides on every frame. It has to stay a small constant.
	if len(body) > 80000 {
		t.Fatalf("shape payload %d bytes", len(body))
	}
}
