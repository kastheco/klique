package taskstate

import (
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPS creates a TaskState backed by an in-memory SQLite store for testing.
func newTestPS(t *testing.T) *TaskState {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "test-proj", t.TempDir())
	require.NoError(t, err)
	return ps
}

// newTestPSWithStore creates a TaskState and returns the store for direct inspection.
func newTestPSWithStore(t *testing.T) (*TaskState, taskstore.Store) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "test-proj", t.TempDir())
	require.NoError(t, err)
	return ps, store
}

func TestLoad(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "my-plan", Status: taskstore.StatusReady,
	}))
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "done-plan", Status: taskstore.StatusDone,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 2)
	assert.Equal(t, StatusReady, ps.Plans["my-plan"].Status)
	assert.Equal(t, StatusDone, ps.Plans["done-plan"].Status)
}

func TestLoadMissing(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, ps.Plans)
}

func TestLoad_BackfillsGoalFromContent(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	content := "# Test\n\n**Goal:** ship planner metadata\n\n## Wave 1\n\n### Task 1: parse goal\n\nDo it.\n"
	plan, err := taskparser.Parse(content)
	require.NoError(t, err)
	require.NotEmpty(t, plan.Goal)

	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "goal-backfill",
		Status:   taskstore.StatusReady,
		Content:  content,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, plan.Goal, ps.Plans["goal-backfill"].Goal)

	entry, err := store.Get("proj", "goal-backfill")
	require.NoError(t, err)
	assert.Equal(t, plan.Goal, entry.Goal)
}

func TestUnfinished(t *testing.T) {
	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			"a": {Status: StatusReady},
			"b": {Status: StatusImplementing},
			"c": {Status: StatusReviewing},
			"d": {Status: StatusDone},
			"e": {Status: StatusDone},
		},
	}

	unfinished := ps.Unfinished()
	// done and completed are both excluded
	assert.Len(t, unfinished, 3)
	for _, p := range unfinished {
		assert.NotEqual(t, "d", p.Filename, "done should be excluded")
		assert.NotEqual(t, "e", p.Filename, "completed should be excluded")
	}
}

func TestIsDone(t *testing.T) {
	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			"a": {Status: StatusDone},
			"b": {Status: StatusDone},
		},
	}

	assert.True(t, ps.IsDone("a"))
	ps.Plans["c"] = TaskEntry{Status: StatusImplementing}
	assert.True(t, ps.IsDone("a"))
	assert.False(t, ps.IsDone("missing"))

	// Non-terminal statuses are not done
	ps.Plans["rev"] = TaskEntry{Status: StatusReviewing}
	assert.False(t, ps.IsDone("rev"), "reviewing should not be treated as done")
	ps.Plans["impl"] = TaskEntry{Status: StatusImplementing}
	assert.False(t, ps.IsDone("impl"), "implementing should not be treated as done")
}

func TestPlanLifecycle(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "test-plan", Status: taskstore.StatusReady,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Coder picks it up
	require.NoError(t, ps.setStatus("test-plan", StatusImplementing))
	unfinished := ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, StatusImplementing, unfinished[0].Status)
	assert.False(t, ps.IsDone("test-plan"))

	// Coder finishes — transitions to reviewing
	require.NoError(t, ps.setStatus("test-plan", StatusReviewing))
	assert.False(t, ps.IsDone("test-plan"))
	unfinished = ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, StatusReviewing, unfinished[0].Status)

	// Reviewer approves — FSM transitions to done (terminal)
	require.NoError(t, ps.setStatus("test-plan", StatusDone))
	assert.True(t, ps.IsDone("test-plan"))
	assert.Empty(t, ps.Unfinished())

	// Verify persistence: reload and check final state
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusDone, ps2.Plans["test-plan"].Status)
}

