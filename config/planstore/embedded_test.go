package planstore_test

import (
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedServer_StartsAndStops(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	srv, err := planstore.StartEmbedded(dbPath, 0) // port 0 = auto-assign
	require.NoError(t, err)
	defer srv.Stop()

	assert.NotEmpty(t, srv.URL())

	// Verify the server is reachable
	client := planstore.NewHTTPStore(srv.URL(), "test")
	require.NoError(t, client.Ping())
}

func TestEmbeddedServer_StopIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	srv, err := planstore.StartEmbedded(dbPath, 0)
	require.NoError(t, err)
	srv.Stop()
	srv.Stop() // should not panic
}

func TestEmbeddedServer_ContentEndpoint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	srv, err := planstore.StartEmbedded(dbPath, 0)
	require.NoError(t, err)
	defer srv.Stop()

	client := planstore.NewHTTPStore(srv.URL(), "test")
	require.NoError(t, client.Create("proj", planstore.PlanEntry{
		Filename: "test.md", Status: planstore.StatusReady, Content: "# Hello",
	}))

	content, err := client.GetContent("proj", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "# Hello", content)
}
