package nl

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// Procedures exercise fd, bk, jump, setxy, move-to, and the patch
// reporters. Both the closure compiler and the tree walker run them.
const edgeSrc = `
globals [ flag num ]
to reset
  clear-all
  crt 1 [ setxy 0 0 set heading 90 ]
end
to aim [ h ]
  ask turtles [ set heading h ]
end
to go-fd [ d ]
  ask turtles [ fd d ]
end
to go-forward [ d ]
  ask turtles [ forward d ]
end
to go-bk [ d ]
  ask turtles [ bk d ]
end
to go-back [ d ]
  ask turtles [ back d ]
end
to go-jump [ d ]
  ask turtles [ jump d ]
end
to go-setxy [ x y ]
  ask turtles [ setxy x y ]
end
to go-xcor [ x ]
  ask turtles [ set xcor x ]
end
to go-move-patch [ x y ]
  ask turtles [ move-to patch x y ]
end
to go-move-turtle
  crt 1 [ setxy -1 1 set heading 0 ]
  ask min-one-of turtles [ who ] [ move-to max-one-of turtles [ who ] ]
end
to bad-nobody
  ask turtles [ move-to nobody ]
end
to bad-link
  crt 1 [ setxy 1 0 ]
  ask min-one-of turtles [ who ] [
    create-link-with max-one-of turtles [ who ]
    move-to one-of my-links
  ]
end
to place-pair
  clear-all
  crt 1 [ setxy -2 0 ]
  crt 1 [ setxy 2 0 ]
end
to place-vert
  clear-all
  crt 1 [ setxy 0 -2 ]
  crt 1 [ setxy 0 2 ]
end
to place-half
  clear-all
  crt 1 [ setxy -2.5 0 ]
  crt 1 [ setxy 0 0 ]
end
to probe-can [ d ]
  ask turtles [ set flag can-move? d ]
end
to probe-ahead [ d ]
  ask turtles [ set flag patch-ahead d = nobody ]
end
to probe-ahead-px [ d ]
  ask turtles [ set num [ pxcor ] of patch-ahead d ]
end
to probe-at [ dx dy ]
  ask turtles [ set flag patch-at dx dy = nobody ]
end
to probe-at-px [ dx dy ]
  ask turtles [ set num [ pxcor ] of patch-at dx dy ]
end
to probe-at-py [ dx dy ]
  ask turtles [ set num [ pycor ] of patch-at dx dy ]
end
to probe-right [ ang dist ]
  ask turtles [ set flag patch-right-and-ahead ang dist = nobody ]
end
to probe-right-px [ ang dist ]
  ask turtles [ set num [ pxcor ] of patch-right-and-ahead ang dist ]
end
to probe-dist
  ask min-one-of turtles [ who ] [
    set num distance max-one-of turtles [ who ]
  ]
end
to probe-towards
  ask min-one-of turtles [ who ] [
    set num towards max-one-of turtles [ who ]
  ]
end
to probe-radius
  ask min-one-of turtles [ who ] [
    set num count turtles in-radius 1.5
  ]
end
to probe-far
  ask turtles [
    set num count patches in-radius 1.5 with [ pxcor = -2 and pycor = 0 ]
  ]
end
to probe-far-y
  ask max-one-of turtles [ ycor ] [
    set num count patches in-radius 1.5 with [ pxcor = 0 and pycor = -2 ]
  ]
end
to probe-neighbors
  ask patch 2 2 [ set num count neighbors ]
end
to probe-neighbors4
  ask patch 2 2 [ set num count neighbors4 ]
end
to probe-middle
  ask patch 0 0 [ set num count neighbors ]
end
to bad-towards
  ask turtles [ set num towards self ]
end
`

func TestNonWrappingEdges(t *testing.T) {
	topos := []struct {
		name         string
		wrapX, wrapY bool
	}{
		{"torus", true, true},
		{"vert-cylinder", true, false},
		{"horiz-cylinder", false, true},
		{"box", false, false},
	}
	for _, topo := range topos {
		for _, walk := range []bool{false, true} {
			mode := "closure compiler"
			if walk {
				mode = "tree walker"
			}
			t.Run(topo.name+"/"+mode, func(t *testing.T) {
				runEdgeCase(t, topo.wrapX, topo.wrapY, walk)
			})
		}
	}
}