// TestFullLifecycleNoRespawnLoop walks the complete orchestration state machine and
// asserts that the terminal `done` status is correctly reflected in query methods.
//
// The respawn loop is now prevented by the FSM: once a plan is `done`, the FSM
// rejects any further ReviewApproved events, so a reviewer cannot be re-spawned.
func TestFullLifecycleNoRespawnLoop(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "feature", Status: taskstore.StatusReady,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Step 1: ready → implementing
	require.NoError(t, ps.setStatus("feature", StatusImplementing))
	assert.False(t, ps.IsDone("feature"))
	assert.Len(t, ps.Unfinished(), 1)

	// Step 2: implementing → reviewing
	require.NoError(t, ps.setStatus("feature", StatusReviewing))
	assert.False(t, ps.IsDone("feature"), "reviewing is not done")
	assert.Len(t, ps.Unfinished(), 1, "reviewing should appear in sidebar")

	// Step 3: reviewer approves → done (terminal)
	require.NoError(t, ps.setStatus("feature", StatusDone))
	assert.True(t, ps.IsDone("feature"), "done must satisfy IsDone")
	assert.Empty(t, ps.Unfinished(), "done must not appear in sidebar unfinished list")

	// Verify persistence
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusDone, ps2.Plans["feature"].Status)
	assert.True(t, ps2.IsDone("feature"))
	assert.Empty(t, ps2.Unfinished())
}

func TestSetStatus(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "a", Status: taskstore.StatusImplementing,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	require.NoError(t, ps.setStatus("a", StatusReviewing))
	assert.Equal(t, StatusReviewing, ps.Plans["a"].Status)

	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusReviewing, ps2.Plans["a"].Status)
}

func TestTaskEntryWithTopic(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	createdAt := time.Date(2026, 2, 21, 14, 30, 0, 0, time.UTC)
	require.NoError(t, store.CreateTopic("proj", taskstore.TopicEntry{
		Name: "ui-refactor", CreatedAt: createdAt,
	}))
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename:    "sidebar",
		Status:      taskstore.StatusImplementing,
		Description: "refactor sidebar",
		Branch:      "plan/sidebar",
		Topic:       "ui-refactor",
		CreatedAt:   createdAt,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	entry := ps.Plans["sidebar"]
	assert.Equal(t, StatusImplementing, entry.Status)
	assert.Equal(t, "refactor sidebar", entry.Description)
	assert.Equal(t, "plan/sidebar", entry.Branch)
	assert.Equal(t, "ui-refactor", entry.Topic)

	topics := ps.Topics()
	require.Len(t, topics, 1)
	assert.Equal(t, "ui-refactor", topics[0].Name)
}

func TestPlansByTopic(t *testing.T) {
	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			"a": {Status: StatusImplementing, Topic: "ui"},
			"b": {Status: StatusReady, Topic: "ui"},
			"c": {Status: StatusReady, Topic: ""},
		},
		TopicEntries: map[string]TopicEntry{
			"ui": {CreatedAt: time.Now()},
		},
	}

	byTopic := ps.TasksByTopic("ui")
	assert.Len(t, byTopic, 2)

	ungrouped := ps.UngroupedTasks()
	assert.Len(t, ungrouped, 1)
	assert.Equal(t, "c", ungrouped[0].Filename)
}

func TestCreatePlanWithTopic(t *testing.T) {
	ps := newTestPS(t)

	now := time.Now().UTC()
	require.NoError(t, ps.Create("feat", "a feature", "plan/feat", "my-topic", now))

	// Topic should be auto-created
	topics := ps.Topics()
	require.Len(t, topics, 1)
	assert.Equal(t, "my-topic", topics[0].Name)

	entry := ps.Plans["feat"]
	assert.Equal(t, "my-topic", entry.Topic)
	assert.Equal(t, StatusReady, entry.Status)
}

func TestCreatePlanUngrouped(t *testing.T) {
	ps := newTestPS(t)

	now := time.Now().UTC()
	require.NoError(t, ps.Create("fix", "a fix", "plan/fix", "", now))

	topics := ps.Topics()
	assert.Len(t, topics, 0)

	entry := ps.Plans["fix"]
	assert.Equal(t, "", entry.Topic)
}

