package session

import (
	"strings"
	"testing"
)

func TestHasProcedure(t *testing.T) {
	s := newSession()
	t.Cleanup(s.Close)

	mixed := "to Setup\n  error \"boom\"\nend\nto setup-normal\nend\nto setup-random\nend\n"
	if err := s.LoadModelSource("life.nlogo", mixed); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"setup", "SETUP", "Setup", " setup "} {
		if !s.HasProcedure(name) {
			t.Fatalf("HasProcedure(%q) = false", name)
		}
	}
	for _, name := range []string{"Setup-Normal", "SETUP-RANDOM"} {
		if !s.HasProcedure(name) {
			t.Fatalf("HasProcedure(%q) = false", name)
		}
	}
	for _, name := range []string{"go", "", "   ", "setup-norma"} {
		if s.HasProcedure(name) {
			t.Fatalf("HasProcedure(%q) = true", name)
		}
	}
	names := s.Session().Procedures
	for _, want := range []string{"setup", "setup-normal", "setup-random"} {
		if !containsString(names, want) {
			t.Fatalf("procedures %v, want %q", names, want)
		}
	}
	if err := s.Setup(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("setup of a defined procedure: %v", err)
	}

	if err := s.LoadModelSource("nosetup.nlogo", "to setup-normal\nend\nto setup-random\nend\n"); err != nil {
		t.Fatal(err)
	}
	if s.HasProcedure("setup") || s.HasProcedure("Setup") {
		t.Fatal("setup-normal counted as setup")
	}
	names = s.Session().Procedures
	if containsString(names, "setup") {
		t.Fatalf("procedures %v includes setup", names)
	}
	if err := s.Setup(); err == nil || !strings.Contains(err.Error(), "nothing named setup") {
		t.Fatalf("missing setup: %v", err)
	}

	if err := s.LoadModelSource("reporter.nlogo", "to-report setup\n  report 1\nend\n"); err != nil {
		t.Fatal(err)
	}
	if !s.HasProcedure("SeTuP") {
		t.Fatal("a reporter named setup was ignored")
	}
	if err := s.Setup(); err == nil || strings.Contains(err.Error(), "nothing named") {
		t.Fatalf("reporter setup: %v", err)
	}
}

func containsString(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