func newEdgeVM(t *testing.T, wrapX, wrapY, walk bool) *VM {
	t.Helper()
	w := NewWorld(-2, 2, -2, 2, wrapX, wrapY)
	w.Seed(1)
	vm := NewVM(mustParse(t, edgeSrc), w)
	if walk {
		forceTreeWalker(vm.Prog)
		if vm.Prog.Proc("go-fd").cmdFn != nil {
			t.Fatal("tree walker test compiled go-fd")
		}
	} else {
		vm.ensureCompiled()
		if vm.Prog.Proc("go-fd").cmdFn == nil {
			t.Fatal("closure compiler did not compile go-fd")
		}
	}
	return vm
}

func callOK(t *testing.T, vm *VM, name string, args ...Value) {
	t.Helper()
	if _, err := vm.Call(name, args...); err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
}

func oldestTurtle(t *testing.T, vm *VM) *Turtle {
	t.Helper()
	var best *Turtle
	for _, a := range vm.World.Turtles().Alive() {
		tu := a.(*Turtle)
		if best == nil || tu.Who < best.Who {
			best = tu
		}
	}
	if best == nil {
		t.Fatal("no turtles")
	}
	return best
}

func expectXY(t *testing.T, vm *VM, x, y float64) {
	t.Helper()
	tu := oldestTurtle(t, vm)
	if math.Abs(tu.X-x) > 1e-9 || math.Abs(tu.Y-y) > 1e-9 {
		t.Fatalf("turtle %d at %v %v, want %v %v", tu.Who, tu.X, tu.Y, x, y)
	}
}

func boolGlobal(t *testing.T, vm *VM, name string) bool {
	t.Helper()
	g, ok := vm.World.Global(name)
	if !ok || g.K != KindBool {
		t.Fatalf("%s = %#v", name, g)
	}
	return g.B
}

func numGlobal(t *testing.T, vm *VM, name string) float64 {
	t.Helper()
	g, ok := vm.World.Global(name)
	if !ok || g.K != KindNum {
		t.Fatalf("%s = %#v", name, g)
	}
	return g.N
}

func wantErr(t *testing.T, vm *VM, name, fragment string, args ...Value) {
	t.Helper()
	_, err := vm.Call(name, args...)
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("%s: got %v, want %q", name, err, fragment)
	}
}

