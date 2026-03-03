package planstore_test

import (
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) planstore.Store {
	t.Helper()
	store, err := planstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{
		Filename:    "2026-02-28-test-plan.md",
		Status:      planstore.StatusReady,
		Description: "test plan",
		Branch:      "plan/test-plan",
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	got, err := store.Get("kasmos", "2026-02-28-test-plan.md")
	require.NoError(t, err)
	assert.Equal(t, planstore.StatusReady, got.Status)
	assert.Equal(t, "test plan", got.Description)
}

func TestSQLiteStore_ListByStatus(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "a.md", Status: planstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "b.md", Status: planstore.StatusDone}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "c.md", Status: planstore.StatusReady}))

	plans, err := store.ListByStatus("kasmos", planstore.StatusReady)
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestSQLiteStore_ProjectIsolation(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("project-a", planstore.PlanEntry{Filename: "x.md", Status: planstore.StatusReady}))
	require.NoError(t, store.Create("project-b", planstore.PlanEntry{Filename: "y.md", Status: planstore.StatusReady}))

	plans, err := store.List("project-a")
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	assert.Equal(t, "x.md", plans[0].Filename)
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{
		Filename:    "2026-02-28-update-test.md",
		Status:      planstore.StatusReady,
		Description: "original description",
		Branch:      "plan/update-test",
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	entry.Status = planstore.StatusImplementing
	entry.Description = "updated description"
	require.NoError(t, store.Update("kasmos", "2026-02-28-update-test.md", entry))

	got, err := store.Get("kasmos", "2026-02-28-update-test.md")
	require.NoError(t, err)
	assert.Equal(t, planstore.StatusImplementing, got.Status)
	assert.Equal(t, "updated description", got.Description)
}

func TestSQLiteStore_Rename(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{
		Filename:  "2026-02-28-old-name.md",
		Status:    planstore.StatusReady,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	require.NoError(t, store.Rename("kasmos", "2026-02-28-old-name.md", "2026-02-28-new-name.md"))

	_, err := store.Get("kasmos", "2026-02-28-old-name.md")
	assert.Error(t, err)

	got, err := store.Get("kasmos", "2026-02-28-new-name.md")
	require.NoError(t, err)
	assert.Equal(t, planstore.StatusReady, got.Status)
}

func TestSQLiteStore_ListByTopic(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "a.md", Status: planstore.StatusReady, Topic: "auth"}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "b.md", Status: planstore.StatusReady, Topic: "auth"}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "c.md", Status: planstore.StatusReady, Topic: "storage"}))

	plans, err := store.ListByTopic("kasmos", "auth")
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestSQLiteStore_Topics(t *testing.T) {
	store := newTestStore(t)
	topic := planstore.TopicEntry{
		Name:      "auth",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.CreateTopic("kasmos", topic))

	topics, err := store.ListTopics("kasmos")
	require.NoError(t, err)
	assert.Len(t, topics, 1)
	assert.Equal(t, "auth", topics[0].Name)
}

func TestSQLiteStore_Ping(t *testing.T) {
	store := newTestStore(t)
	assert.NoError(t, store.Ping())
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get("kasmos", "nonexistent.md")
	assert.Error(t, err)
}

func TestSQLiteStore_CreateDuplicate(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{Filename: "dup.md", Status: planstore.StatusReady}
	require.NoError(t, store.Create("kasmos", entry))
	err := store.Create("kasmos", entry)
	assert.Error(t, err)
}

func TestSQLiteStore_ListSortedByFilename(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "c.md", Status: planstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "a.md", Status: planstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", planstore.PlanEntry{Filename: "b.md", Status: planstore.StatusReady}))

	plans, err := store.List("kasmos")
	require.NoError(t, err)
	require.Len(t, plans, 3)
	assert.Equal(t, "a.md", plans[0].Filename)
	assert.Equal(t, "b.md", plans[1].Filename)
	assert.Equal(t, "c.md", plans[2].Filename)
}

func TestSQLiteStore_CreateWithContent(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{
		Filename: "2026-02-28-test.md",
		Status:   planstore.StatusReady,
		Content:  "# Test Plan\n\n## Wave 1\n\n### Task 1: Do thing\n",
	}
	require.NoError(t, store.Create("proj", entry))
	got, err := store.Get("proj", "2026-02-28-test.md")
	require.NoError(t, err)
	assert.Equal(t, entry.Content, got.Content)
}

func TestSQLiteStore_GetContent(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{
		Filename: "2026-02-28-test.md",
		Status:   planstore.StatusReady,
		Content:  "# Full Plan Content",
	}
	require.NoError(t, store.Create("proj", entry))
	content, err := store.GetContent("proj", "2026-02-28-test.md")
	require.NoError(t, err)
	assert.Equal(t, "# Full Plan Content", content)
}

func TestSQLiteStore_SetContent(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{Filename: "2026-02-28-test.md", Status: planstore.StatusReady}
	require.NoError(t, store.Create("proj", entry))
	require.NoError(t, store.SetContent("proj", "2026-02-28-test.md", "# Updated"))
	content, err := store.GetContent("proj", "2026-02-28-test.md")
	require.NoError(t, err)
	assert.Equal(t, "# Updated", content)
}

func TestClickUpTaskIDRoundTrip(t *testing.T) {
	store := newTestStore(t)
	entry := planstore.PlanEntry{Filename: "2026-02-28-clickup-test.md", Status: planstore.StatusReady}
	require.NoError(t, store.Create("proj", entry))

	// Initially no task ID
	got, err := store.Get("proj", "2026-02-28-clickup-test.md")
	require.NoError(t, err)
	assert.Equal(t, "", got.ClickUpTaskID, "task ID must be empty before set")

	// Set the task ID
	require.NoError(t, store.SetClickUpTaskID("proj", "2026-02-28-clickup-test.md", "CU-abc123"))

	// Verify it round-trips through Get
	got, err = store.Get("proj", "2026-02-28-clickup-test.md")
	require.NoError(t, err)
	assert.Equal(t, "CU-abc123", got.ClickUpTaskID, "task ID must be persisted after SetClickUpTaskID")

	// Verify it appears in List
	plans, err := store.List("proj")
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "CU-abc123", plans[0].ClickUpTaskID, "task ID must appear in List results")
}

func TestClickUpTaskIDRoundTrip_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.SetClickUpTaskID("proj", "nonexistent.md", "CU-xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
