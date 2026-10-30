package gui

import (
	"log/slog"
	"math"
	"strings"
	"time"

	"gonetlogo/pkg/nl"
)

func isSetupButton(display, source string) bool {
	return strings.EqualFold(display, "setup") || strings.EqualFold(strings.TrimSpace(source), "setup")
}

func logNetLogo(err error, args ...any) {
	if err == nil {
		return
	}
	fields := make([]any, 0, len(args)+6)
	fields = append(fields, "err", err)
	if proc, line, ok := nl.LocateError(err); ok {
		if proc != "" {
			fields = append(fields, "proc", proc)
		}
		if line > 0 {
			fields = append(fields, "line", line)
		}
	}
	fields = append(fields, args...)
	slog.Warn("netlogo", fields...)
}

func warnLockWait(where string, d time.Duration) {
	if d > 250*time.Millisecond {
		slog.Warn("lock wait", "where", where, "wait", d)
	}
}

func (s *session) resetSetupRNG(display, source string, vm *nl.VM) {
	if vm == nil || vm.World == nil || !isSetupButton(display, source) {
		return
	}
	vm.World.ResetRNGDraws()
}

// logRunStart records a forever button that just began. Caller holds s.mu.
func (s *session) logRunStart(name string, forever bool, vm *nl.VM) {
	s.runStart = time.Now()
	s.runRNG = 0
	if vm != nil && vm.World != nil {
		s.runRNG = vm.World.RNGDraws()
	}
	slog.Info("button", "action", "started", "name", name, "forever", forever)
	slog.Debug("run", "event", "start", "button", name, "forever", forever)
}

// logRunStop records why a button finished. Caller holds s.mu.
func (s *session) logRunStop(name string, forever bool, reason string, vm *nl.VM) {
	if reason == "" {
		reason = "stop"
	}
	var draws uint64
	if vm != nil && vm.World != nil {
		draws = vm.World.RNGDraws()
	}
	rate := 0.0
	if !s.runStart.IsZero() {
		if dt := time.Since(s.runStart).Seconds(); dt > 0 {
			delta := draws
			if draws >= s.runRNG {
				delta = draws - s.runRNG
			}
			rate = float64(delta) / dt
		}
	}
	slog.Info("button", "action", "stopped", "name", name, "forever", forever, "reason", reason)
	slog.Debug("run", "event", "stop", "button", name, "forever", forever, "reason", reason,
		"rng_calls", draws, "rng_per_sec", perSec(rate))
}

// noteProgress logs one debug line about every five seconds while a button runs.
func (s *session) noteProgress(vm *nl.VM, tps string, publish time.Duration) {
	if vm == nil || s.runIdx.Load() < 0 {
		return
	}
	now := time.Now()
	if !s.progressAt.IsZero() && now.Sub(s.progressAt) < 5*time.Second {
		if publish > 250*time.Millisecond {
			slog.Warn("frame publish", "took", publish)
		}
		return
	}
	p := vm.Progress()
	rate := 0.0
	if !s.rngMarkAt.IsZero() {
		if dt := now.Sub(s.rngMarkAt).Seconds(); dt > 0 {
			delta := p.RNGDraws
			if p.RNGDraws >= s.rngMark {
				delta = p.RNGDraws - s.rngMark
			}
			rate = float64(delta) / dt
		}
	}
	s.progressAt = now
	s.rngMark = p.RNGDraws
	s.rngMarkAt = now
	ticks := p.Ticks
	if !p.Started {
		ticks = 0
	}
	slog.Debug("progress",
		"ticks", ticks,
		"tps", tps,
		"turtles", p.Turtles,
		"patches", p.Patches,
		"drawing", p.Drawing,
		"plot_points", p.PlotPoints,
		"output_len", p.OutputLen,
		"rng_calls", p.RNGDraws,
		"rng_per_sec", perSec(rate),
		"publish", publish.Round(time.Microsecond),
	)
	if publish > 250*time.Millisecond {
		slog.Warn("frame publish", "took", publish)
	}
	if p.Drawing >= nl.MaxDrawingMarks && !s.capDrawing {
		s.capDrawing = true
		slog.Warn("drawing cap", "marks", p.Drawing, "cap", nl.MaxDrawingMarks)
	}
	if p.Drawing < nl.MaxDrawingMarks {
		s.capDrawing = false
	}
	if p.PlotPoints >= nl.PlotPointCap && !s.capPlot {
		s.capPlot = true
		slog.Warn("plot cap", "points", p.PlotPoints, "cap", nl.PlotPointCap)
	}
	if p.PlotPoints < nl.PlotPointCap {
		s.capPlot = false
	}
}

func (s *session) noteSplit() {
	if s.split != nil && s.split.Offset != s.splitOff {
		if s.splitOff != 0 {
			slog.Debug("split", "pane", "horizontal", "offset", s.split.Offset)
		}
		s.splitOff = s.split.Offset
	}
	if s.vsplit != nil && s.vsplit.Offset != s.vsplitOff {
		if s.vsplitOff != 0 {
			slog.Debug("split", "pane", "vertical", "offset", s.vsplit.Offset)
		}
		s.vsplitOff = s.vsplit.Offset
	}
}

func perSec(rate float64) int64 {
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	return int64(math.Round(rate))
}

func trimLogLine(line string) string {
	const max = 200
	if len(line) <= max {
		return line
	}
	return line[:max] + "…"
}
