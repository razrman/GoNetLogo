package nl

import (
	"fmt"
	"runtime"
	"time"
)

// BenchPoint is one sample of a headless Wolf Sheep run.
type BenchPoint struct {
	Tick        int
	Sheep       int
	Wolves      int
	Turtles     int
	Window      time.Duration
	TicksPerSec float64
}

// BenchResult is a fixed-seed Wolf Sheep run on the model's own world.
type BenchResult struct {
	Width      int
	Height     int
	PatchSize  float64
	Seed       int64
	Ticks      int
	Points     []BenchPoint
	TotalAlloc uint64
	Mallocs    uint64
	Elapsed    time.Duration
}

// BenchWolfSheep runs setup and ticks of go. Samples are taken at every
// sampleEvery tick (and at the final tick). The window rate is ticks in
// that sample divided by the time spent inside go, not counting the count.
func BenchWolfSheep(m *FileModel, ticks int, seed int64, sampleEvery int) (BenchResult, error) {
	return BenchWolfSheepProgress(m, ticks, seed, sampleEvery, nil)
}

// BenchWolfSheepProgress is BenchWolfSheep plus a callback after each sample.
func BenchWolfSheepProgress(m *FileModel, ticks int, seed int64, sampleEvery int, onSample func(BenchPoint)) (BenchResult, error) {
	if ticks < 1 {
		return BenchResult{}, fmt.Errorf("ticks must be positive")
	}
	if sampleEvery < 1 {
		sampleEvery = ticks
	}
	vm, err := m.NewVM(seed)
	if err != nil {
		return BenchResult{}, err
	}
	if _, err := vm.Call("setup"); err != nil {
		return BenchResult{}, fmt.Errorf("setup: %w", err)
	}
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	windowStart := start
	var points []BenchPoint
	for tick := 1; tick <= ticks; tick++ {
		if _, err := vm.Call("go"); err != nil {
			return BenchResult{}, fmt.Errorf("go %d: %w", tick, err)
		}
		if tick%sampleEvery == 0 || tick == ticks {
			now := time.Now()
			window := now.Sub(windowStart)
			span := tick
			if len(points) > 0 {
				span = tick - points[len(points)-1].Tick
			}
			sheep, wolves, n := countPop(vm.World)
			rate := 0.0
			if window > 0 {
				rate = float64(span) / window.Seconds()
			}
			point := BenchPoint{
				Tick: tick, Sheep: sheep, Wolves: wolves, Turtles: n,
				Window: window, TicksPerSec: rate,
			}
			points = append(points, point)
			if onSample != nil {
				onSample(point)
			}
			windowStart = time.Now()
		}
	}
	elapsed := time.Since(start)
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	return BenchResult{
		Width: vm.World.Width(), Height: vm.World.Height(), PatchSize: vm.World.PatchSize,
		Seed: seed, Ticks: ticks, Points: points,
		TotalAlloc: after.TotalAlloc - before.TotalAlloc,
		Mallocs:    after.Mallocs - before.Mallocs,
		Elapsed:    elapsed,
	}, nil
}

func countPop(w *World) (sheep, wolves, n int) {
	for _, t := range w.liveTurtles() {
		n++
		switch t.Breed {
		case "sheep":
			sheep++
		case "wolves":
			wolves++
		}
	}
	return sheep, wolves, n
}

// FormatBench renders a stable text table for the CLI and PR notes.
func FormatBench(r BenchResult) string {
	s := fmt.Sprintf("world %dx%d patch-size %g seed %d ticks %d\n", r.Width, r.Height, r.PatchSize, r.Seed, r.Ticks)
	s += fmt.Sprintf("elapsed %s  alloc %d bytes  mallocs %d\n", r.Elapsed.Round(time.Millisecond), r.TotalAlloc, r.Mallocs)
	s += "tick\tsheep\twolves\tturtles\twindow_ms\tticks/sec\n"
	for _, p := range r.Points {
		s += fmt.Sprintf("%d\t%d\t%d\t%d\t%d\t%.2f\n", p.Tick, p.Sheep, p.Wolves, p.Turtles, p.Window.Milliseconds(), p.TicksPerSec)
	}
	return s
}
