package nl

import "testing"

// NetLogo 6: clear-turtles and clear-all reset the who counter so the
// next turtle is turtle 0. die does not, so who numbers are never reused
// while turtles merely die.
func TestWhoCounterReset(t *testing.T) {
	cases := []struct {
		name, run, want string
	}{
		{"clear-turtles resets who", `crt 5 clear-turtles crt 1`, "[0]"},
		{"ct alias resets who", `crt 3 ct crt 2`, "[0 1]"},
		{"clear-all resets who", `crt 5 clear-all crt 1`, "[0]"},
		{"ca alias resets who", `crt 4 ca crt 1`, "[0]"},
		{"die does not reset who", `crt 3 ask turtles [ die ] crt 1`, "[3]"},
		{"die of the newest does not reuse its who", `crt 3 ask turtle 2 [ die ] crt 1`, "[0 1 3]"},
		{"hatch after clear-turtles starts at 0", `crt 2 ct crt 1 ask turtle 0 [ hatch 1 ]`, "[0 1]"},
		{"sprout after clear-all starts at 0", `crt 2 ca ask one-of patches [ sprout 1 ]`, "[0]"},
		{"clear-patches keeps who", `crt 2 clear-patches crt 1`, "[0 1 2]"},
	}
	worlds := []struct {
		name  string
		newVM func(t *testing.T) *VM
	}{
		{"2D", func(t *testing.T) *VM { return newTestVM(t, "", -3, 3, true, 1) }},
		{"3D", func(t *testing.T) *VM { return newTestVM3D(t, "", -3, 3, 1) }},
	}
	for _, w := range worlds {
		for _, c := range cases {
			t.Run(w.name+"/"+c.name, func(t *testing.T) {
				vm := w.newVM(t)
				if _, err := vm.ExecuteLine(c.run); err != nil {
					t.Fatalf("%s: %v", c.run, err)
				}
				if got := mustEval(t, vm, `sort [who] of turtles`); got != c.want {
					t.Fatalf("after %s: who = %s, want %s", c.run, got, c.want)
				}
			})
		}
	}
}

// A setup that clears and then asks turtle 0 must work when run twice,
// which is what pressing setup again in the UI does.
func TestSetupTwiceFindsTurtleZero(t *testing.T) {
	src := `to setup
  clear-all
  crt 10
  ask turtle 0 [ set color red ]
end`
	vm := newTestVM(t, src, -3, 3, true, 1)
	mustCall(t, vm, "setup")
	mustCall(t, vm, "setup")
	if got := mustEval(t, vm, `[color] of turtle 0`); got != "15" {
		t.Fatalf("turtle 0 color %s, want 15", got)
	}
	if got := mustEval(t, vm, `max [who] of turtles`); got != "9" {
		t.Fatalf("max who %s, want 9", got)
	}

	// The same setup in a 3D world, as Flocking 3D does.
	vm3 := newTestVM3D(t, src, -3, 3, 1)
	mustCall(t, vm3, "setup")
	mustCall(t, vm3, "setup")
	if got := mustEval(t, vm3, `[color] of turtle 0`); got != "15" {
		t.Fatalf("3D turtle 0 color %s, want 15", got)
	}
}

// World-level checks for callers that clear without the interpreter.
func TestWorldClearResetsWho(t *testing.T) {
	for _, w := range []*World{NewWorld(-2, 2, -2, 2, true, true), NewWorld3D(-2, 2, -2, 2, -2, 2)} {
		w.CreateTurtle("", 0, 0, 0, 0)
		w.CreateTurtle("", 0, 0, 0, 0)
		w.ClearTurtles()
		if tt := w.CreateTurtle("", 0, 0, 0, 0); tt.Who != 0 {
			t.Fatalf("3d=%v after ClearTurtles: who %d, want 0", w.Is3D(), tt.Who)
		}
		w.CreateTurtle("", 0, 0, 0, 0)
		w.ClearAll()
		if tt := w.CreateTurtle("", 0, 0, 0, 0); tt.Who != 0 {
			t.Fatalf("3d=%v after ClearAll: who %d, want 0", w.Is3D(), tt.Who)
		}
		a := w.CreateTurtle("", 0, 0, 0, 0)
		w.Die(a)
		if tt := w.CreateTurtle("", 0, 0, 0, 0); tt.Who != a.Who+1 {
			t.Fatalf("3d=%v after Die: who %d, want %d", w.Is3D(), tt.Who, a.Who+1)
		}
	}
}
