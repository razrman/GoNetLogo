package logcfg

import (
	"bytes"
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"INFO", slog.LevelInfo, false},
		{" warn ", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"", 0, true},
		{"verbose", 0, true},
		{"trace", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseLevel(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseLevel(%q) err = nil, want error", tc.in)
			}
			if !strings.Contains(err.Error(), "invalid log level") {
				t.Fatalf("ParseLevel(%q) err = %v", tc.in, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseLevel(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseFormat(t *testing.T) {
	text, err := ParseFormat("Text")
	if err != nil || text != "text" {
		t.Fatalf("text format %q %v", text, err)
	}
	js, err := ParseFormat("json")
	if err != nil || js != "json" {
		t.Fatalf("json format %q %v", js, err)
	}
	if _, err := ParseFormat("yaml"); err == nil || !strings.Contains(err.Error(), "invalid log format") {
		t.Fatalf("yaml err = %v", err)
	}
}

func TestFlagOverridesEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse([]string{"-log-level", "debug", "-log-format", "json"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(key string) string {
		if key == EnvLevel {
			return "error"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Fatalf("level = %v, want debug (flag wins over env error)", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Fatalf("format = %q", cfg.Format)
	}
}

func TestEnvironmentUsedWhenFlagOmitted(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse([]string{"-log-format", "text"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(key string) string {
		if key == EnvLevel {
			return "warn"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelWarn {
		t.Fatalf("level = %v, want warn from the environment", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Fatalf("format = %q", cfg.Format)
	}
}

func TestDefaultIsInfo(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(string) string { return "  " })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelInfo || cfg.Format != "text" || cfg.File != "-" {
		t.Fatalf("cfg = %+v, want info text on stderr", cfg)
	}
}

func TestInvalidFlagWinsOverValidEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	flags := Register(fs)
	if err := fs.Parse([]string{"-log-level", "verbose"}); err != nil {
		t.Fatal(err)
	}
	_, err := flags.Resolve(func(string) string { return "info" })
	if err == nil || !strings.Contains(err.Error(), "verbose") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	_, err := flags.Resolve(func(string) string { return "trace" })
	if err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("err = %v", err)
	}
}

func TestHelpMentionsEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	Register(fs)
	fl := fs.Lookup("log-level")
	if fl == nil || !strings.Contains(fl.Usage, EnvLevel) || fl.DefValue != "info" {
		t.Fatalf("log-level flag = %+v", fl)
	}
	format := fs.Lookup("log-format")
	if format == nil || format.DefValue != "text" || !strings.Contains(format.Usage, "json") {
		t.Fatalf("log-format flag = %+v", format)
	}
	file := fs.Lookup("log-file")
	if file == nil || file.DefValue != "-" || !strings.Contains(file.Usage, EnvFile) || !strings.Contains(file.Usage, "stderr") {
		t.Fatalf("log-file flag = %+v", file)
	}
}

func TestLogFileFlagOverridesEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse([]string{"-log-file", "/tmp/from-flag.log"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(key string) string {
		if key == EnvFile {
			return "/tmp/from-env.log"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.File != "/tmp/from-flag.log" {
		t.Fatalf("file = %q, want the flag path", cfg.File)
	}
}

func TestLogFileEnvironmentWhenFlagOmitted(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(key string) string {
		if key == EnvFile {
			return " /tmp/from-env.log "
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.File != "/tmp/from-env.log" {
		t.Fatalf("file = %q, want the environment path", cfg.File)
	}

	dash, err := flags.Resolve(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if dash.File != "-" {
		t.Fatalf("blank env file = %q, want stderr", dash.File)
	}
}

func TestLogFileDashOverridesEnvironment(t *testing.T) {
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	if err := fs.Parse([]string{"-log-file", "-"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := flags.Resolve(func(key string) string {
		if key == EnvFile {
			return "/tmp/from-env.log"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.File != "-" {
		t.Fatalf("file = %q, want stderr", cfg.File)
	}
}

func TestUnwritableLogFile(t *testing.T) {
	dir := t.TempDir()
	fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
	flags := Register(fs)
	// A directory cannot be opened for append, including when the test runs as root.
	if err := fs.Parse([]string{"-log-file", dir}); err != nil {
		t.Fatal(err)
	}
	_, err := flags.Apply(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "open log file") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogFileAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gonetlogo.log")
	write := func(msg string) {
		t.Helper()
		fs := flag.NewFlagSet("gonetlogo-server", flag.ContinueOnError)
		flags := Register(fs)
		if err := fs.Parse([]string{"-log-file", path, "-log-level", "info"}); err != nil {
			t.Fatal(err)
		}
		if _, err := flags.Apply(func(string) string { return "" }); err != nil {
			t.Fatal(err)
		}
		slog.Info(msg)
		if err := Close(); err != nil {
			t.Fatal(err)
		}
	}
	write("first-line")
	write("second-line")
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		log.SetOutput(io.Discard)
	})

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "first-line") || !strings.Contains(text, "second-line") {
		t.Fatalf("log file = %q", text)
	}
	first := strings.Index(text, "first-line")
	second := strings.Index(text, "second-line")
	if first < 0 || second < first {
		t.Fatalf("lines out of order in %q", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644", info.Mode().Perm())
	}
}
