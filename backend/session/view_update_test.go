package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gonetlogo/backend/graph/model"
	"gonetlogo/pkg/nl"
)

func viewUpdateModel(mode string) string {
	return strings.Join([]string{
		"to go",
		"  crt 1",
		"  repeat 300 [ wait 0.02 ]",
		"  ask turtles [ die ]",
		"  tick",
		"end",
		nlSection,
		"GRAPHICS-WINDOW",
		"0", "0", "0", "0", "-1", "-1", "12.0", "1", "10", "1", "1", "1", "0", "0", "0", "1",
		"-2", "2", "-2", "2",
		mode,
		"1", "1", "ticks", "30.0",
		"",
		"BUTTON",
		"16", "20", "110", "53",
		"go",
		"go",
		"NIL",
		"1",
		"T",
		"OBSERVER",
		"NIL", "NIL", "NIL", "NIL",
		"1",
		nlSection,
		nlSection,
		nlSection,
		nlSection,
		"NetLogo 6.4.0",
		nlSection,
	}, "\n")
}

const nlSection = "@#$#@#$#@"

func TestViewUpdatesChangePublishAndFile(t *testing.T) {
	s := newSession()
	defer s.Close()
	src := viewUpdateModel("1")
	if err := s.LoadModelSource("view.nlogo", src); err != nil {
		t.Fatal(err)
	}
	if got := s.Frame().World.ViewUpdates; got != "ticks" {
		t.Fatalf("loaded mode %q", got)
	}
	settings, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ViewUpdates != "ticks" {
		t.Fatalf("settings %q", settings.ViewUpdates)
	}

	done := make(chan error, 1)
	go func() {
		s.mu.Lock()
		i := s.findButtonLocked("go")
		_, err := s.runIndexLocked(i)
		s.mu.Unlock()
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 4; i++ {
		select {
		case err := <-done:
			t.Fatalf("go finished before the on-ticks samples: %v", err)
		default:
		}
		if n := len(s.Frame().World.Turtles); n != 0 {
			t.Fatalf("on-ticks frame showed %d turtles", n)
		}
		time.Sleep(40 * time.Millisecond)
	}
	if err := s.SetViewUpdates("continuous"); err != nil {
		t.Fatal(err)
	}
	if got := s.Frame().World.ViewUpdates; got != "continuous" {
		t.Fatalf("mode after change %q", got)
	}
	seen := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && seen == 0 {
		select {
		case err := <-done:
			t.Fatalf("go finished before a continuous frame: %v", err)
		default:
		}
		seen = len(s.Frame().World.Turtles)
		if seen == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if seen == 0 {
		t.Fatal("continuous mode did not publish the turtle")
	}
	s.Stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("go did not stop")
	}

	stored := s.ModelSource()
	m, err := nl.ParseModel("view.nlogo", stored)
	if err != nil {
		t.Fatal(err)
	}
	if m.View.UpdateMode != nl.UpdateContinuous {
		t.Fatalf("stored mode %d", m.View.UpdateMode)
	}
	body, err := os.ReadFile(s.uploadPath)
	if err != nil {
		t.Fatal(err)
	}
	uploaded, err := nl.ParseModel("view.nlogo", string(body))
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.View.UpdateMode != nl.UpdateContinuous {
		t.Fatalf("uploaded file mode %d", uploaded.View.UpdateMode)
	}
	if err := s.SetViewUpdates("ticks"); err != nil {
		t.Fatal(err)
	}
	if s.Frame().World.ViewUpdates != "ticks" {
		t.Fatalf("mode after restore %q", s.Frame().World.ViewUpdates)
	}
	restored, err := nl.ParseModel("view.nlogo", s.ModelSource())
	if err != nil {
		t.Fatal(err)
	}
	if restored.View.UpdateMode != nl.UpdateOnTicks {
		t.Fatalf("restored mode %d", restored.View.UpdateMode)
	}
}

func TestNoDisplayKeepsTheSnapshot(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("view.nlogo", viewUpdateModel("1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("crt 1"); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Frame().World.Turtles); n != 1 {
		t.Fatalf("after crt: %d turtles", n)
	}
	if _, err := s.RunCommand("no-display"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunCommand("crt 1"); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Frame().World.Turtles); n != 1 {
		t.Fatalf("no-display showed %d turtles, want the frozen one", n)
	}
	if _, err := s.RunCommand("display"); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Frame().World.Turtles); n != 2 {
		t.Fatalf("display showed %d turtles, want 2", n)
	}
}

func TestContinuousModelPublishesOnTheTimer(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("view.nlogo", viewUpdateModel("0")); err != nil {
		t.Fatal(err)
	}
	if got := s.Frame().World.ViewUpdates; got != "continuous" {
		t.Fatalf("loaded mode %q", got)
	}
	done := make(chan error, 1)
	go func() {
		s.mu.Lock()
		i := s.findButtonLocked("go")
		_, err := s.runIndexLocked(i)
		s.mu.Unlock()
		done <- err
	}()
	seen := 0
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && seen == 0 {
		select {
		case err := <-done:
			t.Fatalf("go finished before a frame: %v", err)
		default:
		}
		seen = len(s.Frame().World.Turtles)
		if seen == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	s.Stop()
	<-done
	if seen == 0 {
		t.Fatal("continuous model published no turtle before tick")
	}
}

func TestRobbyOnTicksHidesCans(t *testing.T) {
	for _, path := range []string{
		"../../models/Robby the Robot.nlogo",
		"../../models/library/Sample Models/Computer Science/Robby the Robot.nlogox",
	} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s := newSession()
			defer s.Close()
			if err := s.useModel(path, string(raw)); err != nil {
				t.Fatal(err)
			}
			if got := s.Frame().World.ViewUpdates; got != "ticks" {
				t.Fatalf("mode %q", got)
			}
			dots := runAndCountDots(t, s, "setup", 45*time.Second)
			if dots != 0 {
				t.Fatalf("setup published %d visible cans", dots)
			}
			people := 0
			for _, turtle := range s.Frame().World.Turtles {
				if !turtle.Hidden && turtle.Shape == "person" {
					people++
				}
			}
			if people == 0 {
				t.Fatal("setup left no population on the view")
			}
			dots = runAndCountDots(t, s, "go", 8*time.Second)
			if dots != 0 {
				t.Fatalf("go published %d visible cans", dots)
			}
		})
	}
}

