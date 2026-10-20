package session

import "testing"

func TestFrameLabelsAndDrawing(t *testing.T) {
	s := newSession()
	if err := s.useModel("labels.nlogo", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand(`
create-turtles 2 [
  ifelse who = 0 [
    set shape "person"
    set color red
    set size 2
    setxy 1 2
    set heading 90
    set label "8.00"
    set label-color red
    stamp
    fd 1
  ] [
    set label ""
  ]
]
ask patch 0 0 [ set plabel "nest" set plabel-color blue ]
ask patch -1 1 [ set plabel "" ]
`); err != nil {
		t.Fatal(err)
	}

	frame := s.Frame()
	if len(frame.World.Drawing) != 1 {
		t.Fatalf("drawing marks = %d, want the one stamp", len(frame.World.Drawing))
	}
	mark := frame.World.Drawing[0]
	if mark.Pen || mark.Erase || mark.Shape != "person" || mark.X != 1 || mark.Y != 2 || mark.Heading != 90 || mark.Size != 2 {
		t.Fatalf("stamp mark %+v", mark)
	}
	if mark.R < 180 || mark.G > 90 {
		t.Fatalf("stamp rgb %d %d %d, want red", mark.R, mark.G, mark.B)
	}

	var turtleLabel, patchLabel int
	for _, lb := range frame.World.Labels {
		switch {
		case lb.Who != nil:
			turtleLabel++
			if *lb.Who != 0 || lb.Text != "8" || lb.Px != nil || lb.Py != nil {
				t.Fatalf("turtle label %+v text %q", lb, lb.Text)
			}
			if lb.R < 180 || lb.G > 90 {
				t.Fatalf("label-color rgb %d %d %d", lb.R, lb.G, lb.B)
			}
		case lb.Px != nil && lb.Py != nil:
			patchLabel++
			if *lb.Px != 0 || *lb.Py != 0 || lb.Text != "nest" || lb.Who != nil {
				t.Fatalf("patch label %+v", lb)
			}
			if lb.B < 140 || lb.B < lb.R {
				t.Fatalf("plabel-color rgb %d %d %d", lb.R, lb.G, lb.B)
			}
		default:
			t.Fatalf("label has no agent %+v", lb)
		}
	}
	if turtleLabel != 1 || patchLabel != 1 {
		t.Fatalf("encoded turtle labels %d, patch labels %d", turtleLabel, patchLabel)
	}

	if _, err := s.RunCommand("clear-drawing"); err != nil {
		t.Fatal(err)
	}
	cleared := s.Frame()
	if len(cleared.World.Drawing) != 0 {
		t.Fatalf("clear-drawing left %d marks", len(cleared.World.Drawing))
	}
	if len(cleared.World.Labels) != 2 {
		t.Fatalf("clear-drawing dropped labels: %+v", cleared.World.Labels)
	}
}
