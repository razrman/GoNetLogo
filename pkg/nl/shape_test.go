package nl

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func TestParseRobbyShapes(t *testing.T) {
	m := loadModel(t, "Robby the Robot.nlogo")
	if m.View.FontSize != 10 {
		t.Fatalf("label font %d, want 10", m.View.FontSize)
	}
	if m.Shapes == nil || m.Shapes.Len() < 10 {
		t.Fatalf("shapes %v", m.Shapes)
	}
	for _, name := range []string{"square", "dot", "person"} {
		sh := m.Shapes.Get(name)
		if sh == nil {
			t.Fatalf("missing %s", name)
		}
		if sh.Rotatable {
			t.Fatalf("%s is rotatable; NetLogo draws it upright", name)
		}
		if len(sh.Elements) == 0 {
			t.Fatalf("%s has no elements", name)
		}
	}
	sq := m.Shapes.Get("square").Elements[0]
	if sq.Kind != "rectangle" || !sq.Filled || !sq.Marked || sq.X != 5 || sq.Y != 4 || sq.W != 289 || sq.H != 291 {
		t.Fatalf("square element %+v", sq)
	}
	dot := m.Shapes.Get("dot").Elements[0]
	if dot.Kind != "circle" || dot.W != 120 {
		t.Fatalf("dot element %+v", dot)
	}
	arrow := m.Shapes.Get("arrow")
	if arrow == nil || !arrow.Rotatable {
		t.Fatalf("arrow %+v", arrow)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if vm.World.LabelFont != 10 || vm.World.Shapes.Get("square") == nil {
		t.Fatalf("world font %d shapes %v", vm.World.LabelFont, vm.World.Shapes)
	}
}

func TestSquareIgnoresHeading(t *testing.T) {
	up := stampShape(t, "square", 0)
	tilt := stampShape(t, "square", 45)
	if !bytes.Equal(up.Pix, tilt.Pix) {
		t.Fatal("non-rotatable square changed with heading")
	}
	person := stampShape(t, "person", 0)
	personTurned := stampShape(t, "person", 90)
	if !bytes.Equal(person.Pix, personTurned.Pix) {
		t.Fatal("non-rotatable person changed with heading")
	}
	east := stampShape(t, "arrow", 90)
	north := stampShape(t, "arrow", 0)
	if bytes.Equal(north.Pix, east.Pix) {
		t.Fatal("rotatable arrow ignored heading")
	}

	w := NewWorld(0, 3, 0, 3, false, false)
	tu := w.CreateTurtle("", 1, 1, 109, 45)
	tu.Shape = "square"
	tu.Size = 1
	tu.Color = 109
	w.Stamp(tu, false)
	if _, ok := w.DrawingAt(1, 1); !ok {
		t.Fatal("square missed its own patch")
	}
	if _, ok := w.DrawingAt(2, 2); ok {
		t.Fatal("heading 45 made square cover the diagonal neighbor")
	}
	if _, ok := w.DrawingAt(2, 1); ok {
		t.Fatal("square covered an orthogonal neighbor")
	}
}

func TestDotSmallerThanCircle(t *testing.T) {
	dot := countInk(stampShape(t, "dot", 0))
	circle := countInk(stampShape(t, "circle", 0))
	if dot == 0 || circle == 0 || dot*3 > circle {
		t.Fatalf("dot ink %d, circle ink %d; the dot shape is a small circle", dot, circle)
	}
}

func TestLabelSitsInsideBottomEdge(t *testing.T) {
	w := NewWorld(0, 2, 0, 2, false, false)
	for _, a := range w.Patches().Agents {
		a.(*Patch).PColor = 9.9
	}
	w.PatchSize = 30
	w.LabelFont = 10
	tu := w.CreateTurtle("", 0, 0, 0, 0)
	tu.Shape = "person"
	tu.Size = 1
	tu.Color = 105
	tu.Label = Num(0)
	tu.Vars["label-color"] = Num(0)
	img := RenderWorld(w, 30)
	// The anchor is the bottom-right of the size-1 turtle, x = one patch.
	// "0" grows left from there and up from the bottom edge.
	dark, right := countBottomInk(img, img.Bounds().Dy()-16, img.Bounds().Dy())
	if dark < 8 {
		t.Fatalf("label ink %d, the 0 was clipped off the bottom", dark)
	}
	if right > 31 {
		t.Fatalf("label reaches x %d, past the turtle's bottom-right corner", right)
	}
	if right < 24 {
		t.Fatalf("label ends at x %d, short of the bottom-right corner", right)
	}
}

func TestNLogoXShapesAndFont(t *testing.T) {
	const src = `<?xml version="1.0" encoding="utf-8"?>
<model version="NetLogo 7.0.0">
  <code>to setup clear-all end</code>
  <view minPxcor="0" maxPxcor="2" minPycor="0" maxPycor="2" patchSize="20" fontSize="14" wrappingAllowedX="false" wrappingAllowedY="false"/>
  <turtleShapes>
    <shape name="square" rotatable="false" editableColorIndex="0">
      <rectangle color="-7500403" filled="true" marked="true" startX="5" startY="4" endX="294" endY="295"/>
    </shape>
    <shape name="badge" rotatable="true" editableColorIndex="0">
      <polygon color="-16777216" filled="true" marked="false">
        <point x="150" y="20"/>
        <point x="40" y="280"/>
        <point x="260" y="280"/>
      </polygon>
      <circle color="-7500403" filled="true" marked="true" x="120" y="80" diameter="60"/>
    </shape>
  </turtleShapes>
</model>
`
	m, err := ParseModel("shapes.nlogox", src)
	if err != nil {
		t.Fatal(err)
	}
	if m.View.FontSize != 14 {
		t.Fatalf("font %d", m.View.FontSize)
	}
	sq := m.Shapes.Get("square")
	if sq == nil || sq.Rotatable || sq.Elements[0].Kind != "rectangle" || sq.Elements[0].W != 289 {
		t.Fatalf("square %+v", sq)
	}
	badge := m.Shapes.Get("badge")
	if badge == nil || !badge.Rotatable || len(badge.Elements) != 2 {
		t.Fatalf("badge %+v", badge)
	}
	if badge.Elements[0].Kind != "polygon" || badge.Elements[0].Marked || badge.Elements[0].R != 0 {
		t.Fatalf("badge polygon %+v", badge.Elements[0])
	}
	if badge.Elements[1].Kind != "circle" || !badge.Elements[1].Marked || badge.Elements[1].W != 60 {
		t.Fatalf("badge circle %+v", badge.Elements[1])
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if vm.World.LabelFont != 14 || vm.World.Shapes.Get("badge") == nil {
		t.Fatal("xml shapes were not installed on the world")
	}
}

func TestDefaultShapesPresent(t *testing.T) {
	w := NewWorld(-1, 1, -1, 1, true, true)
	if w.Shapes.Get("default") == nil || w.Shapes.Get("square") == nil || w.LabelFont != 10 {
		t.Fatalf("default library font %d square %v", w.LabelFont, w.Shapes.Get("square"))
	}
	if strings.TrimSpace(defaultShapesText) == "" {
		t.Fatal("embedded shape library is empty")
	}
}

func stampShape(t *testing.T, shape string, heading float64) *image.RGBA {
	t.Helper()
	w := NewWorld(-2, 2, -2, 2, false, false)
	for _, a := range w.Patches().Agents {
		a.(*Patch).PColor = 9.9
	}
	tu := w.CreateTurtle("", 0, 0, 15, int(heading))
	tu.Shape = shape
	tu.Size = 1
	tu.Color = 15
	w.Stamp(tu, false)
	tu.Hidden = true
	return RenderWorld(w, 20)
}

func countInk(img *image.RGBA) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 250 || c.G < 250 || c.B < 250 {
				n++
			}
		}
	}
	return n
}

func countBottomInk(img *image.RGBA, y0, y1 int) (dark, right int) {
	right = -1
	b := img.Bounds()
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 40 && c.G < 40 && c.B < 40 {
				dark++
				if x > right {
					right = x
				}
			}
		}
	}
	return dark, right
}
