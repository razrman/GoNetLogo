package session

import (
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gonetlogo/pkg/nl"
)

// TestRobbyDrawingStaysBounded is the regression for the OOM at tick 133.
// With visuals on, every fitness action strokes the drawing layer. The
// layer used to grow by tens of thousands of marks per generation, and
// every published frame copied the whole layer. Presses during go must
// be ignored so they neither halt the run nor stack another command.
func TestRobbyDrawingStaysBounded(t *testing.T) {
	raw, err := os.ReadFile("../../models/Robby the Robot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("Robby the Robot.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	pop := widgetIndex(s, "slider", "population-size")
	if pop < 0 {
		t.Fatal("no population-size slider")
	}
	if err := s.SetSlider(pop, 20); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(); err != nil {
		t.Fatal(err)
	}
	startTicks := sessionTicks(s)
	if startTicks < 1 {
		t.Fatalf("ticks after one step = %v", startTicks)
	}
	view := widgetIndex(s, "button", "view Robby's Environment")
	if view < 0 {
		t.Fatal("no view button")
	}
	if err := s.PressButton(view); err != nil {
		t.Fatal(err)
	}

	wake, cancel := s.Subscribe()
	defer cancel()
	var drawing atomic.Int64
	stopSub := make(chan struct{})
	subDone := make(chan struct{})
	go func() {
		defer close(subDone)
		var epoch, seq = -1, 0
		var plots map[string]nl.PlotCursor
		for {
			select {
			case <-stopSub:
				return
			case _, ok := <-wake:
				if !ok {
					return
				}
				frame, nextEpoch, nextSeq, nextPlots := s.FrameSince(epoch, seq, plots)
				epoch, seq, plots = nextEpoch, nextSeq, nextPlots
				if frame != nil {
					drawing.Store(int64(len(frame.World.Drawing)))
				}
			}
		}
	}()

	go s.loop()
	s.SetSpeed(100)
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}

	var ignored atomic.Int32
	bad := make(chan error, 1)
	note := func(err error) {
		if err != nil && strings.Contains(err.Error(), "ignored: the model is running") {
			ignored.Add(1)
			return
		}
		select {
		case bad <- err:
		default:
		}
	}
	stopHammer := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopHammer:
				return
			default:
			}
			note(s.PressButton(view))
			note(s.Setup())
			note(s.Step())
			s.SetSpeed(100)
			time.Sleep(20 * time.Millisecond)
		}
	}()

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if sessionTicks(s) >= startTicks+4 && drawing.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(stopHammer)
	time.Sleep(30 * time.Millisecond)
	select {
	case err := <-bad:
		t.Fatalf("press during go: %v", err)
	default:
	}

	gotTicks := sessionTicks(s)
	marks := int(drawing.Load())
	if marks == 0 {
		marks = drawingCount(s)
	}
	if gotTicks < startTicks+4 {
		t.Fatalf("ticks %v, want at least %v (presses must not stop go)", gotTicks, startTicks+4)
	}
	if !s.Session().Running {
		t.Fatal("go stopped while buttons were pressed")
	}
	if marks > nl.MaxDrawingMarks {
		t.Fatalf("drawing marks %d exceed cap %d", marks, nl.MaxDrawingMarks)
	}
	if marks < 1000 {
		t.Fatalf("drawing marks %d, want the visuals layer to fill toward the cap", marks)
	}
	if ignored.Load() == 0 {
		t.Fatal("no press was ignored")
	}

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	goroutines := runtime.NumGoroutine()
	t.Logf("after visuals generations: ticks %.0f→%.0f drawing=%d heap=%dMB goroutines=%d ignored=%d",
		startTicks, gotTicks, marks, ms.HeapAlloc>>20, goroutines, ignored.Load())
	if ms.HeapAlloc > 64<<20 {
		t.Fatalf("heap alloc %d bytes", ms.HeapAlloc)
	}
	if goroutines > 40 {
		t.Fatalf("goroutines %d", goroutines)
	}
	s.Stop()
	close(stopSub)
	select {
	case <-subDone:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not stop")
	}
}

func widgetIndex(s *Session, kind, name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return -1
	}
	for i, w := range s.model.Widgets {
		if w.Kind != kind {
			continue
		}
		if strings.EqualFold(w.Variable, name) || strings.EqualFold(w.Display, name) || strings.EqualFold(strings.TrimSpace(w.Source), name) {
			return i
		}
	}
	return -1
}

func sessionTicks(s *Session) float64 {
	frame := s.Frame()
	if frame == nil || frame.Ticks == nil {
		return 0
	}
	return *frame.Ticks
}

func drawingCount(s *Session) int {
	frame := s.Frame()
	if frame == nil {
		return 0
	}
	return len(frame.World.Drawing)
}