func runEdgeCase(t *testing.T, wrapX, wrapY, walk bool) {
	vm := newEdgeVM(t, wrapX, wrapY, walk)

	// fd 3 east. Unit steps stop at x=2. Wrapping x lands on -2.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-fd", Num(3))
	if wrapX {
		expectXY(t, vm, -2, 0)
	} else {
		expectXY(t, vm, 2, 0)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "go-forward", Num(3))
	if wrapX {
		expectXY(t, vm, -2, 0)
	} else {
		expectXY(t, vm, 2, 0)
	}

	// The fractional step is attempted. 2.25 is still inside. 2.6 is not,
	// so fd stops on 2 unless x wraps.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-fd", Num(2.25))
	expectXY(t, vm, 2.25, 0)
	callOK(t, vm, "reset")
	callOK(t, vm, "go-fd", Num(2.6))
	if wrapX {
		expectXY(t, vm, -2.4, 0)
	} else {
		expectXY(t, vm, 2, 0)
	}

	// A turtle 0.5 from the edge cannot take a unit step, even though a
	// shorter jump would fit. Wrapping accepts the step.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(1.5), Num(0))
	callOK(t, vm, "aim", Num(90))
	callOK(t, vm, "go-fd", Num(1))
	if wrapX {
		expectXY(t, vm, -2.5, 0)
	} else {
		expectXY(t, vm, 1.5, 0)
	}

	// jump is one step. It does not walk up to the edge, and it does not error.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-jump", Num(3))
	if wrapX {
		expectXY(t, vm, -2, 0)
	} else {
		expectXY(t, vm, 0, 0)
	}

	// North uses the y axis the same way.
	callOK(t, vm, "reset")
	callOK(t, vm, "aim", Num(0))
	callOK(t, vm, "go-fd", Num(3))
	if wrapY {
		expectXY(t, vm, 0, -2)
	} else {
		expectXY(t, vm, 0, 2)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "aim", Num(0))
	callOK(t, vm, "go-jump", Num(3))
	if wrapY {
		expectXY(t, vm, 0, -2)
	} else {
		expectXY(t, vm, 0, 0)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "aim", Num(0))
	callOK(t, vm, "go-bk", Num(3))
	if wrapY {
		expectXY(t, vm, 0, 2)
	} else {
		expectXY(t, vm, 0, -2)
	}

	// bk and a negative fd walk west.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-bk", Num(3))
	if wrapX {
		expectXY(t, vm, 2, 0)
	} else {
		expectXY(t, vm, -2, 0)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "go-back", Num(3))
	if wrapX {
		expectXY(t, vm, 2, 0)
	} else {
		expectXY(t, vm, -2, 0)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "go-fd", Num(-3))
	if wrapX {
		expectXY(t, vm, 2, 0)
	} else {
		expectXY(t, vm, -2, 0)
	}

	// Already on the west edge, bk stops unless x wraps.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(-2), Num(0))
	callOK(t, vm, "aim", Num(90))
	callOK(t, vm, "go-bk", Num(4))
	if wrapX {
		// The first step wraps from -3 to 2, then three more steps end at -1.
		expectXY(t, vm, -1, 0)
	} else {
		expectXY(t, vm, -2, 0)
	}

	// A diagonal step off a blocked axis does not slide along the other axis.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(2), Num(2))
	callOK(t, vm, "aim", Num(45))
	callOK(t, vm, "go-fd", Num(1))
	tu := oldestTurtle(t, vm)
	if wrapX && wrapY {
		if math.Abs(tu.X-2) < 1e-9 && math.Abs(tu.Y-2) < 1e-9 {
			t.Fatal("diagonal fd did not wrap")
		}
		if tu.X >= 2.5 || tu.X < -2.5 || tu.Y >= 2.5 || tu.Y < -2.5 {
			t.Fatalf("diagonal fd left the torus at %v %v", tu.X, tu.Y)
		}
	} else {
		expectXY(t, vm, 2, 2)
	}

	// setxy and set xcor error when the point is outside a non-wrapping axis.
	callOK(t, vm, "reset")
	if wrapX {
		callOK(t, vm, "go-setxy", Num(3), Num(0))
		expectXY(t, vm, -2, 0)
	} else {
		wantErr(t, vm, "go-setxy", "outside", Num(3), Num(0))
		expectXY(t, vm, 0, 0)
	}
	callOK(t, vm, "reset")
	if wrapY {
		callOK(t, vm, "go-setxy", Num(0), Num(3))
		expectXY(t, vm, 0, -2)
	} else {
		wantErr(t, vm, "go-setxy", "outside", Num(0), Num(3))
		expectXY(t, vm, 0, 0)
	}
	callOK(t, vm, "reset")
	if wrapX && wrapY {
		callOK(t, vm, "go-setxy", Num(3), Num(3))
		expectXY(t, vm, -2, -2)
	} else {
		wantErr(t, vm, "go-setxy", "outside", Num(3), Num(3))
		expectXY(t, vm, 0, 0)
	}
	callOK(t, vm, "reset")
	if wrapX {
		callOK(t, vm, "go-setxy", Num(2.5), Num(0))
		expectXY(t, vm, -2.5, 0)
	} else {
		wantErr(t, vm, "go-setxy", "outside", Num(2.5), Num(0))
		expectXY(t, vm, 0, 0)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(-2.5), Num(0))
	expectXY(t, vm, -2.5, 0)
	callOK(t, vm, "reset")
	if wrapX {
		callOK(t, vm, "go-xcor", Num(9))
		expectXY(t, vm, -1, 0)
	} else {
		wantErr(t, vm, "go-xcor", "outside", Num(9))
		expectXY(t, vm, 0, 0)
	}

	callOK(t, vm, "reset")
	callOK(t, vm, "go-move-patch", Num(2), Num(-2))
	expectXY(t, vm, 2, -2)
	callOK(t, vm, "reset")
	callOK(t, vm, "go-move-turtle")
	expectXY(t, vm, -1, 1)
	callOK(t, vm, "reset")
	wantErr(t, vm, "bad-nobody", "dead")
	expectXY(t, vm, 0, 0)
	callOK(t, vm, "reset")
	wantErr(t, vm, "bad-link", "link")
	expectXY(t, vm, 0, 0)

	// can-move? is the full jump, same as patch-ahead.
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(2), Num(0))
	callOK(t, vm, "aim", Num(90))
	callOK(t, vm, "probe-can", Num(1))
	if boolGlobal(t, vm, "flag") != wrapX {
		t.Fatalf("can-move? 1 = %v, wrapX %v", boolGlobal(t, vm, "flag"), wrapX)
	}
	callOK(t, vm, "probe-can", Num(0.4))
	if !boolGlobal(t, vm, "flag") {
		t.Fatal("can-move? 0.4")
	}
	callOK(t, vm, "probe-can", Num(0.5))
	if boolGlobal(t, vm, "flag") != wrapX {
		t.Fatalf("can-move? 0.5 = %v", boolGlobal(t, vm, "flag"))
	}
	callOK(t, vm, "probe-can", Num(0))
	if !boolGlobal(t, vm, "flag") {
		t.Fatal("can-move? 0")
	}
	callOK(t, vm, "probe-ahead", Num(1))
	if boolGlobal(t, vm, "flag") == wrapX {
		t.Fatalf("patch-ahead nobody = %v, wrapX %v", boolGlobal(t, vm, "flag"), wrapX)
	}
	if wrapX {
		callOK(t, vm, "probe-ahead-px", Num(1))
		if numGlobal(t, vm, "num") != -2 {
			t.Fatalf("patch-ahead px %v", numGlobal(t, vm, "num"))
		}
	}
	callOK(t, vm, "probe-at", Num(1), Num(0))
	if boolGlobal(t, vm, "flag") == wrapX {
		t.Fatalf("patch-at nobody = %v", boolGlobal(t, vm, "flag"))
	}
	if wrapX {
		callOK(t, vm, "probe-at-px", Num(1), Num(0))
		if numGlobal(t, vm, "num") != -2 {
			t.Fatalf("patch-at px %v", numGlobal(t, vm, "num"))
		}
	}
	callOK(t, vm, "aim", Num(0))
	callOK(t, vm, "probe-right", Num(90), Num(1))
	if boolGlobal(t, vm, "flag") == wrapX {
		t.Fatalf("patch-right-and-ahead nobody = %v", boolGlobal(t, vm, "flag"))
	}
	if wrapX {
		callOK(t, vm, "probe-right-px", Num(90), Num(1))
		if numGlobal(t, vm, "num") != -2 {
			t.Fatalf("patch-right px %v", numGlobal(t, vm, "num"))
		}
	}

	callOK(t, vm, "reset")
	callOK(t, vm, "probe-at", Num(6), Num(0))
	if wrapX {
		if boolGlobal(t, vm, "flag") {
			t.Fatal("patch-at 6 0 was nobody on a wrapping x axis")
		}
		callOK(t, vm, "probe-at-px", Num(6), Num(0))
		if numGlobal(t, vm, "num") != 1 {
			t.Fatalf("patch-at 6 px %v", numGlobal(t, vm, "num"))
		}
	} else if !boolGlobal(t, vm, "flag") {
		t.Fatal("patch-at 6 0 wrapped when x does not")
	}
	callOK(t, vm, "go-setxy", Num(0), Num(2))
	callOK(t, vm, "probe-at", Num(0), Num(1))
	if boolGlobal(t, vm, "flag") == wrapY {
		t.Fatalf("patch-at 0 1 nobody = %v, wrapY %v", boolGlobal(t, vm, "flag"), wrapY)
	}
	if wrapY {
		callOK(t, vm, "probe-at-py", Num(0), Num(1))
		if numGlobal(t, vm, "num") != -2 {
			t.Fatalf("patch-at py %v", numGlobal(t, vm, "num"))
		}
	}

	callOK(t, vm, "place-pair")
	callOK(t, vm, "probe-dist")
	wantDist := 4.0
	if wrapX {
		wantDist = 1
	}
	if math.Abs(numGlobal(t, vm, "num")-wantDist) > 1e-9 {
		t.Fatalf("distance %v, want %v", numGlobal(t, vm, "num"), wantDist)
	}
	callOK(t, vm, "probe-towards")
	wantHead := 90.0
	if wrapX {
		wantHead = 270
	}
	if math.Abs(numGlobal(t, vm, "num")-wantHead) > 1e-6 {
		t.Fatalf("towards %v, want %v", numGlobal(t, vm, "num"), wantHead)
	}
	callOK(t, vm, "probe-radius")
	wantN := 1.0
	if wrapX {
		wantN = 2
	}
	if numGlobal(t, vm, "num") != wantN {
		t.Fatalf("in-radius turtles %v, want %v", numGlobal(t, vm, "num"), wantN)
	}
	callOK(t, vm, "reset")
	callOK(t, vm, "go-setxy", Num(2), Num(0))
	callOK(t, vm, "probe-far")
	wantFar := 0.0
	if wrapX {
		wantFar = 1
	}
	if numGlobal(t, vm, "num") != wantFar {
		t.Fatalf("in-radius far patch %v, want %v", numGlobal(t, vm, "num"), wantFar)
	}

	callOK(t, vm, "place-vert")
	callOK(t, vm, "probe-dist")
	wantDist = 4
	if wrapY {
		wantDist = 1
	}
	if math.Abs(numGlobal(t, vm, "num")-wantDist) > 1e-9 {
		t.Fatalf("vertical distance %v, want %v", numGlobal(t, vm, "num"), wantDist)
	}
	callOK(t, vm, "probe-towards")
	wantHead = 0
	if wrapY {
		wantHead = 180
	}
	if math.Abs(numGlobal(t, vm, "num")-wantHead) > 1e-6 {
		t.Fatalf("vertical towards %v, want %v", numGlobal(t, vm, "num"), wantHead)
	}
	callOK(t, vm, "probe-far-y")
	wantFar = 0
	if wrapY {
		wantFar = 1
	}
	if numGlobal(t, vm, "num") != wantFar {
		t.Fatalf("vertical in-radius %v, want %v", numGlobal(t, vm, "num"), wantFar)
	}

	// Halfway around a wrapping x axis ties toward the negative heading.
	callOK(t, vm, "place-half")
	callOK(t, vm, "probe-dist")
	if math.Abs(numGlobal(t, vm, "num")-2.5) > 1e-9 {
		t.Fatalf("half distance %v", numGlobal(t, vm, "num"))
	}
	callOK(t, vm, "probe-towards")
	wantHead = 90
	if wrapX {
		wantHead = 270
	}
	if math.Abs(numGlobal(t, vm, "num")-wantHead) > 1e-6 {
		t.Fatalf("half towards %v, want %v", numGlobal(t, vm, "num"), wantHead)
	}

	callOK(t, vm, "probe-neighbors")
	if got, want := numGlobal(t, vm, "num"), float64(cornerCount(wrapX, wrapY, false)); got != want {
		t.Fatalf("neighbors corner %v, want %v", got, want)
	}
	callOK(t, vm, "probe-neighbors4")
	if got, want := numGlobal(t, vm, "num"), float64(cornerCount(wrapX, wrapY, true)); got != want {
		t.Fatalf("neighbors4 corner %v, want %v", got, want)
	}
	callOK(t, vm, "probe-middle")
	if numGlobal(t, vm, "num") != 8 {
		t.Fatalf("middle neighbors %v", numGlobal(t, vm, "num"))
	}

	callOK(t, vm, "reset")
	wantErr(t, vm, "bad-towards", "heading")
}

