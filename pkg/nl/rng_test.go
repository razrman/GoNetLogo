package nl

import (
	"math"
	"strings"
	"testing"
)

// TestRNGDrawCounter checks that every underlying draw is counted and that
// random-seed and clear-all put the counter back to zero. The values after
// seed 1 are the sequence the unwrapped math/rand source produced.
func TestRNGDrawCounter(t *testing.T) {
	m, err := ParseModel("rng.nlogo", "to setup\n  clear-all\nend\n@#$#@#$#@\n")
	if err != nil {
		t.Fatal(err)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := vm.World.RNGDraws(); got != 0 {
		t.Fatalf("fresh world draws = %d", got)
	}

	vm.World.Seed(1)
	wantInt := []int{81, 87, 47, 59, 81}
	for i, want := range wantInt {
		got, err := vm.World.RandomInt(100)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("random %d = %d, want %d (sequence changed)", i, got, want)
		}
	}
	wantFloat := []float64{0.6868230729, 0.0656370192, 0.1565192547}
	for i, want := range wantFloat {
		got, err := vm.World.RandomFloat(1)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("random-float %d = %.10f, want %.10f", i, got, want)
		}
	}
	if got := vm.World.RNGDraws(); got == 0 {
		t.Fatal("random and random-float did not count")
	}

	before := vm.World.RNGDraws()
	if _, err := vm.ExecuteLine("print random 10"); err != nil {
		t.Fatal(err)
	}
	afterRandom := vm.World.RNGDraws()
	if afterRandom <= before {
		t.Fatalf("random draws %d -> %d", before, afterRandom)
	}
	if _, err := vm.ExecuteLine("print random-float 1"); err != nil {
		t.Fatal(err)
	}
	afterFloat := vm.World.RNGDraws()
	if afterFloat <= afterRandom {
		t.Fatalf("random-float draws %d -> %d", afterRandom, afterFloat)
	}
	if _, err := vm.ExecuteLine("print one-of [1 2 3]"); err != nil {
		t.Fatal(err)
	}
	afterOne := vm.World.RNGDraws()
	if afterOne <= afterFloat {
		t.Fatalf("one-of draws %d -> %d", afterFloat, afterOne)
	}

	if _, err := vm.ExecuteLine("crt 3"); err != nil {
		t.Fatal(err)
	}
	afterCreate := vm.World.RNGDraws()
	if _, err := vm.ExecuteLine("ask turtles []"); err != nil {
		t.Fatal(err)
	}
	afterAsk := vm.World.RNGDraws()
	if afterAsk <= afterCreate {
		t.Fatalf("ask shuffle draws %d -> %d", afterCreate, afterAsk)
	}
	shuffled := vm.World.Shuffle(vm.World.Turtles().Agents)
	if vm.World.RNGDraws() <= afterAsk || len(shuffled) != 3 {
		t.Fatalf("shuffle draws %d agents %d", vm.World.RNGDraws(), len(shuffled))
	}

	if _, err := vm.ExecuteLine("random-seed 1"); err != nil {
		t.Fatal(err)
	}
	if got := vm.World.RNGDraws(); got != 0 {
		t.Fatalf("random-seed left %d draws", got)
	}
	out, err := vm.ExecuteLine("random 100")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "81" {
		t.Fatalf("random 100 after random-seed 1 = %q", out)
	}
	if vm.World.RNGDraws() == 0 {
		t.Fatal("random after random-seed did not count")
	}

	if _, err := vm.ExecuteLine("clear-all"); err != nil {
		t.Fatal(err)
	}
	if got := vm.World.RNGDraws(); got != 0 {
		t.Fatalf("clear-all left %d draws", got)
	}
}
