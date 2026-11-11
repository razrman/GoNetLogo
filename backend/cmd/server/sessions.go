package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// envMaxSessions overrides -max-sessions when the flag is omitted.
const envMaxSessions = "GONETLOGO_MAX_SESSIONS"

// resolveMaxSessions applies flag-over-environment precedence.
// Zero means unlimited. A negative flag or environment value is an error.
// getenv may be nil when explicit is true.
func resolveMaxSessions(explicit bool, flagVal int, getenv func(string) string) (int, error) {
	if explicit {
		if flagVal < 0 {
			return 0, fmt.Errorf("-max-sessions: want a non-negative integer, got %d", flagVal)
		}
		return flagVal, nil
	}
	raw := ""
	if getenv != nil {
		raw = strings.TrimSpace(getenv(envMaxSessions))
	}
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: want a non-negative integer, got %q", envMaxSessions, raw)
	}
	return n, nil
}

func flagExplicit(name string) bool {
	set := false
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
