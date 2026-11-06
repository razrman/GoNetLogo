package session

import (
	"strings"
	"testing"
	"time"
)

// paceModel reads its slider and switch on every go. seen records what
// that tick observed, so a widget edit during the run is visible without
// stopping the model.
const paceModel = `globals [seen]
to setup
  clear-all
  set seen 0
  reset-ticks
end
to go
  ifelse flag
    [ set seen pace ]
    [ set seen pace * -1 ]
  tick
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

SWITCH
16
55
160
88
flag
flag
1
1
-1000

SLIDER
16
96
200
129
pace
pace
0
100
1
1
1
NIL
HORIZONTAL
@#$#@#$#@
@#$#@#$#@
`

func TestWidgetUpdatesWhileGoRuns(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("pace.nlogo", paceModel); err != nil {
		t.Fatal(err)
	}
	pace := widgetIndex(s, "slider", "pace")
	flag := widgetIndex(s, "switch", "flag")
	if pace < 0 || flag < 0 {
		t.Fatalf("widgets slider %d switch %d", pace, flag)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	go s.loop()
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	// go itself returns in microseconds, so s.busy is usually false again
	// before a poll can see it. Running stays set until stop, and
	// rejectIfActiveLocked treats that the same as busy.
	waitRunning(t, s)

	if _, err := s.RunCommand("clear-all"); err == nil || !strings.Contains(err.Error(), "model is running") {
		t.Fatalf("command during go: %v", err)
	}
	if err := s.SetSlider(pace, 40); err != nil {
		t.Fatalf("slider during go: %v", err)
	}
	if err := s.SetSwitch(flag, true); err != nil {
		t.Fatalf("switch during go: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var seen float64
	for time.Now().Before(deadline) {
		s.mu.Lock()
		vm := s.vm
		s.mu.Unlock()
		if vm != nil {
			vm.DoLocked(func() {
				if v, ok := vm.World.Global("seen"); ok {
					seen = v.N
				}
			})
		}
		if seen == 40 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if seen != 40 {
		t.Fatalf("go observed pace/flag as %v, want 40", seen)
	}
	if !s.Session().Running {
		t.Fatal("widget update stopped the run")
	}
}

func waitRunning(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Session().Running {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("go did not start")
}
