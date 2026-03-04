package cmd

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestPlanState creates an in-memory SQLite store pre-populated with two
// test plans and returns the store and a temp plans directory.
// The plans directory is structured as <root>/docs/plans so that
// projectFromPlansDir returns a stable project name derived from <root>.
func setupTestPlanState(t *testing.T) (taskstore.Store, string) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)

	// Build a plans dir with the expected structure so projectFromPlansDir
	// returns a known project name.
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	project := projectFromPlansDir(plansDir)

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "test-plan.md",
		Status:      taskstore.StatusReady,
		Description: "test plan",
		Branch:      "plan/test-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "implementing-plan.md",
		Status:      taskstore.Status("implementing"),
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
		CreatedAt:   time.Now(),
	}))

	return store, plansDir
}

func TestPlanList(t *testing.T) {
	store, dir := setupTestPlanState(t)

	tests := []struct {
		name           string
		statusFilter   string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "all plans",
			wantContains: []string{"test-plan.md", "implementing-plan.md"},
		},
		{
			name:           "filter by ready",
			statusFilter:   "ready",
			wantContains:   []string{"test-plan.md"},
			wantNotContain: []string{"implementing-plan.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := executeTaskList(dir, tt.statusFilter, store)
			for _, want := range tt.wantContains {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}

func TestPlanSetStatus(t *testing.T) {
	store, dir := setupTestPlanState(t)

	// Requires --force
	err := executeTaskSetStatus(dir, "test-plan.md", "done", false, store)
	assert.Error(t, err, "should require --force flag")

	// Valid override
	err = executeTaskSetStatus(dir, "test-plan.md", "done", true, store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, ok := ps.Entry("test-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.Status("done"), entry.Status)

	// Invalid status
	err = executeTaskSetStatus(dir, "test-plan.md", "bogus", true, store)
	assert.Error(t, err, "should reject invalid status")
}

func TestPlanTransition(t *testing.T) {
	store, dir := setupTestPlanState(t)

	// Valid transition: ready → planning via plan_start
	newStatus, err := executeTaskTransition(dir, "test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)

	// Invalid transition (plan is now in "planning" state)
	_, err = executeTaskTransition(dir, "test-plan.md", "review_approved", store)
	assert.Error(t, err)
}

func TestPlanCLI_EndToEnd(t *testing.T) {
	store, dir := setupTestPlanState(t)
	// dir is <root>/docs/plans; signals go to <root>/.kasmos/signals/
	repoRoot := filepath.Dir(filepath.Dir(dir))
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// List all
	output := executeTaskList(dir, "", store)
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "implementing")

	// Transition ready → planning
	status, err := executeTaskTransition(dir, "test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", status)

	// Force set back to ready
	err = executeTaskSetStatus(dir, "test-plan.md", "ready", true, store)
	require.NoError(t, err)

	// Implement with wave signal
	err = executeTaskImplement(dir, "test-plan.md", 2, store)
	require.NoError(t, err)

	// Verify signal file
	entries, _ := os.ReadDir(signalsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "implement-wave-2-test-plan.md")

	// Verify final status
	ps, err := taskstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("test-plan.md")
	assert.Equal(t, taskstate.Status("implementing"), entry.Status)
}

func TestPlanImplement(t *testing.T) {
	store, dir := setupTestPlanState(t)
	// dir is <root>/docs/plans; signals go to <root>/.kasmos/signals/
	repoRoot := filepath.Dir(filepath.Dir(dir))
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	err := executeTaskImplement(dir, "test-plan.md", 1, store)
	require.NoError(t, err)

	// Verify plan transitioned to implementing
	ps, err := taskstate.Load(store, projectFromPlansDir(dir), dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("test-plan.md")
	assert.Equal(t, taskstate.Status("implementing"), entry.Status)

	// Verify signal file created
	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	var found bool
	for _, e := range entries {
		if e.Name() == "implement-wave-1-test-plan.md" {
			found = true
		}
	}
	assert.True(t, found, "signal file should exist")
}

func TestPlanRegister(t *testing.T) {
	store, dir := setupTestPlanState(t)
	project := projectFromPlansDir(dir)

	planFile := "new-feature.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, planFile),
		[]byte("# New Feature Plan\n\nSome content."),
		0o644,
	))

	err := executeTaskRegister(dir, planFile, "", "", "", store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	assert.Equal(t, "New Feature Plan", entry.Description)
	assert.Equal(t, "plan/new-feature", entry.Branch)
	assert.Equal(t, "", entry.Topic)
}

func TestPlanRegister_WithTopicAndDescription(t *testing.T) {
	store, dir := setupTestPlanState(t)
	project := projectFromPlansDir(dir)

	planFile := "stub-plan.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, planFile),
		[]byte("# Stub Plan\n"),
		0o644,
	))

	err := executeTaskRegister(dir, planFile, "", "brain phase 1", "Implement circuit breaker", store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	assert.Equal(t, "Implement circuit breaker", entry.Description)
	assert.Equal(t, "brain phase 1", entry.Topic)

	topics := ps.Topics()
	topicNames := make([]string, len(topics))
	for i, topicInfo := range topics {
		topicNames[i] = topicInfo.Name
	}
	assert.Contains(t, topicNames, "brain phase 1")
}

func TestExecutePlanLinkClickUp(t *testing.T) {
	store, err := taskstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create a plan with ClickUp source in content.
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "test.md",
		Status:   taskstore.StatusReady,
	}))
	require.NoError(t, store.SetContent("proj", "test.md", "# Test\n\n**Source:** ClickUp abc123 (https://app.clickup.com/t/abc123)\n"))

	n, err := executeTaskLinkClickUp("proj", store)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := store.Get("proj", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "abc123", got.ClickUpTaskID)
}

func TestExecutePlanLinkClickUp_SkipsAlreadyLinked(t *testing.T) {
	store, err := taskstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Plan already has a ClickUp task ID — should be skipped.
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename:      "linked.md",
		Status:        taskstore.StatusReady,
		ClickUpTaskID: "already-set",
	}))
	require.NoError(t, store.SetContent("proj", "linked.md", "# Test\n\n**Source:** ClickUp newid\n"))

	n, err := executeTaskLinkClickUp("proj", store)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "already linked plan should not be updated")
}

