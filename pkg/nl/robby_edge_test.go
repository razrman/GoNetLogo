package nl

import (
	"math"
	"testing"
)

// Robby's world is 0..9 with wrapping off. display-fitness does
// `fd chromosome-distance self myself` after placing a turtle on the
// midline, and create-next-generation does `fd random-float 3.0` inside
// hatch. Both distances can run past the edge. NetLogo's fd and bk stop
// on the last legal unit step and do not raise an error.
const robbyEdgeSrc = `
breed [ individuals individual ]
individuals-own [ allele-distribution scaled-fitness tag span ]
globals [ x-offset ]

to-report chromosome-distance [individual1 individual2]
  let max-dist 273 * sqrt 2
  let dist2 sum (map [ [a1 a2] -> (a1 - a2) ^ 2 ] [allele-distribution] of individual1 [allele-distribution] of individual2)
  let dist-candidate max-pxcor * sqrt dist2 / (max-dist / 10)
  report ifelse-value dist-candidate > max-pxcor [ max-pxcor ] [ dist-candidate ]
end

to place-fitness
  clear-all
  set x-offset 0
  create-individuals 1 [
    set allele-distribution [10 10 10 10 10 10 10]
    set scaled-fitness 0.5
    set tag 0
    setxy (scaled-fitness * max-pxcor + x-offset) (max-pycor / 2)
  ]
  create-individuals 1 [
    set allele-distribution [10 10 10 10 10 10 10]
    set scaled-fitness 0.25
    set tag 1
  ]
  create-individuals 1 [
    set allele-distribution [10 10 10 10 10 10 9]
    set scaled-fitness 0.75
    set tag 2
  ]
  create-individuals 1 [
    set allele-distribution [0 0 0 0 0 0 0]
    set scaled-fitness 1
    set tag 3
  ]
end

to display-fitness [h]
  let mid-y max-pycor / 2
  ask min-one-of individuals [ who ] [
    setxy (scaled-fitness * max-pxcor + x-offset) mid-y
    ask other individuals [
      setxy (scaled-fitness * max-pxcor + x-offset) mid-y
      set heading h
      set span chromosome-distance self myself
      fd chromosome-distance self myself
    ]
  ]
end

to display-fitness-either
  let mid-y max-pycor / 2
  ask min-one-of individuals [ who ] [
    ask other individuals [
      setxy (scaled-fitness * max-pxcor + x-offset) mid-y
      set heading one-of [0 180]
      fd chromosome-distance self myself
    ]
  ]
end

;; Large and fractional distances, including a reporter result, on a
;; turtle that is already against the edge.
to march [d]
  ask individuals with [ tag = 3 ] [
    setxy 9 9.2
    set heading 0
    fd d
  ]
end

to march-reporter
  ask individuals with [ tag = 3 ] [
    setxy 0 4.5
    set heading 90
    fd chromosome-distance self self
  ]
end

to retreat [d]
  ask individuals with [ tag = 3 ] [
    setxy 0.2 4.5
    set heading 90
    bk d
  ]
end

to place-corners
  clear-all
  create-individuals 1 [ setxy 0 0 set tag 1 ]
  create-individuals 1 [ setxy 9 0 set tag 2 ]
  create-individuals 1 [ setxy 0 9 set tag 3 ]
  create-individuals 1 [ setxy 9 9 set tag 4 ]
  create-individuals 1 [ setxy 9.4 9.4 set tag 5 ]
  create-individuals 1 [ setxy -0.4 -0.4 set tag 6 ]
end

;; The create-next-generation hatch body. Parents sit on the corners,
;; so a random heading and a distance in [0, 3) often points off the world.
to create-next-generation
  ask individuals [
    hatch 1 [
      rt random 360
      fd random-float 3.0
      set tag 0
    ]
  ]
end

to hatch-fixed
  ask individuals with [ tag = 4 ] [
    hatch 1 [ set tag 10 set heading 0 fd 50 ]
    hatch 1 [ set tag 11 set heading 90 fd 50 ]
    hatch 1 [ set tag 12 set heading 180 fd 100.5 ]
    hatch 1 [ set tag 13 set heading 270 fd 100.5 ]
    hatch 1 [ set tag 14 set heading 0 fd 0.75 ]
    hatch 1 [ set tag 15 set heading 45 fd 0.3 ]
    hatch 1 [ set tag 16 set heading 90 bk 40.5 ]
    hatch 1 [ set tag 17 set heading 0 bk 0.4 ]
  ]
  ask individuals with [ tag = 5 ] [
    hatch 1 [ set tag 18 set heading 90 fd 0.75 ]
    hatch 1 [ set tag 19 set heading 0 fd 0.2 ]
    hatch 1 [ set tag 20 rt random 360 fd random-float 3.0 ]
  ]
end
`

func TestRobbyFdStopsAtEdge(t *testing.T) {
	for _, walk := range []bool{false, true} {
		mode := "closure compiler"
		if walk {
			mode = "tree walker"
		}
		t.Run(mode, func(t *testing.T) {
			runRobbyEdge(t, walk)
		})
	}
}

