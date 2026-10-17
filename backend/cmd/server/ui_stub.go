//go:build !web

package main

import "io/fs"

// embeddedUI is empty unless the binary is built with -tags web.
func embeddedUI() fs.FS { return nil }
