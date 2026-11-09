package nl

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// The sr extension drives a real R process, the same requirement as
// NetLogo's Simple R extension: R must be installed and on PATH.
//
// Not covered, because there is no display and the packages are not part of
// base R:
//   - the interactive console (sr:show-console succeeds and does not open a window)
//   - an on-screen plot device (plots are drawn by R on a null PDF device)
//   - contributed packages such as spatstat and vegan; library() fails with R's own error
//
// Results are R's. A missing R or a failed call is an error, not a stand-in value.

var srCommands = map[string]primSpec{
	"sr:setup":                {argc: 0},
	"sr:run":                  {argc: 1, variadic: true},
	"sr:set":                  {argc: 2},
	"sr:set-agent":            {argc: 3, variadic: true},
	"sr:set-agent-data-frame": {argc: 3, variadic: true},
	"sr:set-data-frame":       {argc: 3, variadic: true},
	"sr:set-list":             {argc: 2, variadic: true},
	"sr:set-named-list":       {argc: 3, variadic: true},
	"sr:set-plot-device":      {argc: 0},
	"sr:show-console":         {argc: 0},
}

var srReporters = map[string]primSpec{
	"sr:runresult": {argc: 1, prec: precNormal},
}

var srZero = map[string]bool{
	"sr:r-home": true,
}

func (p *parser) installSR() {
	p.addCommands(srCommands)
	p.addReporters(srReporters)
}

