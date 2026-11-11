// Package logcfg configures the process logger from flags and the environment.
// GONETLOGO_LOG_LEVEL supplies the level when -log-level is omitted.
// GONETLOGO_LOG_FILE supplies the file when -log-file is omitted.
// An explicit flag wins. -log-format selects text or json.
package logcfg

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
)

// EnvLevel is the environment variable that sets the log level when the
// -log-level flag is left unset.
const EnvLevel = "GONETLOGO_LOG_LEVEL"

// EnvFile is the environment variable that sets the log file when the
// -log-file flag is left unset. "-" and an empty value mean stderr.
const EnvFile = "GONETLOGO_LOG_FILE"

const (
	levelUsage  = "log verbosity: debug, info, warn, or error. " + EnvLevel + " is used when this flag is omitted; an explicit flag wins"
	formatUsage = "log format: text or json"
	fileUsage   = "append logs to this path instead of stderr; - means stderr. " + EnvFile + " is used when this flag is omitted; an explicit flag wins"
)

// Flags are the -log-level, -log-format, and -log-file values registered on a FlagSet.
type Flags struct {
	fs     *flag.FlagSet
	level  *string
	format *string
	file   *string
}

// Register adds -log-level, -log-format, and -log-file to fs.
// The level default shown in -help is info. The file default is - (stderr).
// Environment variables are consulted only when the matching flag is absent.
func Register(fs *flag.FlagSet) *Flags {
	if fs == nil {
		fs = flag.CommandLine
	}
	f := &Flags{fs: fs}
	f.level = fs.String("log-level", "info", levelUsage)
	f.format = fs.String("log-format", "text", formatUsage)
	f.file = fs.String("log-file", "-", fileUsage)
	return f
}

// Config is a resolved level, format, and destination.
// File is "-" when logs go to stderr.
type Config struct {
	Level  slog.Level
	Format string
	File   string
}

// Resolve applies flag-over-environment precedence.
// getenv is typically os.Getenv. A blank environment value is ignored.
func (f *Flags) Resolve(getenv func(string) string) (Config, error) {
	if f == nil {
		return Config{}, fmt.Errorf("nil log flags")
	}
	name := "info"
	if f.level != nil {
		name = *f.level
	}
	explicit := false
	f.fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "log-level" {
			explicit = true
		}
	})
	if !explicit && getenv != nil {
		if env := strings.TrimSpace(getenv(EnvLevel)); env != "" {
			name = env
		}
	}
	level, err := ParseLevel(name)
	if err != nil {
		return Config{}, err
	}
	formatName := "text"
	if f.format != nil {
		formatName = *f.format
	}
	format, err := ParseFormat(formatName)
	if err != nil {
		return Config{}, err
	}
	return Config{Level: level, Format: format, File: f.fileName(getenv)}, nil
}

// fileName applies the same flag-over-environment rule as the level.
// "-" and a blank value select stderr.
func (f *Flags) fileName(getenv func(string) string) string {
	explicit := false
	f.fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "log-file" {
			explicit = true
		}
	})
	name := "-"
	if explicit {
		if f.file != nil {
			name = strings.TrimSpace(*f.file)
		}
	} else if getenv != nil {
		if env := strings.TrimSpace(getenv(EnvFile)); env != "" {
			name = env
		}
	}
	if name == "" || name == "-" {
		return "-"
	}
	return name
}

// Apply resolves the flags, opens the log file when one is requested, and
// installs the logger as slog's default. The standard log package writes
// through that logger. A log.Fatal line is also copied to stderr when the
// destination is a file, so the exit reason stays on the terminal.
func (f *Flags) Apply(getenv func(string) string) (Config, error) {
	cfg, err := f.Resolve(getenv)
	if err != nil {
		return Config{}, err
	}
	w, file, err := openLog(cfg.File)
	if err != nil {
		return Config{}, err
	}
	logger := cfg.Logger(w)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(stdBridge{mirrorStderr: file != nil})
	setDest(file)
	return cfg, nil
}

// openLog opens path for append, creating it mode 0644 when it does not exist.
// "-" uses stderr and returns a nil file so Close does not close the terminal.
func openLog(path string) (io.Writer, *os.File, error) {
	if path == "" || path == "-" {
		return os.Stderr, nil, nil
	}
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	if created {
		if err := os.Chmod(path, 0o644); err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
		}
	}
	return f, f, nil
}

var (
	destMu sync.Mutex
	dest   *os.File
)

func setDest(f *os.File) {
	destMu.Lock()
	old := dest
	dest = f
	destMu.Unlock()
	if old != nil && old != f {
		_ = old.Sync()
		_ = old.Close()
	}
}

