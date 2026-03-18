package cmd

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
)

// adminFallbackHandler returns an http.Handler that serves static files from
// root and falls back to /index.html for unrecognised paths without a file
// extension, enabling client-side SPA routing.
//
// Rules:
//   - "/" is handled by delegating to the file server, which serves index.html.
//   - If a path resolves to a real file, the standard file server handles it.
//   - If a path has no extension and is missing, it rewrites to /index.html so
//     react-router can take over (e.g. /tasks/some-plan).
//   - If a path has an extension and is missing (broken asset), a hard 404 is
//     returned — never fall back to index.html for assets.
func adminFallbackHandler(root http.FileSystem) http.Handler {
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure the path is clean and has a leading slash.
		cleanPath := path.Clean("/" + r.URL.Path)
		if cleanPath == "/" {
			cleanPath = "/index.html"
		}

		// Try to open the resolved path.
		f, err := root.Open(cleanPath)
		if err == nil {
			f.Close()
			// File exists — delegate to the standard file server.
			fileServer.ServeHTTP(w, r)
			return
		}

		if errors.Is(err, fs.ErrNotExist) {
			// No extension: rewrite to / so the file server finds and serves
			// index.html via its built-in directory index lookup.
			// We must not rewrite to /index.html directly because http.FileServer
			// unconditionally redirects paths that end with "/index.html" to "./".
			if filepath.Ext(cleanPath) == "" {
				r2 := new(http.Request)
				*r2 = *r
				u2 := *r.URL
				u2.Path = "/"
				r2.URL = &u2
				fileServer.ServeHTTP(w, r2)
				return
			}
			// Extension present but asset is missing — hard 404.
			http.NotFound(w, r)
			return
		}

		// Unexpected error opening the file.
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
}
