// Package main is the modelcheck command. Check walks NetLogo model files
// and reports whether GoNetLogo can parse and compile each one.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gonetlogo/pkg/nl"
)

// Model extensions the checker treats as models. Includes and data files
// that sit next to a model are not models.
var modelExts = map[string]bool{
	".nlogo":    true,
	".nlogox":   true,
	".nlogox3d": true,
	".nlogo3d":  true,
}

// Options controls one Check call.
type Options struct {
	// Timeout is the limit for setup and for each go tick.
	Timeout time.Duration
	// Ticks is how many times to call go after a successful setup.
	// go is still called when the model has no setup procedure.
	Ticks int
	// Seed is passed to NewVM. Zero keeps the clock seed.
	Seed int64
	// SkipRun parses and compiles only.
	SkipRun bool
	// MaxSide skips setup and go when either world axis is longer than this.
	// Zero means nl.MaxWorldSide.
	MaxSide int
}

func (o Options) timeout() time.Duration {
	if o.Timeout <= 0 {
		return 4 * time.Second
	}
	return o.Timeout
}

func (o Options) ticks() int {
	if o.Ticks < 0 {
		return 0
	}
	if o.Ticks == 0 {
		return 3
	}
	return o.Ticks
}

func (o Options) maxSide() int {
	if o.MaxSide <= 0 {
		return nl.MaxWorldSide
	}
	return o.MaxSide
}

