package main

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// openUI returns the built React app. A -web directory wins over files
// embedded by `go build -tags web` (see build.sh).
func openUI(dir string) (fs.FS, error) {
	if dir != "" {
		fsys := os.DirFS(dir)
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		return fsys, nil
	}
	fsys := embeddedUI()
	if fsys == nil {
		return nil, nil
	}
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return nil, nil
	}
	return fsys, nil
}

func spaHandler(content fs.FS) http.Handler {
	files := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			files.ServeHTTP(w, r)
			return
		}
		info, err := fs.Stat(content, name)
		if err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		// A missing script or stylesheet should 404. A client-side route
		// falls through to the app shell.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		index, err := content.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer index.Close()
		stat, err := index.Stat()
		if err != nil {
			http.Error(w, "index.html", http.StatusInternalServerError)
			return
		}
		rs, ok := index.(io.ReadSeeker)
		if !ok {
			http.Error(w, "index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
	})
}
