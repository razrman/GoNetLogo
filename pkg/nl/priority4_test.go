package nl

import (
	"testing"
)

func TestPriority4RuntimeFixes(t *testing.T) {
	code := `
globals [ g-num g-turtle ]
turtles-own [ partner who-var ]

to test-truncation
  let r 0
  repeat 3.8 [ set r r + 1 ]
  set g-num r
end

to test-patch-round
  let p patch 2.7 0.2
  set g-num [pxcor] of p
end

to test-remove-string
  let s remove "a" "banana"
  if s != "bnn" [ error (word "expected bnn got " s) ]
end

to test-turtle-set-flatten
  crt 3
  let ts (turtle-set (turtle 0) (list (turtle 1) (turtle 2) nobody))
  set g-num count ts
end

to test-random-float-zero
  set g-num random-float 0
end

to test-create-link-duplicate
  crt 2
  ask turtle 0 [ create-link-with turtle 1 ]
  ask turtle 0 [ create-link-with turtle 1 ]
end

to test-sort-by-agentset
  crt 3 [ set who-var who ]
  let sorted sort-by [ [a b] -> [who] of a > [who] of b ] turtles
  set g-num length sorted
end

to test-dead-turtle-variable-is-nobody
  crt 2
  ask turtle 0 [
    set partner turtle 1
    ask turtle 1 [ die ]
    if partner != nobody [ error "expected partner to read as nobody" ]
  ]
end

to test-sort-on-turtles
  crt 3 [ set ycor who * 2 ]
  let sorted sort-on [ycor] turtles
  set g-num length sorted
end
`

	prog, err := Parse(code)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	tests := []struct {
		proc string
		want float64
	}{
		{"test-truncation", 3},
		{"test-patch-round", 3},
		{"test-turtle-set-flatten", 3},
		{"test-random-float-zero", 0},
		{"test-sort-by-agentset", 3},
		{"test-sort-on-turtles", 3},
	}

	for _, tt := range tests {
		t.Run(tt.proc, func(t *testing.T) {
			vm := NewVM(prog, nil)
			if _, err := vm.Call(tt.proc); err != nil {
				t.Fatalf("%s call failed: %v", tt.proc, err)
			}
			v, ok := vm.World.Global("g-num")
			if !ok {
				t.Fatalf("global g-num not found")
			}
			got, err := v.Num()
			if err != nil || got != tt.want {
				t.Fatalf("%s got %v, want %v", tt.proc, got, tt.want)
			}
		})
	}

	t.Run("test-remove-string", func(t *testing.T) {
		vm := NewVM(prog, nil)
		if _, err := vm.Call("test-remove-string"); err != nil {
			t.Fatalf("call failed: %v", err)
		}
	})
	t.Run("test-create-link-duplicate", func(t *testing.T) {
		vm := NewVM(prog, nil)
		if _, err := vm.Call("test-create-link-duplicate"); err != nil {
			t.Fatalf("call failed: %v", err)
		}
	})
	t.Run("test-dead-turtle-variable-is-nobody", func(t *testing.T) {
		vm := NewVM(prog, nil)
		if _, err := vm.Call("test-dead-turtle-variable-is-nobody"); err != nil {
			t.Fatalf("call failed: %v", err)
		}
	})
}
