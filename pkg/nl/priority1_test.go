package nl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEveryTiming(t *testing.T) {
	code := `
globals [ counter ]
to test-every
  every 0.05 [
    set counter counter + 1
  ]
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	vm := NewVM(prog, nil)
	mustEval(t, vm, "set counter 0")

	// First time should run immediately
	mustEval(t, vm, "test-every")
	if got := mustEval(t, vm, "counter"); got != "1" {
		t.Fatalf("first run counter = %v, want 1", got)
	}

	// Immediately calling it again should throttle (no increment)
	mustEval(t, vm, "test-every")
	if got := mustEval(t, vm, "counter"); got != "1" {
		t.Fatalf("immediate second run counter = %v, want 1", got)
	}

	// Sleep 60ms and call again
	time.Sleep(60 * time.Millisecond)
	mustEval(t, vm, "test-every")
	if got := mustEval(t, vm, "counter"); got != "2" {
		t.Fatalf("after delay counter = %v, want 2", got)
	}
}

func TestTieMovementAndTurn(t *testing.T) {
	code := `
to setup-tie
  clear-all
  create-turtles 1 [
    setxy 0 0
    set heading 0
  ]
  create-turtles 1 [
    setxy 0 2
    set heading 0
  ]
  ask turtle 0 [
    create-link-to turtle 1 [
      tie
    ]
  ]
end

to move-root
  ask turtle 0 [
    fd 3
    rt 90
  ]
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	w := NewWorld(-10, 10, -10, 10, false, false)
	vm := NewVM(prog, w)
	mustEval(t, vm, "setup-tie")

	// Turtle 0 at (0,0) heading 0; turtle 1 at (0,2) heading 0.
	// turtle 0 does `fd 3` (moves to 0,3), then `rt 90` (heading becomes 90).
	// With turtle 0 moving to (0,3), tied turtle 1 moves to (0,5).
	// Then turtle 0 turns rt 90 (facing east).
	// Turtle 1 rotates 90 degrees around turtle 0:
	// relative position was (0, 2) [north]; clockwise 90 degrees makes it (2, 0) [east] from (0,3), so (2, 3)!
	// and turtle 1's heading becomes 90 (fixed tie).
	mustEval(t, vm, "move-root")

	x0 := mustEval(t, vm, "[xcor] of turtle 0")
	y0 := mustEval(t, vm, "[ycor] of turtle 0")
	h0 := mustEval(t, vm, "[heading] of turtle 0")
	x1 := mustEval(t, vm, "[xcor] of turtle 1")
	y1 := mustEval(t, vm, "[ycor] of turtle 1")
	h1 := mustEval(t, vm, "[heading] of turtle 1")

	if x0 != "0" || y0 != "3" || h0 != "90" {
		t.Fatalf("turtle 0: got (%v, %v) h=%v, want (0, 3) h=90", x0, y0, h0)
	}
	if x1 != "2" || y1 != "3" || h1 != "90" {
		t.Fatalf("turtle 1: got (%v, %v) h=%v, want (2, 3) h=90", x1, y1, h1)
	}
}

func TestFileIORoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "netlogo_file_io_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "test.txt")

	code := `
globals [ res-str res-num at-end-before at-end-after exists-before exists-after ]

to write-test [ path ]
  if file-exists? path [ file-delete path ]
  set exists-before file-exists? path
  file-open path
  file-print "hello world"
  file-write 123
  file-close
  set exists-after file-exists? path
end

to read-test [ path ]
  file-open path
  set at-end-before file-at-end?
  set res-str file-read-line
  set res-num file-read
  set at-end-after file-at-end?
  file-close
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	vm := NewVM(prog, nil)
	vm.ModelDir = tempDir

	mustEval(t, vm, fmt.Sprintf(`write-test %q`, filePath))
	if got := mustEval(t, vm, "exists-before"); got != "false" {
		t.Fatalf("exists-before: %v, want false", got)
	}
	if got := mustEval(t, vm, "exists-after"); got != "true" {
		t.Fatalf("exists-after: %v, want true", got)
	}

	mustEval(t, vm, fmt.Sprintf(`read-test %q`, filePath))
	if got := mustEval(t, vm, "at-end-before"); got != "false" {
		t.Fatalf("at-end-before: %v, want false", got)
	}
	if got := mustEval(t, vm, "res-str"); got != "hello world" {
		t.Fatalf("res-str: %v, want 'hello world'", got)
	}
	if got := mustEval(t, vm, "res-num"); got != "123" {
		t.Fatalf("res-num: %v, want 123", got)
	}
	if got := mustEval(t, vm, "at-end-after"); got != "true" {
		t.Fatalf("at-end-after: %v, want true", got)
	}
}

func TestLayoutSpring(t *testing.T) {
	code := `
to test-layout
  clear-all
  create-turtles 3 [
    setxy 0 0
  ]
  ask turtle 0 [ create-link-with turtle 1 ]
  ask turtle 1 [ create-link-with turtle 2 ]
  layout-spring turtles links 0.5 2.0 1.0
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	vm := NewVM(prog, nil)
	mustEval(t, vm, "test-layout")

	// Verify that positions have moved due to repulsion/spring forces
	d := mustEval(t, vm, "[ distance turtle 1 ] of turtle 0")
	if d == "0" {
		t.Fatal("turtles did not separate during layout-spring")
	}
}
