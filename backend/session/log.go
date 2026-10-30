package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gonetlogo/pkg/nl"
)

func debugEnabled() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}

func (s *Session) buttonNameLocked(i int) string {
	if s.model == nil || i < 0 || i >= len(s.model.Widgets) {
		return ""
	}
	return s.model.Widgets[i].Display
}

// setRunningLocked records the forever button and logs the transition.
// Caller holds s.mu.
func (s *Session) setRunningLocked(i int, reason string) {
	prev := s.running
	s.running = i
	if prev == i {
		return
	}
	if prev >= 0 && i < 0 {
		s.flushDropsLocked()
	}
	if prev < 0 && i >= 0 {
		s.runStart = time.Now()
		s.runRNG = 0
		if s.vm != nil && s.vm.World != nil {
			s.runRNG = s.vm.World.RNGDraws()
		}
	}
	if !debugEnabled() {
		return
	}
	if prev < 0 && i >= 0 {
		slog.Debug("step loop started", "session", s.id, "button", s.buttonNameLocked(i))
		return
	}
	if prev >= 0 && i < 0 {
		draws, rate := s.rngRateLocked()
		slog.Debug("step loop stopped", "session", s.id, "reason", reason, "rng_calls", draws, "rng_per_sec", perSec(rate))
	}
}

// rngRateLocked is the draw counter and the draws per second since the
// forever button started. Caller holds s.mu.
func (s *Session) rngRateLocked() (uint64, float64) {
	var draws uint64
	if s.vm != nil && s.vm.World != nil {
		draws = s.vm.World.RNGDraws()
	}
	if s.runStart.IsZero() {
		return draws, 0
	}
	dt := time.Since(s.runStart).Seconds()
	if dt <= 0 {
		return draws, 0
	}
	delta := draws
	if draws >= s.runRNG {
		delta = draws - s.runRNG
	}
	return draws, float64(delta) / dt
}

func perSec(rate float64) int64 {
	if rate <= 0 {
		return 0
	}
	return int64(rate + 0.5)
}

// haltVM asks the interpreter to stop and logs it once per request.
func (s *Session) haltVM(vm *nl.VM, reason string) {
	if vm != nil {
		vm.RequestHalt()
	}
	if debugEnabled() {
		slog.Debug("halt", "session", s.id, "reason", reason)
	}
}

// rejectIfActiveLocked reports that a button or command cannot start.
// Caller holds s.mu. Stop of the running forever button does not come here.
func (s *Session) rejectIfActiveLocked(action string) error {
	if !s.busy && s.running < 0 {
		return nil
	}
	s.noteIgnoredLocked(action)
	return fmt.Errorf("%s ignored: the model is running", action)
}

func (s *Session) noteIgnoredLocked(action string) {
	s.ignored++
	now := time.Now()
	if !s.ignoredAt.IsZero() && now.Sub(s.ignoredAt) < time.Second {
		return
	}
	slog.Warn("request ignored", "session", s.id, "action", action, "count", s.ignored, "reason", "run active")
	s.ignored = 0
	s.ignoredAt = now
}

// noteDropsLocked accumulates skipped steps and slow-subscriber misses.
// Caller holds s.mu. Nothing is formatted unless debug logging is on.
func (s *Session) noteDropsLocked(capN, slowN int) {
	if capN <= 0 && slowN <= 0 {
		return
	}
	if !debugEnabled() {
		return
	}
	now := time.Now()
	if s.dropWindow.IsZero() {
		s.dropWindow = now
	}
	s.dropCap += capN
	s.dropSlow += slowN
	if now.Sub(s.dropWindow) < time.Second {
		return
	}
	s.flushDropsLocked()
}

func (s *Session) flushDropsLocked() {
	if s.dropCap == 0 && s.dropSlow == 0 {
		return
	}
	slog.Warn("dropped frames", "session", s.id, "dropped", s.dropCap, "slow", s.dropSlow)
	s.dropCap = 0
	s.dropSlow = 0
	s.dropWindow = time.Now()
}

func isSetupName(display, source string) bool {
	return strings.EqualFold(display, "setup") || strings.EqualFold(strings.TrimSpace(source), "setup")
}

func warnNetLogo(msg string, err error, args ...any) {
	fields := make([]any, 0, len(args)+6)
	fields = append(fields, args...)
	if err != nil {
		fields = append(fields, "err", err)
		if proc, line, ok := nl.LocateError(err); ok {
			if proc != "" {
				fields = append(fields, "proc", proc)
			}
			if line > 0 {
				fields = append(fields, "line", line)
			}
		}
	}
	slog.Warn(msg, fields...)
}

// noteProgress writes one debug line about every five seconds during a run.
func (s *Session) noteProgress(vm *nl.VM) {
	if vm == nil || !debugEnabled() {
		return
	}
	now := time.Now()
	s.mu.Lock()
	if s.running < 0 || (!s.progressAt.IsZero() && now.Sub(s.progressAt) < 5*time.Second) {
		s.mu.Unlock()
		return
	}
	s.progressAt = now
	prev, prevAt := s.rngMark, s.rngMarkAt
	id := s.id
	s.mu.Unlock()

	p := vm.Progress()
	rate := 0.0
	if !prevAt.IsZero() {
		if dt := now.Sub(prevAt).Seconds(); dt > 0 {
			delta := p.RNGDraws
			if p.RNGDraws >= prev {
				delta = p.RNGDraws - prev
			}
			rate = float64(delta) / dt
		}
	}
	ticks := 0.0
	if p.Started {
		ticks = p.Ticks
	}
	slog.Debug("progress", "session", id, "ticks", ticks, "turtles", p.Turtles, "patches", p.Patches,
		"drawing", p.Drawing, "plot_points", p.PlotPoints, "output_len", p.OutputLen,
		"rng_calls", p.RNGDraws, "rng_per_sec", perSec(rate))

	s.mu.Lock()
	s.rngMark = p.RNGDraws
	s.rngMarkAt = now
	if p.Drawing >= nl.MaxDrawingMarks && !s.capDrawing {
		s.capDrawing = true
		slog.Warn("drawing cap", "session", id, "marks", p.Drawing, "cap", nl.MaxDrawingMarks)
	}
	if p.Drawing < nl.MaxDrawingMarks {
		s.capDrawing = false
	}
	if p.PlotPoints >= nl.PlotPointCap && !s.capPlot {
		s.capPlot = true
		slog.Warn("plot cap", "session", id, "points", p.PlotPoints, "cap", nl.PlotPointCap)
	}
	if p.PlotPoints < nl.PlotPointCap {
		s.capPlot = false
	}
	s.mu.Unlock()
}

func logModel(msg string, sessionID, name string, bytes int, err error) {
	if err != nil {
		slog.Warn(msg, "session", sessionID, "model", name, "bytes", bytes, "err", err)
		return
	}
	if debugEnabled() {
		slog.Debug(msg, "session", sessionID, "model", name, "bytes", bytes)
	}
}
