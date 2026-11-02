// Command modelcheck parses and compiles NetLogo models with GoNetLogo
// and, unless -skip-run is set, runs setup and a few go ticks.
//
//	go run ./cmd/modelcheck
//	go run ./cmd/modelcheck -skip-run
//	go run ./cmd/modelcheck -json -out modelcheck.json
//
// The default set is models/library plus models/Robby the Robot.nlogo.
// Compile is the code tab plus button, monitor, and plot command lists.
// Setup and each go tick give up after -timeout. A world longer than
// -max-side patches on either axis is compiled and not run.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gonetlogo/models"
)

func main() {
	fs := flag.NewFlagSet("modelcheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	library := fs.String("library", "", "models library directory (default: <models root>/library)")
	var extras multiFlag
	fs.Var(&extras, "extra", "additional model file (repeatable); default also checks models/Robby the Robot.nlogo")
	checkOne := fs.String("check-one", "", "check a single file and print one JSON object (internal)")
	timeout := fs.Duration("timeout", 4*time.Second, "limit for setup and for each go tick")
	ticks := fs.Int("ticks", 3, "go calls after setup")
	seed := fs.Int64("seed", 1, "random seed (0 keeps the clock seed)")
	skipRun := fs.Bool("skip-run", false, "parse and compile only")
	maxSide := fs.Int("max-side", 0, "skip runs when a world axis exceeds this (default: GoNetLogo's max)")
	jobs := fs.Int("jobs", 1, "how many models to run at once")
	asJSON := fs.Bool("json", false, "print the full report as JSON")
	outPath := fs.String("out", "", "write the JSON report to this path")
	topN := fs.Int("top", 40, "how many raw first-error signatures to print (0 hides them)")
	wall := fs.Duration("wall", 0, "kill a model process after this (0 means timeout × calls plus 15s)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	opt := Options{
		Timeout: *timeout,
		Ticks:   *ticks,
		Seed:    *seed,
		SkipRun: *skipRun,
		MaxSide: *maxSide,
	}
	if opt.Ticks <= 0 {
		fmt.Fprintln(os.Stderr, "modelcheck: -ticks must be positive")
		os.Exit(2)
	}
	if *jobs < 1 {
		fmt.Fprintln(os.Stderr, "modelcheck: -jobs must be positive")
		os.Exit(2)
	}

	if *checkOne != "" {
		res := Check(*checkOne, opt)
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
			os.Exit(1)
		}
		return
	}

	lib, extra, err := resolveInputs(*library, extras, fs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
		os.Exit(1)
	}
	files, err := collect(lib, extra)
	if err != nil {
		fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "modelcheck: no models found")
		os.Exit(1)
	}

	wallLimit := *wall
	if wallLimit <= 0 {
		n := 1 + opt.ticks()
		wallLimit = opt.timeout()*time.Duration(n) + 15*time.Second
	}

	results := make([]Result, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *jobs)
	for i, path := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = checkOneFile(path, opt, wallLimit)
			fmt.Fprintf(os.Stderr, "[%d/%d] %s %s\n", i+1, len(files), results[i].Stage, results[i].Path)
		}(i, path)
	}
	wg.Wait()

	sum := Summarize(results)
	report := struct {
		Summary    Summary    `json:"summary"`
		Signatures []SigCount `json:"signatures"`
		Models     []Result   `json:"models"`
	}{
		Summary:    sum,
		Signatures: TopSignatures(results, 0),
		Models:     results,
	}

	if *outPath != "" {
		if err := writeJSON(*outPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
			os.Exit(1)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
			os.Exit(1)
		}
	} else {
		writeText(os.Stdout, sum, results, *topN)
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(s string) error {
	*m = append(*m, s)
	return nil
}

func resolveInputs(library string, extras multiFlag, fs *flag.FlagSet) (string, []string, error) {
	lib := strings.TrimSpace(library)
	if lib == "" {
		root, err := models.ModelsRoot()
		if err != nil {
			return "", nil, err
		}
		lib = filepath.Join(root, "library")
	}
	lib, err := filepath.Abs(lib)
	if err != nil {
		return "", nil, err
	}
	var extra []string
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "extra" {
			explicit = true
		}
	})
	if explicit {
		extra = append(extra, extras...)
	} else {
		robby := filepath.Join(filepath.Dir(lib), "Robby the Robot.nlogo")
		if st, err := os.Stat(robby); err == nil && st.Mode().IsRegular() {
			extra = append(extra, robby)
		}
		extra = append(extra, extras...)
	}
	return lib, extra, nil
}

