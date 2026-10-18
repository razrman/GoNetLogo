package nl

import (
	"testing"
	"time"
)

func TestFrameCapSkipsIntermediateSteps(t *testing.T) {
	var p FramePublisher
	now := time.Unix(1_700_000_000, 0)
	if p.Publish(now, 0) {
		t.Fatal("publish with nothing pending")
	}
	p.Produce(6)
	p.Produce(6)
	if !p.Publish(now, 0) {
		t.Fatal("expected a publish")
	}
	if p.dropped != 11 || p.published != 1 || p.pending != 0 {
		t.Fatalf("after 12 steps: dropped %d published %d pending %d", p.dropped, p.published, p.pending)
	}
	if p.Publish(now.Add(time.Millisecond), 0) {
		t.Fatal("second publish without new steps")
	}
	p.Produce(1)
	if !p.Publish(now.Add(50*time.Millisecond), 0) {
		t.Fatal("single step should publish")
	}
	if p.dropped != 11 {
		t.Fatalf("single step increased drops to %d", p.dropped)
	}
	p.NoteSlow(2)
	if p.droppedSlow != 2 || p.dropped != 11 {
		t.Fatalf("slow %d cap %d", p.droppedSlow, p.dropped)
	}
	p.Reset()
	if p.dropped != 0 || p.droppedSlow != 0 || p.published != 0 {
		t.Fatal("reset left counters")
	}
}

func TestFrameCapClampAndInterval(t *testing.T) {
	if ClampFrameRate(0) != 1 || ClampFrameRate(500) != 120 || ClampFrameRate(24) != 24 {
		t.Fatalf("clamp 0=%d 500=%d 24=%d", ClampFrameRate(0), ClampFrameRate(500), ClampFrameRate(24))
	}
	if FrameInterval(24) != time.Second/24 {
		t.Fatalf("24fps interval %s", FrameInterval(24))
	}
	if FrameInterval(0) != time.Second/DefaultFrameRate {
		t.Fatalf("default interval %s", FrameInterval(0))
	}
}

func TestFrameCapDueInterval(t *testing.T) {
	var p FramePublisher
	now := time.Unix(1_700_000_000, 0)
	if p.Due(now, FrameInterval(24)) {
		t.Fatal("due with nothing pending")
	}
	p.Produce(2)
	if !p.Due(now, FrameInterval(24)) {
		t.Fatal("first snapshot should be due")
	}
	if !p.Publish(now, FrameInterval(24)) {
		t.Fatal("publish")
	}
	p.Produce(3)
	if p.Due(now.Add(FrameInterval(24)-time.Millisecond), FrameInterval(24)) {
		t.Fatal("due before the interval")
	}
	if !p.Due(now.Add(FrameInterval(24)), FrameInterval(24)) {
		t.Fatal("due once the interval elapses")
	}
	if !p.Publish(now.Add(FrameInterval(24)), FrameInterval(24)) {
		t.Fatal("second publish")
	}
	if p.dropped != 1+2 {
		t.Fatalf("dropped %d, want 3", p.dropped)
	}
}

func TestPublishedFPSWindow(t *testing.T) {
	var p FramePublisher
	start := time.Unix(1_700_000_000, 0)
	interval := FrameInterval(24)
	for i := 0; i < 24; i++ {
		p.Produce(1)
		if !p.Publish(start.Add(time.Duration(i)*interval), interval) {
			t.Fatalf("publish %d refused", i)
		}
	}
	if p.publishedFPS != 0 {
		t.Fatalf("fps before a full second = %v", p.publishedFPS)
	}
	p.Produce(1)
	if !p.Publish(start.Add(time.Second+interval), interval) {
		t.Fatal("publish that closes the window was refused")
	}
	if p.publishedFPS < 20 || p.publishedFPS > 24 {
		t.Fatalf("published fps = %v, want about 24 and not above the cap", p.publishedFPS)
	}
}

func TestPublishedFPSStaysWithinCap(t *testing.T) {
	var p FramePublisher
	start := time.Unix(1_700_000_000, 0)
	interval := FrameInterval(24)
	published := 0
	// A step every millisecond, plus a timer on a half-interval phase.
	// Both used to publish, so the measured rate climbed past the cap.
	for ms := 0; ms <= 2000; ms++ {
		now := start.Add(time.Duration(ms) * time.Millisecond)
		p.Produce(1)
		if p.Publish(now, interval) {
			published++
		}
		if ms%20 == 10 {
			p.Produce(1)
			if p.Publish(now, interval) {
				published++
			}
		}
	}
	if published > 24*2+1 {
		t.Fatalf("published %d times in 2s at a 24fps cap", published)
	}
	if p.publishedFPS > 24 {
		t.Fatalf("published fps %v exceeds cap 24", p.publishedFPS)
	}
}
