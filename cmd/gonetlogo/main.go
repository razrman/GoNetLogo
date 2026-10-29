// GoNetLogo is a desktop agent-based modeling environment.
// It is a Go implementation of a subset of the NetLogo language and interface.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"time"

	"gonetlogo/internal/gui"
	"gonetlogo/internal/locale"
	"gonetlogo/internal/logcfg"
	"gonetlogo/models"
	"gonetlogo/pkg/nl"
)

func main() {
	logs := logcfg.Register(flag.CommandLine)
	snapshot := flag.String("snapshot", "", "write a PNG of the interface after running and exit")
	model := flag.String("model", "fire.nlogo", "built-in model name or path to a .nlogo/.nlogox file")
	steps := flag.Int("steps", 30, "forever-button steps to run before -snapshot")
	seed := flag.Int64("seed", 1, "random seed used with -snapshot or -bench (0 keeps a clock seed for -snapshot only)")
	bench := flag.Bool("bench", false, "run a headless Wolf Sheep-style benchmark and exit")
	ticks := flag.Int("ticks", 1000, "go ticks to run with -bench")
	sampleEvery := flag.Int("sample", 100, "record a benchmark sample every N ticks")
	cpuprofile := flag.String("cpuprofile", "", "write a CPU profile while -bench runs")
	flag.Parse()
	if _, err := logs.Apply(os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "gonetlogo: %v\n", err)
		os.Exit(2)
	}
	defer logcfg.Close()
	defer logcfg.Recover()
	logcfg.CloseOnSignal()

	if *bench {
		name := *model
		if name == "fire.nlogo" {
			name = "wolfsheep.nlogo"
		}
		if err := runBench(name, *ticks, *seed, *sampleEvery, *cpuprofile); err != nil {
			commandErr(err)
		}
		return
	}

	if *snapshot != "" {
		if err := writeSnapshot(*model, *snapshot, *steps, *seed); err != nil {
			commandErr(err)
		}
		return
	}
	logDesktopStart(logs)
	gui.Run(models.FS)
}

func logDesktopStart(logs *logcfg.Flags) {
	cfg, err := logs.Resolve(os.Getenv)
	version, commit := "devel", ""
	if info, ok := debug.ReadBuildInfo(); ok && info != nil {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				commit = s.Value
			}
		}
	}
	from, to, changed := locale.Override()
	if err != nil {
		slog.Info("desktop", "version", version, "commit", commit, "locale_from", from, "locale_to", to, "locale_override", changed)
		return
	}
	slog.Info("desktop",
		"version", version,
		"commit", commit,
		"level", cfg.Level.String(),
		"format", cfg.Format,
		"file", cfg.File,
		"locale_from", from,
		"locale_to", to,
		"locale_override", changed,
	)
}

// commandErr prints a command failure on stderr and records it in the log file.
func commandErr(err error) {
	if logcfg.ToFile() {
		slog.Error("exit", "err", err)
	}
	fmt.Fprintln(os.Stderr, err)
	logcfg.Close()
	os.Exit(1)
}

func runBench(model string, ticks int, seed int64, sampleEvery int, cpuprofile string) error {
	if seed == 0 {
		seed = 1
	}
	content, name, err := readModel(model)
	if err != nil {
		return err
	}
	m, err := nl.ParseModel(name, content)
	if err != nil {
		return err
	}
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}
	fmt.Printf("world pending  seed %d  ticks %d\n", seed, ticks)
	result, err := nl.BenchWolfSheepProgress(m, ticks, seed, sampleEvery, func(p nl.BenchPoint) {
		fmt.Printf("tick %d  sheep %d  wolves %d  turtles %d  window %s  %.2f ticks/sec\n",
			p.Tick, p.Sheep, p.Wolves, p.Turtles, p.Window.Round(time.Millisecond), p.TicksPerSec)
	})
	if err != nil {
		return err
	}
	fmt.Print(nl.FormatBench(result))
	return nil
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