func TestHasRunningCoderInTopic(t *testing.T) {
	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			"a": {Status: StatusImplementing, Topic: "ui"},
			"b": {Status: StatusReady, Topic: "ui"},
		},
	}

	running, planFile := ps.HasRunningCoderInTopic("ui", "b")
	assert.True(t, running)
	assert.Equal(t, "a", planFile)

	running, _ = ps.HasRunningCoderInTopic("ui", "a")
	assert.False(t, running, "should not flag self")

	running, _ = ps.HasRunningCoderInTopic("other", "x")
	assert.False(t, running)
}

func TestRegisterPlan(t *testing.T) {
	ps := newTestPS(t)

	now := time.Date(2026, 2, 21, 15, 4, 5, 0, time.UTC)
	err := ps.Register("auth-refactor", "refactor auth flow", "plan/auth-refactor", now)
	require.NoError(t, err)

	entry, ok := ps.Entry("auth-refactor")
	require.True(t, ok)
	assert.Equal(t, StatusReady, entry.Status)
	assert.Equal(t, "refactor auth flow", entry.Description)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
	assert.Equal(t, now, entry.CreatedAt)
}

func TestRegisterPlan_RejectsDuplicate(t *testing.T) {
	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			"auth-refactor": {
				Status:      StatusReady,
				Description: "existing",
				Branch:      "plan/auth-refactor",
				CreatedAt:   time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	err := ps.Register(
		"auth-refactor",
		"new description",
		"plan/auth-refactor",
		time.Now().UTC(),
	)
	assert.Error(t, err)
}

func TestDisplayName_NoSuffix(t *testing.T) {
	assert.Equal(t, "auth-refactor", DisplayName("auth-refactor"))
}

func TestRename(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	oldFile := "my-feature"
	newFile := "auth-refactor"

	// Seed the store
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: oldFile, Status: taskstore.StatusReady, Branch: "plan/my-feature",
	}))

	ps, err := Load(store, "proj", dir)
	require.NoError(t, err)

	newFilename, err := ps.Rename(oldFile, "auth-refactor")
	require.NoError(t, err)
	assert.Equal(t, newFile, newFilename)

	// Old key removed, new key added with same entry
	assert.NotContains(t, ps.Plans, oldFile)
	assert.Contains(t, ps.Plans, newFile)
	assert.Equal(t, StatusReady, ps.Plans[newFile].Status)
	assert.Equal(t, "plan/my-feature", ps.Plans[newFile].Branch)

	// Persisted to store
	ps2, err := Load(store, "proj", dir)
	require.NoError(t, err)
	assert.Contains(t, ps2.Plans, newFile)
	assert.NotContains(t, ps2.Plans, oldFile)
}

func TestRenameNonExistentPlan(t *testing.T) {
	ps := newTestPS(t)

	_, err := ps.Rename("nonexistent", "new-name")
	assert.Error(t, err)
}

func TestRenameNoFileOnDisk(t *testing.T) {
	// Rename should succeed even if the file doesn't exist on disk
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()
	oldFile := "my-feature"
	newFile := "new-name"

	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: oldFile, Status: taskstore.StatusPlanning,
	}))

	ps, err := Load(store, "proj", dir)
	require.NoError(t, err)

	got, err := ps.Rename(oldFile, "new-name")
	require.NoError(t, err)
	assert.Equal(t, newFile, got)
	assert.Contains(t, ps.Plans, newFile)
	assert.NotContains(t, ps.Plans, oldFile)
}

func TestTaskState_WithStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)

	// Create via store
	require.NoError(t, store.Create("test-project", taskstore.TaskEntry{
		Filename: "test", Status: "ready", Description: "remote plan",
	}))

	// Load TaskState with store
	ps, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 1)
	assert.Equal(t, StatusReady, ps.Plans["test"].Status)
}

