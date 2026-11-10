package nl

import (
	"math"
	"strings"
	"testing"
)

func newTestVM3D(t *testing.T, src string, min, max int, seed int64) *VM {
	t.Helper()
	prog, err := Parse3D(src)
	if err != nil {
		t.Fatalf("parse3d: %v\n%s", err, src)
	}
	w := NewWorld3D(min, max, min, max, min, max)
	w.Seed(seed)
	return NewVM(prog, w)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// near360 compares angles in degrees, treating 0 and 360 as equal.
func near360(a, b float64) bool {
	d := math.Mod(math.Abs(a-b), 360)
	return d < 1e-9 || 360-d < 1e-9
}

func TestWorld3DShapeAndWrap(t *testing.T) {
	w := NewWorld3D(-2, 2, -3, 3, -1, 1)
	if !w.Is3D() || w.Depth() != 3 || w.Width() != 5 || w.Height() != 7 {
		t.Fatalf("3D dims: 3d=%v w=%d h=%d d=%d", w.Is3D(), w.Width(), w.Height(), w.Depth())
	}
	if got := len(w.Patches().Agents); got != 5*7*3 {
		t.Fatalf("patch count %d, want %d", got, 5*7*3)
	}
	if !w.WrapX || !w.WrapY || !w.WrapZ {
		t.Fatalf("3D worlds wrap on every axis")
	}
	// Every patch is where its coordinates say.
	for _, a := range w.Patches().Agents {
		p := a.(*Patch)
		if w.GetPatch3(p.PX, p.PY, p.PZ) != p {
			t.Fatalf("patch %v is not indexed at its coordinates", p.AgentString())
		}
	}
	// The first patch is the top layer's top-left corner.
	first := w.Patches().Agents[0].(*Patch)
	if first.PX != -2 || first.PY != 3 || first.PZ != 1 {
		t.Fatalf("first patch %s", first.AgentString())
	}
	cases := []struct{ in, want float64 }{
		{1.6, -1.4}, {-1.6, 1.4}, {1.4, 1.4}, {-1.5, -1.5}, {1.5, -1.5}, {4.5, 1.5 - 3},
	}
	for _, c := range cases {
		got, err := w.wrapCoordZ(c.in)
		if err != nil || !near(got, c.want) {
			t.Errorf("wrapCoordZ(%v) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	if p := w.PatchAt3(0, 0, 1.6); p == nil || p.PZ != -1 {
		t.Fatalf("PatchAt3 z wrap: %v", p)
	}
	if p := w.PatchAtPcor3(0, 0, 2); p == nil || p.PZ != -1 {
		t.Fatalf("PatchAtPcor3 z wrap: %v", p)
	}
	if p := w.PatchAtPcor3(0, 0, -2); p == nil || p.PZ != 1 {
		t.Fatalf("PatchAtPcor3 z wrap down: %v", p)
	}
}

func TestWorld3DDistancePitch(t *testing.T) {
	w := NewWorld3D(-5, 5, -5, 5, -5, 5)
	if d := w.Distance3(0, 0, 0, 1, 2, 2); !near(d, 3) {
		t.Fatalf("distance %v, want 3", d)
	}
	// 5 to -5 is one step around the z wrap.
	if d := w.Distance3(0, 0, 5, 0, 0, -5); !near(d, 1) {
		t.Fatalf("wrapped z distance %v, want 1", d)
	}
	if d := distanceNoWrap(0, 0, 5, 0, 0, -5); !near(d, 10) {
		t.Fatalf("nowrap distance %v, want 10", d)
	}
	pitchCases := []struct {
		x, y, z float64
		want    float64
	}{
		{0, 0, 1, 90}, {0, 0, -1, 270}, {1, 0, 0, 0}, {0, 1, 1, 45}, {1, 0, -1, 315},
	}
	for _, c := range pitchCases {
		p, err := w.PitchTowards(0, 0, 0, c.x, c.y, c.z, true)
		if err != nil || !near(p, c.want) {
			t.Errorf("pitch to %v,%v,%v = %v, %v; want %v", c.x, c.y, c.z, p, err, c.want)
		}
	}
	if _, err := w.PitchTowards(1, 1, 1, 1, 1, 1, true); err == nil {
		t.Fatalf("pitch from a point to itself should fail")
	}
	// Across the z wrap, straight up is the short way to -5 from 5.
	if p, _ := w.PitchTowards(0, 0, 5, 0, 0, -5, true); !near(p, 90) {
		t.Fatalf("wrapped pitch %v, want 90", p)
	}
	if p, _ := w.PitchTowards(0, 0, 5, 0, 0, -5, false); !near(p, 270) {
		t.Fatalf("nowrap pitch %v, want 270", p)
	}
}

func TestOrientationMath(t *testing.T) {
	dx, dy, dz := orientDelta(90, 0, 1)
	if !near(dx, 1) || !near(dy, 0) || !near(dz, 0) {
		t.Fatalf("level east: %v %v %v", dx, dy, dz)
	}
	dx, dy, dz = orientDelta(0, 90, 2)
	if !near(dx, 0) || !near(dy, 0) || !near(dz, 2) {
		t.Fatalf("straight up: %v %v %v", dx, dy, dz)
	}
	dx, dy, dz = orientDelta(0, 30, 1)
	if !near(dy, math.Cos(math.Pi/6)) || !near(dz, 0.5) || !near(dx, 0) {
		t.Fatalf("pitch 30: %v %v %v", dx, dy, dz)
	}
	// Round trip through the frame for a spread of orientations.
	for _, h := range []float64{0, 37, 90, 200, 359} {
		for _, p := range []float64{0, 20, 80, 300} {
			for _, r := range []float64{0, 45, 180, 270} {
				f, u, _ := orientFrame(h, p, r)
				h2, p2, r2 := frameAngles(f, u, h)
				if !near360(h, h2) || !near360(p, p2) || !near360(r, r2) {
					t.Fatalf("round trip %v %v %v -> %v %v %v", h, p, r, h2, p2, r2)
				}
			}
		}
	}

	w := NewWorld3D(-5, 5, -5, 5, -5, 5)
	tt := w.CreateTurtle3("", 0, 0, 0, 0, 0)

	// Level and unrolled, right only changes heading.
	if err := w.TurnRight3D(tt, 30); err != nil || tt.Heading != 30 || tt.Pitch != 0 || tt.Roll != 0 {
		t.Fatalf("level rt: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
	// With no roll, tilt-up only changes pitch.
	tt.setOrientation(0, 0, 0)
	w.TiltUp(tt, 90)
	if tt.Heading != 0 || tt.Pitch != 90 || tt.Roll != 0 {
		t.Fatalf("tilt-up 90: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
	// Nose up, a right turn is about the up vector (which now points
	// south): the nose swings east and the turtle lies on its right side.
	if err := w.TurnRight3D(tt, 90); err != nil {
		t.Fatal(err)
	}
	if !near360(tt.Heading, 90) || !near360(tt.Pitch, 0) || !near360(tt.Roll, 90) {
		t.Fatalf("rt 90 while pitched up: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
	// Rolled 90 right, the up vector points east, so tilting up turns
	// the nose east.
	tt.setOrientation(0, 0, 90)
	w.TiltUp(tt, 90)
	if !near360(tt.Heading, 90) || !near360(tt.Pitch, 0) || !near360(tt.Roll, 90) {
		t.Fatalf("tilt-up while rolled: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
	tt.setOrientation(10, 20, 350)
	w.RollRight(tt, 20)
	if tt.Heading != 10 || tt.Pitch != 20 || !near(tt.Roll, 10) {
		t.Fatalf("roll-right: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
	// tilt-down undoes tilt-up even with roll.
	tt.setOrientation(33, 12, 40)
	w.TiltUp(tt, 25)
	w.TiltUp(tt, -25)
	if !near360(tt.Heading, 33) || !near360(tt.Pitch, 12) || !near360(tt.Roll, 40) {
		t.Fatalf("tilt round trip: %v %v %v", tt.Heading, tt.Pitch, tt.Roll)
	}
}

func TestNeighbors3D(t *testing.T) {
	w := NewWorld3D(-2, 2, -2, 2, -2, 2)
	p := w.GetPatch3(0, 0, 0)
	if n := len(w.Neighbors(p)); n != 26 {
		t.Fatalf("neighbors %d, want 26", n)
	}
	if n := len(w.Neighbors6(p)); n != 6 {
		t.Fatalf("neighbors6 %d, want 6", n)
	}
	// neighbors4 stays on the patch's own layer.
	n4 := w.Neighbors4(p)
	if len(n4) != 4 {
		t.Fatalf("neighbors4 %d, want 4", len(n4))
	}
	for _, q := range n4 {
		if q.PZ != 0 {
			t.Fatalf("neighbors4 left the layer: %s", q.AgentString())
		}
	}
	// A corner still has 26 because the world wraps.
	if n := len(w.Neighbors(w.GetPatch3(2, 2, 2))); n != 26 {
		t.Fatalf("corner neighbors %d, want 26", n)
	}
	// A world one layer deep folds the layers above and below onto the
	// patch's own layer, leaving the 8 ring neighbors.
	flat := NewWorld3D(-2, 2, -2, 2, 0, 0)
	if n := len(flat.Neighbors(flat.GetPatch3(0, 0, 0))); n != 8 {
		t.Fatalf("one-layer 3D neighbors %d, want 8", n)
	}
	if n := len(flat.Neighbors6(flat.GetPatch3(0, 0, 0))); n != 4 {
		t.Fatalf("one-layer 3D neighbors6 %d, want 4", n)
	}
}

func TestPrimitives3D(t *testing.T) {
	src := `
breed [ birds bird ]
turtles-own [ seen ]
patches-own [ heat ]
to-report approx [ x ] report precision x 6 end
`
	cases := []struct {
		name, setup, eval, want string
	}{
		{"world reporters", ``, `(list min-pzcor max-pzcor world-depth world-width world-height)`, "[-3 3 7 7 7]"},
		{"patch 3 args", ``, `[(list pxcor pycor pzcor)] of patch 1 -2 3`, "[1 -2 3]"},
		{"patch wraps z", ``, `[pzcor] of patch 0 0 4`, "-3"},
		{"patch prints z", ``, `patch 1 2 3`, "(patch 1 2 3)"},
		{"setxyz", `crt 1 [ setxyz 1 2 3 ]`, `[(list xcor ycor zcor pzcor)] of turtle 0`, "[1 2 3 3]"},
		{"setxyz wraps", `crt 1 [ setxyz 0 0 3.6 ]`, `[approx zcor] of turtle 0`, "-3.4"},
		{"setxy keeps z", `crt 1 [ setxyz 1 1 2 setxy 0 0 ]`, `[zcor] of turtle 0`, "2"},
		{"set zcor", `crt 1 [ set zcor -2 ]`, `[(list zcor pzcor)] of turtle 0`, "[-2 -2]"},
		{"fd along pitch", `crt 1 [ set heading 0 set pitch 90 fd 2 ]`, `[map approx (list xcor ycor zcor)] of turtle 0`, "[0 0 2]"},
		{"bk along pitch", `crt 1 [ set heading 90 set pitch 270 bk 1 ]`, `[map approx (list xcor ycor zcor)] of turtle 0`, "[0 0 1]"},
		{"fd wraps z", `crt 1 [ set pitch 90 fd 4 ]`, `[approx zcor] of turtle 0`, "-3"},
		{"dx dy dz", `crt 1 [ set heading 90 set pitch 30 ]`, `[map approx (list dx dy dz)] of turtle 0`, "[0.866025 0 0.5]"},
		{"tilt-up and tilt-down", `crt 1 [ set heading 0 tilt-up 30 tilt-down 10 ]`, `[pitch] of turtle 0`, "20"},
		{"roll-left", `crt 1 [ set heading 0 roll-left 30 ]`, `[roll] of turtle 0`, "330"},
		{"set pitch normalizes", `crt 1 [ set pitch -90 set roll 400 ]`, `[list pitch roll] of turtle 0`, "[270 40]"},
		{"distancexyz", `crt 1 [ setxyz 0 0 0 ]`, `[distancexyz 1 2 2] of turtle 0`, "3"},
		{"distancexyz wraps", `crt 1 [ setxyz 0 0 3 ]`, `[distancexyz 0 0 -3] of turtle 0`, "1"},
		{"distancexyz-nowrap", `crt 1 [ setxyz 0 0 3 ]`, `[distancexyz-nowrap 0 0 -3] of turtle 0`, "6"},
		{"distance in 3D", `crt 2 ask turtle 1 [ setxyz 0 0 2 ]`, `[distance turtle 1] of turtle 0`, "2"},
		{"towards-pitch", `crt 2 [ setxyz 0 0 0 ] ask turtle 1 [ setxyz 0 0 2 ]`, `[towards-pitch turtle 1] of turtle 0`, "90"},
		{"towards-pitch-xyz", `crt 1 [ setxyz 0 0 0 ]`, `[towards-pitch-xyz 0 1 -1] of turtle 0`, "315"},
		{"towards-pitch-nowrap", `crt 2 [ setxyz 0 0 3 ] ask turtle 1 [ setxyz 0 0 -3 ]`, `[towards-pitch-nowrap turtle 1] of turtle 0`, "270"},
		{"towards-pitch-xyz-nowrap", `crt 1 [ setxyz 0 0 3 ]`, `[towards-pitch-xyz-nowrap 0 0 -3] of turtle 0`, "270"},
		{"towardsxyz", `crt 1 [ setxyz 0 0 0 ]`, `[towardsxyz 0 1 1] of turtle 0`, "0"},
		{"towardsxyz east", `crt 1 [ setxyz 0 0 0 ]`, `[towardsxyz 1 0 5] of turtle 0`, "90"},
		{"towardsxyz-nowrap ignores the long way", `crt 1 [ setxyz 3 0 0 ]`, `[towardsxyz-nowrap -3 0 0] of turtle 0`, "270"},
		{"towardsxyz wraps", `crt 1 [ setxyz 3 0 0 ]`, `[towardsxyz -3 0 0] of turtle 0`, "90"},
		{"facexyz", `crt 1 [ setxyz 0 0 0 facexyz 1 0 1 ]`, `[map approx (list heading pitch)] of turtle 0`, "[90 45]"},
		{"facexyz straight up keeps heading", `crt 1 [ set heading 33 facexyz 0 0 2 ]`, `[list heading pitch] of turtle 0`, "[33 90]"},
		{"face in 3D", `crt 2 [ setxyz 0 0 0 ] ask turtle 1 [ setxyz 0 -1 -1 ] ask turtle 0 [ face turtle 1 ]`, `[map approx (list heading pitch)] of turtle 0`, "[180 315]"},
		{"neighbors", ``, `[count neighbors] of patch 0 0 0`, "26"},
		{"neighbors6", ``, `[count neighbors6] of patch 0 0 0`, "6"},
		{"neighbors4 same layer", ``, `[remove-duplicates [pzcor] of neighbors4] of patch 0 0 1`, "[1]"},
		{"patch-at 3 args", ``, `[patch-at 1 0 -1] of patch 0 0 0`, "(patch 1 0 -1)"},
		{"turtles-at 3 args", `crt 1 [ setxyz 1 1 1 ]`, `[count turtles-at 1 1 1] of patch 0 0 0`, "1"},
		{"breed-at 3 args", `create-birds 1 [ setxyz 0 0 2 ]`, `[count birds-at 0 0 2] of patch 0 0 0`, "1"},
		{"patch-ahead along pitch", `crt 1 [ set pitch 90 ]`, `[patch-ahead 1] of turtle 0`, "(patch 0 0 1)"},
		{"patch-at-heading-pitch-and-distance", ``, `[patch-at-heading-pitch-and-distance 90 90 2] of patch 0 0 0`, "(patch 0 0 2)"},
		{"in-radius is a ball", ``, `[count patches in-radius 1] of patch 0 0 0`, "7"},
		{"in-radius 3D distance", `crt 2 ask turtle 1 [ setxyz 0 0 1.5 ]`, `[count other turtles in-radius 1] of turtle 0`, "0"},
		{"in-cone 3D", `crt 3 [ set heading 0 set pitch 90 ] ask turtle 1 [ setxyz 0 0 2 ] ask turtle 2 [ setxyz 0 2 0 ]`, `[sort [who] of other turtles in-cone 3 60] of turtle 0`, "[1]"},
		{"at-points triples", ``, `[count patches at-points [[0 0 1] [0 0 -1] [1 0 0]]] of patch 0 0 0`, "3"},
		{"random-zcor in range", ``, `reduce and n-values 50 [ (random-zcor >= -3.5) and (random-zcor < 3.5) ]`, "true"},
		{"random-pzcor in range", ``, `reduce and n-values 50 [ member? random-pzcor [-3 -2 -1 0 1 2 3] ]`, "true"},
		{"sprout carries pzcor", `ask patch 1 2 3 [ sprout 1 ]`, `[(list xcor ycor zcor pitch roll)] of turtle 0`, "[1 2 3 0 0]"},
		{"hatch carries z and orientation", `crt 1 [ setxyz 1 1 2 set pitch 10 set roll 20 hatch 1 ]`, `[(list zcor pitch roll)] of turtle 1`, "[2 10 20]"},
		{"move-to patch in 3D", `crt 1 [ move-to patch 1 1 -2 ]`, `[(list xcor ycor zcor)] of turtle 0`, "[1 1 -2]"},
		{"home in 3D", `crt 1 [ setxyz 1 1 1 home ]`, `[(list xcor ycor zcor)] of turtle 0`, "[0 0 0]"},
		{"link-pitch and link-length", `crt 2 ask turtle 1 [ setxyz 0 0 2 ] ask turtle 0 [ create-link-with turtle 1 ]`, `[list link-pitch link-length] of link 0 1`, "[90 2]"},
		{"pzcor of patches", ``, `count patches with [pzcor = 3]`, "49"},
		{"patch var by turtle", `crt 1 [ setxyz 0 0 2 set heat 5 ]`, `[heat] of patch 0 0 2`, "5"},
		{"resize-world 6 args", `resize-world -1 1 -1 1 0 2`, `(list world-width world-depth count patches min-pzcor)`, "[3 3 27 0]"},
		{"observer setxyz", `setxyz 1 2 3`, `(list __oxcor __oycor __ozcor)`, "[1 2 3]"},
		// From straight above at 10.5, orbit-down 90 lands south of the
		// target, level with it; orbit-right 90 then swings it east.
		{"observer orbit-down", `reset-perspective orbit-down 90`, `map approx (list __oxcor __oycor __ozcor)`, "[0 -10.5 0]"},
		{"observer orbit-right", `reset-perspective orbit-down 90 orbit-right 90`, `map approx (list __oxcor __oycor __ozcor)`, "[10.5 0 0]"},
		{"observer orbit-left and up", `reset-perspective orbit-down 90 orbit-left 90 orbit-up 45`, `map approx (list __oxcor __oycor __ozcor)`, "[-7.424621 0 7.424621]"},
		{"observer orbit stops overhead", `reset-perspective orbit-up 30`, `map approx (list __oxcor __oycor __ozcor)`, "[0 0 10.5]"},
		{"observer zoom", `reset-perspective zoom 4 zoom -1`, `map approx (list __oxcor __oycor __ozcor)`, "[0 0 7.5]"},
		{"observer zoom stops at target", `reset-perspective zoom 100`, `map approx (list __oxcor __oycor __ozcor)`, "[0 0 0.01]"},
		{"observer facexyz keeps position", `setxyz 0 -10 0 facexyz 0 0 5 orbit-up 0`, `map approx (list __oxcor __oycor __ozcor)`, "[0 -10 0]"},
		{"observer face then orbit", `crt 1 [ setxyz 2 0 0 ] setxyz 2 -5 0 face turtle 0 orbit-right 180`, `map approx (list __oxcor __oycor __ozcor)`, "[2 5 0]"},
		{"reset-perspective homes the camera", `setxyz 1 2 3 reset-perspective`, `(list __oxcor __oycor __ozcor)`, "[0 0 10.5]"},
		{"diffuse keeps total", `ask patch 0 0 0 [ set heat 26 ] diffuse heat 1`, `(list sum [heat] of patches [heat] of patch 1 1 1 [heat] of patch 0 0 0)`, "[26 1 0]"},
		{"uphill in 3D", `ask patch 0 0 1 [ set heat 9 ] crt 1 [ uphill heat ]`, `[(list zcor pitch)] of turtle 0`, "[1 90]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vm := newTestVM3D(t, src, -3, 3, 1)
			if c.setup != "" {
				if _, err := vm.ExecuteLine(c.setup); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			if got := mustEval(t, vm, c.eval); got != c.want {
				t.Fatalf("%s = %s, want %s", c.eval, got, c.want)
			}
		})
	}
}

func TestRightTurnUsesUpVector3D(t *testing.T) {
	vm := newTestVM3D(t, "", -3, 3, 1)
	if _, err := vm.ExecuteLine(`crt 1 [ set heading 0 set pitch 90 rt 90 ]`); err != nil {
		t.Fatal(err)
	}
	got := mustEval(t, vm, `[map [x -> precision x 6] (list heading pitch roll)] of turtle 0`)
	if got != "[90 0 90]" {
		t.Fatalf("rt with pitch: %s", got)
	}
	// The compiled fast path for rt/lt takes the same 3D turn.
	vm = newTestVM3D(t, "to go ask turtles [ lt 90 ] end", -3, 3, 1)
	if _, err := vm.ExecuteLine(`crt 1 [ set heading 0 set pitch 90 ]`); err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "go")
	got = mustEval(t, vm, `[map [x -> precision x 6] (list heading pitch roll)] of turtle 0`)
	if got != "[270 0 270]" {
		t.Fatalf("compiled lt with pitch: %s", got)
	}
}

func TestErrors3D(t *testing.T) {
	vm := newTestVM3D(t, "", -2, 2, 1)
	for _, c := range []struct{ src, frag string }{
		{`crt 1 [ set pzcor 1 ]`, "can't change pzcor"},
		{`ask patch 0 0 0 [ set pzcor 1 ]`, "can't change pzcor"},
		{`resize-world -1 1 -1 1 1 2`, "origin"},
		{`resize-world -200 200 -200 200 -200 200`, "at most"},
		{`ask patch 0 0 0 [ show count patches at-points [[1 0]] ]`, "triples"},
		{`crt 1 [ orbit-up 5 ]`, "observer"},
	} {
		_, err := vm.ExecuteLine(c.src)
		if err == nil || !strings.Contains(err.Error(), c.frag) {
			t.Errorf("%s: err %v, want %q", c.src, err, c.frag)
		}
	}
	// The 2D arity is a compile error in the 3D dialect.
	if _, err := Parse3D("to f show patch 0 0 end"); err == nil {
		t.Fatalf("patch with 2 args should not compile in 3D")
	}
}

// --- 2D regression: the 3D work must leave 2D worlds alone ---

func TestWorld2DUnchanged(t *testing.T) {
	w := NewWorld(-3, 3, -2, 2, true, false)
	if w.Is3D() || w.Depth() != 1 || w.MinPzcor != 0 || w.MaxPzcor != 0 {
		t.Fatalf("2D world has depth %d [%d %d]", w.Depth(), w.MinPzcor, w.MaxPzcor)
	}
	if n := len(w.Patches().Agents); n != 7*5 {
		t.Fatalf("2D patch count %d", n)
	}
	p := w.GetPatch(0, 0)
	if n := len(w.Neighbors(p)); n != 8 {
		t.Fatalf("2D neighbors %d", n)
	}
	if n := len(w.Neighbors4(p)); n != 4 {
		t.Fatalf("2D neighbors4 %d", n)
	}
	// A non-wrapping y edge still cuts neighbors.
	if n := len(w.Neighbors(w.GetPatch(0, 2))); n != 5 {
		t.Fatalf("2D edge neighbors %d, want 5", n)
	}
	if p.AgentString() != "(patch 0 0)" {
		t.Fatalf("2D patch prints %s", p.AgentString())
	}
	tt := w.CreateTurtle("", 0, 0, 0, 0)
	if err := w.Turn(tt, 45); err != nil || tt.Heading != 45 {
		t.Fatalf("2D turn %v", tt.Heading)
	}
	if err := w.Forward(tt, math.Sqrt2); err != nil || !near(tt.X, 1) || !near(tt.Y, 1) || tt.Z != 0 {
		t.Fatalf("2D fd to %v %v %v", tt.X, tt.Y, tt.Z)
	}
	// MoveTo3 in 2D ignores z.
	if err := w.MoveTo3(tt, 0, 0, 5); err != nil || tt.Z != 0 {
		t.Fatalf("2D MoveTo3 z %v err %v", tt.Z, err)
	}
	// The non-wrapping y edge still refuses a jump.
	tt.Heading = 0
	if err := w.Jump(tt, 10); err == nil {
		t.Fatalf("2D jump past the edge should fail")
	}
}

func TestPrimitives2DUnchanged(t *testing.T) {
	src := `
turtles-own [ pitch roll ]
patches-own [ pzcor ]
to-report approx [ x ] report precision x 6 end
`
	cases := []struct{ setup, eval, want string }{
		{`crt 1 [ set heading 90 ]`, `[map approx (list dx dy)] of turtle 0`, "[1 0]"},
		{`crt 1 [ setxy 1 2 ]`, `[list xcor ycor] of turtle 0`, "[1 2]"},
		{``, `[count neighbors] of patch 0 0`, "8"},
		{``, `[count neighbors4] of patch 0 0`, "4"},
		{``, `patch 1 2`, "(patch 1 2)"},
		{``, `[patch-at 1 1] of patch 0 0`, "(patch 1 1)"},
		{``, `[count patches in-radius 1] of patch 0 0`, "5"},
		{`crt 1 [ set heading 0 rt 30 ]`, `[heading] of turtle 0`, "30"},
		// Names that are 3D built-ins stay ordinary variables in 2D.
		{`crt 1 [ set pitch 7 set roll 8 ]`, `[list pitch roll] of turtle 0`, "[7 8]"},
		{`ask patch 0 0 [ set pzcor 3 ]`, `[pzcor] of patch 0 0`, "3"},
	}
	for _, c := range cases {
		vm := newTestVM(t, src, -3, 3, true, 1)
		if c.setup != "" {
			if _, err := vm.ExecuteLine(c.setup); err != nil {
				t.Fatalf("setup %s: %v", c.setup, err)
			}
		}
		if got := mustEval(t, vm, c.eval); got != c.want {
			t.Fatalf("%s = %s, want %s", c.eval, got, c.want)
		}
	}
	// 3D-only primitives are still undefined in a 2D program.
	for _, code := range []string{
		"to f crt 1 [ setxyz 0 0 0 ] end",
		"to f crt 1 [ tilt-up 5 ] end",
		"to f show world-depth end",
		"to f show patch 0 0 0 end",
		"to f crt 1 [ show zcor ] end",
	} {
		prog, err := Parse(code)
		if err == nil {
			vm := NewVM(prog, NewWorld(-3, 3, -3, 3, true, true))
			_, err = vm.Call("f")
		}
		if err == nil {
			t.Errorf("%q should fail in 2D", code)
		}
	}
}

func TestPerspective3D(t *testing.T) {
	vm := newTestVM3D(t, "", -4, 4, 1)
	mustRun(t, vm, `crt 1 [ setxyz 1 2 3 set heading 90 ] follow turtle 0`)
	mode, agent := vm.World.Perspective()
	if mode != "follow" || agent == nil || agent.(*Turtle).Who != 0 {
		t.Fatalf("follow mode %q agent %v", mode, agent)
	}
	mustRun(t, vm, `ride turtle 0`)
	mode, agent = vm.World.Perspective()
	if mode != "ride" || agent == nil || agent.(*Turtle).Who != 0 {
		t.Fatalf("ride mode %q agent %v", mode, agent)
	}
	mustRun(t, vm, `watch turtle 0`)
	mode, _ = vm.World.Perspective()
	if mode != "watch" {
		t.Fatalf("watch mode %q", mode)
	}
	// Move the camera, then reset. The subject is cleared and the camera
	// goes home, which is where a fresh world of this size starts.
	mustRun(t, vm, `setxyz 4 0 1 reset-perspective`)
	mode, agent = vm.World.Perspective()
	if mode != "" || agent != nil {
		t.Fatalf("reset left mode %q agent %v", mode, agent)
	}
	fresh := NewWorld3D(-4, 4, -4, 4, -4, 4)
	fx, fy, fz := fresh.ObserverXYZ()
	x, y, z := vm.World.ObserverXYZ()
	if !near(x, fx) || !near(y, fy) || !near(z, fz) {
		t.Fatalf("camera (%v %v %v), home (%v %v %v)", x, y, z, fx, fy, fz)
	}
	mustRun(t, vm, `crt 1 [ watch-me die ]`)
	mode, agent = vm.World.Perspective()
	if mode != "" || agent != nil {
		t.Fatalf("die left mode %q agent %v", mode, agent)
	}
}

func TestPenTrails3D(t *testing.T) {
	vm := newTestVM3D(t, "", -4, 4, 1)
	mustRun(t, vm, `
crt 1 [
  set color red
  set pen-size 3
  pen-down
  set heading 0
  set pitch 90
  fd 2
  pen-up
  fd 1
]
`)
	marks := vm.World.Marks()
	if len(marks) != 2 {
		t.Fatalf("marks %d, want two pen-down steps and no pen-up step: %+v", len(marks), marks)
	}
	if !marks[0].Pen || marks[0].Erase || marks[0].Size != 3 || marks[0].Z != 0 || marks[0].Z2 != 1 {
		t.Fatalf("first 3D stroke %+v", marks[0])
	}
	if !near(marks[1].X, 0) || !near(marks[1].Y, 0) || marks[1].Z != 1 || marks[1].Z2 != 2 {
		t.Fatalf("second 3D stroke %+v", marks[1])
	}
	if got := mustEval(t, vm, "[pen-mode] of turtle 0"); got != "up" {
		t.Fatalf("pen-mode after pen-up = %s", got)
	}
	mustRun(t, vm, `ask turtle 0 [ pen-erase hatch 1 ]`)
	if got := mustEval(t, vm, "[pen-mode] of turtle 1"); got != "erase" {
		t.Fatalf("hatched pen-mode = %s", got)
	}
	mustRun(t, vm, `ask turtle 0 [ set pitch 270 fd 2 ]`)
	marks = vm.World.Marks()
	erases := 0
	for _, m := range marks {
		if m.Pen && m.Erase {
			erases++
			if !near(m.X, 0) || !near(m.Y, 0) {
				t.Fatalf("erase stroke left the z axis: %+v", m)
			}
		}
	}
	if erases != 2 {
		t.Fatalf("erase strokes %d, marks %+v", erases, marks)
	}
	mustRun(t, vm, "clear-drawing")
	if len(vm.World.Marks()) != 0 {
		t.Fatal("clear-drawing left 3D pen trails")
	}
	mustRun(t, vm, `crt 1 [ pen-down set pitch 90 fd 1 ]`)
	mustRun(t, vm, "clear-all")
	if len(vm.World.Marks()) != 0 {
		t.Fatal("clear-all left 3D pen trails")
	}

	// The same cap that bounds a 2D stamp log bounds a 3D pen.
	w := NewWorld3D(-2, 2, -2, 2, -2, 2)
	turt := w.CreateTurtle("", 0, 0, 0, 0)
	turt.PenDown = true
	turt.PenSize = 1
	for i := 0; i < MaxDrawingMarks+3; i++ {
		if err := w.MoveTo3(turt, turt.X+0.1, 0, float64(i%5)); err != nil {
			t.Fatal(err)
		}
	}
	got := w.Marks()
	if len(got) != MaxDrawingMarks {
		t.Fatalf("3D pen marks %d, want cap %d", len(got), MaxDrawingMarks)
	}
	last := got[len(got)-1]
	if !last.Pen || last.Erase {
		t.Fatalf("newest kept mark is not a pen stroke: %+v", last)
	}
}

func TestLoad3DModels(t *testing.T) {
	xmlModel := `<?xml version="1.0" encoding="utf-8"?>
<model version="NetLogo 3D 7.0.0" snapToGrid="false">
  <code><![CDATA[to setup ca crt 1 [ setxyz 1 1 1 ] end]]></code>
  <widgets>
    <view3d wrappingAllowedZ="true" wrappingAllowedX="true" wrappingAllowedY="true" minPxcor="-4" maxPxcor="4" minPycor="-3" maxPycor="3" minPzcor="-2" maxPzcor="5" patchSize="10.0"></view3d>
  </widgets>
</model>`
	m, err := ParseModel("m.nlogox3d", xmlModel)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Is3D || !m.View.Is3D || m.View.MinPzcor != -2 || m.View.MaxPzcor != 5 || m.View.Depth() != 8 {
		t.Fatalf("view %+v is3d=%v", m.View, m.Is3D)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	if got := mustEval(t, vm, `(list world-depth [zcor] of turtle 0)`); got != "[8 1]" {
		t.Fatalf("got %s", got)
	}

	// Classic .nlogo3d: min-pzcor and max-pzcor follow max-pycor.
	view := []string{"GRAPHICS-WINDOW", "0", "0", "100", "100", "-1", "-1", "12.0", "1", "10", "1", "1", "1", "0", "1", "1", "1",
		"-3", "3", "-2", "2", "-1", "4", "1", "1", "1", "ticks", "30.0"}
	classic := "to setup ca end\n" + sectionSep + "\n" + strings.Join(view, "\n") + "\n" + sectionSep + "\n" + sectionSep + "\n" + sectionSep + "\nNetLogo 3D 6.4.0\n" + sectionSep + "\n"
	m, err = ParseModel("m.nlogo3d", classic)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Is3D || m.View.MinPzcor != -1 || m.View.MaxPzcor != 4 || m.View.MaxPxcor != 3 {
		t.Fatalf("classic 3D view %+v", m.View)
	}
	if _, err := m.Compile(); err != nil {
		t.Fatal(err)
	}

	// The same classic view in a 2D file stays 2D.
	m, err = ParseModel("m.nlogo", strings.Replace(classic, "NetLogo 3D 6.4.0", "NetLogo 6.4.0", 1))
	if err != nil {
		t.Fatal(err)
	}
	if m.Is3D || m.View.Is3D || m.View.Depth() != 1 {
		t.Fatalf("2D model read as 3D: %+v", m.View)
	}
}

func TestVoxelsAndObserverPose(t *testing.T) {
	w := NewWorld3D(-1, 1, -1, 1, -1, 1)
	if len(w.Voxels()) != 0 {
		t.Fatalf("a black world has no voxels")
	}
	w.GetPatch3(1, 0, -1).PColor = 15
	w.GetPatch3(-1, 1, 1).PColor = 55
	vs := w.Voxels()
	// Patch order: highest layer first.
	if len(vs) != 2 || vs[0].Z != 1 || vs[0].X != -1 || vs[1].Z != -1 || vs[1].C != ColorRGBA(15) {
		t.Fatalf("voxels %+v", vs)
	}
	if NewWorld(-1, 1, -1, 1, true, true).Voxels() != nil {
		t.Fatalf("a 2D world has no voxels")
	}
	o := w.Observer()
	if o.Revision != 0 || o.Z != 4.5 || o.TZ != 0 {
		t.Fatalf("home pose %+v", o)
	}
	w.ObserverOrbit(0, -45)
	w.ObserverZoom(1)
	if o2 := w.Observer(); o2.Revision != 2 {
		t.Fatalf("revision %d, want 2", o2.Revision)
	}
	w.Resize3(-2, 2, -2, 2, -2, 2)
	if o3 := w.Observer(); o3.Revision != 2 || o3.Z != 7.5 {
		t.Fatalf("resize homes without a camera command: %+v", o3)
	}
}

func TestTopColorProjection(t *testing.T) {
	w := NewWorld3D(-1, 1, -1, 1, -1, 1)
	w.GetPatch3(0, 0, -1).PColor = 15
	w.GetPatch3(0, 0, 0).PColor = 55
	if c, ok := w.TopColor(0, 0); !ok || c != ColorRGBA(55) {
		t.Fatalf("top color %v %v", c, ok)
	}
	if _, ok := w.TopColor(1, 1); ok {
		t.Fatalf("an all-black column has no top color")
	}
}

func TestLayout3D(t *testing.T) {
	vm := newTestVM3D(t, "", -10, 10, 1)
	mustEval(t, vm, `
crt 2 [
  ifelse who = 0 [ setxyz 0 0 0 ] [ setxyz 0 0 4 ]
]
ask turtle 0 [ create-link-with turtle 1 ]
layout-spring turtles links 1 1 0
`)
	z0 := vm.World.Turtle(0).Z
	z1 := vm.World.Turtle(1).Z
	if z1-z0 >= 4-1e-9 {
		t.Fatalf("layout-spring left z at %v %v", z0, z1)
	}
	mustEval(t, vm, `crt 6 layout-sphere turtles 5`)
	var zs []float64
	for _, a := range vm.World.Turtles().Alive() {
		tu := a.(*Turtle)
		d := vm.World.Distance3(0, 0, 0, tu.X, tu.Y, tu.Z)
		if math.Abs(d-5) > 1e-6 {
			t.Fatalf("turtle %d is %v from the origin, want 5", tu.Who, d)
		}
		zs = append(zs, tu.Z)
	}
	span := 0.0
	for _, z := range zs {
		if math.Abs(z) > span {
			span = math.Abs(z)
		}
	}
	if span < 4 {
		t.Fatalf("layout-sphere z span %v, want the poles", span)
	}

	flat := newTestVM(t, "", -2, 2, true, 1)
	if _, err := flat.ExecuteLine("crt 1 layout-sphere turtles 1"); err == nil || !strings.Contains(err.Error(), "LAYOUT-SPHERE") {
		t.Fatalf("2D layout-sphere: %v", err)
	}
}
