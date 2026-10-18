package session

import (
	"testing"
	"time"
)

func TestFrameCapSkipsIntermediateSteps(t *testing.T) {
	var p framePublisher
	now := time.Unix(1_700_000_000, 0)
	if p.publish(now) {
		t.Fatal("publish with nothing pending")
	}
	p.produce(6)
	p.produce(6)
	if !p.publish(now) {
		t.Fatal("expected a publish")
	}
	if p.dropped != 11 || p.published != 1 || p.pending != 0 {
		t.Fatalf("after 12 steps: dropped %d published %d pending %d", p.dropped, p.published, p.pending)
	}
	if p.publish(now.Add(time.Millisecond)) {
		t.Fatal("second publish without new steps")
	}
	p.produce(1)
	if !p.publish(now.Add(50 * time.Millisecond)) {
		t.Fatal("single step should publish")
	}
	if p.dropped != 11 {
		t.Fatalf("single step increased drops to %d", p.dropped)
	}
	p.noteSlow(2)
	if p.droppedSlow != 2 || p.dropped != 11 {
		t.Fatalf("slow %d cap %d", p.droppedSlow, p.dropped)
	}
	p.reset()
	if p.dropped != 0 || p.droppedSlow != 0 || p.published != 0 {
		t.Fatal("reset left counters")
	}
}

func TestFrameCapClampAndInterval(t *testing.T) {
	if clampFrameRate(0) != 1 || clampFrameRate(500) != 120 || clampFrameRate(24) != 24 {
		t.Fatalf("clamp 0=%d 500=%d 24=%d", clampFrameRate(0), clampFrameRate(500), clampFrameRate(24))
	}
	if frameInterval(24) != time.Second/24 {
		t.Fatalf("24fps interval %s", frameInterval(24))
	}
	if frameInterval(0) != time.Second/DefaultFrameRate {
		t.Fatalf("default interval %s", frameInterval(0))
	}
}

func TestFrameCapDueInterval(t *testing.T) {
	var p framePublisher
	now := time.Unix(1_700_000_000, 0)
	if p.due(now, frameInterval(24)) {
		t.Fatal("due with nothing pending")
	}
	p.produce(2)
	if !p.due(now, frameInterval(24)) {
		t.Fatal("first snapshot should be due")
	}
	if !p.publish(now) {
		t.Fatal("publish")
	}
	p.produce(3)
	if p.due(now.Add(frameInterval(24)-time.Millisecond), frameInterval(24)) {
		t.Fatal("due before the interval")
	}
	if !p.due(now.Add(frameInterval(24)), frameInterval(24)) {
		t.Fatal("due once the interval elapses")
	}
	if !p.publish(now.Add(frameInterval(24))) {
		t.Fatal("second publish")
	}
	if p.dropped != 1+2 {
		t.Fatalf("dropped %d, want 3", p.dropped)
	}
}

func TestPublishedFPSWindow(t *testing.T) {
	var p framePublisher
	start := time.Unix(1_700_000_000, 0)
	for i := 0; i < 24; i++ {
		p.produce(1)
		p.publish(start.Add(time.Duration(i) * time.Second / 24))
	}
	if p.publishedFPS != 0 {
		t.Fatalf("fps before a full second = %v", p.publishedFPS)
	}
	p.produce(1)
	p.publish(start.Add(time.Second + frameInterval(24)))
	if p.publishedFPS < 20 || p.publishedFPS > 28 {
		t.Fatalf("published fps = %v, want about 24", p.publishedFPS)
	}
}
