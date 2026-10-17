package session

import (
	"os"
	"testing"
	"time"
)

func TestMaxSpeedWakesOncePerFrame(t *testing.T) {
	raw, err := os.ReadFile("../../models/wolfsheep.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	wake, cancel := s.Subscribe()
	defer cancel()
	if err := s.useModel("wolfsheep.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("no initial wake")
	}

	s.mu.Lock()
	setup := s.findButtonLocked("setup")
	if setup < 0 {
		s.mu.Unlock()
		t.Fatal("no setup button")
	}
	if _, err := s.runIndexLocked(setup); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	goBtn := s.findButtonLocked("go")
	if goBtn < 0 {
		s.mu.Unlock()
		t.Fatal("no go button")
	}
	s.running = goBtn
	s.speed = 100
	s.frameTickLocked()
	s.mu.Unlock()

	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("frame did not wake")
	}
	select {
	case <-wake:
		t.Fatal("max speed queued a second frame")
	default:
	}

	frame := s.Frame()
	if frame.Ticks == nil || *frame.Ticks != 6 {
		t.Fatalf("ticks = %v, want 6 (one frame at speed 100)", frame.Ticks)
	}
	if frame.World.Width != 101 || frame.World.Height != 101 || frame.World.PatchSize != 5 {
		t.Fatalf("world %+v", frame.World)
	}
	if frame.World.PatchColors == "" {
		t.Fatal("frame omitted patch colors")
	}
}
