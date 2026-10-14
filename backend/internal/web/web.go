package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var embeddedFS embed.FS

// Handler returns an http.Handler that serves the embedded React single-page application.
func Handler() http.Handler {
	distSub, err := fs.Sub(embeddedFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Embedded frontend assets not found", http.StatusNotFound)
		})
	}

	fileServer := http.FileServer(http.FS(distSub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Check if the requested file exists in embedded dist
		f, err := distSub.Open(path)
		if err == nil {
			stat, err := f.Stat()
			_ = f.Close()
			if err == nil && !stat.IsDir() {
				// File exists on disk: serve directly (JS, CSS, images, etc.)
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Route fallback for client-side React routing: serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
