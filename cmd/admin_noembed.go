//go:build !embedadmin

package cmd

import "net/http"

// getEmbeddedAdminFS returns nil when the embedadmin build tag is absent,
// meaning no assets are embedded and the admin UI is only reachable via
// --admin-dir. A future build step can add an //go:build embedadmin
// counterpart that embeds web/admin/dist/ and returns an http.FileSystem.
func getEmbeddedAdminFS() http.FileSystem {
	return nil
}