func newRobbyVM(t *testing.T, walk bool) *VM {
	t.Helper()
	w := NewWorld(0, 9, 0, 9, false, false)
	w.Seed(1)
	vm := NewVM(mustParse(t, robbyEdgeSrc), w)
	if walk {
		forceTreeWalker(vm.Prog)
		if vm.Prog.Proc("display-fitness").cmdFn != nil || vm.Prog.Proc("create-next-generation").cmdFn != nil {
			t.Fatal("tree walker test compiled a procedure")
		}
	} else {
		vm.ensureCompiled()
		if vm.Prog.Proc("display-fitness").cmdFn == nil || vm.Prog.Proc("hatch-fixed").cmdFn == nil {
			t.Fatal("closure compiler did not compile fd procedures")
		}
	}
	return vm
}

func runRobbyEdge(t *testing.T, walk bool) {
	vm := newRobbyVM(t, walk)

	callOK(t, vm, "place-fitness")
	callOK(t, vm, "display-fitness", Num(0))
	byTag := turtlesByTag(t, vm)
	best := byTag[0]
	same := byTag[1]
	slight := byTag[2]
	far := byTag[3]
	if math.Abs(best.X-4.5) > 1e-9 || math.Abs(best.Y-4.5) > 1e-9 {
		t.Fatalf("best moved to %v %v", best.X, best.Y)
	}
	if span := turtleNum(t, same, "span"); span != 0 {
		t.Fatalf("identical chromosomes span %v", span)
	}
	if math.Abs(same.X-2.25) > 1e-9 || math.Abs(same.Y-4.5) > 1e-9 {
		t.Fatalf("zero fd moved turtle to %v %v", same.X, same.Y)
	}
	frac := turtleNum(t, slight, "span")
	if frac <= 0 || frac > 1 {
		t.Fatalf("fractional chromosome-distance %v", frac)
	}
	if math.Abs(slight.X-6.75) > 1e-9 || math.Abs(slight.Y-(4.5+frac)) > 1e-9 {
		t.Fatalf("fractional fd at %v %v, span %v", slight.X, slight.Y, frac)
	}
	// Seven alleles differing by 10 is past the midline. Heading 0 takes
	// four unit steps to y=8.5; the next step would land on 9.5.
	capped := turtleNum(t, far, "span")
	if capped < 5 {
		t.Fatalf("far chromosome-distance %v, want a distance that crosses the edge", capped)
	}
	if math.Abs(far.X-9) > 1e-9 || math.Abs(far.Y-8.5) > 1e-9 {
		t.Fatalf("north fd %v stopped at %v %v, want 9 8.5", capped, far.X, far.Y)
	}

	callOK(t, vm, "place-fitness")
	callOK(t, vm, "display-fitness", Num(180))
	far = turtlesByTag(t, vm)[3]
	// Five steps of 1 reach y=-0.5. The following step would leave.
	if math.Abs(far.X-9) > 1e-6 || math.Abs(far.Y-(-0.5)) > 1e-9 {
		t.Fatalf("south fd stopped at %v %v, want 9 -0.5", far.X, far.Y)
	}

	callOK(t, vm, "place-fitness")
	callOK(t, vm, "display-fitness-either")
	assertInside(t, vm, "display-fitness-either")

	// fd of a large number, a fraction that still fits, and a fraction
	// that would leave. A distance of 0 (self vs self) does not move.
	callOK(t, vm, "place-fitness")
	callOK(t, vm, "march", Num(50))
	far = turtlesByTag(t, vm)[3]
	if math.Abs(far.X-9) > 1e-9 || math.Abs(far.Y-9.2) > 1e-9 {
		t.Fatalf("fd 50 from the top stopped at %v %v", far.X, far.Y)
	}
	callOK(t, vm, "march", Num(0.2))
	far = turtlesByTag(t, vm)[3]
	if math.Abs(far.X-9) > 1e-9 || math.Abs(far.Y-9.4) > 1e-9 {
		t.Fatalf("fd 0.2 stopped at %v %v, want 9 9.4", far.X, far.Y)
	}
	callOK(t, vm, "march", Num(0.75))
	far = turtlesByTag(t, vm)[3]
	// 9.2 + 0.75 = 9.95, which is outside, so the fractional step is refused.
	if math.Abs(far.X-9) > 1e-9 || math.Abs(far.Y-9.2) > 1e-9 {
		t.Fatalf("fd 0.75 past the edge moved to %v %v, want 9 9.2", far.X, far.Y)
	}
	callOK(t, vm, "march-reporter")
	far = turtlesByTag(t, vm)[3]
	// Self-distance is 0, so the turtle stays on the midline.
	if math.Abs(far.X-0) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("fd chromosome-distance self self at %v %v", far.X, far.Y)
	}
	// A large eastward fd from x=0 stops on the last patch center, x=9.
	callOK(t, vm, "place-fitness")
	if _, err := vm.ExecuteLine("ask individuals with [ tag = 3 ] [ setxy 0 4.5 set heading 90 fd 100.25 ]"); err != nil {
		t.Fatal(err)
	}
	far = turtlesByTag(t, vm)[3]
	if math.Abs(far.X-9) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("fd 100.25 east stopped at %v %v, want 9 4.5", far.X, far.Y)
	}
	callOK(t, vm, "retreat", Num(20))
	far = turtlesByTag(t, vm)[3]
	// First step from 0.2 reaches -0.8, which is outside, so bk stays put.
	if math.Abs(far.X-0.2) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("bk 20 stopped at %v %v, want 0.2 4.5", far.X, far.Y)
	}
	callOK(t, vm, "retreat", Num(0.6))
	far = turtlesByTag(t, vm)[3]
	// 0.2 - 0.6 = -0.4, which is still inside the open edge at -0.5.
	if math.Abs(far.X-(-0.4)) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("bk 0.6 stopped at %v %v, want -0.4 4.5", far.X, far.Y)
	}
	callOK(t, vm, "retreat", Num(0.8))
	far = turtlesByTag(t, vm)[3]
	// 0.2 - 0.8 = -0.6, which is outside, so the fractional step is refused.
	if math.Abs(far.X-0.2) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("bk 0.8 stopped at %v %v, want 0.2 4.5", far.X, far.Y)
	}
	if _, err := vm.ExecuteLine("ask individuals with [ tag = 3 ] [ setxy 1 4.5 set heading 90 bk 4.5 ]"); err != nil {
		t.Fatal(err)
	}
	far = turtlesByTag(t, vm)[3]
	if math.Abs(far.X-0) > 1e-9 || math.Abs(far.Y-4.5) > 1e-9 {
		t.Fatalf("bk 4.5 stopped at %v %v, want 0 4.5", far.X, far.Y)
	}

	// Hatch, including the random-float body from create-next-generation.
	vm = newRobbyVM(t, walk)
	callOK(t, vm, "place-corners")
	before := len(vm.World.Turtles().Alive())
	for i := 0; i < 8; i++ {
		callOK(t, vm, "create-next-generation")
		assertInside(t, vm, "create-next-generation")
	}
	if got := len(vm.World.Turtles().Alive()); got <= before {
		t.Fatalf("hatch did not create turtles: %d", got)
	}

	callOK(t, vm, "hatch-fixed")
	assertInside(t, vm, "hatch-fixed")
	hatched := turtlesByTag(t, vm)
	// Parent tag 4 is at (9, 9). North and east are already on the edge.
	expectTagXY(t, hatched, 10, 9, 9)
	expectTagXY(t, hatched, 11, 9, 9)
	// South and west stop on the last coordinate still inside, 0.
	expectTagXY(t, hatched, 12, 9, 0)
	expectTagXY(t, hatched, 13, 0, 9)
	// 9.75 is outside, so the fractional step is refused.
	expectTagXY(t, hatched, 14, 9, 9)
	dx, dy := headingDelta(45, 0.3)
	expectTagXY(t, hatched, 15, 9+dx, 9+dy)
	// bk heading 90 walks west and stops at x=0.
	expectTagXY(t, hatched, 16, 0, 9)
	// bk 0.4 heading 0 steps south to 8.6, which is still inside.
	expectTagXY(t, hatched, 17, 9, 8.6)
	// Parent tag 5 is at (9.4, 9.4). Both of these fractions would leave.
	expectTagXY(t, hatched, 18, 9.4, 9.4)
	expectTagXY(t, hatched, 19, 9.4, 9.4)
	if _, ok := hatched[20]; !ok {
		t.Fatal("missing random hatch child")
	}
}