// Result is the first-error report for one model.
type Result struct {
	Path       string   `json:"path"`
	Ext        string   `json:"ext"`
	ParseOK    bool     `json:"parse_ok"`
	CodeOK     bool     `json:"code_ok"`
	CompileOK  bool     `json:"compile_ok"`
	Stage      string   `json:"stage"`
	Error      string   `json:"error,omitempty"`
	Line       int      `json:"line,omitempty"`
	Snippet    string   `json:"snippet,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	Includes   bool     `json:"includes,omitempty"`
	HasSetup   bool     `json:"has_setup"`
	HasGo      bool     `json:"has_go"`
	Setup      string   `json:"setup"`
	Go         string   `json:"go"`
	GoTicks    int      `json:"go_ticks"`
	WorldW     int      `json:"world_w"`
	WorldH     int      `json:"world_h"`
	// WorldD is the number of patch layers: 1 for a 2D model.
	WorldD    int      `json:"world_d"`
	Notes     []string `json:"notes,omitempty"`
	ElapsedMS int64    `json:"elapsed_ms"`
}

// Walk returns model files under root, in slash-sorted order.
// root itself may be a model file.
func Walk(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !modelExts[strings.ToLower(filepath.Ext(root))] {
			return nil, fmt.Errorf("%s is not a NetLogo model", root)
		}
		return []string{root}, nil
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if modelExts[strings.ToLower(filepath.Ext(path))] {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return filepath.ToSlash(out[i]) < filepath.ToSlash(out[j])
	})
	return out, nil
}

// Check parses and compiles path. When SkipRun is false and the model
// compiled, it also builds a world and runs setup and go when those
// procedures exist.
func Check(path string, opt Options) (res Result) {
	start := time.Now()
	defer func() {
		res.ElapsedMS = time.Since(start).Milliseconds()
		if r := recover(); r != nil {
			res.Stage = "panic"
			res.Error = oneLine(fmt.Sprintf("panic: %v", r))
			if res.HasSetup && (res.Setup == "" || res.Setup == "not-run") {
				res.Setup = "panic"
			}
			if res.HasGo && (res.Go == "" || res.Go == "not-run") {
				res.Go = "panic"
			}
		}
	}()

	res.Path = filepath.ToSlash(path)
	res.Ext = strings.ToLower(filepath.Ext(path))
	res.Setup = "absent"
	res.Go = "absent"

	b, err := os.ReadFile(path)
	if err != nil {
		res.Stage = "parse"
		res.Error = oneLine(err.Error())
		return res
	}
	text := string(b)
	// Fall back to the raw file when the code section cannot be split out.
	// A successful parse replaces this with the code tab so Info-tab prose
	// does not look like an extensions declaration.
	res.Extensions = extensionsOf(text)
	res.Includes = hasIncludes(text)

	m, err := nl.ParseModel(path, text)
	if err != nil {
		res.Stage = "parse"
		res.Error = oneLine(err.Error())
		return res
	}
	m.Path = path
	res.ParseOK = true
	res.Extensions = extensionsOf(m.Code)
	res.Includes = hasIncludes(m.Code)
	if len(m.Notes) > 0 {
		res.Notes = append([]string(nil), m.Notes...)
	}
	res.WorldW = m.View.MaxPxcor - m.View.MinPxcor + 1
	res.WorldH = m.View.MaxPycor - m.View.MinPycor + 1
	res.WorldD = m.View.Depth()

	prog, err := m.Compile()
	if err != nil {
		res.Stage = "compile"
		res.Error = oneLine(err.Error())
		res.Line, res.Snippet = snippetAt(m.Code, res.Error)
		return res
	}
	res.CodeOK = true
	setupAction := findSetupAction(prog, m.Widgets)
	res.HasSetup = setupAction != ""
	goAction := findGoAction(prog, m.Widgets)
	res.HasGo = goAction != ""

	if err := compileWidgets(prog, m.Widgets); err != nil {
		res.Stage = "widget"
		res.Error = oneLine(err.msg)
		res.Line, res.Snippet = snippetAt(err.src, res.Error)
		if res.HasSetup {
			res.Setup = "not-run"
		}
		if res.HasGo {
			res.Go = "not-run"
		}
		return res
	}
	res.CompileOK = true
	res.Stage = "ok"

	if reason, ok := worldRunnable(m.View, opt.maxSide()); !ok {
		res.Stage = "world"
		res.Error = reason
		if res.HasSetup {
			res.Setup = "skipped"
		}
		if res.HasGo {
			res.Go = "skipped"
		}
		return res
	}
	if opt.SkipRun {
		markNotRun(&res)
		return res
	}

	vm, err := m.NewVM(opt.Seed)
	if err != nil {
		res.Stage = "vm"
		res.Error = oneLine(err.Error())
		res.Setup = "skipped"
		res.Go = "skipped"
		return res
	}
	defer vm.Close()
	runModel(vm, &res, opt, setupAction, goAction)
	if res.Line == 0 && res.Error != "" {
		res.Line, res.Snippet = snippetAt(m.Code, res.Error)
	}
	return res
}

func markNotRun(res *Result) {
	if res.HasSetup {
		res.Setup = "not-run"
	}
	if res.HasGo {
		res.Go = "not-run"
	}
}

func worldRunnable(v nl.ViewSpec, maxSide int) (string, bool) {
	w := v.MaxPxcor - v.MinPxcor + 1
	h := v.MaxPycor - v.MinPycor + 1
	if v.MaxPxcor < v.MinPxcor || v.MaxPycor < v.MinPycor || w < 1 || h < 1 {
		return "the world is empty", false
	}
	if w > maxSide || h > maxSide {
		return fmt.Sprintf("world is %dx%d, GoNetLogo allows at most %d patches on a side", w, h, maxSide), false
	}
	if v.Is3D {
		d := v.Depth()
		if d < 1 {
			return "the world is empty", false
		}
		if d > maxSide {
			return fmt.Sprintf("world is %dx%dx%d, GoNetLogo allows at most %d patches on a side", w, h, d, maxSide), false
		}
		if err := nl.ValidWorld3D(v.MinPxcor, v.MaxPxcor, v.MinPycor, v.MaxPycor, v.MinPzcor, v.MaxPzcor); err != nil {
			return fmt.Sprintf("world is %dx%dx%d: %s", w, h, d, err.Error()), false
		}
	}
	return "", true
}

func isCommand(prog *nl.Program, name string) bool {
	p := prog.Proc(name)
	return p != nil && !p.IsReporter
}

func findSetupAction(prog *nl.Program, widgets []nl.Widget) string {
	p := prog.Proc("setup")
	if p != nil && !p.IsReporter && len(p.Params) == 0 {
		return "setup"
	}
	best := ""
	bestScore := 0
	for _, w := range widgets {
		if w.Kind != "button" {
			continue
		}
		src := strings.TrimSpace(w.Source)
		score := setupButtonScore(prog, src, w.Display)
		if score > bestScore {
			best = src
			bestScore = score
		}
	}
	if best != "" {
		return best
	}
	if p != nil && !p.IsReporter {
		return "setup"
	}
	return ""
}

// setupButtonScore ranks a button as setup. The command has to start
// with setup, or the display has to be exactly "setup". A display that
// merely contains that substring is not enough.
func setupButtonScore(prog *nl.Program, src, display string) int {
	fields := strings.Fields(src)
	if len(fields) == 0 {
		return 0
	}
	first := strings.ToLower(fields[0])
	sourceSetup := first == "setup" || strings.HasPrefix(first, "setup-") || strings.HasPrefix(first, "setup_")
	exactDisplay := strings.EqualFold(strings.TrimSpace(display), "setup")
	score := 0
	if sourceSetup {
		score = 3
		if len(fields) == 1 && isCommand(prog, first) {
			score = 4
		}
	}
	if exactDisplay && score < 2 {
		score = 2
	}
	return score
}

func findGoAction(prog *nl.Program, widgets []nl.Widget) string {
	// A button or procedure named go wins. Several 3D code examples have
	// no go and instead loop on a forever button (roll-right, go-distance).
	// That button is the run the checker should exercise.
	if src := foreverAction(widgets, true); src != "" {
		return src
	}
	if isCommand(prog, "go") {
		return "go"
	}
	return foreverAction(widgets, false)
}

func foreverAction(widgets []nl.Widget, namedGo bool) string {
	for _, w := range widgets {
		if w.Kind != "button" {
			continue
		}
		src := strings.TrimSpace(w.Source)
		if src == "" {
			continue
		}
		named := src == "go" || strings.EqualFold(strings.TrimSpace(w.Display), "go")
		if namedGo {
			if !named {
				continue
			}
		} else if !w.Forever || named {
			continue
		}
		if strings.EqualFold(w.Agent, "turtle") || strings.EqualFold(w.Agent, "turtles") {
			return "ask turtles [ " + src + " ]"
		}
		if strings.EqualFold(w.Agent, "patch") || strings.EqualFold(w.Agent, "patches") {
			return "ask patches [ " + src + " ]"
		}
		return src
	}
	return ""
}

type widgetError struct {
	msg string
	src string
}

func compileWidgets(prog *nl.Program, widgets []nl.Widget) *widgetError {
	for i := range widgets {
		w := &widgets[i]
		switch w.Kind {
		case "button":
			src := strings.TrimSpace(w.Source)
			if src == "" {
				continue
			}
			if _, err := nl.CompileButton(prog, w.Source, w.Agent); err != nil {
				return &widgetError{msg: fmt.Sprintf("button %q: %s", widgetName(w), oneLine(err.Error())), src: w.Source}
			}
		case "monitor":
			src := strings.TrimSpace(w.Source)
			if src == "" {
				continue
			}
			if _, err := nl.ParseReporter(w.Source, prog); err != nil {
				return &widgetError{msg: fmt.Sprintf("monitor %q: %s", widgetName(w), oneLine(err.Error())), src: w.Source}
			}
		case "plot":
			if err := compilePlotSource(prog, w); err != nil {
				return err
			}
		}
	}
	return nil
}

func compilePlotSource(prog *nl.Program, w *nl.Widget) *widgetError {
	name := widgetName(w)
	check := func(part, src string) *widgetError {
		src = strings.TrimSpace(src)
		if src == "" {
			return nil
		}
		if _, err := nl.ParseCommands(src, prog); err != nil {
			return &widgetError{msg: fmt.Sprintf("plot %q %s: %s", name, part, oneLine(err.Error())), src: src}
		}
		return nil
	}
	if err := check("setup", w.PlotSetup); err != nil {
		return err
	}
	if err := check("update", w.PlotUpdate); err != nil {
		return err
	}
	for i := range w.Pens {
		pen := w.Pens[i].Name
		if strings.TrimSpace(pen) == "" {
			pen = "default"
		}
		if err := check(fmt.Sprintf("pen %q setup", pen), w.Pens[i].Setup); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("pen %q update", pen), w.Pens[i].Update); err != nil {
			return err
		}
	}
	return nil
}

func widgetName(w *nl.Widget) string {
	if s := strings.TrimSpace(w.Display); s != "" {
		return oneLine(s)
	}
	s := strings.TrimSpace(w.Source)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return w.Kind
	}
	return oneLine(s)
}

func runModel(vm *nl.VM, res *Result, opt Options, setupAction, goAction string) {
	if res.HasSetup {
		status, err := callAction(vm, setupAction, opt.timeout())
		res.Setup = status
		if status != "ok" {
			res.Stage = "setup"
			res.Error = callErrText(status, err)
			res.Go = "skipped"
			return
		}
	}
	if !res.HasGo {
		res.Stage = "ok"
		return
	}
	ticks := opt.ticks()
	for i := 0; i < ticks; i++ {
		status, err := callAction(vm, goAction, opt.timeout())
		if status == "stopped" {
			res.Go = "stopped"
			res.GoTicks = i + 1
			res.Stage = "ok"
			return
		}
		if status != "ok" {
			res.Go = status
			res.GoTicks = i
			res.Stage = "go"
			res.Error = callErrText(status, err)
			return
		}
		res.GoTicks = i + 1
	}
	res.Go = "ok"
	res.Stage = "ok"
}

func callErrText(status string, err error) string {
	if err == nil {
		return status
	}
	return oneLine(err.Error())
}

func callAction(vm *nl.VM, action string, timeout time.Duration) (status string, err error) {
	type out struct {
		stopped bool
		err     error
		pan     any
	}
	ch := make(chan out, 1)
	vm.ClearHalt()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- out{pan: r}
			}
		}()
		trimmed := strings.TrimSpace(action)
		var stopped bool
		var callErr error
		if p := vm.Prog.Proc(trimmed); p != nil && !p.IsReporter && len(p.Params) == 0 {
			stopped, callErr = vm.Call(trimmed)
		} else {
			_, callErr = vm.ExecuteLine(trimmed)
		}
		ch <- out{stopped: stopped, err: callErr}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case o := <-ch:
		return classifyCall(vm, o.stopped, o.err, o.pan, timeout)
	case <-timer.C:
		vm.RequestHalt()
		grace := time.NewTimer(2 * time.Second)
		defer grace.Stop()
		select {
		case o := <-ch:
			return classifyCall(vm, o.stopped, o.err, o.pan, timeout)
		case <-grace.C:
			return "timeout", fmt.Errorf("timed out after %s", timeout)
		}
	}
}

func callProc(vm *nl.VM, name string, timeout time.Duration) (status string, err error) {
	type out struct {
		stopped bool
		err     error
		pan     any
	}
	ch := make(chan out, 1)
	vm.ClearHalt()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- out{pan: r}
			}
		}()
		stopped, callErr := vm.Call(name)
		ch <- out{stopped: stopped, err: callErr}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case o := <-ch:
		return classifyCall(vm, o.stopped, o.err, o.pan, timeout)
	case <-timer.C:
		vm.RequestHalt()
		grace := time.NewTimer(2 * time.Second)
		defer grace.Stop()
		select {
		case o := <-ch:
			return classifyCall(vm, o.stopped, o.err, o.pan, timeout)
		case <-grace.C:
			return "timeout", fmt.Errorf("timed out after %s", timeout)
		}
	}
}

func classifyCall(vm *nl.VM, stopped bool, err error, pan any, timeout time.Duration) (string, error) {
	if pan != nil {
		return "panic", fmt.Errorf("panic: %v", pan)
	}
	if err != nil {
		return "error", err
	}
	if !stopped {
		return "ok", nil
	}
	if vm.Halted() {
		return "timeout", fmt.Errorf("timed out after %s", timeout)
	}
	return "stopped", nil
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > 400 {
		s = s[:400] + "..."
	}
	return s
}

var linePrefix = regexp.MustCompile(`line (\d+):`)

func snippetAt(code, msg string) (int, string) {
	m := linePrefix.FindStringSubmatch(msg)
	if m == nil {
		return 0, ""
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, ""
	}
	lines := strings.Split(code, "\n")
	if n > len(lines) {
		return n, ""
	}
	s := strings.TrimSpace(lines[n-1])
	if len(s) > 180 {
		s = s[:180] + "..."
	}
	return n, s
}

var extDecl = regexp.MustCompile(`(?is)extensions\s*\[(.*?)\]`)

func extensionsOf(code string) []string {
	code = stripLineComments(code)
	seen := map[string]bool{}
	var out []string
	for _, m := range extDecl.FindAllStringSubmatch(code, -1) {
		for _, f := range strings.Fields(m[1]) {
			f = strings.ToLower(strings.Trim(f, "[]"))
			if f == "" || seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func hasIncludes(code string) bool {
	return strings.Contains(strings.ToLower(stripLineComments(code)), "__includes")
}

func stripLineComments(code string) string {
	var b strings.Builder
	for _, line := range strings.Split(code, "\n") {
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// Summary counts a slice of results. SetupOK and GoOK count only models
// that ran those procedures without error. GoOK includes a go that stopped
// itself before the tick budget. CompileFail is every model that did not
// compile, which is the main failure count.
type Summary struct {
	Total        int `json:"total"`
	ParseOK      int `json:"parse_ok"`
	CodeOK       int `json:"code_ok"`
	CompileOK    int `json:"compile_ok"`
	CompileFail  int `json:"compile_fail"`
	SetupOK      int `json:"setup_ok"`
	SetupAbsent  int `json:"setup_absent"`
	SetupFail    int `json:"setup_fail"`
	SetupTimeout int `json:"setup_timeout"`
	SetupSkipped int `json:"setup_skipped"`
	GoOK         int `json:"go_ok"`
	GoStopped    int `json:"go_stopped"`
	GoAbsent     int `json:"go_absent"`
	GoFail       int `json:"go_fail"`
	GoTimeout    int `json:"go_timeout"`
	GoSkipped    int `json:"go_skipped"`
	WorldSkipped int `json:"world_skipped"`
}

// Summarize aggregates results.
func Summarize(rs []Result) Summary {
	var s Summary
	s.Total = len(rs)
	for _, r := range rs {
		if r.ParseOK {
			s.ParseOK++
		}
		if r.CodeOK {
			s.CodeOK++
		}
		if r.CompileOK {
			s.CompileOK++
		} else {
			s.CompileFail++
		}
		switch r.Setup {
		case "ok":
			s.SetupOK++
		case "absent":
			s.SetupAbsent++
		case "timeout":
			s.SetupTimeout++
		case "skipped":
			s.SetupSkipped++
		case "not-run":
		default:
			if r.Setup != "" {
				s.SetupFail++
			}
		}
		switch r.Go {
		case "ok":
			s.GoOK++
		case "stopped":
			s.GoOK++
			s.GoStopped++
		case "absent":
			s.GoAbsent++
		case "timeout":
			s.GoTimeout++
		case "skipped":
			s.GoSkipped++
		case "not-run":
		default:
			if r.Go != "" {
				s.GoFail++
			}
		}
		if r.Stage == "world" {
			s.WorldSkipped++
		}
	}
	return s
}

// Signature is a short root-cause key for the first error, used to print
// the raw frequency table. The written report groups these further.
func Signature(r Result) string {
	if r.CompileOK && r.Stage == "ok" {
		return ""
	}
	msg := r.Error
	if i := strings.Index(msg, "line "); i >= 0 {
		if j := strings.Index(msg[i:], ": "); j >= 0 {
			msg = msg[i+j+2:]
		}
	}
	msg = strings.TrimSpace(msg)
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "non-standard character"):
		return charSignature(r, low)
	case strings.Contains(low, "nothing named"):
		n := named(low)
		// extension:primitive is one identifier. An unimplemented
		// extension still fails here; group it with the other
		// extension failures instead of as a bare missing name.
		if i := strings.Index(n, ":"); i > 0 {
			return "extension " + n[:i]
		}
		return "missing " + n
	case strings.Contains(low, "expected a declaration or procedure"):
		return "declaration " + gotToken(low)
	case strings.Contains(low, "system-dynamics"):
		return "system-dynamics"
	case r.Stage == "parse":
		return "parse " + trimSig(msg)
	case r.Stage == "world":
		return "world too large"
	case r.Stage == "vm":
		return "vm " + trimSig(msg)
	case r.Stage == "setup" || r.Stage == "go":
		return r.Stage + " " + trimSig(msg)
	default:
		return trimSig(msg)
	}
}

var namedRe = regexp.MustCompile(`nothing named\s+([^\s]+)`)
var declRe = regexp.MustCompile(`got\s+(\S+)`)

// Hyphen is included so import-a: and view2.5d: stay one extension name.
var colonRe = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_.?-]*)\s*:`)

