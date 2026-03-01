package planstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPS creates a PlanState backed by an in-memory SQLite store for testing.
func newTestPS(t *testing.T) *PlanState {
	t.Helper()
	store := planstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "test-proj", t.TempDir())
	require.NoError(t, err)
	return ps
}

// newTestPSWithStore creates a PlanState and returns the store for direct inspection.
func newTestPSWithStore(t *testing.T) (*PlanState, planstore.Store) {
	t.Helper()
	store := planstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "test-proj", t.TempDir())
	require.NoError(t, err)
	return ps, store
}

func TestLoad(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "my-plan.md", Status: planstore.StatusReady,
	}))
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "done-plan.md", Status: planstore.StatusDone,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 2)
	assert.Equal(t, StatusReady, ps.Plans["my-plan.md"].Status)
	assert.Equal(t, StatusDone, ps.Plans["done-plan.md"].Status)
}

func TestLoadMissing(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, ps.Plans)
}

func TestUnfinished(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: StatusReady},
			"b.md": {Status: StatusImplementing},
			"c.md": {Status: StatusReviewing},
			"d.md": {Status: StatusDone},
			"e.md": {Status: StatusDone},
		},
	}

	unfinished := ps.Unfinished()
	// done and completed are both excluded
	assert.Len(t, unfinished, 3)
	for _, p := range unfinished {
		assert.NotEqual(t, "d.md", p.Filename, "done should be excluded")
		assert.NotEqual(t, "e.md", p.Filename, "completed should be excluded")
	}
}

func TestIsDone(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: StatusDone},
			"b.md": {Status: StatusDone},
		},
	}

	assert.True(t, ps.IsDone("a.md"))
	ps.Plans["c.md"] = PlanEntry{Status: StatusImplementing}
	assert.True(t, ps.IsDone("a.md"))
	assert.False(t, ps.IsDone("missing.md"))

	// Non-terminal statuses are not done
	ps.Plans["rev.md"] = PlanEntry{Status: StatusReviewing}
	assert.False(t, ps.IsDone("rev.md"), "reviewing should not be treated as done")
	ps.Plans["impl.md"] = PlanEntry{Status: StatusImplementing}
	assert.False(t, ps.IsDone("impl.md"), "implementing should not be treated as done")
}

func TestPlanLifecycle(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "test-plan.md", Status: planstore.StatusReady,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Coder picks it up
	require.NoError(t, ps.setStatus("test-plan.md", StatusImplementing))
	unfinished := ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, StatusImplementing, unfinished[0].Status)
	assert.False(t, ps.IsDone("test-plan.md"))

	// Coder finishes — transitions to reviewing
	require.NoError(t, ps.setStatus("test-plan.md", StatusReviewing))
	assert.False(t, ps.IsDone("test-plan.md"))
	unfinished = ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, StatusReviewing, unfinished[0].Status)

	// Reviewer approves — FSM transitions to done (terminal)
	require.NoError(t, ps.setStatus("test-plan.md", StatusDone))
	assert.True(t, ps.IsDone("test-plan.md"))
	assert.Empty(t, ps.Unfinished())

	// Verify persistence: reload and check final state
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusDone, ps2.Plans["test-plan.md"].Status)
}

// TestFullLifecycleNoRespawnLoop walks the complete orchestration state machine and
// asserts that the terminal `done` status is correctly reflected in query methods.
//
// The respawn loop is now prevented by the FSM: once a plan is `done`, the FSM
// rejects any further ReviewApproved events, so a reviewer cannot be re-spawned.
func TestFullLifecycleNoRespawnLoop(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "feature.md", Status: planstore.StatusReady,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Step 1: ready → implementing
	require.NoError(t, ps.setStatus("feature.md", StatusImplementing))
	assert.False(t, ps.IsDone("feature.md"))
	assert.Len(t, ps.Unfinished(), 1)

	// Step 2: implementing → reviewing
	require.NoError(t, ps.setStatus("feature.md", StatusReviewing))
	assert.False(t, ps.IsDone("feature.md"), "reviewing is not done")
	assert.Len(t, ps.Unfinished(), 1, "reviewing should appear in sidebar")

	// Step 3: reviewer approves → done (terminal)
	require.NoError(t, ps.setStatus("feature.md", StatusDone))
	assert.True(t, ps.IsDone("feature.md"), "done must satisfy IsDone")
	assert.Empty(t, ps.Unfinished(), "done must not appear in sidebar unfinished list")

	// Verify persistence
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusDone, ps2.Plans["feature.md"].Status)
	assert.True(t, ps2.IsDone("feature.md"))
	assert.Empty(t, ps2.Unfinished())
}