func runAndCountDots(t *testing.T, s *Session, button string, limit time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		s.mu.Lock()
		i := s.findButtonLocked(button)
		var err error
		if i < 0 {
			err = fmt.Errorf("no %s button", button)
		} else {
			_, err = s.runIndexLocked(i)
		}
		s.mu.Unlock()
		done <- err
	}()
	dots := 0
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			return dots
		default:
		}
		for _, turtle := range s.Frame().World.Turtles {
			if !turtle.Hidden && turtle.Shape == "dot" {
				dots++
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	s.Stop()
	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "stop") && !strings.Contains(err.Error(), "halt") {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("button did not stop")
	}
	return dots
}

func TestWolfSheepStillAdvances(t *testing.T) {
	raw, err := os.ReadFile("../../models/wolfsheep.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession()
	defer s.Close()
	if err := s.useModel("wolfsheep.nlogo", string(raw)); err != nil {
		t.Fatal(err)
	}
	if got := s.Frame().World.ViewUpdates; got != "ticks" {
		t.Fatalf("mode %q, wolf sheep is on ticks and still ticks forward", got)
	}
	runButton(t, s, "setup")
	first := turtleSpots(s.Frame())
	if len(first) == 0 {
		t.Fatal("setup produced no turtles")
	}
	runButton(t, s, "go")
	second := turtleSpots(s.Frame())
	moved := false
	for who, at := range first {
		if next, ok := second[who]; ok && (next[0] != at[0] || next[1] != at[1]) {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("no turtle moved after one go")
	}
	if s.Frame().Ticks == nil || *s.Frame().Ticks < 1 {
		t.Fatalf("ticks %v", s.Frame().Ticks)
	}
}

func runButton(t *testing.T, s *Session, name string) {
	t.Helper()
	s.mu.Lock()
	i := s.findButtonLocked(name)
	if i < 0 {
		s.mu.Unlock()
		t.Fatalf("no %s button", name)
	}
	if _, err := s.runIndexLocked(i); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
}

func turtleSpots(frame *model.Frame) map[int][2]float64 {
	out := map[int][2]float64{}
	if frame == nil {
		return out
	}
	for _, turtle := range frame.World.Turtles {
		if turtle.Hidden {
			continue
		}
		out[turtle.Who] = [2]float64{turtle.X, turtle.Y}
	}
	return out
}