func TestSetTopic_WithStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)

	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "feat", Status: "ready", Topic: "old-topic",
	}))
	require.NoError(t, store.CreateTopic("proj", taskstore.TopicEntry{Name: "old-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Change to a new topic (auto-creates topic entry)
	require.NoError(t, ps.SetTopic("feat", "new-topic"))
	assert.Equal(t, "new-topic", ps.Plans["feat"].Topic)

	// Topic entry should be auto-created
	topics := ps.Topics()
	topicNames := make([]string, len(topics))
	for i, t := range topics {
		topicNames[i] = t.Name
	}
	assert.Contains(t, topicNames, "new-topic")

	// Persisted to store
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "new-topic", ps2.Plans["feat"].Topic)
}

func TestSetTopic_ClearTopic(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "feat", Status: "ready", Topic: "some-topic",
	}))
	require.NoError(t, store.CreateTopic("proj", taskstore.TopicEntry{Name: "some-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Clear topic (empty string)
	require.NoError(t, ps.SetTopic("feat", ""))
	assert.Equal(t, "", ps.Plans["feat"].Topic)

	// Persisted to store
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "", ps2.Plans["feat"].Topic)
}

func TestSetTopic_NotFound(t *testing.T) {
	ps := newTestPS(t)

	err := ps.SetTopic("nonexistent", "some-topic")
	assert.Error(t, err)
}

func TestSetTopic_RemoteStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)

	// Create plan via store
	require.NoError(t, store.Create("test-project", taskstore.TaskEntry{
		Filename: "test", Status: "ready", Description: "remote plan", Topic: "old-topic",
	}))
	require.NoError(t, store.CreateTopic("test-project", taskstore.TopicEntry{Name: "old-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)

	// Change topic via SetTopic — must write through to store
	require.NoError(t, ps.SetTopic("test", "new-topic"))
	assert.Equal(t, "new-topic", ps.Plans["test"].Topic)

	// Verify persisted in store by reloading
	ps2, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)
	assert.Equal(t, "new-topic", ps2.Plans["test"].Topic)
}

func TestTaskState_CreateWithContent(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	content := "# Auth Refactor\n\n## Wave 1\n"
	err = ps.CreateWithContent("auth", "auth refactor", "plan/auth", "", time.Now(), content)
	require.NoError(t, err)

	got, err := store.GetContent("proj", "auth")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestTaskState_GetContent(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	content := "# Plan Content"
	require.NoError(t, ps.CreateWithContent("test", "", "", "", time.Now(), content))

	got, err := ps.GetContent("test")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestTaskState_IngestContent_PopulatesGoalAndSubtasks(t *testing.T) {
	ps := newTestPS(t)
	require.NoError(t, ps.Create("plan", "", "plan/plan", "", time.Now().UTC()))

	content := `# Plan

**Goal:** improve info tab metadata

## Wave 1

### Task 1: persist goal
write goal to store

### Task 2: ingest subtasks
write subtask rows

## Wave 2

### Task 3: orchestrator persistence
persist task status updates
`

	require.NoError(t, ps.IngestContent("plan", content))

	storedContent, err := ps.GetContent("plan")
	require.NoError(t, err)
	assert.Equal(t, content, storedContent)

	entry, ok := ps.Entry("plan")
	require.True(t, ok)
	assert.Equal(t, "improve info tab metadata", entry.Goal)

	subtasks, err := ps.GetSubtasks("plan")
	require.NoError(t, err)
	require.Len(t, subtasks, 3)

	assert.Equal(t, 1, subtasks[0].TaskNumber)
	assert.Equal(t, "persist goal", subtasks[0].Title)
	assert.Equal(t, taskstore.SubtaskStatusPending, subtasks[0].Status)

	assert.Equal(t, 2, subtasks[1].TaskNumber)
	assert.Equal(t, "ingest subtasks", subtasks[1].Title)
	assert.Equal(t, taskstore.SubtaskStatusPending, subtasks[1].Status)

	assert.Equal(t, 3, subtasks[2].TaskNumber)
	assert.Equal(t, "orchestrator persistence", subtasks[2].Title)
	assert.Equal(t, taskstore.SubtaskStatusPending, subtasks[2].Status)
}

func TestTaskState_IngestContent_ParseFailureStillStoresContent(t *testing.T) {
	ps, store := newTestPSWithStore(t)
	require.NoError(t, ps.Create("plan", "", "plan/plan", "", time.Now().UTC()))

	seeded := []taskstore.SubtaskEntry{{
		TaskNumber: 1,
		Title:      "existing",
		Status:     taskstore.SubtaskStatusDone,
	}}
	require.NoError(t, store.SetSubtasks("test-proj", "plan", seeded))

	invalidContent := "# Plan\n\n**Goal:** parsed but no waves"
	err := ps.IngestContent("plan", invalidContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse plan content")
	var warn *IngestWarning
	assert.ErrorAs(t, err, &warn, "expected IngestWarning so callers can treat it as non-fatal")

	storedContent, contentErr := ps.GetContent("plan")
	require.NoError(t, contentErr)
	assert.Equal(t, invalidContent, storedContent)

	subtasks, subtaskErr := ps.GetSubtasks("plan")
	require.NoError(t, subtaskErr)
	require.Len(t, subtasks, 1)
	assert.Equal(t, seeded[0].TaskNumber, subtasks[0].TaskNumber)
	assert.Equal(t, seeded[0].Title, subtasks[0].Title)
	assert.Equal(t, seeded[0].Status, subtasks[0].Status)
}

func TestTaskState_LoadRequiresStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "test", Status: taskstore.StatusReady,
	}))

	// Load should work with a store and no plan-state.json on disk
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 1)
}

