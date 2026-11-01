package models

import (
	"os"
	"path/filepath"
	"testing"

	"gonetlogo/pkg/nl"
)

// Sample Models from several sections. Parse must succeed. The models in
// compileOK must also build a VM. The others parse today and fail in the
// compiler for reasons left as follow-up work; the log line records that.
func TestSampleModelsParseAndCompile(t *testing.T) {
	t.Setenv("MODELS_DIR", "")
	t.Setenv("GONETLOGO_MODELS_DIR", "")
	root, err := ModelsRoot()
	if err != nil {
		t.Fatal(err)
	}
	compileOK := []string{
		"Sample Models/Earth Science/Fire.nlogox",
		"Sample Models/Biology/Wolf Sheep Predation.nlogox",
		"Sample Models/Art/Follower.nlogox",
		"Sample Models/Computer Science/Robby the Robot.nlogox",
		"Sample Models/Chemistry & Physics/Thermostat.nlogox",
	}
	knownCompileGaps := []string{
		"Sample Models/Biology/Flocking.nlogox",
		"Sample Models/Social Science/Traffic Basic.nlogox",
		"Sample Models/Games/Pac-Man.nlogox",
		"Sample Models/Networks/Giant Component.nlogox",
		"Sample Models/System Dynamics/Exponential Growth.nlogox",
	}
	for _, name := range compileOK {
		m := parseLibraryModel(t, root, name)
		if _, err := m.NewVM(1); err != nil {
			t.Errorf("%s: compile: %v", name, err)
		}
	}
	for _, name := range knownCompileGaps {
		m := parseLibraryModel(t, root, name)
		if _, err := m.NewVM(1); err != nil {
			t.Logf("known load failure %s: %v", name, err)
			continue
		}
		t.Logf("previously failing model compiled: %s", name)
	}
}

func parseLibraryModel(t *testing.T, root, name string) *nl.FileModel {
	t.Helper()
	path := filepath.Join(root, "library", filepath.FromSlash(name))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing %s: %v", name, err)
	}
	m, err := nl.ParseModel(name, string(b))
	if err != nil {
		t.Fatalf("%s: parse: %v", name, err)
	}
	return m
}
