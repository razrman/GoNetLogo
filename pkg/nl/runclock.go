package nl

import (
	"fmt"
	"strconv"
	"time"
)

// A run sample is one (time, tick-count) pair. The rate uses about one
// second of these, taken a few times a second from the frame loop.
// World.Tick does not record them, so the headless benchmark is unchanged.
const (
	tpsWindow = time.Second
	tpsEvery  = 200 * time.Millisecond
)

type runSample struct {
	at    time.Time
	ticks float64
}

// RunClock is the shared ticks-per-second and run-timer state.
// Desktop and the web session both call Sync. The caller serializes it.
type RunClock struct {
	running   bool
	elapsed   time.Duration
	segment   time.Time
	seenEpoch bool
	epoch     uint64
	samples   []runSample
	tps       float64
}

// Sync applies the latest run flag, reset epoch, and tick count.
// It returns the smoothed ticks per second (0 while stopped) and the
// paused elapsed time formatted as mm:ss.s, or h:mm:ss from one hour.
func (c *RunClock) Sync(now time.Time, running bool, epoch uint64, ticks float64) (tps float64, timer string) {
	c.setRunning(now, running)
	c.noteEpoch(now, epoch)
	c.sample(now, ticks)
	return c.read(now)
}

func (c *RunClock) setRunning(now time.Time, running bool) {
	if c.running == running {
		return
	}
	if running {
		c.segment = now
		c.running = true
		c.samples = nil
		c.tps = 0
		return
	}
	c.elapsed += now.Sub(c.segment)
	if c.elapsed < 0 {
		c.elapsed = 0
	}
	c.running = false
	c.samples = nil
	c.tps = 0
}

func (c *RunClock) noteEpoch(now time.Time, epoch uint64) {
	if !c.seenEpoch {
		c.seenEpoch = true
		c.epoch = epoch
		return
	}
	if epoch == c.epoch {
		return
	}
	c.epoch = epoch
	c.elapsed = 0
	c.samples = nil
	c.tps = 0
	if c.running {
		c.segment = now
	}
}

func (c *RunClock) sample(now time.Time, ticks float64) {
	if !c.running {
		return
	}
	if n := len(c.samples); n > 0 && now.Sub(c.samples[n-1].at) < tpsEvery {
		return
	}
	c.samples = append(c.samples, runSample{at: now, ticks: ticks})
	cut := now.Add(-tpsWindow)
	drop := 0
	for drop+1 < len(c.samples) && c.samples[drop+1].at.Before(cut) {
		drop++
	}
	if drop > 0 {
		c.samples = append([]runSample(nil), c.samples[drop:]...)
	}
	c.recompute()
}

func (c *RunClock) recompute() {
	if len(c.samples) < 2 {
		c.tps = 0
		return
	}
	a := c.samples[0]
	b := c.samples[len(c.samples)-1]
	span := b.at.Sub(a.at)
	if span < tpsEvery {
		c.tps = 0
		return
	}
	rate := (b.ticks - a.ticks) / span.Seconds()
	if rate < 0 {
		rate = 0
	}
	c.tps = rate
}

func (c *RunClock) read(now time.Time) (float64, string) {
	elapsed := c.elapsed
	if c.running {
		elapsed += now.Sub(c.segment)
	}
	if elapsed < 0 {
		elapsed = 0
	}
	tps := 0.0
	if c.running {
		tps = c.tps
	}
	return tps, FormatTimer(elapsed)
}

// FormatTimer renders a run length. Under an hour it is mm:ss.s.
// From one hour it is h:mm:ss.
func FormatTimer(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(100 * time.Millisecond)
	tenths := int(d / (100 * time.Millisecond))
	secs := tenths / 10
	frac := tenths % 10
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d.%d", m, s, frac)
}

// FormatTPS renders the rate. A stopped model is a dash.
func FormatTPS(tps float64, running bool) string {
	if !running || tps < 0 {
		return "—"
	}
	return strconv.FormatFloat(tps, 'f', 1, 64)
}
