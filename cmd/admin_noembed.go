//go:build !embedadmin

package cmd

import "net/http"

// getEmbeddedAdminFS returns nil when the embedadmin build tag is absent.
// The real implementation (with //go:embed) is supplied during production builds.
func getEmbeddedAdminFS() http.FileSystem {
	return nil
}
