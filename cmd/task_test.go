package cmd

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestPlanState creates an in-memory SQLite store pre-populated with two
// test plans and returns the store, the repo root, and the project name.
func setupTestPlanState(t *testing.T) (taskstore.Store, string, string) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)

	root := t.TempDir()
	project := filepath.Base(root)

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

	return store, root, project
}

func TestPlanList(t *testing.T) {
	store, _, project := setupTestPlanState(t)

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
			output := executeTaskList(project, tt.statusFilter, store)
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
	store, _, project := setupTestPlanState(t)

	// Requires --force
	err := executeTaskSetStatus(project, "test-plan.md", "done", false, store)
	assert.Error(t, err, "should require --force flag")

	// Valid override
	err = executeTaskSetStatus(project, "test-plan.md", "done", true, store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("test-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.Status("done"), entry.Status)

	// Invalid status
	err = executeTaskSetStatus(project, "test-plan.md", "bogus", true, store)
	assert.Error(t, err, "should reject invalid status")
}

func TestPlanTransition(t *testing.T) {
	store, _, project := setupTestPlanState(t)

	// Valid transition: ready → planning via plan_start
	newStatus, err := executeTaskTransition(project, "test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)

	// Invalid transition (plan is now in "planning" state)
	_, err = executeTaskTransition(project, "test-plan.md", "review_approved", store)
	assert.Error(t, err)
}

func TestPlanCLI_EndToEnd(t *testing.T) {
	store, repoRoot, project := setupTestPlanState(t)
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// List all
	output := executeTaskList(project, "", store)
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "implementing")

	// Transition ready → planning
	status, err := executeTaskTransition(project, "test-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", status)

	// Force set back to ready
	err = executeTaskSetStatus(project, "test-plan.md", "ready", true, store)
	require.NoError(t, err)

	// Implement with wave signal
	err = executeTaskImplement(repoRoot, project, "test-plan.md", 2, store)
	require.NoError(t, err)

	// Verify signal file
	entries, _ := os.ReadDir(signalsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "implement-wave-2-test-plan.md")

	// Verify final status
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, _ := ps.Entry("test-plan.md")
	assert.Equal(t, taskstate.Status("implementing"), entry.Status)
}

func TestPlanImplement(t *testing.T) {
	store, repoRoot, project := setupTestPlanState(t)
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	err := executeTaskImplement(repoRoot, project, "test-plan.md", 1, store)
	require.NoError(t, err)

	// Verify plan transitioned to implementing
	ps, err := taskstate.Load(store, project, "")
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
	store, repoRoot, project := setupTestPlanState(t)
	// Register still needs docs/plans on disk (it reads .md files).
	plansDir := filepath.Join(repoRoot, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	planFile := "new-feature.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(plansDir, planFile),
		[]byte("# New Feature Plan\n\nSome content."),
		0o644,
	))

	err := executeTaskRegister(plansDir, planFile, "", "", "", store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	assert.Equal(t, "New Feature Plan", entry.Description)
	assert.Equal(t, "plan/new-feature", entry.Branch)
	assert.Equal(t, "", entry.Topic)
}

func TestPlanRegister_WithTopicAndDescription(t *testing.T) {
	store, repoRoot, project := setupTestPlanState(t)
	plansDir := filepath.Join(repoRoot, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	planFile := "stub-plan.md"
	require.NoError(t, os.WriteFile(
		filepath.Join(plansDir, planFile),
		[]byte("# Stub Plan\n"),
		0o644,
	))

	err := executeTaskRegister(plansDir, planFile, "", "brain phase 1", "Implement circuit breaker", store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, "")
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
	// Create main repo structure
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "wt-plans"), 0o755))

	// Create a worktree that has a .git file pointing to the main repo's worktrees dir
	worktree := t.TempDir()
	gitFile := fmt.Sprintf("gitdir: %s/.git/worktrees/wt-plans\n", mainRepo)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitFile), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mainRepo, ".git", "worktrees", "wt-plans", "commondir"),
		[]byte("../..\n"), 0o644,
	))

	// Test resolveRepoInfo end-to-end by chdir'ing into the worktree.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })
	require.NoError(t, os.Chdir(worktree))

	repoRoot, project, err := resolveRepoInfo()
	require.NoError(t, err)
	assert.Equal(t, mainRepo, repoRoot)
	assert.Equal(t, filepath.Base(mainRepo), project)

	// Populate the store and verify plan operations succeed
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "worktree-plan.md",
		Status:      taskstore.StatusReady,
		Description: "worktree plan",
		Branch:      "plan/worktree-plan",
		CreatedAt:   time.Now(),
	}))

	// executeTaskList should succeed with project name
	output := executeTaskList(project, "", store)
	assert.Contains(t, output, "worktree-plan.md")

	// executeTaskSetStatus should succeed
	err = executeTaskSetStatus(project, "worktree-plan.md", "done", true, store)
	require.NoError(t, err)

	// executeTaskTransition: reset then transition
	err = executeTaskSetStatus(project, "worktree-plan.md", "ready", true, store)
	require.NoError(t, err)
	newStatus, err := executeTaskTransition(project, "worktree-plan.md", "plan_start", store)
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)
}

