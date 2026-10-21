package nl

import (
	"testing"
	"time"
)

func TestHaltStopsLongRepeat(t *testing.T) {
	const src = `
to go
  repeat 100000000 [ ]
end
to spin
  loop [ ]
end
`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, walk := range []bool{false, true} {
		vm := NewVM(prog, nil)
		if walk {
			forceTreeWalker(prog)
		}
		done := make(chan struct{})
		go func() {
			time.Sleep(30 * time.Millisecond)
			vm.RequestHalt()
			close(done)
		}()
		start := time.Now()
		stopped, err := vm.Call("go")
		elapsed := time.Since(start)
		<-done
		if err != nil {
			t.Fatalf("walk=%v: %v", walk, err)
		}
		if !stopped {
			t.Fatalf("walk=%v: halt did not stop the button", walk)
		}
		if elapsed > 500*time.Millisecond {
			t.Fatalf("walk=%v: halt took %s", walk, elapsed)
		}
	}
}

func TestRunStringCacheMatchesParse(t *testing.T) {
	const src = `
to move-north
  fd 1
end
to go
  create-turtles 1 [
    set heading 0
    run "move-north"
    run "fd 1"
  ]
end
`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(prog, nil)
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	turtles := vm.World.Turtles().Alive()
	if len(turtles) != 1 {
		t.Fatalf("turtles = %d", len(turtles))
	}
	turtle := turtles[0].(*Turtle)
	if turtle.Y != 2 {
		t.Fatalf("ycor = %v, want 2 (bare command plus cached fd)", turtle.Y)
	}
}
