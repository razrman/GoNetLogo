package session

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"gonetlogo/pkg/nl"
)

// longGo is a forever button whose body does not return on its own.
const longGo = `to setup
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

func TestStopDuringLongGoReturnsPromptly(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("long.nlogo", longGo); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	go s.loop()
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	waitBusy(t, s)
	start := time.Now()
	s.Stop()
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("Stop took %s", elapsed)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !s.Session().Running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("model still running after Stop")
}

func TestSetSpeedAndFrameRateDuringLongGo(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("long.nlogo", longGo); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	go s.loop()
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	waitBusy(t, s)
	start := time.Now()
	s.SetSpeed(10)
	if got := s.SetFrameRate(12); got != 12 {
		t.Fatalf("frame rate %d", got)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("speed and frame rate took %s", elapsed)
	}
	snap := s.Session()
	if snap.Speed != 10 || snap.FrameRateCap != 12 {
		t.Fatalf("speed %v cap %d", snap.Speed, snap.FrameRateCap)
	}
	s.Stop()
}

func TestStopDuringRobbyGeneration(t *testing.T) {
	raw, err := os.ReadFile("../../models/Robby the Robot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("Robby the Robot.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	go s.loop()
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	waitBusy(t, s)
	start := time.Now()
	s.Stop()
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("Stop took %s", elapsed)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !s.Session().Running {
			t.Logf("robby stopped in %s", time.Since(start))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("robby still running after Stop")
}

func TestRobbyFramePayloadStaysBounded(t *testing.T) {
	raw, err := os.ReadFile("../../models/Robby the Robot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("Robby the Robot.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	goBtn := s.findButtonLocked("go")
	s.mu.Unlock()
	if goBtn < 0 {
		t.Fatal("no go button")
	}
	var epoch, seq = -1, 0
	var plots map[string]nl.PlotCursor
	sizes := map[int]int{}
	const gens = 40
	for tick := 1; tick <= gens; tick++ {
		s.mu.Lock()
		stopped, err := s.runIndexLocked(goBtn)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if stopped {
			t.Fatalf("go stopped at tick %d", tick)
		}
		frame, nextEpoch, nextSeq, nextPlots := s.FrameSince(epoch, seq, plots)
		epoch, seq, plots = nextEpoch, nextSeq, nextPlots
		body, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		if frame.OutputReset && tick > 1 {
			t.Fatalf("tick %d reset output (%d lines)", tick, len(frame.OutputAppend))
		}
		if len(frame.PlotAppend) > 2 {
			t.Fatalf("tick %d plot appends %d", tick, len(frame.PlotAppend))
		}
		for _, d := range frame.PlotAppend {
			if !d.Reset && len(d.Points) > 2 {
				t.Fatalf("tick %d pen grew by %d points", tick, len(d.Points))
			}
		}
		if len(frame.OutputAppend) > 8 {
			t.Fatalf("tick %d appended %d output lines", tick, len(frame.OutputAppend))
		}
		sizes[tick] = len(body)
	}
	if sizes[20] == 0 || sizes[40] == 0 {
		t.Fatalf("sizes %+v", sizes)
	}
	// A frame that resent every previous output line would be larger at
	// tick 40 than at tick 20 by thousands of bytes. Allow slack for the
	// turtle positions and one more strategy line.
	if sizes[40] > sizes[20]+8192 {
		t.Fatalf("payload grew from %d at tick 20 to %d at tick 40", sizes[20], sizes[40])
	}
	if sizes[20] > 200000 {
		t.Fatalf("tick 20 payload %d bytes", sizes[20])
	}
	t.Logf("payload bytes tick 20=%d tick 40=%d", sizes[20], sizes[40])
}

func waitBusy(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		busy := s.busy
		s.mu.Unlock()
		if busy {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("go did not enter the interpreter")
}