func turtlesByTag(t *testing.T, vm *VM) map[float64]*Turtle {
	t.Helper()
	out := map[float64]*Turtle{}
	for _, a := range vm.World.Turtles().Alive() {
		tu := a.(*Turtle)
		tag, ok := tu.Vars["tag"]
		if !ok || tag.K != KindNum {
			t.Fatalf("turtle %d has no tag", tu.Who)
		}
		out[tag.N] = tu
	}
	return out
}

func turtleNum(t *testing.T, tu *Turtle, name string) float64 {
	t.Helper()
	if tu == nil {
		t.Fatalf("no turtle for %s", name)
	}
	v, ok := tu.Vars[name]
	if !ok || v.K != KindNum {
		t.Fatalf("turtle %d %s = %#v", tu.Who, name, v)
	}
	return v.N
}

func expectTagXY(t *testing.T, by map[float64]*Turtle, tag, x, y float64) {
	t.Helper()
	tu := by[tag]
	if tu == nil {
		t.Fatalf("no turtle tagged %v", tag)
	}
	if math.Abs(tu.X-x) > 1e-9 || math.Abs(tu.Y-y) > 1e-9 {
		t.Fatalf("tag %v at %v %v, want %v %v", tag, tu.X, tu.Y, x, y)
	}
}

func assertInside(t *testing.T, vm *VM, what string) {
	t.Helper()
	for _, a := range vm.World.Turtles().Alive() {
		tu := a.(*Turtle)
		if tu.X < -0.5 || tu.X >= 9.5 || tu.Y < -0.5 || tu.Y >= 9.5 {
			t.Fatalf("%s: turtle %d left the world at %v %v heading %v", what, tu.Who, tu.X, tu.Y, tu.Heading)
		}
	}
}
