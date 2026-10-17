//go:build web

package main

import (
	"embed"
	"io/fs"
)

// webdist is the production frontend. build.sh copies frontend/dist here
// before compiling with -tags web, then deletes the copy. The directory
// is not part of the source tree.
//
//go:embed all:webdist
var embeddedWeb embed.FS

func embeddedUI() fs.FS {
	sub, err := fs.Sub(embeddedWeb, "webdist")
	if err != nil {
		return nil
	}
	return sub
}
