package webassets

import (
	"embed"
	"io/fs"
	"net/http"
)

// adminDist embeds the built admin SPA so kas serve can expose /admin/
// without requiring --admin-dir.
//
//go:embed all:admin/dist
var adminDist embed.FS

// AdminFS returns the embedded admin SPA as an http.FileSystem rooted at dist/.
func AdminFS() http.FileSystem {
	sub, err := fs.Sub(adminDist, "admin/dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