func TestSetStatus(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "a.md", Status: planstore.StatusImplementing,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	require.NoError(t, ps.setStatus("a.md", StatusReviewing))
	assert.Equal(t, StatusReviewing, ps.Plans["a.md"].Status)

	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, StatusReviewing, ps2.Plans["a.md"].Status)
}

func TestPlanEntryWithTopic(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	createdAt := time.Date(2026, 2, 21, 14, 30, 0, 0, time.UTC)
	require.NoError(t, store.CreateTopic("proj", planstore.TopicEntry{
		Name: "ui-refactor", CreatedAt: createdAt,
	}))
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename:    "2026-02-21-sidebar.md",
		Status:      planstore.StatusImplementing,
		Description: "refactor sidebar",
		Branch:      "plan/sidebar",
		Topic:       "ui-refactor",
		CreatedAt:   createdAt,
	}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	entry := ps.Plans["2026-02-21-sidebar.md"]
	assert.Equal(t, StatusImplementing, entry.Status)
	assert.Equal(t, "refactor sidebar", entry.Description)
	assert.Equal(t, "plan/sidebar", entry.Branch)
	assert.Equal(t, "ui-refactor", entry.Topic)

	topics := ps.Topics()
	require.Len(t, topics, 1)
	assert.Equal(t, "ui-refactor", topics[0].Name)
}

func TestPlansByTopic(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: StatusImplementing, Topic: "ui"},
			"b.md": {Status: StatusReady, Topic: "ui"},
			"c.md": {Status: StatusReady, Topic: ""},
		},
		TopicEntries: map[string]TopicEntry{
			"ui": {CreatedAt: time.Now()},
		},
	}

	byTopic := ps.PlansByTopic("ui")
	assert.Len(t, byTopic, 2)

	ungrouped := ps.UngroupedPlans()
	assert.Len(t, ungrouped, 1)
	assert.Equal(t, "c.md", ungrouped[0].Filename)
}

func TestCreatePlanWithTopic(t *testing.T) {
	ps := newTestPS(t)

	now := time.Now().UTC()
	require.NoError(t, ps.Create("2026-02-21-feat.md", "a feature", "plan/feat", "my-topic", now))

	// Topic should be auto-created
	topics := ps.Topics()
	require.Len(t, topics, 1)
	assert.Equal(t, "my-topic", topics[0].Name)

	entry := ps.Plans["2026-02-21-feat.md"]
	assert.Equal(t, "my-topic", entry.Topic)
	assert.Equal(t, StatusReady, entry.Status)
}

func TestCreatePlanUngrouped(t *testing.T) {
	ps := newTestPS(t)

	now := time.Now().UTC()
	require.NoError(t, ps.Create("2026-02-21-fix.md", "a fix", "plan/fix", "", now))

	topics := ps.Topics()
	assert.Len(t, topics, 0)

	entry := ps.Plans["2026-02-21-fix.md"]
	assert.Equal(t, "", entry.Topic)
}

func TestHasRunningCoderInTopic(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: StatusImplementing, Topic: "ui"},
			"b.md": {Status: StatusReady, Topic: "ui"},
		},
	}

	running, planFile := ps.HasRunningCoderInTopic("ui", "b.md")
	assert.True(t, running)
	assert.Equal(t, "a.md", planFile)

	running, _ = ps.HasRunningCoderInTopic("ui", "a.md")
	assert.False(t, running, "should not flag self")

	running, _ = ps.HasRunningCoderInTopic("other", "x.md")
	assert.False(t, running)
}

func TestRegisterPlan(t *testing.T) {
	ps := newTestPS(t)

	now := time.Date(2026, 2, 21, 15, 4, 5, 0, time.UTC)
	err := ps.Register("2026-02-21-auth-refactor.md", "refactor auth flow", "plan/auth-refactor", now)
	require.NoError(t, err)

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, StatusReady, entry.Status)
	assert.Equal(t, "refactor auth flow", entry.Description)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
	assert.Equal(t, now, entry.CreatedAt)
}