func TestFinished_SortedByDoneAtDescending(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	ps := &TaskState{
		Dir: "/tmp",
		Plans: map[string]TaskEntry{
			// created newest but done oldest — should sort last
			"oldest-done": {Status: StatusDone, CreatedAt: t3, DoneAt: t1},
			// created oldest but done newest — should sort first
			"newest-done": {Status: StatusDone, CreatedAt: t1, DoneAt: t3},
			// in the middle by done time
			"middle-done": {Status: StatusDone, CreatedAt: t2, DoneAt: t2},
			// non-done plans must not appear
			"active": {Status: StatusImplementing, CreatedAt: t3, DoneAt: time.Time{}},
		},
	}

	finished := ps.Finished()
	require.Len(t, finished, 3)
	assert.Equal(t, "newest-done", finished[0].Filename, "newest done must sort first")
	assert.Equal(t, "middle-done", finished[1].Filename)
	assert.Equal(t, "oldest-done", finished[2].Filename, "oldest done must sort last")
}

func TestSetClickUpTaskID(t *testing.T) {
	ps := newTestPS(t)
	require.NoError(t, ps.Create("cu-test", "clickup test", "plan/cu-test", "", time.Now()))

	// Initially empty
	entry, ok := ps.Entry("cu-test")
	require.True(t, ok)
	assert.Equal(t, "", entry.ClickUpTaskID, "task ID must be empty before set")

	// Set the task ID
	require.NoError(t, ps.SetClickUpTaskID("cu-test", "CU-abc456"))

	// In-memory state is updated
	entry, ok = ps.Entry("cu-test")
	require.True(t, ok)
	assert.Equal(t, "CU-abc456", entry.ClickUpTaskID, "in-memory task ID must be updated")
}

func TestClickUpTaskIDEmpty(t *testing.T) {
	ps := newTestPS(t)
	require.NoError(t, ps.Create("empty-cu", "no clickup", "plan/empty-cu", "", time.Now()))

	// Entry without ClickUpTaskID must have empty string
	entry, ok := ps.Entry("empty-cu")
	require.True(t, ok)
	assert.Equal(t, "", entry.ClickUpTaskID, "new plan must have empty ClickUpTaskID")
}

func TestSetClickUpTaskID_NotFound(t *testing.T) {
	ps := newTestPS(t)
	err := ps.SetClickUpTaskID("nonexistent", "CU-xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTaskState_ReviewCycle(t *testing.T) {
	ps := newTestPS(t)
	require.NoError(t, ps.Create("test", "desc", "plan/test", "", time.Now()))

	cycle, err := ps.ReviewCycle("test")
	require.NoError(t, err)
	assert.Equal(t, 0, cycle)

	require.NoError(t, ps.IncrementReviewCycle("test"))
	cycle, err = ps.ReviewCycle("test")
	require.NoError(t, err)
	assert.Equal(t, 1, cycle)
}