func named(low string) string {
	m := namedRe.FindStringSubmatch(low)
	if m == nil {
		return "?"
	}
	return strings.Trim(m[1], `"'`)
}

func gotToken(low string) string {
	m := declRe.FindStringSubmatch(low)
	if m == nil {
		return "?"
	}
	return strings.Trim(m[1], `"'`)
}

func charSignature(r Result, low string) string {
	snip := r.Snippet
	if m := colonRe.FindStringSubmatch(snip); m != nil {
		return "extension " + strings.ToLower(m[1])
	}
	for _, ext := range r.Extensions {
		if strings.Contains(strings.ToLower(snip), ext) && strings.Contains(ext, ".") {
			return "extension " + ext
		}
	}
	if strings.Contains(low, "':'") || strings.Contains(snip, ":") {
		return "extension primitive"
	}
	if strings.Contains(strings.ToLower(snip), "view2.5d") {
		return "extension view2.5d"
	}
	return "lexer " + trimSig(low)
}

func trimSig(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

// TopSignatures returns the most common first-error signatures, highest
// count first. Models that compiled and ran cleanly are omitted. Ties
// break by signature text.
func TopSignatures(rs []Result, n int) []SigCount {
	counts := map[string]int{}
	for _, r := range rs {
		sig := Signature(r)
		if sig == "" {
			continue
		}
		counts[sig]++
	}
	out := make([]SigCount, 0, len(counts))
	for sig, c := range counts {
		out = append(out, SigCount{Signature: sig, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Signature < out[j].Signature
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// SigCount is one row of the raw frequency table.
type SigCount struct {
	Signature string `json:"signature"`
	Count     int    `json:"count"`
}