const srDriver = `
options(echo = FALSE)
try(Sys.setlocale("LC_NUMERIC", "C"), silent = TRUE)
options(OutDec = ".")
options(device = function(...) {
  grDevices::pdf(NULL)
})

sr_bytes <- function(s) {
  s <- enc2utf8(paste(s, collapse = ""))
  nchar(s, type = "bytes")
}

sr_send <- function(s) {
  cat(s)
  flush(stdout())
}

sr_fail <- function(msg) {
  msg <- gsub("[\r\n]+", " ", paste(as.character(msg), collapse = " "))
  msg <- enc2utf8(msg)
  sr_send(paste0("ERR ", sr_bytes(msg), "\n", msg))
}

sr_write_scalar <- function(x) {
  if (is.character(x)) {
    if (length(x) == 0 || is.na(x[[1]])) {
      sr_send("NA\n")
    } else {
      s <- enc2utf8(x[[1]])
      sr_send(paste0("STR ", sr_bytes(s), "\n", s, "\n"))
    }
    return()
  }
  if (is.logical(x)) {
    if (length(x) == 0 || is.na(x[[1]])) sr_send("NA\n")
    else if (isTRUE(x[[1]])) sr_send("BOOL 1\n")
    else sr_send("BOOL 0\n")
    return()
  }
  if (is.numeric(x)) {
    if (length(x) == 0) {
      sr_send("NULL\n")
      return()
    }
    v <- x[[1]]
    if (is.nan(v)) sr_send("NAN\n")
    else if (is.na(v)) sr_send("NA\n")
    else if (is.infinite(v)) sr_send(if (v > 0) "INF 1\n" else "INF -1\n")
    else sr_send(paste0("NUM ", sprintf("%.17g", v), "\n"))
    return()
  }
  s <- enc2utf8(paste(as.character(x), collapse = " "))
  sr_send(paste0("STR ", sr_bytes(s), "\n", s, "\n"))
}

sr_write <- function(x) {
  if (is.null(x)) {
    sr_send("NULL\n")
    return()
  }
  if (is.data.frame(x)) {
    sr_send(paste0("DF ", ncol(x), "\n"))
    nms <- names(x)
    for (i in seq_along(nms)) {
      nm <- enc2utf8(nms[[i]])
      sr_send(paste0("COL ", sr_bytes(nm), "\n", nm, "\n"))
      col <- x[[i]]
      if (is.factor(col)) col <- as.character(col)
      sr_write(col)
    }
    return()
  }
  if (is.matrix(x)) {
    sr_write(as.vector(x))
    return()
  }
  if (is.factor(x)) {
    sr_write(as.character(x))
    return()
  }
  if (is.list(x)) {
    nms <- names(x)
    sr_send(paste0("LIST ", length(x), "\n"))
    for (i in seq_along(x)) {
      nm <- ""
      if (!is.null(nms)) nm <- nms[[i]]
      nm <- enc2utf8(nm)
      sr_send(paste0("NM ", sr_bytes(nm), "\n", nm, "\n"))
      sr_write(x[[i]])
    }
    return()
  }
  if (is.atomic(x)) {
    nms <- names(x)
    if (!is.null(nms) && any(nzchar(nms))) {
      sr_send(paste0("NVEC ", length(x), "\n"))
      for (i in seq_along(x)) {
        nm <- nms[[i]]
        if (is.na(nm)) nm <- ""
        nm <- enc2utf8(nm)
        sr_send(paste0("NM ", sr_bytes(nm), "\n", nm, "\n"))
        sr_write_scalar(x[i])
      }
      return()
    }
    if (length(x) == 1) {
      sr_write_scalar(x)
      return()
    }
    sr_send(paste0("VEC ", length(x), "\n"))
    for (i in seq_along(x)) sr_write_scalar(x[i])
    return()
  }
  s <- enc2utf8(paste(capture.output(print(x)), collapse = "\n"))
  sr_send(paste0("STR ", sr_bytes(s), "\n", s, "\n"))
}

con <- file("stdin", open = "rb", blocking = TRUE)
repeat {
  hdr <- readLines(con, n = 1, warn = FALSE)
  if (length(hdr) == 0) quit(save = "no", status = 0)
  if (identical(hdr, "QUIT")) quit(save = "no", status = 0)
  parts <- strsplit(hdr, " ", fixed = TRUE)[[1]]
  if (length(parts) < 2) {
    sr_fail(paste("bad header", hdr))
    next
  }
  op <- parts[[1]]
  n <- suppressWarnings(as.integer(parts[[2]]))
  if (is.na(n) || n < 0) {
    sr_fail("bad length")
    next
  }
  lines <- character()
  if (n > 0) {
    lines <- readLines(con, n = n, warn = FALSE)
    if (length(lines) < n) {
      sr_fail("incomplete R message")
      quit(save = "no", status = 1)
    }
  }
  code <- paste(lines, collapse = "\n")
  err <- NULL
  warns <- character()
  val <- NULL
  out <- character()
  withCallingHandlers(
    tryCatch({
      out <- capture.output(type = "output", {
        val <- eval(parse(text = code), envir = .GlobalEnv)
      })
    }, error = function(e) {
      err <<- conditionMessage(e)
    }),
    warning = function(w) {
      warns <<- c(warns, conditionMessage(w))
      invokeRestart("muffleWarning")
    }
  )
  if (!is.null(err)) {
    sr_fail(err)
    next
  }
  if (length(warns)) {
    out <- c(paste("Warning from R environment:", warns), out)
  }
  text <- enc2utf8(paste(out, collapse = "\n"))
  sr_send(paste0("OUT ", sr_bytes(text), "\n"))
  if (nzchar(text)) cat(text)
  if (identical(op, "EXPR")) {
    cat("VAL\n")
    sr_write(val)
    cat("OK\n")
  } else {
    cat("OK\n")
  }
  flush(stdout())
}
`

type srSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	data   chan []byte
	done   chan struct{}
	stop   sync.Once
	hold   []byte
	stderr string
	errMu  sync.Mutex
}

func (vm *VM) srCommand(name string, args []Value, pos Pos) (error, bool) {
	switch name {
	case "sr:setup":
		return vm.srSetup(), true
	case "sr:show-console":
		if _, err := vm.srReady(); err != nil {
			return err, true
		}
		// Headless run: the R session stays available through sr:run.
		return nil, true
	case "sr:set-plot-device":
		_, out, err := vm.srEval("RUN", "dev.new()")
		vm.srEmit(out)
		return err, true
	case "sr:run":
		code, err := srCode(args, "sr:run")
		if err != nil {
			return err, true
		}
		_, out, err := vm.srEval("RUN", code)
		vm.srEmit(out)
		return err, true
	case "sr:set":
		return vm.srSet(args), true
	case "sr:set-agent":
		return vm.srSetAgents(args, false), true
	case "sr:set-agent-data-frame":
		return vm.srSetAgents(args, true), true
	case "sr:set-data-frame":
		return vm.srSetFrame(args, true), true
	case "sr:set-named-list":
		return vm.srSetFrame(args, false), true
	case "sr:set-list":
		return vm.srSetList(args), true
	default:
		return nil, false
	}
}

