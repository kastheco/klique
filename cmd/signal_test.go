package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSignalTestStore creates an in-memory SQLite store pre-populated with
// plans in various states suitable for signal processing tests.
func setupSignalTestStore(t *testing.T) (taskstore.Store, string, string) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)

	root := t.TempDir()
	project := filepath.Base(root)

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "implementing-plan.md",
		Status:      taskstore.Status(taskfsm.StatusImplementing),
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "reviewing-plan.md",
		Status:      taskstore.Status(taskfsm.StatusReviewing),
		Description: "reviewing plan",
		Branch:      "plan/reviewing-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "planning-plan.md",
		Status:      taskstore.Status(taskfsm.StatusPlanning),
		Description: "planning plan",
		Branch:      "plan/planning-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "ready-plan.md",
		Status:      taskstore.Status(taskfsm.StatusReady),
		Description: "ready plan",
		Branch:      "plan/ready-plan",
		CreatedAt:   time.Now(),
	}))

	return store, root, project
}

// --- executeSignalList tests ---

func TestExecuteSignalList_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	result := executeSignalList(dir)
	assert.Equal(t, "no pending signals\n", result)
}

func TestExecuteSignalList_MissingDirectory(t *testing.T) {
	result := executeSignalList("/nonexistent/path/signals")
	assert.Equal(t, "no pending signals\n", result)
}

func TestExecuteSignalList_WithFSMSignal_ImplementFinished(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-finished-my-plan.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "implement_finished")
	assert.Contains(t, result, "my-plan.md")
}

func TestExecuteSignalList_WithFSMSignal_ReviewApproved(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-approved-auth-plan.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "review_approved")
	assert.Contains(t, result, "auth-plan.md")
}

func TestExecuteSignalList_WithFSMSignal_PlannerFinished(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "planner-finished-my-plan.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "planner_finished")
	assert.Contains(t, result, "my-plan.md")
}

func TestExecuteSignalList_WithFSMSignal_ReviewChanges(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-changes-auth-plan.md"), []byte("fix the tests"), 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "review_changes_requested")
	assert.Contains(t, result, "auth-plan.md")
}

func TestExecuteSignalList_WithWaveSignal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-wave-2-big-plan.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "implement_wave")
	assert.Contains(t, result, "big-plan.md")
	assert.Contains(t, result, "wave 2")
}

func TestExecuteSignalList_WithElaborationSignal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "elaborator-finished-my-plan.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "elaborator_finished")
	assert.Contains(t, result, "my-plan.md")
}

func TestExecuteSignalList_MultipleSignals(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-finished-plan-a.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-approved-plan-b.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-wave-1-plan-c.md"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "plan-a.md")
	assert.Contains(t, result, "plan-b.md")
	assert.Contains(t, result, "plan-c.md")
}

// --- executeSignalProcess tests ---

// TestExecuteSignalProcess_ImplementFinished verifies that an implement-finished
// signal triggers the FSM transition implementing→reviewing and the signal is consumed.
func TestExecuteSignalProcess_ImplementFinished_TransitionToReviewing(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write implement-finished signal for the implementing plan.
	sigFile := filepath.Join(signalsDir, "implement-finished-implementing-plan.md")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo", // no-op program so no real tmux needed
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed, "expected 1 signal processed")

	// Signal file should be consumed.
	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr), "signal file should be deleted after processing")

	// FSM should have transitioned to reviewing.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("implementing-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReviewing, entry.Status)
}

// TestExecuteSignalProcess_ReviewApproved verifies review-approved → done transition.
func TestExecuteSignalProcess_ReviewApproved_TransitionToDone(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write review-approved signal for the reviewing plan.
	sigFile := filepath.Join(signalsDir, "review-approved-reviewing-plan.md")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Signal consumed.
	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))

	// FSM → done.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("reviewing-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusDone, entry.Status)
}

// TestExecuteSignalProcess_PlannerFinished verifies planner-finished → ready transition.
func TestExecuteSignalProcess_PlannerFinished_TransitionToReady(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	sigFile := filepath.Join(signalsDir, "planner-finished-planning-plan.md")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))

	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("planning-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
}

// TestExecuteSignalProcess_UnknownPlan verifies that signals for unknown plans
// are consumed without error (and no FSM transition is applied).
func TestExecuteSignalProcess_UnknownPlan_SignalConsumed(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	sigFile := filepath.Join(signalsDir, "implement-finished-nonexistent-plan.md")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	// Should not return error — just log and consume.
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 0, processed, "unknown plan signals should not count as processed")

	// Signal should still be consumed.
	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))
}

// TestExecuteSignalProcess_EmptyDir verifies that no-op when signals dir is empty.
func TestExecuteSignalProcess_EmptyDirectory(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
}

// TestExecuteSignalProcess_MultipleSignals processes several signals in one pass.
func TestExecuteSignalProcess_MultipleSignals(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write two valid signals.
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-planning-plan.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "review-approved-reviewing-plan.md"), nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 2, processed)

	// Both signals consumed.
	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Both plans transitioned.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	planningEntry, _ := ps.Entry("planning-plan.md")
	reviewingEntry, _ := ps.Entry("reviewing-plan.md")
	assert.Equal(t, taskstate.StatusReady, planningEntry.Status)
	assert.Equal(t, taskstate.StatusDone, reviewingEntry.Status)
}

// TestExecuteSignalProcess_ReviewChangesRequested verifies review-changes → implementing
// and increments the review cycle.
func TestExecuteSignalProcess_ReviewChanges_TransitionToImplementing(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	feedback := "Please fix the tests"
	sigFile := filepath.Join(signalsDir, "review-changes-reviewing-plan.md")
	require.NoError(t, os.WriteFile(sigFile, []byte(feedback), 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
		program:    "echo",
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))

	// FSM → implementing.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("reviewing-plan.md")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusImplementing, entry.Status)
	// Review cycle should be incremented.
	assert.Equal(t, 1, entry.ReviewCycle)
}

// TestResolveAgentProgram_DefaultsToProgram verifies fallback when no config profile exists.
func TestResolveAgentProgram_DefaultsToProgram(t *testing.T) {
	// Without a kasmos config, should fall back to the supplied default.
	program := resolveAgentProgram("coder", "opencode")
	assert.NotEmpty(t, program, "should return a non-empty program string")
}

// TestNewSignalCmd_Structure verifies the cobra command tree structure.
func TestNewSignalCmd_Structure(t *testing.T) {
	cmd := NewSignalCmd()
	assert.Equal(t, "signal", cmd.Use)

	subcmds := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcmds[sub.Use] = true
	}
	assert.True(t, subcmds["list"], "signal list command should exist")

	// process command has --once flag
	var processCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Use == "process" {
			processCmd = sub
			break
		}
	}
	require.NotNil(t, processCmd, "signal process command should exist")
	assert.NotNil(t, processCmd.Flags().Lookup("once"), "--once flag should exist on process command")
}
