package logcfg

import (
	"bytes"
	"flag"
	"log/slog"
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
	cfg, err := flags.Resolve(func(string) string { return "warn" })
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
	if cfg.Level != slog.LevelInfo || cfg.Format != "text" {
		t.Fatalf("cfg = %+v, want info text", cfg)
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
}
