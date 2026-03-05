package taskstore_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_CreateAndGetPlan(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	body := `{"filename":"test.md","status":"ready","description":"test"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/test.md")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got taskstore.TaskEntry
	json.NewDecoder(resp.Body).Decode(&got)
	assert.Equal(t, taskstore.StatusReady, got.Status)
}

func TestServer_ListByStatus(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	// Create plans with different statuses
	for _, p := range []taskstore.TaskEntry{
		{Filename: "a.md", Status: taskstore.StatusReady},
		{Filename: "b.md", Status: taskstore.StatusDone},
	} {
		store.Create("kasmos", p)
	}

	resp, err := http.Get(srv.URL + "/v1/projects/kasmos/tasks?status=ready")
	require.NoError(t, err)
	var plans []taskstore.TaskEntry
	json.NewDecoder(resp.Body).Decode(&plans)
	assert.Len(t, plans, 1)
}

func TestServer_Ping(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/ping")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_SetClickUpTaskID(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	// Create a plan first
	body := `{"filename":"plan.md","status":"ready"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// PUT clickup-task-id
	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/clickup-task-id",
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
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/nonexistent.md/clickup-task-id",
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
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	// Create a plan first
	body := `{"filename":"plan.md","status":"ready","content":"# Initial"}`
	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// GET content
	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/plan.md/content")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/markdown", resp.Header.Get("Content-Type"))
	gotBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "# Initial", string(gotBody))

	// PUT content
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/kasmos/tasks/plan.md/content", strings.NewReader("# Updated"))
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// GET content again to verify update
	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/plan.md/content")
	require.NoError(t, err)
	gotBody, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, "# Updated", string(gotBody))
}

func TestServer_SubtasksEndpoints(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(`{"filename":"plan.md","status":"ready"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/plan.md/subtasks")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var got []taskstore.SubtaskEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	resp.Body.Close()
	assert.Len(t, got, 0)

	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/subtasks",
		strings.NewReader(`[{"task_number":1,"title":"first","status":"pending"}]`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/subtasks/1/status",
		strings.NewReader(`{"status":"done"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/plan.md/subtasks")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated []taskstore.SubtaskEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	resp.Body.Close()
	assert.Equal(t, taskstore.SubtaskStatusDone, updated[0].Status)
}

func TestServer_Subtasks_ContractErrors(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(`{"filename":"plan.md","status":"ready"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/v1/projects/kasmos/tasks/missing/subtasks")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var notFound map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&notFound))
	resp.Body.Close()
	assert.Contains(t, notFound["error"], "plan not found")

	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/missing/subtasks",
		strings.NewReader(`{"task_number":1,"title":"bad","status":"pending"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&notFound))
	resp.Body.Close()
	assert.Contains(t, notFound["error"], "plan not found")

	req, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/subtasks",
		strings.NewReader("{"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var badRequest map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&badRequest))
	resp.Body.Close()
	assert.Contains(t, badRequest["error"], "invalid request body")

	req, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/subtasks/1/status",
		strings.NewReader("{"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&badRequest))
	resp.Body.Close()
	assert.Contains(t, badRequest["error"], "invalid request body")

	req, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/subtasks/1/status",
		strings.NewReader(`{"status":"done"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&badRequest))
	resp.Body.Close()
	assert.Contains(t, badRequest["error"], "subtask not found")
}

func TestServer_PhaseTimestampAndGoalEndpoints(t *testing.T) {
	store := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects/kasmos/tasks", "application/json", strings.NewReader(`{"filename":"plan.md","status":"ready"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/phase-timestamp",
		strings.NewReader(`{"phase":"planning","timestamp":"2026-01-02T03:04:05Z"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/goal",
		strings.NewReader(`{"goal":"ship faster"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	got, err := store.Get("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, "ship faster", got.Goal)
	assert.False(t, got.PlanningAt.IsZero())

	bad, err := http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/plan.md/phase-timestamp",
		strings.NewReader("{"))
	require.NoError(t, err)
	bad.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(bad)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var badPayload map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&badPayload))
	resp.Body.Close()
	assert.Contains(t, badPayload["error"], "invalid request body")

	bad, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/missing/goal",
		strings.NewReader(`{"goal":"ship faster"}`))
	require.NoError(t, err)
	bad.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(bad)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var missing map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&missing))
	resp.Body.Close()
	assert.Contains(t, missing["error"], "plan not found")

	bad, err = http.NewRequest(http.MethodPut,
		srv.URL+"/v1/projects/kasmos/tasks/missing/phase-timestamp",
		strings.NewReader(`{"phase":"planning","timestamp":"2026-01-02T03:04:05Z"}`))
	require.NoError(t, err)
	bad.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(bad)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&missing))
	resp.Body.Close()
	assert.Contains(t, missing["error"], "plan not found")
}
