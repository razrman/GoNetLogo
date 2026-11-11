package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// envShutdownTimeout overrides the drain limit when -shutdown-timeout is omitted.
const envShutdownTimeout = "GONETLOGO_SHUTDOWN_TIMEOUT"

// defaultShutdownTimeout is under ECS Fargate's 30s stopTimeout so the
// process can finish draining and close session scratch directories
// before the agent sends SIGKILL.
const defaultShutdownTimeout = 20 * time.Second

// resolveShutdownTimeout applies flag-over-environment precedence.
// An empty flag and an empty environment value keep the 20s default.
// getenv may be nil.
func resolveShutdownTimeout(flagVal string, getenv func(string) string) (time.Duration, error) {
	raw := strings.TrimSpace(flagVal)
	source := "-shutdown-timeout"
	if raw == "" && getenv != nil {
		raw = strings.TrimSpace(getenv(envShutdownTimeout))
		source = envShutdownTimeout
	}
	if raw == "" {
		return defaultShutdownTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: want a positive duration, got %q", source, raw)
	}
	return d, nil
}

// shutdownHTTP drains srv until timeout, then closes the session store.
// A drain that hits the deadline force-closes leftover connections so
// handlers unblock before closeStore runs. closeStore still runs when
// Shutdown returns an error. Websockets are hijacked, so Shutdown does
// not wait for them; closing the store ends those sessions, and the
// process exits after the log is flushed.
func shutdownHTTP(srv *http.Server, timeout time.Duration, closeStore func()) error {
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := srv.Shutdown(ctx)
	if err != nil {
		_ = srv.Close()
	}
	if closeStore != nil {
		closeStore()
	}
	return err
}