func cornerCount(wrapX, wrapY, four bool) int {
	if four {
		n := 2 // south and west are always inside at max,max
		if wrapX {
			n++ // east
		}
		if wrapY {
			n++ // north
		}
		return n
	}
	// south, west, southwest
	n := 3
	if wrapX {
		n += 2 // east, southeast
	}
	if wrapY {
		n += 2 // north, northwest
	}
	if wrapX && wrapY {
		n++ // northeast
	}
	return n
}

func TestNarrowWorldNeighbors(t *testing.T) {
	src := `
globals [ num ]
to count8
  ask patch 0 0 [ set num count neighbors ]
end
to count4
  ask patch 0 0 [ set num count neighbors4 ]
end
`
	for _, walk := range []bool{false, true} {
		mode := "closure compiler"
		if walk {
			mode = "tree walker"
		}
		// One column and one extra row on each side. Wrapping x folds
		// east and west onto this patch, so they are not neighbors.
		w := NewWorld(0, 0, -1, 1, true, true)
		vm := NewVM(mustParse(t, src), w)
		if walk {
			forceTreeWalker(vm.Prog)
		}
		callOK(t, vm, "count8")
		if numGlobal(t, vm, "num") != 2 {
			t.Fatalf("%s self-neighbors %v", mode, numGlobal(t, vm, "num"))
		}
		callOK(t, vm, "count4")
		if numGlobal(t, vm, "num") != 2 {
			t.Fatalf("%s self-neighbors4 %v", mode, numGlobal(t, vm, "num"))
		}

		// Two columns. East and west are the same patch and count once.
		w = NewWorld(0, 1, -1, 1, true, false)
		vm = NewVM(mustParse(t, src), w)
		if walk {
			forceTreeWalker(vm.Prog)
		}
		callOK(t, vm, "count8")
		if numGlobal(t, vm, "num") != 5 {
			t.Fatalf("%s duplicate neighbors %v", mode, numGlobal(t, vm, "num"))
		}
		callOK(t, vm, "count4")
		if numGlobal(t, vm, "num") != 3 {
			t.Fatalf("%s duplicate neighbors4 %v", mode, numGlobal(t, vm, "num"))
		}
	}
}

