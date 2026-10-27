// Package logcfg configures the process logger from flags and the environment.
// GONETLOGO_LOG_LEVEL supplies the level when -log-level is omitted.
// An explicit flag wins. -log-format selects text or json.
package logcfg

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// EnvLevel is the environment variable that sets the log level when the
// -log-level flag is left unset.
const EnvLevel = "GONETLOGO_LOG_LEVEL"

const (
	levelUsage  = "log verbosity: debug, info, warn, or error. " + EnvLevel + " is used when this flag is omitted; an explicit flag wins"
	formatUsage = "log format: text or json"
)

// Flags are the -log-level and -log-format values registered on a FlagSet.
type Flags struct {
	fs     *flag.FlagSet
	level  *string
	format *string
}

// Register adds -log-level and -log-format to fs.
// The level default shown in -help is info. The environment variable is
// consulted only when the flag is not present on the command line.
func Register(fs *flag.FlagSet) *Flags {
	if fs == nil {
		fs = flag.CommandLine
	}
	f := &Flags{fs: fs}
	f.level = fs.String("log-level", "info", levelUsage)
	f.format = fs.String("log-format", "text", formatUsage)
	return f
}

// Config is a resolved level and format.
type Config struct {
	Level  slog.Level
	Format string
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
	return Config{Level: level, Format: format}, nil
}

// Apply resolves the flags and installs the logger as slog's default.
func (f *Flags) Apply(getenv func(string) string) (Config, error) {
	cfg, err := f.Resolve(getenv)
	if err != nil {
		return Config{}, err
	}
	slog.SetDefault(cfg.Logger(os.Stderr))
	return cfg, nil
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
