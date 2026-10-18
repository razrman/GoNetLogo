package nl

import (
	"math"
	"testing"
	"time"
)

func TestRunClockRatePauseAndReset(t *testing.T) {
	var c RunClock
	t0 := time.Unix(1_700_000_000, 0)

	tps, timer := c.Sync(t0, false, 0, 0)
	if tps != 0 || timer != "00:00.0" {
		t.Fatalf("initial tps %v timer %s", tps, timer)
	}
	if FormatTPS(tps, false) != "—" {
		t.Fatalf("stopped label %q", FormatTPS(tps, false))
	}

	// A 1s window of 100 ticks is 100 ticks/sec. A sample inside the
	// 200ms gap is ignored, so the 80-tick blip does not move the rate.
	c.Sync(t0, true, 1, 0)
	c.Sync(t0.Add(200*time.Millisecond), true, 1, 20)
	c.Sync(t0.Add(400*time.Millisecond), true, 1, 40)
	c.Sync(t0.Add(450*time.Millisecond), true, 1, 80)
	tps, timer = c.Sync(t0.Add(time.Second), true, 1, 100)
	if math.Abs(tps-100) > 0.01 {
		t.Fatalf("tps = %v, want 100", tps)
	}
	if timer != "00:01.0" {
		t.Fatalf("timer = %s, want 00:01.0", timer)
	}
	if FormatTPS(tps, true) != "100.0" {
		t.Fatalf("tps label %q", FormatTPS(tps, true))
	}

	_, timer = c.Sync(t0.Add(1500*time.Millisecond), false, 1, 100)
	if timer != "00:01.5" {
		t.Fatalf("pause timer = %s, want 00:01.5", timer)
	}
	tps, timer = c.Sync(t0.Add(3*time.Second), false, 1, 100)
	if tps != 0 || timer != "00:01.5" {
		t.Fatalf("stopped tps %v timer %s, want 0 and 00:01.5", tps, timer)
	}

	c.Sync(t0.Add(3200*time.Millisecond), true, 1, 100)
	_, timer = c.Sync(t0.Add(3400*time.Millisecond), true, 1, 120)
	if timer != "00:01.7" {
		t.Fatalf("resume timer = %s, want 00:01.7", timer)
	}

	tps, timer = c.Sync(t0.Add(4*time.Second), true, 2, 0)
	if tps != 0 || timer != "00:00.0" {
		t.Fatalf("after reset tps %v timer %s", tps, timer)
	}
}

func TestFormatTimer(t *testing.T) {
	if got := FormatTimer(90*time.Second + 400*time.Millisecond); got != "01:30.4" {
		t.Fatalf("got %s", got)
	}
	if got := FormatTimer(59*time.Second + 960*time.Millisecond); got != "01:00.0" {
		t.Fatalf("round got %s", got)
	}
	if got := FormatTimer(time.Hour + 2*time.Minute + 3*time.Second); got != "1:02:03" {
		t.Fatalf("hour got %s", got)
	}
	if got := FormatTimer(-time.Second); got != "00:00.0" {
		t.Fatalf("negative got %s", got)
	}
}
