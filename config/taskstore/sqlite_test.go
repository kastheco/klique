package taskstore_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) taskstore.Store {
	t.Helper()
	store, err := taskstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename:    "test-plan",
		Status:      taskstore.StatusReady,
		Description: "test plan",
		Branch:      "plan/test-plan",
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	got, err := store.Get("kasmos", "test-plan")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReady, got.Status)
	assert.Equal(t, "test plan", got.Description)
}

func TestSQLiteStore_MdSuffixMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "taskstore.db")

	store, err := taskstore.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	// Insert legacy .md-suffixed entries to simulate a pre-migration database.
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "foo.md", Status: taskstore.StatusReady}))
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "bar.md", Status: taskstore.StatusDone}))
	require.NoError(t, store.SetSubtasks("proj", "foo.md", []taskstore.SubtaskEntry{{TaskNumber: 1, Title: "sub1", Status: taskstore.SubtaskStatusPending}}))
	require.NoError(t, store.Close())

	// Reopen — migration must strip '.md' from both tasks and subtasks.
	store2, err := taskstore.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store2.Close()

	plans, err := store2.List("proj")
	require.NoError(t, err)
	for _, plan := range plans {
		assert.False(t, strings.HasSuffix(plan.Filename, ".md"), "filename %q should not have .md suffix after migration", plan.Filename)
	}

	// Subtasks must be retrievable by the stripped filename.
	subs, err := store2.GetSubtasks("proj", "foo")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
}

func TestSQLiteStore_ListByStatus(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "a", Status: taskstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "b", Status: taskstore.StatusDone}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "c", Status: taskstore.StatusReady}))

	plans, err := store.ListByStatus("kasmos", taskstore.StatusReady)
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestSQLiteStore_ProjectIsolation(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("project-a", taskstore.TaskEntry{Filename: "x", Status: taskstore.StatusReady}))
	require.NoError(t, store.Create("project-b", taskstore.TaskEntry{Filename: "y", Status: taskstore.StatusReady}))

	plans, err := store.List("project-a")
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	assert.Equal(t, "x", plans[0].Filename)
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename:    "update-test",
		Status:      taskstore.StatusReady,
		Description: "original description",
		Branch:      "plan/update-test",
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	entry.Status = taskstore.StatusImplementing
	entry.Description = "updated description"
	require.NoError(t, store.Update("kasmos", "update-test", entry))

	got, err := store.Get("kasmos", "update-test")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusImplementing, got.Status)
	assert.Equal(t, "updated description", got.Description)
}

// TestSQLiteStore_UpdatePreservesContent verifies that Update does not
// overwrite content stored via SetContent. This is a regression test for a bug
// where every FSM status transition would nuke the content column because
// Update included content in its SET clause and callers passed empty content.
func TestSQLiteStore_UpdatePreservesContent(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename: "content-preserve",
		Status:   taskstore.StatusPlanning,
		Branch:   "plan/content-preserve",
	}
	require.NoError(t, store.Create("kasmos", entry))
	require.NoError(t, store.SetContent("kasmos", "content-preserve", "# My Plan\n\n## Wave 1\n"))

	// Simulate an FSM transition: update status without setting content.
	entry.Status = taskstore.StatusReady
	require.NoError(t, store.Update("kasmos", "content-preserve", entry))

	content, err := store.GetContent("kasmos", "content-preserve")
	require.NoError(t, err)
	assert.Equal(t, "# My Plan\n\n## Wave 1\n", content, "content must survive a metadata-only Update")
}

func TestSQLiteStore_Rename(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename:  "old-name",
		Status:    taskstore.StatusReady,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.Create("kasmos", entry))

	require.NoError(t, store.Rename("kasmos", "old-name", "new-name"))

	_, err := store.Get("kasmos", "old-name")
	assert.Error(t, err)

	got, err := store.Get("kasmos", "new-name")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReady, got.Status)
}

func TestSQLiteStore_ListByTopic(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "a", Status: taskstore.StatusReady, Topic: "auth"}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "b", Status: taskstore.StatusReady, Topic: "auth"}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "c", Status: taskstore.StatusReady, Topic: "storage"}))

	plans, err := store.ListByTopic("kasmos", "auth")
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestSQLiteStore_Topics(t *testing.T) {
	store := newTestStore(t)
	topic := taskstore.TopicEntry{
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
	_, err := store.Get("kasmos", "nonexistent")
	assert.Error(t, err)
}

func TestSQLiteStore_CreateDuplicate(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{Filename: "dup", Status: taskstore.StatusReady}
	require.NoError(t, store.Create("kasmos", entry))
	err := store.Create("kasmos", entry)
	assert.Error(t, err)
}

func TestSQLiteStore_ListSortedByFilename(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "c", Status: taskstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "a", Status: taskstore.StatusReady}))
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "b", Status: taskstore.StatusReady}))

	plans, err := store.List("kasmos")
	require.NoError(t, err)
	require.Len(t, plans, 3)
	assert.Equal(t, "a", plans[0].Filename)
	assert.Equal(t, "b", plans[1].Filename)
	assert.Equal(t, "c", plans[2].Filename)
}

