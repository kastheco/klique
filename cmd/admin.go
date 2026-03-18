package cmd

import (
	"net/http"
	"path"
	"strings"
)

// adminFallbackHandler returns an http.Handler that serves static files from
// root and falls back to index.html for any path that does not resolve to a
// real file, enabling client-side SPA routing.
//
// Rules:
//   - "/" delegates to the file server, which resolves index.html.
//   - If cleanPath opens to a real file (not a directory), the standard file
//     server handles it — no directory listings are ever served.
//   - If the path is under "/assets/" and the file is missing, a hard 404 is
//     returned.  Vite compiles all JS/CSS/image assets into "/assets/", so a
//     missing asset there is always a genuine 404.
//   - Everything else that does not resolve to a real file (including SPA
//     routes that contain dots, e.g. /tasks/plan-foo.md) rewrites to "/" so
//     the file server returns index.html and react-router takes over.
func adminFallbackHandler(root http.FileSystem) http.Handler {
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure the path is clean and has a leading slash.
		cleanPath := path.Clean("/" + r.URL.Path)
		if cleanPath == "/" {
			cleanPath = "/index.html"
		}

		// Try to open and stat the resolved path.
		f, err := root.Open(cleanPath)
		if err == nil {
			stat, statErr := f.Stat()
			f.Close()
			if statErr == nil && !stat.IsDir() {
				// Real file — delegate to the standard file server.
				fileServer.ServeHTTP(w, r)
				return
			}
			// Directory open succeeded but we never serve listings; fall
			// through to the missing-path handling below.
		}

		// Path is missing (or is a directory).
		// Hard-404 for compiled Vite assets — their hashed names are never
		// valid SPA routes, so a missing asset here is a genuine error.
		if strings.HasPrefix(cleanPath, "/assets/") {
			http.NotFound(w, r)
			return
		}

		// Everything else: rewrite to / so the file server finds and serves
		// index.html via its built-in directory-index lookup.
		// We must not rewrite to /index.html directly because http.FileServer
		// unconditionally redirects paths ending with "/index.html" to "./".
		r2 := new(http.Request)
		*r2 = *r
		u2 := *r.URL
		u2.Path = "/"
		r2.URL = &u2
		fileServer.ServeHTTP(w, r2)
	})
}
