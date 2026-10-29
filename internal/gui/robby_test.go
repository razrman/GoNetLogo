package gui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"gonetlogo/models"
	"gonetlogo/pkg/nl"
)

// TestDesktopRobbyRunsPastFreeze is the regression for the desktop stall
// around tick 14. The output log used to be one canvas.Text, which Fyne
// measures as a single line, so the GL texture widened by a full strategy
// string every generation. Each row stays narrow, and the session still
// accepts Stop, after many generations.
func TestDesktopRobbyRunsPastFreeze(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	s.async = true
	s.modelStop = make(chan struct{})
	go s.modelLoop()
	defer close(s.modelStop)

	b, err := models.FS.ReadFile("Robby the Robot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.load("Robby the Robot.nlogo", string(b))
	s.mu.Lock()
	if s.vm != nil {
		s.vm.World.SetGlobal("population-size", nl.Num(20))
	}
	s.mu.Unlock()
	s.pressNamed("setup")
	waitUntil(t, 20*time.Second, func() bool {
		s.mu.Lock()
		inRun := s.inRun
		vm := s.vm
		s.mu.Unlock()
		if inRun || vm == nil {
			return false
		}
		return vm.Progress().Started
	})
	s.mu.Lock()
	s.vm.World.SetGlobal("num-environments-for-fitness", nl.Num(1))
	s.vm.World.SetGlobal("num-actions-per-environment", nl.Num(2))
	s.mu.Unlock()

	s.pressNamed("go")
	deadline := time.Now().Add(45 * time.Second)
	var slow time.Duration
	var ticks float64
	for time.Now().Before(deadline) {
		start := time.Now()
		s.presentUI()
		if d := time.Since(start); d > slow {
			slow = d
		}
		if vm := s.vmP.Load(); vm != nil {
			if p := vm.Progress(); p.Started {
				ticks = p.Ticks
			}
		}
		if ticks >= 30 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ticks < 30 {
		t.Fatalf("ticks = %v after the run, want at least 30", ticks)
	}
	if slow > 200*time.Millisecond {
		t.Fatalf("presentUI took %s while Robby was running", slow)
	}
	if len(s.areas) != 1 {
		t.Fatalf("output areas %d", len(s.areas))
	}
	area := s.areas[0]
	if !strings.Contains(area.body, "generation") || !strings.Contains(area.body, "best strategy") {
		t.Fatalf("output %q", area.body)
	}
	if got := area.box.MinSize().Width; got > 280 {
		t.Fatalf("output panel min width %v", got)
	}
	for i, row := range area.rows {
		if n := len([]rune(row.Text)); n > outputRowRunes {
			t.Fatalf("row %d has %d runes", i, n)
		}
		if row.MinSize().Width > 4000 {
			t.Fatalf("row %d width %v", i, row.MinSize().Width)
		}
	}

	s.pressNamed("go")
	waitUntil(t, 15*time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.inRun && s.running < 0
	})
}

func waitUntil(t *testing.T, d time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the desktop model")
}
