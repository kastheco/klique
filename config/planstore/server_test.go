package planstore_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_CreateAndGetPlan(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	body := `{"filename":"test.md","status":"ready","description":"test"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/plans", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/plans/test.md")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got planstore.PlanEntry
	json.NewDecoder(resp.Body).Decode(&got)
	assert.Equal(t, planstore.StatusReady, got.Status)
}

func TestServer_ListByStatus(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	// Create plans with different statuses
	for _, p := range []planstore.PlanEntry{
		{Filename: "a.md", Status: planstore.StatusReady},
		{Filename: "b.md", Status: planstore.StatusDone},
	} {
		store.Create("kasmos", p)
	}

	resp, err := http.Get(srv.URL + "/v1/projects/kasmos/plans?status=ready")
	require.NoError(t, err)
	var plans []planstore.PlanEntry
	json.NewDecoder(resp.Body).Decode(&plans)
	assert.Len(t, plans, 1)
}

func TestServer_Ping(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/ping")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_SetClickUpTaskID(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	// Create a plan first
	body := `{"filename":"plan.md","status":"ready"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/plans", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// PUT clickup-task-id
	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/plans/plan.md/clickup-task-id",
		strings.NewReader(`{"clickup_task_id":"CU-abc123"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify it was stored
	got, err := store.Get("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, "CU-abc123", got.ClickUpTaskID)
}

func TestServer_SetClickUpTaskID_NotFound(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/plans/nonexistent.md/clickup-task-id",
		strings.NewReader(`{"clickup_task_id":"CU-xyz"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestServer_ContentEndpoints(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(planstore.NewHandler(store))
	defer srv.Close()

	// Create a plan first
	body := `{"filename":"plan.md","status":"ready","content":"# Initial"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/plans", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// GET content
	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/plans/plan.md/content")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/markdown", resp.Header.Get("Content-Type"))
	gotBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "# Initial", string(gotBody))

	// PUT content
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/kasmos/plans/plan.md/content", strings.NewReader("# Updated"))
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// GET content again to verify update
	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/plans/plan.md/content")
	require.NoError(t, err)
	gotBody, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "# Updated", string(gotBody))
}
