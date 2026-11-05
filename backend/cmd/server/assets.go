package main

import (
	"net/http"
	"path"
	"strings"

	"gonetlogo/backend/session"
)

// modelAssetHandler serves raster images stored next to model files.
// The path is relative to the models directory. The Info tab uses it
// for Markdown images with a relative path or a file: URL.
func modelAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/model-asset/")
		rel = strings.TrimPrefix(rel, "/")
		f, ctype, err := session.OpenModelImage(rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}
