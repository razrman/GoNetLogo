// GoNetLogo is a desktop agent-based modeling environment.
// It is a Go implementation of a subset of the NetLogo language and interface.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gonetlogo/internal/gui"
	"gonetlogo/models"
	"gonetlogo/pkg/nl"
)

func main() {
	snapshot := flag.String("snapshot", "", "write a PNG of the interface after running and exit")
	model := flag.String("model", "fire.nlogo", "built-in model name or path to a .nlogo/.nlogox file")
	steps := flag.Int("steps", 30, "forever-button steps to run before -snapshot")
	seed := flag.Int64("seed", 1, "random seed used with -snapshot (0 keeps the clock seed)")
	flag.Parse()

	if *snapshot != "" {
		if err := writeSnapshot(*model, *snapshot, *steps, *seed); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	gui.Run(models.FS)
}

func writeSnapshot(model, path string, steps int, seed int64) error {
	content, name, err := readModel(model)
	if err != nil {
		return err
	}
	m, err := nl.ParseModel(name, content)
	if err != nil {
		return err
	}
	vm, err := m.NewVM(seed)
	if err != nil {
		return err
	}
	if err := runButton(vm, m, "setup"); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	for i := 0; i < steps; i++ {
		stopped, err := runButtonStopped(vm, m, "go")
		if err != nil {
			return fmt.Errorf("go: %w", err)
		}
		if stopped {
			break
		}
	}
	return nl.Snapshot(m, vm, path)
}

func readModel(model string) (string, string, error) {
	if b, err := os.ReadFile(model); err == nil {
		return string(b), model, nil
	}
	name := model
	if !strings.Contains(name, ".") {
		name += ".nlogo"
	}
	b, err := models.FS.ReadFile(name)
	if err != nil {
		return "", "", fmt.Errorf("open model %s: %w", model, err)
	}
	return string(b), name, nil
}

func runButton(vm *nl.VM, m *nl.FileModel, name string) error {
	_, err := runButtonStopped(vm, m, name)
	return err
}

func runButtonStopped(vm *nl.VM, m *nl.FileModel, name string) (bool, error) {
	if w := m.ButtonByName(name); w != nil {
		stmts, err := nl.CompileButton(vm.Prog, w.Source, w.Agent)
		if err != nil {
			return false, err
		}
		return vm.RunCompiled(stmts)
	}
	if vm.Prog.Proc(name) != nil {
		return vm.Call(name)
	}
	return false, fmt.Errorf("no button or procedure named %s", name)
}