func TestResolveRepoRoot_Worktree(t *testing.T) {
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, "docs", "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "test-wt"), 0o755))

	worktree := t.TempDir()
	gitFile := fmt.Sprintf("gitdir: %s/.git/worktrees/test-wt\n", mainRepo)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitFile), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mainRepo, ".git", "worktrees", "test-wt", "commondir"),
		[]byte("../..\n"), 0o644,
	))

	root, err := resolveRepoRoot(worktree)
	require.NoError(t, err)
	assert.Equal(t, mainRepo, root)
}

func TestResolveRepoRoot_MainRepo(t *testing.T) {
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git"), 0o755))

	root, err := resolveRepoRoot(mainRepo)
	require.NoError(t, err)
	assert.Equal(t, mainRepo, root)
}

func TestPlanCLI_FromWorktreeContext(t *testing.T) {
	// Create main repo structure with docs/plans
	mainRepo := t.TempDir()
	plansDir := filepath.Join(mainRepo, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "wt-plans"), 0o755))

	// Create a worktree that has a .git file pointing to the main repo's worktrees dir
	worktree := t.TempDir()
	gitFile := fmt.Sprintf("gitdir: %s/.git/worktrees/wt-plans\n", mainRepo)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitFile), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mainRepo, ".git", "worktrees", "wt-plans", "commondir"),
		[]byte("../..\n"), 0o644,
	))

	// Test resolvePlansDir end-to-end by chdir'ing into the worktree.
	// The worktree has no docs/plans/ — resolvePlansDir must resolve via
	// resolveRepoRoot back to the main repo.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })
	require.NoError(t, os.Chdir(worktree))

	resolvedPlansDir, err := resolvePlansDir()
	require.NoError(t, err)
	assert.Equal(t, plansDir, resolvedPlansDir)

	// Populate the store and verify plan operations succeed
	store := taskstore.NewTestSQLiteStore(t)
	project := projectFromPlansDir(resolvedPlansDir)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "worktree-plan.md",
		Status:      taskstore.StatusReady,
		Description: "worktree plan",
		Branch:      "plan/worktree-plan",
		CreatedAt:   time.Now(),
	}))

	// executeTaskList should succeed from the resolved plansDir
	output := executeTaskList(resolvedPlansDir, "", store)
	assert.Contains(t, output, "worktree-plan.md")

	// executeTaskSetStatus should succeed
	err = executeTaskSetStatus(resolvedPlansDir, "worktree-plan.md", "done", true, store)
	require.NoError(t, err)

	// executeTaskTransition: reset then transition
	err = executeTaskSetStatus(resolvedPlansDir, "worktree-plan.md", "ready", true, store)
	require.NoError(t, err)
	newStatus, err := executeTaskTransition(resolvedPlansDir, "worktree-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)
}

// TestPlanList_WithStore verifies that executeTaskListWithStore works with a
// store-backed HTTP server, returning plan entries from the remote store.
func TestExecuteTaskRegisterIngestsContent(t *testing.T) {
	store, err := taskstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create a temp plansDir with the expected structure.
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	planContent := "# My Plan\n\n## Wave 1\n\n### Task 1: Do something\n\nDo it.\n"
	planFile := "my-plan.md"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	err = executeTaskRegister(plansDir, planFile, "", "", "", store)
	require.NoError(t, err)

	// Verify content was ingested into the store.
	got, err := store.GetContent(projectFromPlansDir(plansDir), planFile)
	require.NoError(t, err)
	assert.Equal(t, planContent, got)
}

func TestPlanList_WithStore(t *testing.T) {
	backend := taskstore.NewTestSQLiteStore(t)
	srv := httptest.NewServer(taskstore.NewHandler(backend))
	defer srv.Close()

	err := backend.Create("test-project", taskstore.TaskEntry{
		Filename: "test.md", Status: "ready", Description: "test plan",
	})
	require.NoError(t, err)

	output := executeTaskListWithStore(srv.URL, "test-project")
	assert.Contains(t, output, "test.md")
	assert.Contains(t, output, "ready")
}