func TestSQLiteStore_CreateWithContent(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename: "test",
		Status:   taskstore.StatusReady,
		Content:  "# Test Plan\n\n## Wave 1\n\n### Task 1: Do thing\n",
	}
	require.NoError(t, store.Create("proj", entry))
	got, err := store.Get("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, entry.Content, got.Content)
}

func TestSQLiteStore_GetContent(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename: "test",
		Status:   taskstore.StatusReady,
		Content:  "# Full Plan Content",
	}
	require.NoError(t, store.Create("proj", entry))
	content, err := store.GetContent("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, "# Full Plan Content", content)
}

func TestSQLiteStore_SetContent(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{Filename: "test", Status: taskstore.StatusReady}
	require.NoError(t, store.Create("proj", entry))
	require.NoError(t, store.SetContent("proj", "test", "# Updated"))
	content, err := store.GetContent("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, "# Updated", content)
}

func TestClickUpTaskIDRoundTrip(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{Filename: "clickup-test", Status: taskstore.StatusReady}
	require.NoError(t, store.Create("proj", entry))

	// Initially no task ID
	got, err := store.Get("proj", "clickup-test")
	require.NoError(t, err)
	assert.Equal(t, "", got.ClickUpTaskID, "task ID must be empty before set")

	// Set the task ID
	require.NoError(t, store.SetClickUpTaskID("proj", "clickup-test", "CU-abc123"))

	// Verify it round-trips through Get
	got, err = store.Get("proj", "clickup-test")
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
	err := store.SetClickUpTaskID("proj", "nonexistent", "CU-xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteMigration_PlansTableToTasks(t *testing.T) {
	store, err := taskstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Store should work — the migration creates the tasks table
	err = store.Create("proj", taskstore.TaskEntry{Filename: "test", Status: taskstore.StatusReady})
	require.NoError(t, err)

	entries, err := store.List("proj")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "test", entries[0].Filename)
}

func TestSQLiteStore_ReviewCycle(t *testing.T) {
	store := newTestStore(t)
	entry := taskstore.TaskEntry{
		Filename: "test",
		Status:   taskstore.StatusReady,
	}
	require.NoError(t, store.Create("proj", entry))

	// Default review cycle is 0.
	got, err := store.Get("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, 0, got.ReviewCycle)

	// Increment and verify.
	require.NoError(t, store.IncrementReviewCycle("proj", "test"))
	got, err = store.Get("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, 1, got.ReviewCycle)

	// Increment again.
	require.NoError(t, store.IncrementReviewCycle("proj", "test"))
	got, err = store.Get("proj", "test")
	require.NoError(t, err)
	assert.Equal(t, 2, got.ReviewCycle)
}

func TestSQLiteStore_SubtaskCRUD(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	require.NoError(t, store.SetSubtasks("kasmos", "plan", []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "one", Status: taskstore.SubtaskStatusPending},
		{TaskNumber: 2, Title: "two", Status: taskstore.SubtaskStatusPending},
	}))

	got, err := store.GetSubtasks("kasmos", "plan")
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, 1, got[0].TaskNumber)
	assert.Equal(t, taskstore.SubtaskStatusPending, got[0].Status)

	require.NoError(t, store.UpdateSubtaskStatus("kasmos", "plan", 1, taskstore.SubtaskStatusClosed))
	updated, err := store.GetSubtasks("kasmos", "plan")
	require.NoError(t, err)
	assert.Equal(t, taskstore.SubtaskStatusClosed, updated[0].Status)

	require.NoError(t, store.SetSubtasks("kasmos", "plan", []taskstore.SubtaskEntry{
		{TaskNumber: 2, Title: "replacement", Status: taskstore.SubtaskStatusDone},
	}))
	replaced, err := store.GetSubtasks("kasmos", "plan")
	require.NoError(t, err)
	assert.Len(t, replaced, 1)
	assert.Equal(t, "replacement", replaced[0].Title)
	assert.Equal(t, taskstore.SubtaskStatusDone, replaced[0].Status)
}