func TestGraphicsWindowWrapFlags(t *testing.T) {
	cases := []struct {
		x, y         string
		wrapX, wrapY bool
	}{
		{"1", "1", true, true},
		{"1", "0", true, false},
		{"0", "1", false, true},
		{"0", "0", false, false},
	}
	for _, c := range cases {
		src := fmt.Sprintf(`to setup
  clear-all
end
@#$#@#$#@
GRAPHICS-WINDOW
0
0
0
0
-1
-1
12.0
1
10
1
1
1
0
%s
%s
0
-4
4
-3
3
1
1
1
ticks
30.0
@#$#@#$#@
`, c.x, c.y)
		m, err := ParseModel("wrap.nlogo", src)
		if err != nil {
			t.Fatal(err)
		}
		if m.View.WrapX != c.wrapX || m.View.WrapY != c.wrapY || m.View.MinPxcor != -4 || m.View.MaxPycor != 3 || m.View.PatchSize != 12 {
			t.Fatalf("wrap %s %s view %+v", c.x, c.y, m.View)
		}
		vm, err := m.NewVM(1)
		if err != nil {
			t.Fatal(err)
		}
		if vm.World.WrapX != c.wrapX || vm.World.WrapY != c.wrapY || vm.World.MinPxcor != -4 || vm.World.MaxPxcor != 4 || vm.World.MinPycor != -3 || vm.World.MaxPycor != 3 {
			t.Fatalf("world wrap %v %v bounds %d %d %d %d", vm.World.WrapX, vm.World.WrapY, vm.World.MinPxcor, vm.World.MaxPxcor, vm.World.MinPycor, vm.World.MaxPycor)
		}
	}
}

