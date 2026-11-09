package session

import (
	"strings"
	"testing"
	"time"

	"gonetlogo/pkg/nl"
)

// paceModel reads its slider and switch on every go. seen records what
// that tick observed, so a widget edit during the run is visible without
// stopping the model.
const paceModel = `globals [seen bumped]
to setup
  clear-all
  set seen 0
  reset-ticks
end
to go
  ifelse flag
    [ set seen pace ]
    [ set seen pace * -1 ]
  if picked = "fast" [ set seen seen + 1 ]
  tick
end
to bump
  set bumped bumped + 1
end
to wander
  set bumped bumped + 10
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

BUTTON
16
140
80
173
bump
bump
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
140
160
173
wander
wander
T
1
T
OBSERVER
NIL
NIL
NIL
NIL
1

CHOOSER
16
180
180
220
picked
picked
"slow" "fast"
0

INPUTBOX
16
230
180
260
note
hello
1
0
String

MONITOR
16
270
80
310
seen
seen
17
1
11

PLOT
16
320
220
460
Seen
time
seen
0.0
10.0
0.0
10.0
true
false
"" ""
PENS
"seen" 1.0 0 -16777216 true "" "plot seen"
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

	if _, err := s.RunCommand("clear-all"); err == nil || !strings.Contains(err.Error(), "Stop the model before running a command.") {
		t.Fatalf("command during go: %v", err)
	}
	if err := s.Step(); err == nil || !strings.Contains(err.Error(), "Stop the model before stepping.") {
		t.Fatalf("step during go: %v", err)
	}
	if err := s.SetCode("to go\nend\n"); err == nil || !strings.Contains(err.Error(), "Stop the model before changing the code.") {
		t.Fatalf("set code during go: %v", err)
	}
	if err := s.useModel("other.nlogo", "to go\nend\n"); err == nil || !strings.Contains(err.Error(), "Stop the model before loading another model.") {
		t.Fatalf("load during go: %v", err)
	}
	if err := s.SetSlider(pace, 40); err != nil {
		t.Fatalf("slider during go: %v", err)
	}
	if err := s.SetSwitch(flag, true); err != nil {
		t.Fatalf("switch during go: %v", err)
	}
	s.SetSpeed(15)
	if s.Session().Speed != 15 {
		t.Fatalf("speed during go = %v", s.Session().Speed)
	}
	picked := widgetIndex(s, "chooser", "picked")
	note := widgetIndex(s, "input", "note")
	bump := widgetIndex(s, "button", "bump")
	wander := widgetIndex(s, "button", "wander")
	if picked < 0 || note < 0 || bump < 0 || wander < 0 {
		t.Fatalf("widgets chooser %d input %d bump %d wander %d", picked, note, bump, wander)
	}
	if err := s.SetChooser(picked, 1); err != nil {
		t.Fatalf("chooser during go: %v", err)
	}
	if err := s.SetInput(note, "while-going"); err != nil {
		t.Fatalf("input during go: %v", err)
	}
	if text, ok := globalStr(s, "note"); !ok || text != "while-going" {
		t.Fatalf("note = %q ok=%v", text, ok)
	}
	if err := s.PressButton(wander); err == nil || !strings.Contains(err.Error(), "Stop the model before starting another forever button.") {
		t.Fatalf("second forever button: %v", err)
	}

	waitGlobal(t, s, "seen", 41)
	mon := monitorReading(s, "seen")
	if strings.Contains(strings.ToLower(mon), "running") || mon == "N/A" || !strings.Contains(mon, "41") {
		t.Fatalf("monitor during go = %q", mon)
	}
	frame := s.Frame()
	if frame == nil || len(frame.Plots) == 0 {
		t.Fatal("plot missing while go is running")
	}
	if frame.Plots[0].Error != "" {
		t.Fatalf("plot error during go: %s", frame.Plots[0].Error)
	}
	points := 0
	for _, d := range frame.PlotAppend {
		points += len(d.Points)
	}
	if points == 0 {
		t.Fatal("plot recorded no samples while go is running")
	}

	for i := 0; i < 20; i++ {
		if err := s.PressButton(bump); err != nil {
			t.Fatalf("once-button during go: %v", err)
		}
	}
	waitGlobalAtLeast(t, s, "bumped", 1)
	if !s.Session().Running {
		t.Fatal("once-button stopped the run")
	}
	if err := s.Setup(); err != nil {
		t.Fatalf("setup during go: %v", err)
	}
	waitGlobal(t, s, "bumped", 0)
	if !s.Session().Running {
		t.Fatal("setup stopped the run")
	}

	before, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	seedText := nl.FormatRNGSeed(42)
	if err := s.SetWorld(before.MinPxcor, before.MaxPxcor, before.MinPycor, before.MaxPycor, before.PatchSize, before.WrapX, before.WrapY, nil, nil, nil, &seedText, nil); err != nil {
		t.Fatalf("seed during go: %v", err)
	}
	if got, err := s.GenerateRNGSeed(); err != nil || got == "" {
		t.Fatalf("generate seed during go: %q %v", got, err)
	}
	after, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	if after.MinPxcor != before.MinPxcor || after.MaxPxcor != before.MaxPxcor || after.PatchSize != before.PatchSize {
		t.Fatalf("seed changed the grid: %+v", after)
	}
	if !s.Session().Running {
		t.Fatal("seeding stopped the run")
	}
	if err := s.SetWorld(before.MinPxcor, before.MaxPxcor+1, before.MinPycor, before.MaxPycor, before.PatchSize, before.WrapX, before.WrapY, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "Stop the model before changing the world.") {
		t.Fatalf("resize during go: %v", err)
	}
	still, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	if still.MaxPxcor != before.MaxPxcor {
		t.Fatalf("rejected resize changed max-pxcor to %d", still.MaxPxcor)
	}
	if !s.Session().Running {
		t.Fatal("rejected resize stopped the run")
	}
}

func globalNum(s *Session, name string) (float64, bool) {
	s.mu.Lock()
	vm := s.vm
	s.mu.Unlock()
	if vm == nil {
		return 0, false
	}
	var n float64
	var ok bool
	vm.DoLocked(func() {
		if v, found := vm.World.Global(name); found && v.K == nl.KindNum {
			n = v.N
			ok = true
		}
	})
	return n, ok
}

func globalStr(s *Session, name string) (string, bool) {
	s.mu.Lock()
	vm := s.vm
	s.mu.Unlock()
	if vm == nil {
		return "", false
	}
	var text string
	var ok bool
	vm.DoLocked(func() {
		if v, found := vm.World.Global(name); found && v.K == nl.KindString {
			text = v.S
			ok = true
		}
	})
	return text, ok
}

func waitGlobal(t *testing.T, s *Session, name string, want float64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got float64
	var ok bool
	for time.Now().Before(deadline) {
		got, ok = globalNum(s, name)
		if ok && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s = %v ok=%v, want %v", name, got, ok, want)
}

func waitGlobalAtLeast(t *testing.T, s *Session, name string, want float64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got float64
	var ok bool
	for time.Now().Before(deadline) {
		got, ok = globalNum(s, name)
		if ok && got >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s = %v ok=%v, want at least %v", name, got, ok, want)
}

func monitorReading(s *Session, display string) string {
	for _, w := range s.Session().Widgets {
		if w.Kind == "monitor" && strings.EqualFold(w.Display, display) && w.Reading != nil {
			return *w.Reading
		}
	}
	return ""
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
