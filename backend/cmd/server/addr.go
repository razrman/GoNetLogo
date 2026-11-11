package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	// envAddr is a full listen address used when -addr is omitted.
	envAddr = "ADDR"
	// envPort is a port number used when -addr and ADDR are both omitted.
	// The host stays the host from the -addr default.
	envPort = "PORT"
)

// resolveAddr applies flag-over-environment precedence.
// An explicit -addr wins. Otherwise ADDR is a host:port listen address,
// and PORT replaces the port of flagVal (the flag default when the flag
// was omitted). The flag default itself is unchanged.
func resolveAddr(explicit bool, flagVal string, getenv func(string) string) (string, error) {
	if explicit {
		return checkAddr(strings.TrimSpace(flagVal), "-addr")
	}
	if getenv != nil {
		if addr := strings.TrimSpace(getenv(envAddr)); addr != "" {
			return checkAddr(addr, envAddr)
		}
		if port := strings.TrimSpace(getenv(envPort)); port != "" {
			host, _, err := net.SplitHostPort(flagVal)
			if err != nil || host == "" {
				host = "0.0.0.0"
			}
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return "", fmt.Errorf("%s: want a port number, got %q", envPort, port)
			}
			return net.JoinHostPort(host, strconv.Itoa(n)), nil
		}
	}
	return checkAddr(strings.TrimSpace(flagVal), "-addr")
}

func checkAddr(addr, source string) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("%s: listen address is empty", source)
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return "", fmt.Errorf("%s: %q is not a host:port listen address", source, addr)
	}
	return addr, nil
}
