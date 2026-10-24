package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"gonetlogo/models"
)

func TestDesktopStopDuringLongGo(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	s.async = true
	s.modelStop = make(chan struct{})
	go s.modelLoop()
	defer close(s.modelStop)

	const src = `to setup
  clear-all
  reset-ticks
end
to go
  loop [ ]
end
@#$#@#$#@
GRAPHICS-WINDOW
280
10
400
130
-1
-1
12.0
1
10
1
1
1
0
0
0
1
-2
2
-2
2
1
1
1
ticks
30.0

BUTTON
16
20
80
53
setup
setup
NIL
1
T
OBSERVER
NIL
NIL
NIL
NIL
1

BUTTON
90
20
154
53
go
go
T
1
T
OBSERVER
NIL
NIL
NIL
NIL
0
@#$#@#$#@
@#$#@#$#@
`
	s.load("long.nlogo", src)
	s.pressNamed("setup")
	waitDesktop(t, s, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.inRun && s.vm != nil && s.vm.World.TicksStarted()
	})
	s.pressNamed("go")
	waitDesktop(t, s, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.inRun
	})
	start := time.Now()
	s.pressNamed("go")
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("stop click took %s", elapsed)
	}
	waitDesktop(t, s, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.inRun && s.running < 0
	})
	if time.Since(start) > 400*time.Millisecond {
		t.Fatalf("model still running after %s", time.Since(start))
	}
}

func waitDesktop(t *testing.T, s *session, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the desktop model")
}
