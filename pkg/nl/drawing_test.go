package nl

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func TestDisplayLabel(t *testing.T) {
	cases := []struct {
		in   Value
		want string
	}{
		{Str(""), ""},
		{Str("hello"), "hello"},
		{Str("3.00"), "3.00"},
		{Str("007"), "007"},
		{Str("1e2"), "1e2"},
		{Num(3), "3"},
		{Num(3.0), "3"},
		{Num(1.5), "1.5"},
		{Num(-2), "-2"},
		{Str("nest"), "nest"},
	}
	for _, c := range cases {
		if got := DisplayLabel(c.in); got != c.want {
			t.Errorf("DisplayLabel(%s) = %q, want %q", c.in.Format(true), got, c.want)
		}
	}
}

func TestStampLayer(t *testing.T) {
	vm := newTestVM(t, "", -4, 4, true, 1)
	mustEval(t, vm, `
create-turtles 1 [
  set shape "person"
  set color red
  set size 2
  setxy 1 2
  set heading 90
  stamp
  fd 1
]
`)
	marks := vm.World.Marks()
	if len(marks) != 1 {
		t.Fatalf("marks = %d, want 1", len(marks))
	}
	m := marks[0]
	if m.Pen || m.Erase {
		t.Fatalf("stamp recorded as pen=%v erase=%v", m.Pen, m.Erase)
	}
	if m.Shape != "person" || m.X != 1 || m.Y != 2 || m.Heading != 90 || m.Size != 2 {
		t.Fatalf("stamp = %+v", m)
	}
	if m.Color != 15 {
		t.Fatalf("stamp color %v, want red (15)", m.Color)
	}
	// The turtle moved. The stamp stays, and it did not recolor the patch.
	p := vm.World.GetPatch(1, 2)
	if p.PColor != 0 {
		t.Fatalf("stamp changed pcolor to %v", p.PColor)
	}
	ink, ok := vm.World.DrawingAt(1, 2)
	if !ok || ink.R < 180 || ink.G > 80 {
		t.Fatalf("drawing at stamp = %+v ok=%v, want red ink", ink, ok)
	}
	if _, ok := vm.World.DrawingAt(-4, -4); ok {
		t.Fatal("bare patch has drawing ink")
	}

	mustEval(t, vm, "clear-patches")
	if len(vm.World.Marks()) != 1 {
		t.Fatal("clear-patches cleared the drawing")
	}
	mustEval(t, vm, "clear-drawing")
	if len(vm.World.Marks()) != 0 {
		t.Fatal("clear-drawing left marks")
	}
	if _, ok := vm.World.DrawingAt(1, 2); ok {
		t.Fatal("clear-drawing left ink")
	}
}

func TestStampEraseAndPenOrder(t *testing.T) {
	vm := newTestVM(t, "", -4, 4, true, 1)
	mustEval(t, vm, `
create-turtles 1 [
  set shape "circle"
  set color red
  set size 3
  setxy 0 0
  set heading 0
  stamp
  stamp-erase
  pen-down
  fd 2
]
`)
	marks := vm.World.Marks()
	// fd 2 is two unit steps, so the pen leaves two segments.
	if len(marks) != 4 {
		t.Fatalf("marks = %d, want stamp, erase, and two pen steps", len(marks))
	}
	if marks[0].Pen || marks[0].Erase || marks[0].Shape != "circle" {
		t.Fatalf("first mark %+v", marks[0])
	}
	if !marks[1].Erase || marks[1].Shape != "circle" {
		t.Fatalf("erase mark %+v", marks[1])
	}
	if !marks[2].Pen || marks[2].Y2 != 1 {
		t.Fatalf("first pen step %+v", marks[2])
	}
	if !marks[3].Pen || marks[3].Y2 != 2 {
		t.Fatalf("second pen step %+v", marks[3])
	}
	// The pen starts on the stamped patch, so that patch is inked again.
	// A neighbor the circle covered, and the pen did not, stays bare.
	if _, ok := vm.World.DrawingAt(1, 0); ok {
		t.Fatal("stamp-erase left ink beside the pen trail")
	}
	if _, ok := vm.World.DrawingAt(0, 2); !ok {
		t.Fatal("pen trail missing after the erase")
	}

	mustEval(t, vm, "clear-all")
	if len(vm.World.Marks()) != 0 {
		t.Fatal("clear-all left the drawing layer")
	}
}

func TestStampRequiresTurtle(t *testing.T) {
	vm := newTestVM(t, "", -2, 2, true, 1)
	_, err := vm.ExecuteLine("stamp")
	if err == nil || !strings.Contains(err.Error(), "turtles") {
		t.Fatalf("observer stamp: %v", err)
	}
}

