package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminFallbackHandler_ServesIndex(t *testing.T) {
	handler := adminFallbackHandler(http.Dir("testdata/admin"))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Root serves index.html
	resp, err := http.Get(srv.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(body), "kas admin"))
}

func TestAdminFallbackHandler_SPAFallback(t *testing.T) {
	handler := adminFallbackHandler(http.Dir("testdata/admin"))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Non-existent path falls back to index.html (SPA routing)
	resp, err := http.Get(srv.URL + "/tasks/some-plan")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(body), "kas admin"))
}

func TestAdminFallbackHandler_MissingAsset404(t *testing.T) {
	handler := adminFallbackHandler(http.Dir("testdata/admin"))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Missing asset with a file extension returns 404, not index.html
	resp, err := http.Get(srv.URL + "/missing.js")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
