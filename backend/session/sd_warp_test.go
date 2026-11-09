package session

import (
	"os"
	"testing"
	"time"

	"gonetlogo/pkg/nl"
)

func TestStepsForUnchangedBelowWarp(t *testing.T) {
	if stepsFor(40, 0) != 1 || stepsFor(40, 1) != 0 || stepsFor(40, 3) != 1 {
		t.Fatalf("speed 40: %d %d %d", stepsFor(40, 0), stepsFor(40, 1), stepsFor(40, 3))
	}
	if stepsFor(50, 1) != 1 || stepsFor(60, 1) != 2 || stepsFor(90, 1) != 5 {
		t.Fatalf("mid speeds %d %d %d", stepsFor(50, 1), stepsFor(60, 1), stepsFor(90, 1))
	}
	// The function still reports the old count. The slot runner does not
	// use it at speed 100.
	if stepsFor(100, 1) != 6 {
		t.Fatalf("stepsFor(100) = %d", stepsFor(100, 1))
	}
}

func TestWolfSheepSDWarpReachesFirstPeak(t *testing.T) {
	raw, err := os.ReadFile("../../models/library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("Wolf Sheep Predation (System Dynamics).nlogox", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	go s.loop()
	s.SetSpeed(100)
	started := time.Now()
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}

	var peakSheep, peakTick float64
	var prev float64
	var havePrev bool
	deadline := started.Add(45 * time.Second)
	sawDecline := false
	for time.Now().Before(deadline) {
		sheep, ticks, ok := sdStocks(s)
		if !ok {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		if sheep > peakSheep {
			peakSheep = sheep
			peakTick = ticks
		}
		if havePrev && ticks > 40 && sheep < prev && peakTick > 40 {
			sawDecline = true
			break
		}
		prev = sheep
		havePrev = true
		if ticks > 100 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	elapsed := time.Since(started)
	if !sawDecline {
		sheep, ticks, _ := sdStocks(s)
		t.Fatalf("no sheep peak after %s (ticks %v sheep %v peak %v at %v)", elapsed, ticks, sheep, peakSheep, peakTick)
	}
	if elapsed > 45*time.Second {
		t.Fatalf("first peak took %s, want well under a minute", elapsed)
	}
	if peakTick < 70 || peakTick > 95 {
		t.Fatalf("first sheep peak at t %v, want about 81.5", peakTick)
	}
	t.Logf("first sheep peak %v at t %v in %s", peakSheep, peakTick, elapsed)

	stopStarted := time.Now()
	s.Stop()
	stopDeadline := time.Now().Add(500 * time.Millisecond)
	for s.Session().Running && time.Now().Before(stopDeadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.Session().Running {
		t.Fatal("stop did not finish promptly")
	}
	if time.Since(stopStarted) > 500*time.Millisecond {
		t.Fatalf("stop took %s", time.Since(stopStarted))
	}
}

func sdStocks(s *Session) (sheep, ticks float64, ok bool) {
	s.mu.Lock()
	vm := s.vm
	s.mu.Unlock()
	if vm == nil || vm.World == nil {
		return 0, 0, false
	}
	vm.DoLocked(func() {
		g, found := vm.World.Global("sheep")
		if !found || g.K != nl.KindNum {
			return
		}
		sheep = g.N
		if n, err := vm.World.Ticks(); err == nil {
			ticks = n
			ok = true
		}
	})
	return sheep, ticks, ok
}