func TestRegisterPlan_RejectsDuplicate(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"2026-02-21-auth-refactor.md": {
				Status:      StatusReady,
				Description: "existing",
				Branch:      "plan/auth-refactor",
				CreatedAt:   time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	err := ps.Register(
		"2026-02-21-auth-refactor.md",
		"new description",
		"plan/auth-refactor",
		time.Now().UTC(),
	)
	assert.Error(t, err)
}

func TestRename(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	// Create old plan file on disk
	oldFile := "2026-02-20-my-feature.md"
	newFile := "2026-02-20-auth-refactor.md"
	oldPath := filepath.Join(dir, oldFile)
	newPath := filepath.Join(dir, newFile)
	require.NoError(t, os.WriteFile(oldPath, []byte("# old plan"), 0o644))

	// Seed the store
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: oldFile, Status: planstore.StatusReady, Branch: "plan/my-feature",
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

	// File renamed on disk
	_, err = os.Stat(oldPath)
	assert.True(t, os.IsNotExist(err), "old file should not exist")
	_, err = os.Stat(newPath)
	assert.NoError(t, err, "new file should exist")

	// Persisted to store
	ps2, err := Load(store, "proj", dir)
	require.NoError(t, err)
	assert.Contains(t, ps2.Plans, newFile)
	assert.NotContains(t, ps2.Plans, oldFile)
}

func TestRenameNonExistentPlan(t *testing.T) {
	ps := newTestPS(t)

	_, err := ps.Rename("nonexistent.md", "new-name")
	assert.Error(t, err)
}

func TestRenameNoFileOnDisk(t *testing.T) {
	// Rename should succeed even if the .md file doesn't exist on disk
	store := planstore.NewTestSQLiteStore(t)
	dir := t.TempDir()
	oldFile := "2026-02-20-my-feature.md"
	newFile := "2026-02-20-new-name.md"

	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: oldFile, Status: planstore.StatusPlanning,
	}))

	ps, err := Load(store, "proj", dir)
	require.NoError(t, err)

	got, err := ps.Rename(oldFile, "new-name")
	require.NoError(t, err)
	assert.Equal(t, newFile, got)
	assert.Contains(t, ps.Plans, newFile)
	assert.NotContains(t, ps.Plans, oldFile)
}

func TestPlanState_WithStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)

	// Create via store
	require.NoError(t, store.Create("test-project", planstore.PlanEntry{
		Filename: "test.md", Status: "ready", Description: "remote plan",
	}))

	// Load PlanState with store
	ps, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 1)
	assert.Equal(t, StatusReady, ps.Plans["test.md"].Status)
}

func TestSetTopic_WithStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)

	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "2026-02-28-feat.md", Status: "ready", Topic: "old-topic",
	}))
	require.NoError(t, store.CreateTopic("proj", planstore.TopicEntry{Name: "old-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Change to a new topic (auto-creates topic entry)
	require.NoError(t, ps.SetTopic("2026-02-28-feat.md", "new-topic"))
	assert.Equal(t, "new-topic", ps.Plans["2026-02-28-feat.md"].Topic)

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
	assert.Equal(t, "new-topic", ps2.Plans["2026-02-28-feat.md"].Topic)
}

func TestSetTopic_ClearTopic(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "2026-02-28-feat.md", Status: "ready", Topic: "some-topic",
	}))
	require.NoError(t, store.CreateTopic("proj", planstore.TopicEntry{Name: "some-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	// Clear topic (empty string)
	require.NoError(t, ps.SetTopic("2026-02-28-feat.md", ""))
	assert.Equal(t, "", ps.Plans["2026-02-28-feat.md"].Topic)

	// Persisted to store
	ps2, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "", ps2.Plans["2026-02-28-feat.md"].Topic)
}

func TestSetTopic_NotFound(t *testing.T) {
	ps := newTestPS(t)

	err := ps.SetTopic("nonexistent.md", "some-topic")
	assert.Error(t, err)
}

func TestSetTopic_RemoteStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)

	// Create plan via store
	require.NoError(t, store.Create("test-project", planstore.PlanEntry{
		Filename: "test.md", Status: "ready", Description: "remote plan", Topic: "old-topic",
	}))
	require.NoError(t, store.CreateTopic("test-project", planstore.TopicEntry{Name: "old-topic", CreatedAt: time.Now().UTC()}))

	ps, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)

	// Change topic via SetTopic — must write through to store
	require.NoError(t, ps.SetTopic("test.md", "new-topic"))
	assert.Equal(t, "new-topic", ps.Plans["test.md"].Topic)

	// Verify persisted in store by reloading
	ps2, err := Load(store, "test-project", "/tmp/unused")
	require.NoError(t, err)
	assert.Equal(t, "new-topic", ps2.Plans["test.md"].Topic)
}

func TestPlanState_CreateWithContent(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	content := "# Auth Refactor\n\n## Wave 1\n"
	err = ps.CreateWithContent("2026-02-28-auth.md", "auth refactor", "plan/auth", "", time.Now(), content)
	require.NoError(t, err)

	got, err := store.GetContent("proj", "2026-02-28-auth.md")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestPlanState_GetContent(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	content := "# Plan Content"
	require.NoError(t, ps.CreateWithContent("test.md", "", "", "", time.Now(), content))

	got, err := ps.GetContent("test.md")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestPlanState_LoadRequiresStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", planstore.PlanEntry{
		Filename: "test.md", Status: planstore.StatusReady,
	}))

	// Load should work with a store and no plan-state.json on disk
	ps, err := Load(store, "proj", t.TempDir())
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 1)
}