func TestStampVisibleAndErased(t *testing.T) {
	w := NewWorld(-4, 4, -4, 4, true, true)
	for _, a := range w.Patches().Agents {
		a.(*Patch).PColor = 9.9
	}
	turt := w.CreateTurtle("", 0, 0, 0, 0)
	turt.Color = 15
	turt.Shape = "circle"
	turt.Size = 3
	turt.Heading = 0
	w.Stamp(turt, false)
	turt.Hidden = true
	stamped := RenderWorld(w, 10)
	if !mostlyRed(stamped, 45, 45) {
		t.Fatal("circle stamp is not visible at the origin")
	}

	w.Stamp(turt, true)
	w.dirty.Store(true)
	erased := RenderWorld(w, 10)
	if mostlyRed(erased, 45, 45) {
		t.Fatal("stamp-erase left the red stamp")
	}
	if !mostlyWhite(erased, 45, 45) {
		t.Fatal("erased stamp did not reveal the white patch")
	}
}

func TestShapesAndLabelsPaint(t *testing.T) {
	person := paintShape(t, "person")
	arrow := paintShape(t, "default")
	unknown := paintShape(t, "not-a-shape")
	circle := paintShape(t, "circle")
	if bytes.Equal(person.Pix, arrow.Pix) {
		t.Fatal("person painted the same as the default arrow")
	}
	if !bytes.Equal(unknown.Pix, arrow.Pix) {
		t.Fatal("unknown shape did not fall back to default")
	}
	if bytes.Equal(circle.Pix, arrow.Pix) {
		t.Fatal("circle painted the same as the default arrow")
	}
	for _, shape := range []string{"square", "dot", "box", "face happy", "arrow"} {
		img := paintShape(t, shape)
		if img.Bounds().Dx() < 8 {
			t.Fatalf("%s produced a tiny image", shape)
		}
	}

	w := NewWorld(-4, 4, -4, 4, false, false)
	for _, a := range w.Patches().Agents {
		a.(*Patch).PColor = 9.9
	}
	p := w.GetPatch(0, 0)
	p.Label = Str("Hi")
	p.Vars["plabel-color"] = Num(0)
	img := RenderWorld(w, 14)
	if !hasDark(img, 54, 48, 40, 24) {
		t.Fatal("patch label did not paint")
	}
	blank := NewWorld(-4, 4, -4, 4, false, false)
	for _, a := range blank.Patches().Agents {
		a.(*Patch).PColor = 9.9
	}
	if hasDark(RenderWorld(blank, 14), 54, 48, 40, 24) {
		t.Fatal("white world painted dark pixels without a label")
	}
}

func TestViewLabels(t *testing.T) {
	vm := newTestVM(t, "", -3, 3, true, 1)
	mustEval(t, vm, `
create-turtles 2 [
  ifelse who = 0 [
    set label 3.00
    set label-color red
    set shape "person"
  ] [
    set label ""
  ]
]
ask patch 1 -1 [ set plabel "nest" set plabel-color blue ]
ask turtle 0 [ hide-turtle ]
`)
	// Hidden turtle labels are omitted. Bring it back and check formatting.
	mustEval(t, vm, "ask turtle 0 [ show-turtle ]")
	var gotTurtle, gotPatch bool
	for _, lb := range vm.World.ViewLabels() {
		if lb.Turtle {
			gotTurtle = true
			if lb.Who != 0 || lb.Text != "3" {
				t.Fatalf("turtle label %+v", lb)
			}
			if lb.R < 180 || lb.G > 80 {
				t.Fatalf("label-color %+v, want red", lb)
			}
		} else {
			gotPatch = true
			if lb.PX != 1 || lb.PY != -1 || lb.Text != "nest" {
				t.Fatalf("patch label %+v", lb)
			}
			if lb.B < 140 || lb.B < lb.R {
				t.Fatalf("plabel-color %+v, want blue", lb)
			}
		}
	}
	if !gotTurtle || !gotPatch {
		t.Fatalf("labels turtle=%v patch=%v", gotTurtle, gotPatch)
	}
	mustEval(t, vm, "ask turtle 0 [ hide-turtle ]")
	for _, lb := range vm.World.ViewLabels() {
		if lb.Turtle {
			t.Fatal("hidden turtle label was included")
		}
	}
}

func paintShape(t *testing.T, shape string) *image.RGBA {
	t.Helper()
	w := NewWorld(-6, 6, -6, 6, true, true)
	turt := w.CreateTurtle("", 0, 0, 0, 0)
	turt.Color = 15
	turt.Shape = shape
	turt.Size = 4
	turt.Heading = 0
	w.Stamp(turt, false)
	turt.Hidden = true
	return RenderWorld(w, 8)
}

func mostlyRed(img *image.RGBA, x, y int) bool {
	c := img.RGBAAt(x, y)
	return c.R > 180 && c.G < 90 && c.B < 90
}

func mostlyWhite(img *image.RGBA, x, y int) bool {
	c := img.RGBAAt(x, y)
	return c.R > 240 && c.G > 240 && c.B > 240
}

func hasDark(img *image.RGBA, x, y, w, h int) bool {
	b := img.Bounds()
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if xx < b.Min.X || yy < b.Min.Y || xx >= b.Max.X || yy >= b.Max.Y {
				continue
			}
			c := img.RGBAAt(xx, yy)
			if c.R < 80 && c.G < 80 && c.B < 80 {
				return true
			}
		}
	}
	return false
}