func TestSQLiteStore_PhaseTimestamps(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	require.NoError(t, store.SetPhaseTimestamp("kasmos", "plan", "planning", time.Now().UTC()))
	require.NoError(t, store.SetPhaseTimestamp("kasmos", "plan", "implementing", time.Now().UTC()))
	require.NoError(t, store.SetPhaseTimestamp("kasmos", "plan", "reviewing", time.Now().UTC()))
	require.NoError(t, store.SetPhaseTimestamp("kasmos", "plan", "done", time.Now().UTC()))

	got, err := store.Get("kasmos", "plan")
	require.NoError(t, err)
	assert.False(t, got.PlanningAt.IsZero())
	assert.False(t, got.ImplementingAt.IsZero())
	assert.False(t, got.ReviewingAt.IsZero())
	assert.False(t, got.DoneAt.IsZero())

	err = store.SetPhaseTimestamp("kasmos", "plan", "unknown", time.Now().UTC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown phase")
}

func TestSQLiteStore_PlanGoal(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("kasmos", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	require.NoError(t, store.SetPlanGoal("kasmos", "plan", "ship resilient workflow"))

	got, err := store.Get("kasmos", "plan")
	require.NoError(t, err)
	assert.Equal(t, "ship resilient workflow", got.Goal)
}

func TestSQLiteStore_PRMetadata(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := "test"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	require.NoError(t, store.SetPRURL(project, "plan", "https://github.com/org/repo/pull/42"))
	require.NoError(t, store.SetPRState(project, "plan", "APPROVED", "SUCCESS"))

	entry, err := store.Get(project, "plan")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/org/repo/pull/42", entry.PRURL)
	assert.Equal(t, "APPROVED", entry.PRReviewDecision)
	assert.Equal(t, "SUCCESS", entry.PRCheckStatus)
}

func TestSQLiteStore_PRMetadata_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.SetPRURL("test", "nonexistent", "https://github.com/org/repo/pull/42")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	err = store.SetPRState("test", "nonexistent", "APPROVED", "SUCCESS")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_PRReviews_RecordAndList(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	// Record two reviews
	require.NoError(t, store.RecordPRReview("proj", "plan", 101, "CHANGES_REQUESTED", "fix this", "reviewer1"))
	require.NoError(t, store.RecordPRReview("proj", "plan", 102, "COMMENTED", "nit: rename", "reviewer2"))

	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	assert.Len(t, pending, 2)
	assert.Equal(t, 101, pending[0].ReviewID)
	assert.Equal(t, "CHANGES_REQUESTED", pending[0].ReviewState)
	assert.Equal(t, "fix this", pending[0].ReviewBody)
	assert.Equal(t, "reviewer1", pending[0].ReviewerLogin)
	assert.False(t, pending[0].ReactionPosted)
	assert.False(t, pending[0].FixerDispatched)
	assert.False(t, pending[0].CreatedAt.IsZero())
}

func TestSQLiteStore_PRReviews_DuplicateInsertIdempotent(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	// Insert same review ID twice — second call must be a no-op
	require.NoError(t, store.RecordPRReview("proj", "plan", 42, "APPROVED", "lgtm", "alice"))
	require.NoError(t, store.RecordPRReview("proj", "plan", 42, "CHANGES_REQUESTED", "should error but won't", "bob"))

	// Only one row should exist, with the original data
	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, "APPROVED", pending[0].ReviewState, "first record must win")
	assert.Equal(t, "alice", pending[0].ReviewerLogin, "first reviewer must win")
}

func TestSQLiteStore_PRReviews_IsReviewProcessed(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	// Not recorded yet
	assert.False(t, store.IsReviewProcessed("proj", "plan", 99))

	require.NoError(t, store.RecordPRReview("proj", "plan", 99, "COMMENTED", "looks good", "reviewer"))

	// Now recorded
	assert.True(t, store.IsReviewProcessed("proj", "plan", 99))
}

func TestSQLiteStore_PRReviews_MarkReacted(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))
	require.NoError(t, store.RecordPRReview("proj", "plan", 10, "COMMENTED", "body", "reviewer"))

	require.NoError(t, store.MarkReviewReacted("proj", "plan", 10))

	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.True(t, pending[0].ReactionPosted, "reaction_posted must be true after MarkReviewReacted")

	// Not found error
	err = store.MarkReviewReacted("proj", "plan", 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_PRReviews_MarkFixerDispatched(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))
	require.NoError(t, store.RecordPRReview("proj", "plan", 20, "CHANGES_REQUESTED", "fix it", "reviewer"))

	require.NoError(t, store.MarkReviewFixerDispatched("proj", "plan", 20))

	// After marking fixer dispatched, the review should no longer appear in pending list
	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	assert.Len(t, pending, 0, "fixer-dispatched reviews must not appear in pending list")

	// Not found error
	err = store.MarkReviewFixerDispatched("proj", "plan", 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_PRReviews_EmptyPendingList(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	// Must return empty slice, not nil
	assert.NotNil(t, pending)
	assert.Len(t, pending, 0)
}

func TestSQLiteStore_PRReviews_OrderedByReviewID(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{Filename: "plan", Status: taskstore.StatusReady}))

	// Insert in non-sequential order
	require.NoError(t, store.RecordPRReview("proj", "plan", 300, "COMMENTED", "c", "r3"))
	require.NoError(t, store.RecordPRReview("proj", "plan", 100, "COMMENTED", "a", "r1"))
	require.NoError(t, store.RecordPRReview("proj", "plan", 200, "COMMENTED", "b", "r2"))

	pending, err := store.ListPendingReviews("proj", "plan")
	require.NoError(t, err)
	require.Len(t, pending, 3)
	assert.Equal(t, 100, pending[0].ReviewID)
	assert.Equal(t, 200, pending[1].ReviewID)
	assert.Equal(t, 300, pending[2].ReviewID)
}