func TestEdgeModel(t *testing.T) {
	m := loadModel(t, "edge.nlogo")
	if m.View.WrapX || m.View.WrapY || m.View.MinPxcor != -8 || m.View.MaxPxcor != 8 || m.View.MinPycor != -8 || m.View.MaxPycor != 8 {
		t.Fatalf("view %+v", m.View)
	}
	for _, walk := range []bool{false, true} {
		vm, err := m.NewVM(1)
		if err != nil {
			t.Fatal(err)
		}
		if walk {
			forceTreeWalker(vm.Prog)
		}
		mustCall(t, vm, "setup")
		if mustEval(t, vm, "count turtles") != "8" {
			t.Fatalf("turtles %s", mustEval(t, vm, "count turtles"))
		}
		for i := 0; i < 20; i++ {
			mustCall(t, vm, "go")
		}
		if mustEval(t, vm, "stopped") != "8" {
			t.Fatalf("walk=%v stopped %s", walk, mustEval(t, vm, "stopped"))
		}
		for _, a := range vm.World.Turtles().Alive() {
			tu := a.(*Turtle)
			if tu.X < -8.5 || tu.X >= 8.5 || tu.Y < -8.5 || tu.Y >= 8.5 {
				t.Fatalf("turtle %d left the world at %v %v heading %v", tu.Who, tu.X, tu.Y, tu.Heading)
			}
		}
		east := vm.World.Turtle(2)
		if east == nil || math.Abs(east.X-8) > 1e-9 || math.Abs(east.Heading-90) > 1e-9 {
			t.Fatalf("east turtle %+v", east)
		}
		north := vm.World.Turtle(0)
		if north == nil || math.Abs(north.Y-8) > 1e-9 {
			t.Fatalf("north turtle %+v", north)
		}
		if _, err := vm.ExecuteLine("ask turtles [ setxy 9 0 ]"); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Fatalf("setxy past the edge: %v", err)
		}
	}
}
