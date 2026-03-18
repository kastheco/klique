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

	// Non-existent path without extension falls back to index.html (SPA routing)
	resp, err := http.Get(srv.URL + "/tasks/some-plan")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(body), "kas admin"))
}

func TestAdminFallbackHandler_DotInSPARoute(t *testing.T) {
	handler := adminFallbackHandler(http.Dir("testdata/admin"))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// SPA routes that contain a dot (e.g. a plan filename like plan-foo.md)
	// must also fall back to index.html, not return a hard 404.
	resp, err := http.Get(srv.URL + "/tasks/plan-foo.md")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(body), "kas admin"))
}

func TestAdminFallbackHandler_MissingAsset404(t *testing.T) {
	handler := adminFallbackHandler(http.Dir("testdata/admin"))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Missing compiled asset under /assets/ returns 404, not index.html.
	// Vite always places hashed JS/CSS/images under /assets/, so a missing
	// file there is always a genuine error rather than a SPA route.
	resp, err := http.Get(srv.URL + "/assets/missing-abc123.js")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
