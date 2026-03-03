package taskstore_test

import (
	"net/http/httptest"
	"testing"

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
