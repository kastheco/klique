package taskstore_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHTTPStore creates an HTTPStore backed by an in-memory SQLiteStore
// served over a local httptest.Server. The server is closed when the test ends.
func newTestHTTPStore(t *testing.T) *taskstore.HTTPStore {
	t.Helper()
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	t.Cleanup(srv.Close)
	return taskstore.NewHTTPStore(srv.URL, "kasmos")
}

func TestHTTPStore_ContentRoundTrip(t *testing.T) {
	store := newTestHTTPStore(t)
	entry := taskstore.TaskEntry{
		Filename: "test.md",
		Status:   taskstore.StatusReady,
		Content:  "# My Plan\n\nDetails here.",
	}
	require.NoError(t, store.Create("proj", entry))

	content, err := store.GetContent("proj", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "# My Plan\n\nDetails here.", content)

	require.NoError(t, store.SetContent("proj", "test.md", "# Updated Plan"))
	content, err = store.GetContent("proj", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "# Updated Plan", content)
}

func TestHTTPStore_RoundTrip(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()

	client := taskstore.NewHTTPStore(srv.URL, "kasmos")

	// Create
	entry := taskstore.TaskEntry{Filename: "test.md", Status: taskstore.StatusReady, Description: "test"}
	require.NoError(t, client.Create("kasmos", entry))

	// Get
	got, err := client.Get("kasmos", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Description)

	// Update
	got.Status = taskstore.StatusImplementing
	require.NoError(t, client.Update("kasmos", "test.md", got))

	// List
	plans, err := client.List("kasmos")
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	assert.Equal(t, taskstore.StatusImplementing, plans[0].Status)
}

func TestHTTPStore_ServerUnreachable(t *testing.T) {
	client := taskstore.NewHTTPStore("http://127.0.0.1:1", "kasmos")
	_, err := client.List("kasmos")
	require.Error(t, err)
	// Error should be recognizable as a connectivity issue
	assert.Contains(t, err.Error(), "task store unreachable")
}

func TestHTTPStore_Ping(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()

	client := taskstore.NewHTTPStore(srv.URL, "kasmos")
	require.NoError(t, client.Ping())
}

func TestHTTPStore_SubtasksRoundTrip(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()
	client := taskstore.NewHTTPStore(srv.URL, "kasmos")

	require.NoError(t, client.Create("kasmos", taskstore.TaskEntry{Filename: "plan.md", Status: taskstore.StatusReady}))

	require.NoError(t, client.SetSubtasks("kasmos", "plan.md", []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "first", Status: taskstore.SubtaskStatusPending},
		{TaskNumber: 2, Title: "second", Status: taskstore.SubtaskStatusPending},
	}))

	subtasks, err := client.GetSubtasks("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, 2, len(subtasks))
	assert.Equal(t, 1, subtasks[0].TaskNumber)
	assert.Equal(t, taskstore.SubtaskStatusPending, subtasks[0].Status)

	require.NoError(t, client.UpdateSubtaskStatus("kasmos", "plan.md", 1, taskstore.SubtaskStatusDone))
	updated, err := client.GetSubtasks("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, taskstore.SubtaskStatusDone, updated[0].Status)

	require.NoError(t, client.SetSubtasks("kasmos", "plan.md", nil))
	empty, err := client.GetSubtasks("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Len(t, empty, 0)
}

func TestHTTPStore_PhaseTimestamp(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()
	client := taskstore.NewHTTPStore(srv.URL, "kasmos")

	require.NoError(t, client.Create("kasmos", taskstore.TaskEntry{Filename: "plan.md", Status: taskstore.StatusReady}))

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, client.SetPhaseTimestamp("kasmos", "plan.md", "planning", ts))

	got, err := backend.Get("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, ts, got.PlanningAt)
}

func TestHTTPStore_PlanGoal(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()
	client := taskstore.NewHTTPStore(srv.URL, "kasmos")

	require.NoError(t, client.Create("kasmos", taskstore.TaskEntry{Filename: "plan.md", Status: taskstore.StatusReady}))

	require.NoError(t, client.SetPlanGoal("kasmos", "plan.md", "ship faster"))
	got, err := backend.Get("kasmos", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, "ship faster", got.Goal)
}

func TestHTTPStore_SetPhaseTimestamp_UsesJSONErrorContractOnMalformedBody(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()

	resp, err := http.DefaultClient.Do(func() *http.Request {
		req, rErr := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/kasmos/tasks/plan.md/phase-timestamp", strings.NewReader("{"))
		require.NoError(t, rErr)
		req.Header.Set("Content-Type", "application/json")
		return req
	}())
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	_, has := errResp["error"]
	assert.True(t, has)
}