// ToFile reports whether the active logger is writing to a file.
func ToFile() bool {
	destMu.Lock()
	defer destMu.Unlock()
	return dest != nil
}

// Close flushes and closes the log file. It is safe to call more than once.
// Stderr is left open.
func Close() error {
	destMu.Lock()
	f := dest
	dest = nil
	destMu.Unlock()
	if f == nil {
		return nil
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// OnSignal registers fn to run on SIGINT or SIGTERM before the log file is
// closed. CloseOnSignal reads the hook when the signal arrives, so it may
// be registered before or after CloseOnSignal. A later call replaces the
// hook. A nil hook restores the previous behavior.
//
// The hook runs while the log file is still open. When a hook is set,
// CloseOnSignal does not re-raise the signal: the old re-raise killed the
// process before a server could finish http.Server.Shutdown and session
// cleanup, and it closed the log first so those steps could not record
// anything. The hook should do that work and return. CloseOnSignal then
// flushes the log and exits. Without a hook, the log is flushed and the
// signal is re-raised, which is what the desktop process still relies on.
func OnSignal(fn func(os.Signal)) {
	signalHookMu.Lock()
	signalHook = fn
	signalHookMu.Unlock()
}

var (
	signalHookMu sync.Mutex
	signalHook   func(os.Signal)
)

func currentSignalHook() func(os.Signal) {
	signalHookMu.Lock()
	defer signalHookMu.Unlock()
	return signalHook
}

// signalExit and raiseSignal are the process-exit steps. Tests replace
// them so signal cleanup can be checked without killing the test process.
var signalExit = os.Exit

var raiseSignal = func(s syscall.Signal) error {
	signal.Reset(s)
	return syscall.Kill(os.Getpid(), s)
}

// CloseOnSignal flushes and closes the log file on SIGINT or SIGTERM.
// A hook registered with OnSignal runs first, while the log is still
// open, and the process then exits without re-raising. With no hook, the
// default signal behavior is restored and the signal is re-raised so the
// process still exits.
func CloseOnSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-ch
		// Stop before the hook so a second signal during shutdown uses
		// the default action and terminates a stuck process.
		signal.Stop(ch)
		finishSignal(sig)
	}()
}

// finishSignal runs the shutdown hook, flushes the log, and exits.
// A hook skips the re-raise. See OnSignal.
func finishSignal(sig os.Signal) {
	if fn := currentSignalHook(); fn != nil {
		fn(sig)
		_ = Close()
		signalExit(0)
		return
	}
	_ = Close()
	if s, ok := sig.(syscall.Signal); ok {
		_ = raiseSignal(s)
	}
	signalExit(1)
}

// Recover logs a panic, prints it on stderr, closes the log file, and exits.
// Register it with defer. A normal return does nothing.
func Recover() {
	rec := recover()
	if rec == nil {
		return
	}
	stack := debug.Stack()
	slog.Error("panic", "err", fmt.Sprint(rec), "stack", string(stack))
	fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", rec, stack)
	_ = Close()
	os.Exit(1)
}

// Fail logs at error, copies that record to stderr when logs go to a file,
// closes the log, and exits. The copy keeps a fatal message on the terminal.
func Fail(code int, msg string, args ...any) {
	slog.Error(msg, args...)
	if ToFile() {
		fmt.Fprint(os.Stderr, msg)
		for i := 0; i+1 < len(args); i += 2 {
			fmt.Fprintf(os.Stderr, " %v=%v", args[i], args[i+1])
		}
		fmt.Fprintln(os.Stderr)
	}
	_ = Close()
	os.Exit(code)
}

// stdBridge sends standard-library log lines through slog.
// When slog is aimed at a file, the same bytes are also written to stderr
// so log.Fatal remains visible in the terminal.
type stdBridge struct {
	mirrorStderr bool
}

func (b stdBridge) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		logger := slog.Default()
		if logger.Enabled(context.Background(), slog.LevelInfo) {
			logger.Info(msg)
		} else {
			logger.Error(msg)
		}
	}
	if b.mirrorStderr {
		_, _ = os.Stderr.Write(p)
	}
	return len(p), nil
}

// ParseLevel accepts debug, info, warn, and error, ignoring case and
// surrounding space.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: want debug, info, warn, or error", s)
	}
}

// ParseFormat accepts text and json, ignoring case and surrounding space.
func ParseFormat(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "text":
		return "text", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("invalid log format %q: want text or json", s)
	}
}

// Logger builds a text or JSON slog logger that writes to w.
func (c Config) Logger(w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	opts := &slog.HandlerOptions{Level: c.Level}
	var h slog.Handler
	if c.Format == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}