func collect(library string, extras []string) ([]string, error) {
	files, err := Walk(library)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if seen[abs] {
			return nil
		}
		seen[abs] = true
		out = append(out, abs)
		return nil
	}
	for _, path := range files {
		if err := add(path); err != nil {
			return nil, err
		}
	}
	for _, path := range extras {
		if err := add(path); err != nil {
			return nil, err
		}
	}
	for i, path := range out {
		out[i] = displayPath(path)
	}
	return out, nil
}

func displayPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func checkOneFile(path string, opt Options, wall time.Duration) Result {
	// Compile in this process. Running is a child so a tight loop or a
	// large world that ignores halt cannot stall the whole scan.
	probe := opt
	probe.SkipRun = true
	res := Check(path, probe)
	if opt.SkipRun || !shouldRun(res) {
		return res
	}
	child, ok, killErr := runChild(path, opt, wall)
	if ok {
		child.Path = res.Path
		return child
	}
	if res.HasSetup {
		res.Setup = "timeout"
		res.Stage = "setup"
		res.Go = "skipped"
	} else {
		res.Go = "timeout"
		res.Stage = "go"
	}
	res.Error = "timed out (process killed)"
	if killErr != nil && !strings.Contains(killErr.Error(), "signal: killed") && !strings.Contains(killErr.Error(), "context deadline exceeded") {
		res.Error = oneLine(killErr.Error())
	}
	return res
}

func shouldRun(r Result) bool {
	if !r.CompileOK || r.Stage == "world" || r.Stage == "vm" {
		return false
	}
	return r.HasSetup || r.HasGo
}

func runChild(path string, opt Options, wall time.Duration) (Result, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return Result{}, false, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Result{}, false, err
	}
	args := []string{
		"-check-one", abs,
		"-timeout", opt.timeout().String(),
		"-ticks", fmt.Sprint(opt.ticks()),
		"-seed", fmt.Sprint(opt.Seed),
		"-max-side", fmt.Sprint(opt.maxSide()),
	}
	ctx, cancel := context.WithTimeout(context.Background(), wall)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir, _ = os.Getwd()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	var res Result
	raw := bytes.TrimSpace(stdout.Bytes())
	if json.Unmarshal(raw, &res) == nil && res.Stage != "" {
		return res, true, nil
	}
	if err == nil {
		err = fmt.Errorf("no result")
	}
	if stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, oneLine(stderr.String()))
	}
	return Result{}, false, err
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func writeText(w *os.File, sum Summary, results []Result, topN int) {
	fmt.Fprintf(w, "total\t%d\n", sum.Total)
	fmt.Fprintf(w, "parse_ok\t%d\n", sum.ParseOK)
	fmt.Fprintf(w, "code_ok\t%d\n", sum.CodeOK)
	fmt.Fprintf(w, "compile_ok\t%d\n", sum.CompileOK)
	fmt.Fprintf(w, "compile_fail\t%d\n", sum.CompileFail)
	fmt.Fprintf(w, "setup_ok\t%d\n", sum.SetupOK)
	fmt.Fprintf(w, "setup_absent\t%d\n", sum.SetupAbsent)
	fmt.Fprintf(w, "setup_fail\t%d\n", sum.SetupFail)
	fmt.Fprintf(w, "setup_timeout\t%d\n", sum.SetupTimeout)
	fmt.Fprintf(w, "setup_skipped\t%d\n", sum.SetupSkipped)
	fmt.Fprintf(w, "go_ok\t%d\n", sum.GoOK)
	fmt.Fprintf(w, "go_stopped\t%d\n", sum.GoStopped)
	fmt.Fprintf(w, "go_absent\t%d\n", sum.GoAbsent)
	fmt.Fprintf(w, "go_fail\t%d\n", sum.GoFail)
	fmt.Fprintf(w, "go_timeout\t%d\n", sum.GoTimeout)
	fmt.Fprintf(w, "go_skipped\t%d\n", sum.GoSkipped)
	fmt.Fprintf(w, "world_skipped\t%d\n", sum.WorldSkipped)
	fmt.Fprintln(w)
	if topN > 0 {
		fmt.Fprintln(w, "signature\tcount")
		for _, row := range TopSignatures(results, topN) {
			fmt.Fprintf(w, "%s\t%d\n", row.Signature, row.Count)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "path\tcompile\tsetup\tgo\tticks\tstage\terror")
	for _, r := range results {
		compile := "fail"
		if r.CompileOK {
			compile = "ok"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			r.Path, compile, r.Setup, r.Go, r.GoTicks, r.Stage, r.Error)
	}
}