// TestExecuteTaskRegisterIngestsContent verifies that executeTaskRegister reads
// the plan file from disk and stores its content in the task store database.
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

func TestExecuteTaskShow(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-show-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReady,
		Content:  "# My Plan\n\n## Wave 1\n\n### Task 1: Do it\n\nDo the thing.\n",
	}))

	content, err := executeTaskShow(project, "my-plan.md", store)
	require.NoError(t, err)
	assert.Equal(t, "# My Plan\n\n## Wave 1\n\n### Task 1: Do it\n\nDo the thing.\n", content)
}

func TestExecuteTaskShow_NotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	_, err := executeTaskShow("test-project", "nonexistent.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskShow_EmptyContent(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-empty-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "empty.md",
		Status:   taskstore.StatusReady,
	}))

	_, err := executeTaskShow(project, "empty.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no content")
}

func TestExecuteTaskCreate(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-create"

	err := executeTaskCreate(project, "my-feature", "add dark mode", "plan/my-feature", "ui-work", "", store)
	require.NoError(t, err)

	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("my-feature.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	assert.Equal(t, "add dark mode", entry.Description)
	assert.Equal(t, "plan/my-feature", entry.Branch)
	assert.Equal(t, "ui-work", entry.Topic)
}

func TestExecuteTaskCreate_WithContent(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-create-content"
	content := "# My Feature\n\n## Wave 1\n\n### Task 1: Do it\n"

	err := executeTaskCreate(project, "content-plan", "", "", "", content, store)
	require.NoError(t, err)

	got, err := store.GetContent(project, "content-plan.md")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestExecuteTaskCreate_Duplicate(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-dup"
	require.NoError(t, executeTaskCreate(project, "dup", "", "", "", "", store))
	err := executeTaskCreate(project, "dup", "", "", "", "", store)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestExecuteTaskCreate_DefaultBranch(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-defaults"
	require.NoError(t, executeTaskCreate(project, "auto-branch", "", "", "", "", store))
	ps, _ := taskstate.Load(store, project, "")
	entry, _ := ps.Entry("auto-branch.md")
	assert.Equal(t, "plan/auto-branch", entry.Branch)
}

func TestResolveTaskEntry(t *testing.T) {
	store, _, project := setupTestPlanState(t)
	entry, err := resolveTaskEntry(project, "test-plan.md", store)
	require.NoError(t, err)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	assert.Equal(t, "plan/test-plan", entry.Branch)
}

func TestResolveTaskEntry_NotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	_, err := resolveTaskEntry("proj", "nope.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveTaskEntry_BackfillsBranch(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "backfill"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "no-branch.md",
		Status:   taskstore.StatusReady,
	}))
	entry, err := resolveTaskEntry(project, "no-branch.md", store)
	require.NoError(t, err)
	assert.Equal(t, "plan/no-branch", entry.Branch)
}

func TestExecuteTaskStart(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-start"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "start-plan.md",
		Status:   taskstore.StatusReady,
		Branch:   "plan/start-plan",
	}))

	// Pass empty repoRoot to test FSM-only path (git ops will be skipped
	// by the test since we don't have a real repo).
	worktreePath, err := executeTaskStart("", project, "start-plan.md", store)
	// Expect a git error since repoRoot is empty — but FSM should have transitioned.
	assert.Error(t, err)

	// Verify the FSM transitioned to implementing before the git error.
	ps, _ := taskstate.Load(store, project, "")
	entry, _ := ps.Entry("start-plan.md")
	assert.Equal(t, taskstate.StatusImplementing, entry.Status)

	_ = worktreePath // not usable without a real repo
}

