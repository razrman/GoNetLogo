package session

import (
	"context"
	"fmt"
	"log/slog"
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
	if !debugEnabled() {
		return
	}
	if prev < 0 && i >= 0 {
		slog.Debug("step loop started", "session", s.id, "button", s.buttonNameLocked(i))
		return
	}
	if prev >= 0 && i < 0 {
		slog.Debug("step loop stopped", "session", s.id, "reason", reason)
	}
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
	if debugEnabled() {
		slog.Debug("dropped frames", "session", s.id, "dropped", s.dropCap, "slow", s.dropSlow)
	}
	s.dropCap = 0
	s.dropSlow = 0
	s.dropWindow = time.Now()
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