func (vm *VM) srReporter(name string, args []Value, pos Pos) (Value, error) {
	if name != "sr:runresult" {
		return Value{}, fmt.Errorf("primitive %s is not implemented yet", name)
	}
	if len(args) != 1 || args[0].K != KindString {
		return Value{}, fmt.Errorf("sr:runresult expected a string")
	}
	v, out, err := vm.srEval("EXPR", args[0].S)
	vm.srEmit(out)
	return v, err
}

func (vm *VM) srZero(name string, pos Pos) (Value, error) {
	if name != "sr:r-home" {
		return Value{}, errorAt(pos, "primitive %s is not implemented yet", name)
	}
	v, out, err := vm.srEval("EXPR", "R.home()")
	vm.srEmit(out)
	return v, err
}

func (vm *VM) srSetup() error {
	if vm.sr != nil {
		vm.sr.close()
		vm.sr = nil
	}
	rbin, err := exec.LookPath("R")
	if err != nil {
		return fmt.Errorf("sr:setup could not find R; install R and put the R executable on PATH")
	}
	f, err := os.CreateTemp("", "gonetlogo-sr-*.R")
	if err != nil {
		return err
	}
	path := f.Name()
	if _, err := f.WriteString(srDriver); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	f.Close()
	defer os.Remove(path)

	cmd := exec.Command(rbin, "--vanilla", "--quiet", "--slave", "--no-restore", "-e", fmt.Sprintf("source('%s')", path))
	if vm.ModelDir != "" {
		cmd.Dir = vm.ModelDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sr:setup could not start R: %s", err.Error())
	}
	s := &srSession{
		cmd:   cmd,
		stdin: stdin,
		data:  make(chan []byte, 32),
		done:  make(chan struct{}),
	}
	go func() { _ = cmd.Wait() }()
	go func() {
		b, _ := io.ReadAll(stderr)
		if len(b) > 4000 {
			b = b[len(b)-4000:]
		}
		s.errMu.Lock()
		s.stderr = string(b)
		s.errMu.Unlock()
	}()
	go func() {
		defer close(s.data)
		buf := make([]byte, 8192)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case s.data <- chunk:
				case <-s.done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	vm.sr = s
	v, _, err := s.eval(vm, "EXPR", "1")
	if err != nil || v.K != KindNum || v.N != 1 {
		msg := "sr:setup could not start an R session"
		if err != nil {
			msg = err.Error()
		}
		s.errMu.Lock()
		if s.stderr != "" {
			msg += ": " + oneLineSR(s.stderr)
		}
		s.errMu.Unlock()
		s.close()
		vm.sr = nil
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func oneLineSR(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (s *srSession) close() {
	if s == nil {
		return
	}
	s.stop.Do(func() {
		close(s.done)
		if s.stdin != nil {
			_, _ = io.WriteString(s.stdin, "QUIT 0\n")
			s.stdin.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
}

// Close releases the R process started by sr:setup.
func (vm *VM) Close() {
	if vm == nil || vm.sr == nil {
		return
	}
	vm.sr.close()
	vm.sr = nil
}

func (vm *VM) srReady() (*srSession, error) {
	if vm.sr == nil {
		return nil, fmt.Errorf("sr:setup must be called before using other sr primitives")
	}
	return vm.sr, nil
}

func (vm *VM) srEval(op, code string) (Value, string, error) {
	s, err := vm.srReady()
	if err != nil {
		return Value{}, "", err
	}
	return s.eval(vm, op, code)
}

func (s *srSession) eval(vm *VM, op, code string) (Value, string, error) {
	lines := strings.Split(code, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d\n", op, len(lines))
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	if _, err := s.stdin.Write([]byte(b.String())); err != nil {
		return Value{}, "", fmt.Errorf("R session ended")
	}
	line, err := s.readLine(vm)
	if err != nil {
		return Value{}, "", err
	}
	if strings.HasPrefix(line, "ERR ") {
		n, err := srCount(line)
		if err != nil {
			return Value{}, "", err
		}
		msg, err := s.readFull(vm, n)
		if err != nil {
			return Value{}, "", err
		}
		return Value{}, "", fmt.Errorf("%s", string(msg))
	}
	if !strings.HasPrefix(line, "OUT ") {
		return Value{}, "", fmt.Errorf("R protocol error: %s", line)
	}
	n, err := srCount(line)
	if err != nil {
		return Value{}, "", err
	}
	out, err := s.readFull(vm, n)
	if err != nil {
		return Value{}, "", err
	}
	next, err := s.readLine(vm)
	if err != nil {
		return Value{}, "", err
	}
	if next == "OK" {
		return Value{}, string(out), nil
	}
	if next != "VAL" {
		return Value{}, string(out), fmt.Errorf("R protocol error: %s", next)
	}
	v, err := s.readValue(vm)
	if err != nil {
		return Value{}, string(out), err
	}
	end, err := s.readLine(vm)
	if err != nil {
		return Value{}, string(out), err
	}
	if end != "OK" {
		return Value{}, string(out), fmt.Errorf("R protocol error: %s", end)
	}
	return v, string(out), nil
}

func srCount(line string) (int, error) {
	i := strings.LastIndex(line, " ")
	if i < 0 {
		return 0, fmt.Errorf("R protocol error: %s", line)
	}
	n, err := strconv.Atoi(line[i+1:])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("R protocol error: %s", line)
	}
	return n, nil
}

func (s *srSession) readLine(vm *VM) (string, error) {
	for {
		if i := bytes.IndexByte(s.hold, '\n'); i >= 0 {
			line := string(s.hold[:i])
			s.hold = s.hold[i+1:]
			return line, nil
		}
		chunk, err := s.waitChunk(vm)
		if err != nil {
			return "", err
		}
		s.hold = append(s.hold, chunk...)
	}
}

func (s *srSession) readFull(vm *VM, n int) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	for len(s.hold) < n {
		chunk, err := s.waitChunk(vm)
		if err != nil {
			return nil, err
		}
		s.hold = append(s.hold, chunk...)
	}
	out := append([]byte(nil), s.hold[:n]...)
	s.hold = s.hold[n:]
	return out, nil
}

func (s *srSession) waitChunk(vm *VM) ([]byte, error) {
	deadline := time.Now().Add(60 * time.Second)
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case chunk, ok := <-s.data:
			if !ok {
				return nil, fmt.Errorf("R session ended")
			}
			return chunk, nil
		case <-s.done:
			return nil, fmt.Errorf("R session ended")
		case <-timer.C:
			if vm != nil && vm.Halted() {
				s.close()
				return nil, fmt.Errorf("R session halted")
			}
			if time.Now().After(deadline) {
				s.close()
				return nil, fmt.Errorf("R session timed out")
			}
			timer.Reset(100 * time.Millisecond)
		}
	}
}

func (s *srSession) readValue(vm *VM) (Value, error) {
	line, err := s.readLine(vm)
	if err != nil {
		return Value{}, err
	}
	switch {
	case line == "NULL":
		return List(), nil
	case line == "NA":
		return Str("NA"), nil
	case line == "NAN":
		return Str("NaN"), nil
	case strings.HasPrefix(line, "INF "):
		if strings.HasSuffix(line, "-1") {
			return Str("-Inf"), nil
		}
		return Str("Inf"), nil
	case strings.HasPrefix(line, "BOOL "):
		return Bool(strings.HasSuffix(line, "1")), nil
	case strings.HasPrefix(line, "NUM "):
		n, err := strconv.ParseFloat(strings.TrimSpace(line[4:]), 64)
		if err != nil {
			return Value{}, fmt.Errorf("R protocol error: %s", line)
		}
		return Num(n), nil
	case strings.HasPrefix(line, "STR "):
		return s.readCountedString(vm, line)
	case strings.HasPrefix(line, "VEC "):
		n, err := srCount(line)
		if err != nil {
			return Value{}, err
		}
		items := make([]Value, n)
		for i := 0; i < n; i++ {
			items[i], err = s.readValue(vm)
			if err != nil {
				return Value{}, err
			}
		}
		return List(items...), nil
	case strings.HasPrefix(line, "NVEC "), strings.HasPrefix(line, "LIST "):
		n, err := srCount(line)
		if err != nil {
			return Value{}, err
		}
		namedVec := strings.HasPrefix(line, "NVEC ")
		items := make([]Value, n)
		for i := 0; i < n; i++ {
			nameV, err := s.readCountedString(vm, "")
			if err != nil {
				return Value{}, err
			}
			val, err := s.readValue(vm)
			if err != nil {
				return Value{}, err
			}
			if namedVec || nameV.S != "" {
				items[i] = List(nameV, val)
			} else {
				items[i] = val
			}
		}
		return List(items...), nil
	case strings.HasPrefix(line, "DF "):
		n, err := srCount(line)
		if err != nil {
			return Value{}, err
		}
		items := make([]Value, n)
		for i := 0; i < n; i++ {
			hdr, err := s.readLine(vm)
			if err != nil {
				return Value{}, err
			}
			name, err := s.readCountedString(vm, hdr)
			if err != nil {
				return Value{}, err
			}
			val, err := s.readValue(vm)
			if err != nil {
				return Value{}, err
			}
			items[i] = List(name, val)
		}
		return List(items...), nil
	default:
		return Value{}, fmt.Errorf("R protocol error: %s", line)
	}
}

func (s *srSession) readCountedString(vm *VM, header string) (Value, error) {
	if header == "" {
		var err error
		header, err = s.readLine(vm)
		if err != nil {
			return Value{}, err
		}
	}
	n, err := srCount(header)
	if err != nil {
		return Value{}, err
	}
	b, err := s.readFull(vm, n)
	if err != nil {
		return Value{}, err
	}
	nl, err := s.readFull(vm, 1)
	if err != nil {
		return Value{}, err
	}
	if len(nl) != 1 || nl[0] != '\n' {
		return Value{}, fmt.Errorf("R protocol error: string was not terminated")
	}
	return Str(string(b)), nil
}

func (vm *VM) srEmit(out string) {
	if out == "" {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		vm.emit(line)
	}
}

func srCode(args []Value, name string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("%s expected a string", name)
	}
	var b strings.Builder
	for i, a := range args {
		if a.K != KindString {
			return "", fmt.Errorf("%s expected a string", name)
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(a.S)
	}
	return b.String(), nil
}

func (vm *VM) srSet(args []Value) error {
	if len(args) != 2 || args[0].K != KindString {
		return fmt.Errorf("sr:set expected a string and a value")
	}
	lit, err := vm.rLiteral(args[1], 0)
	if err != nil {
		return err
	}
	code := fmt.Sprintf("assign(%s, %s, envir = .GlobalEnv)", rString(args[0].S), lit)
	_, out, err := vm.srEval("RUN", code)
	vm.srEmit(out)
	return err
}

func (vm *VM) srSetAgents(args []Value, frame bool) error {
	name := "sr:set-agent"
	if frame {
		name = "sr:set-agent-data-frame"
	}
	if len(args) < 3 || args[0].K != KindString {
		return fmt.Errorf("%s expected a name, an agent or agentset, and a variable name", name)
	}
	ags, err := srAgents(args[1])
	if err != nil {
		return err
	}
	vars := args[2:]
	cols := make([]string, len(vars))
	names := make([]string, len(vars))
	for i, v := range vars {
		if v.K != KindString {
			return fmt.Errorf("%s expected variable names", name)
		}
		names[i] = v.S
		vals, err := vm.agentColumn(ags, v.S)
		if err != nil {
			return err
		}
		cols[i], err = rColumn(vals)
		if err != nil {
			return err
		}
	}
	code := rBind(args[0].S, names, cols, frame)
	_, out, err := vm.srEval("RUN", code)
	vm.srEmit(out)
	return err
}

func (vm *VM) srSetFrame(args []Value, frame bool) error {
	name := "sr:set-named-list"
	if frame {
		name = "sr:set-data-frame"
	}
	if len(args) < 3 || (len(args)-1)%2 != 0 || args[0].K != KindString {
		return fmt.Errorf("%s expected a name and pairs of column name and value", name)
	}
	var names, cols []string
	for i := 1; i < len(args); i += 2 {
		if args[i].K != KindString {
			return fmt.Errorf("%s expected a column name", name)
		}
		col, err := rColumnValue(args[i+1])
		if err != nil {
			return err
		}
		names = append(names, args[i].S)
		cols = append(cols, col)
	}
	code := rBind(args[0].S, names, cols, frame)
	_, out, err := vm.srEval("RUN", code)
	vm.srEmit(out)
	return err
}

func (vm *VM) srSetList(args []Value) error {
	if len(args) < 2 || args[0].K != KindString {
		return fmt.Errorf("sr:set-list expected a name and a value")
	}
	parts := make([]string, len(args)-1)
	for i, v := range args[1:] {
		lit, err := vm.rLiteral(v, 0)
		if err != nil {
			return err
		}
		parts[i] = lit
	}
	code := fmt.Sprintf("assign(%s, list(%s), envir = .GlobalEnv)", rString(args[0].S), strings.Join(parts, ", "))
	_, out, err := vm.srEval("RUN", code)
	vm.srEmit(out)
	return err
}

func rBind(varName string, names, cols []string, frame bool) string {
	var b strings.Builder
	b.WriteString("assign(")
	b.WriteString(rString(varName))
	b.WriteString(", local({cols <- list(")
	b.WriteString(strings.Join(cols, ", "))
	b.WriteString("); ")
	if frame {
		b.WriteString("df <- as.data.frame(cols, stringsAsFactors = FALSE, check.names = FALSE); names(df) <- c(")
		for i, n := range names {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(rString(n))
		}
		b.WriteString("); df")
	} else {
		b.WriteString("stats::setNames(cols, c(")
		for i, n := range names {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(rString(n))
		}
		b.WriteString("))")
	}
	b.WriteString("}), envir = .GlobalEnv)")
	return b.String()
}

func srAgents(v Value) ([]Agent, error) {
	switch v.K {
	case KindAgent:
		if v.A == nil || v.A.IsDead() {
			return nil, fmt.Errorf("that agent is dead")
		}
		return []Agent{v.A}, nil
	case KindAgentset:
		if v.Set == nil {
			return nil, fmt.Errorf("sr expected an agent or agentset")
		}
		return v.Set.Alive(), nil
	default:
		return nil, fmt.Errorf("sr expected an agent or agentset, got %s", v.TypeName())
	}
}

func (vm *VM) agentColumn(ags []Agent, name string) ([]Value, error) {
	saved := vm.self
	defer func() { vm.self = saved }()
	out := make([]Value, len(ags))
	for i, a := range ags {
		vm.self = a
		v, ok, err := vm.getAgentVar(name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("nothing named %s has been defined", strings.ToUpper(name))
		}
		out[i] = v
	}
	return out, nil
}

func (vm *VM) rLiteral(v Value, depth int) (string, error) {
	switch v.K {
	case KindNum:
		return rNum(v.N), nil
	case KindBool:
		if v.B {
			return "TRUE", nil
		}
		return "FALSE", nil
	case KindString:
		return rString(v.S), nil
	case KindNobody:
		return "NA", nil
	case KindList:
		return rListLiteral(v.L)
	case KindAgent:
		if v.A == nil || v.A.IsDead() {
			return "NA", nil
		}
		if depth > 0 {
			return rString(v.A.AgentString()), nil
		}
		return vm.rAgent(v.A)
	case KindAgentset:
		if depth > 0 || v.Set == nil {
			return rString(v.Format(false)), nil
		}
		ags := v.Set.Alive()
		if len(ags) == 0 {
			return "list()", nil
		}
		parts := make([]string, len(ags))
		for i, a := range ags {
			s, err := vm.rAgent(a)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "list(" + strings.Join(parts, ", ") + ")", nil
	default:
		return "", fmt.Errorf("sr cannot send a %s to R", v.TypeName())
	}
}

func (vm *VM) rAgent(a Agent) (string, error) {
	names, vals, err := vm.agentFields(a)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(names))
	for i := range names {
		lit, err := vm.rLiteral(vals[i], 1)
		if err != nil {
			return "", err
		}
		parts[i] = rSym(strings.ToUpper(names[i])) + " = " + lit
	}
	return "list(" + strings.Join(parts, ", ") + ")", nil
}

func (vm *VM) agentFields(a Agent) ([]string, []Value, error) {
	var base []string
	switch a.(type) {
	case *Turtle:
		base = []string{"who", "color", "heading", "xcor", "ycor", "shape", "label", "label-color", "breed", "hidden?", "size", "pen-size", "pen-mode"}
		if vm.World != nil && vm.World.threeD {
			base = append(base, "zcor", "pitch", "roll")
		}
	case *Patch:
		base = []string{"pxcor", "pycor", "pcolor", "plabel", "plabel-color"}
		if vm.World != nil && vm.World.threeD {
			base = append(base, "pzcor")
		}
	case *Link:
		base = []string{"end1", "end2", "color", "label", "label-color", "hidden?", "breed", "thickness", "shape", "tie-mode"}
	default:
		return nil, nil, fmt.Errorf("sr expected an agent")
	}
	seen := map[string]bool{}
	for _, n := range base {
		seen[n] = true
	}
	var extra []string
	switch a := a.(type) {
	case *Turtle:
		for k := range a.Vars {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
	case *Patch:
		for k := range a.Vars {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
	case *Link:
		for k := range a.Vars {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
	}
	sort.Strings(extra)
	names := append(base, extra...)
	saved := vm.self
	vm.self = a
	defer func() { vm.self = saved }()
	vals := make([]Value, len(names))
	for i, n := range names {
		v, ok, err := vm.getAgentVar(n)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			vals[i] = Num(0)
			continue
		}
		vals[i] = v
	}
	return names, vals, nil
}

func rColumn(vals []Value) (string, error) {
	if len(vals) == 0 {
		return "numeric(0)", nil
	}
	if uniformAtomic(vals) {
		parts := make([]string, len(vals))
		for i, v := range vals {
			s, err := scalarLiteral(v)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "c(" + strings.Join(parts, ", ") + ")", nil
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		s, err := literalShallow(v)
		if err != nil {
			return "", err
		}
		parts[i] = s
	}
	return "I(list(" + strings.Join(parts, ", ") + "))", nil
}

func rColumnValue(v Value) (string, error) {
	if v.K == KindList {
		return rColumn(v.L)
	}
	s, err := scalarLiteral(v)
	if err != nil {
		return "", err
	}
	return "c(" + s + ")", nil
}

func uniformAtomic(vals []Value) bool {
	k := vals[0].K
	if k != KindNum && k != KindBool && k != KindString {
		return false
	}
	for _, v := range vals[1:] {
		if v.K != k {
			return false
		}
	}
	return true
}

func scalarLiteral(v Value) (string, error) {
	switch v.K {
	case KindNum:
		return rNum(v.N), nil
	case KindBool:
		if v.B {
			return "TRUE", nil
		}
		return "FALSE", nil
	case KindString:
		return rString(v.S), nil
	default:
		return "", fmt.Errorf("sr cannot send a %s to R", v.TypeName())
	}
}

func literalShallow(v Value) (string, error) {
	switch v.K {
	case KindNobody:
		return "NA", nil
	case KindNum, KindBool, KindString:
		return scalarLiteral(v)
	case KindList:
		return rListLiteral(v.L)
	case KindAgent:
		if v.A == nil || v.A.IsDead() {
			return "NA", nil
		}
		return rString(v.A.AgentString()), nil
	default:
		return rString(v.Format(false)), nil
	}
}

func rListLiteral(xs []Value) (string, error) {
	if len(xs) == 0 {
		return "list()", nil
	}
	if uniformAtomic(xs) {
		parts := make([]string, len(xs))
		for i, v := range xs {
			s, err := scalarLiteral(v)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "c(" + strings.Join(parts, ", ") + ")", nil
	}
	parts := make([]string, len(xs))
	for i, v := range xs {
		s, err := literalShallow(v)
		if err != nil {
			return "", err
		}
		parts[i] = s
	}
	return "list(" + strings.Join(parts, ", ") + ")", nil
}

func rNum(n float64) string {
	switch {
	case math.IsNaN(n):
		return "NaN"
	case math.IsInf(n, 1):
		return "Inf"
	case math.IsInf(n, -1):
		return "-Inf"
	default:
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
}

func rString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func rSym(name string) string {
	if name == "" {
		return "``"
	}
	ok := true
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '.' {
				ok = false
				break
			}
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' {
			ok = false
			break
		}
	}
	if ok {
		return name
	}
	return "`" + strings.ReplaceAll(name, "`", "") + "`"
}