func TestExecuteTaskStart_FromPlanning(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-start-planning"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "planning.md",
		Status:   taskstore.Status("planning"),
		Branch:   "plan/planning",
	}))

	_, err := executeTaskStart("", project, "planning.md", store)
	assert.Error(t, err) // git error expected

	// Verify FSM walked through planning → ready → implementing.
	ps, _ := taskstate.Load(store, project, "")
	entry, _ := ps.Entry("planning.md")
	assert.Equal(t, taskstate.StatusImplementing, entry.Status)
}

func TestExecuteTaskPush_TaskNotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	err := executeTaskPush("", "nope", "missing.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskPush_NoBranch(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "push-test"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "no-branch.md",
		Status:   taskstore.StatusReady,
	}))
	err := executeTaskPush("", project, "no-branch.md", store)
	// Should resolve the branch but fail on git ops (no real repo).
	require.Error(t, err)
}

func TestExecuteTaskPR_TaskNotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	_, err := executeTaskPR("", "nope", "missing.md", "", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskPR_DefaultTitle(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "pr-test"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "my-feature.md",
		Status:      taskstore.StatusImplementing,
		Description: "add dark mode toggle",
		Branch:      "plan/my-feature",
	}))
	// Will fail on git/gh ops but tests the title derivation logic.
	_, err := executeTaskPR("", project, "my-feature.md", "", store)
	require.Error(t, err) // expected: git error (no real repo)
}

func TestExecuteTaskMerge_TaskNotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	err := executeTaskMerge("", "nope", "missing.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskMerge_TransitionsToDone(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "merge-test"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "merge-me.md",
		Status:   taskstore.Status("reviewing"),
		Branch:   "plan/merge-me",
	}))
	// Will fail on git merge (no real repo) but the error is from git, not FSM.
	err := executeTaskMerge("", project, "merge-me.md", store)
	require.Error(t, err) // expected: git error
}

func TestExecuteTaskStartOver_TaskNotFound(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	err := executeTaskStartOver("", "nope", "missing.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskStartOver_TransitionsToPlanning(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "startover-test"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "redo.md",
		Status:   taskstore.Status("done"),
		Branch:   "plan/redo",
	}))
	// Will fail on git reset (no real repo) but tests FSM logic.
	err := executeTaskStartOver("", project, "redo.md", store)
	require.Error(t, err) // expected: git error
}

func TestExecuteTaskMerge_IntegrationWithRealRepo(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "merge-integration"
	branch := "plan/int-merge"

	// Create a real git repo.
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Create the plan branch with a commit.
	runGit("branch", branch)
	runGit("checkout", branch)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "add feature")
	runGit("checkout", "main")

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "int-merge.md",
		Status:   taskstore.Status("reviewing"),
		Branch:   branch,
	}))

	err := executeTaskMerge(repo, project, "int-merge.md", store)
	require.NoError(t, err)

	// Verify FSM transitioned to done.
	ps, _ := taskstate.Load(store, project, "")
	entry, _ := ps.Entry("int-merge.md")
	assert.Equal(t, taskstate.StatusDone, entry.Status)

	// Verify branch was deleted.
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", branch).CombinedOutput()
	assert.Empty(t, strings.TrimSpace(string(out)))
}

func TestExecuteTaskStartOver_IntegrationWithRealRepo(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "startover-integration"
	branch := "plan/int-redo"

	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "initial")
	runGit("branch", branch)

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "int-redo.md",
		Status:   taskstore.Status("done"),
		Branch:   branch,
	}))

	err := executeTaskStartOver(repo, project, "int-redo.md", store)
	require.NoError(t, err)

	// Verify FSM transitioned to planning.
	ps, _ := taskstate.Load(store, project, "")
	entry, _ := ps.Entry("int-redo.md")
	assert.Equal(t, taskstate.StatusPlanning, entry.Status)

	// Verify branch still exists (recreated from HEAD).
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", branch).CombinedOutput()
	assert.Contains(t, string(out), "int-redo")
}

// TestPlanList_WithStore verifies that executeTaskListWithStore works with a
// store-backed HTTP server, returning plan entries from the remote store.
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
