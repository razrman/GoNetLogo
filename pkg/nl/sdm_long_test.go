package nl

import (
	"math"
	"testing"
)

// TestWolfSheepLongEuler checks the System Dynamics stock update against
// a hand Euler step using the same products the compiler emits. The
// library dt stays 0.001. A full run to t = 120 is 120,000 steps.
func TestWolfSheepLongEuler(t *testing.T) {
	m := loadModel(t, "library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	dt := numGlobal(t, vm, "dt")
	if dt != 0.001 {
		t.Fatalf("dt %v, the library step must stay 0.001", dt)
	}
	br := numGlobal(t, vm, "sheep-birth-rate")
	pr := numGlobal(t, vm, "predation-rate")
	eff := numGlobal(t, vm, "predator-efficiency")
	wdr := numGlobal(t, vm, "wolf-death-rate")
	sheep := numGlobal(t, vm, "sheep")
	wolves := numGlobal(t, vm, "wolves")

	const steps = 120000
	signChanges := 0
	prevDelta := 0.0
	havePrev := false
	firstPeak := -1.0
	for i := 0; i < steps; i++ {
		births := (br * sheep) * dt
		deaths := ((sheep * pr) * wolves) * dt
		nextSheep := sheep + births - deaths
		if nextSheep < 0 {
			nextSheep = 0
		}
		wbirths := (((wolves * eff) * pr) * sheep) * dt
		wdeaths := (wolves * wdr) * dt
		nextWolves := wolves + wbirths - wdeaths
		if nextWolves < 0 {
			nextWolves = 0
		}
		delta := nextSheep - sheep
		if havePrev && ((prevDelta > 0 && delta < 0) || (prevDelta < 0 && delta > 0)) {
			signChanges++
			if firstPeak < 0 && prevDelta > 0 && delta < 0 {
				firstPeak = float64(i) * dt
			}
		}
		prevDelta = delta
		havePrev = true
		sheep, wolves = nextSheep, nextWolves

		mustCall(t, vm, "go")
		gotSheep := numGlobal(t, vm, "sheep")
		gotWolves := numGlobal(t, vm, "wolves")
		if math.Abs(gotSheep-sheep) > 1e-6 || math.Abs(gotWolves-wolves) > 1e-6 {
			t.Fatalf("step %d t %v sheep %v want %v wolves %v want %v", i, float64(i+1)*dt, gotSheep, sheep, gotWolves, wolves)
		}
	}
	if steps < 100000 {
		t.Fatalf("steps %d", steps)
	}
	ticks, err := vm.World.Ticks()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ticks-120) > 1e-9 {
		t.Fatalf("ticks %v, want 120", ticks)
	}
	if signChanges < 2 {
		t.Fatalf("delta-sheep changed sign %d times by t=120, want at least 2 (first peak %v)", signChanges, firstPeak)
	}
	if firstPeak < 70 || firstPeak > 95 {
		t.Fatalf("first sheep peak at t %v, want about 81.5", firstPeak)
	}
}
