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
		Filename:    "implementing-plan",
		Status:      taskstore.Status(taskfsm.StatusImplementing),
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "reviewing-plan",
		Status:      taskstore.Status(taskfsm.StatusReviewing),
		Description: "reviewing plan",
		Branch:      "plan/reviewing-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "planning-plan",
		Status:      taskstore.Status(taskfsm.StatusPlanning),
		Description: "planning plan",
		Branch:      "plan/planning-plan",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "ready-plan",
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-finished-my-plan"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "implement_finished")
	assert.Contains(t, result, "my-plan")
}

func TestExecuteSignalList_WithFSMSignal_ReviewApproved(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-approved-auth-plan"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "review_approved")
	assert.Contains(t, result, "auth-plan")
}

func TestExecuteSignalList_WithFSMSignal_PlannerFinished(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "planner-finished-my-plan"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "planner_finished")
	assert.Contains(t, result, "my-plan")
}

func TestExecuteSignalList_WithFSMSignal_ReviewChanges(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-changes-auth-plan"), []byte("fix the tests"), 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "review_changes_requested")
	assert.Contains(t, result, "auth-plan")
}

func TestExecuteSignalList_WithWaveSignal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-wave-2-big-plan"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "implement_wave")
	assert.Contains(t, result, "big-plan")
	assert.Contains(t, result, "wave 2")
}

func TestExecuteSignalList_WithElaborationSignal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "elaborator-finished-my-plan"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "elaborator_finished")
	assert.Contains(t, result, "my-plan")
}

func TestExecuteSignalList_MultipleSignals(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-finished-plan-a"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-approved-plan-b"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "implement-wave-1-plan-c"), nil, 0o644))

	result := executeSignalList(dir)
	assert.Contains(t, result, "plan-a")
	assert.Contains(t, result, "plan-b")
	assert.Contains(t, result, "plan-c")
}

// --- executeSignalProcess tests ---

// TestExecuteSignalProcess_ImplementFinished verifies that an implement-finished
// signal triggers the FSM transition implementing→reviewing and the signal is consumed.
func TestExecuteSignalProcess_ImplementFinished_TransitionToReviewing(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write implement-finished signal for the implementing plan.
	sigFile := filepath.Join(signalsDir, "implement-finished-implementing-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
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
	entry, ok := ps.Entry("implementing-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReviewing, entry.Status)
}

// TestExecuteSignalProcess_ReviewApproved verifies review-approved → done transition.
func TestExecuteSignalProcess_ReviewApproved_TransitionToDone(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write review-approved signal for the reviewing plan.
	sigFile := filepath.Join(signalsDir, "review-approved-reviewing-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
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
	entry, ok := ps.Entry("reviewing-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusDone, entry.Status)
}

// TestExecuteSignalProcess_PlannerFinished verifies planner-finished → ready transition.
func TestExecuteSignalProcess_PlannerFinished_TransitionToReady(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	sigFile := filepath.Join(signalsDir, "planner-finished-planning-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))

	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("planning-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
}

// TestExecuteSignalProcess_UnknownPlan verifies that signals for unknown plans
// are consumed without error (and no FSM transition is applied).
func TestExecuteSignalProcess_UnknownPlan_SignalConsumed(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	sigFile := filepath.Join(signalsDir, "implement-finished-nonexistent-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	// Should not return error — just log and consume.
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 0, processed, "unknown plan signals should not count as processed")

	// Signal should still be consumed.
	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))
}

// TestExecuteSignalProcess_RecoverInFlightBeforeScanning verifies that signals
// stranded in the processing/ subdirectory by a previous interrupted run are
// moved back to the base signals directory and processed in the same pass.
// Without the RecoverInFlight call a signal left in processing/ would be
// permanently invisible to future CLI invocations.
func TestExecuteSignalProcess_RecoverInFlightBeforeScanning(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	processingDir := filepath.Join(signalsDir, "processing")
	require.NoError(t, os.MkdirAll(processingDir, 0o755))

	// Simulate a signal that was already moved to processing/ by a prior run
	// that crashed before completing the transition.
	strandedFile := filepath.Join(processingDir, "implement-finished-implementing-plan")
	require.NoError(t, os.WriteFile(strandedFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed, "stranded in-flight signal must be recovered and processed")

	// The stranded file must be gone from processing/ (consumed by recovery + processing).
	_, statErr := os.Stat(strandedFile)
	assert.True(t, os.IsNotExist(statErr), "stranded signal must not remain in processing/")

	// The FSM transition should have applied: implementing → reviewing.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("implementing-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReviewing, entry.Status)
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
		filepath.Join(signalsDir, "planner-finished-planning-plan"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "review-approved-reviewing-plan"), nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 2, processed)

	// Both signals consumed — filter out subdirectories (staging/, processing/, failed/)
	// that EnsureSignalDirs may have created.
	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	var fileEntries []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			fileEntries = append(fileEntries, e)
		}
	}
	assert.Empty(t, fileEntries, "no top-level signal files should remain after processing")

	// Both plans transitioned.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	planningEntry, _ := ps.Entry("planning-plan")
	reviewingEntry, _ := ps.Entry("reviewing-plan")
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
	sigFile := filepath.Join(signalsDir, "review-changes-reviewing-plan")
	require.NoError(t, os.WriteFile(sigFile, []byte(feedback), 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr))

	// FSM → implementing.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("reviewing-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusImplementing, entry.Status)
	// Review cycle should be incremented.
	assert.Equal(t, 1, entry.ReviewCycle)
}

// TestExecuteSignalProcess_AtomicProcessing verifies that a valid signal is
// processed atomically: base file is gone, processing dir is clean, no failed
// entry, and the plan transitions correctly.
func TestExecuteSignalProcess_AtomicProcessing(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, taskfsm.EnsureSignalDirs(signalsDir))

	// planner-finished on a planning-plan → ready.
	sigFile := filepath.Join(signalsDir, "planner-finished-planning-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Base signal file must be gone.
	_, statErr := os.Stat(sigFile)
	assert.True(t, os.IsNotExist(statErr), "base signal file should be deleted after processing")

	// processing/ dir must not contain the signal.
	procPath := filepath.Join(signalsDir, "processing", "planner-finished-planning-plan")
	_, statErr = os.Stat(procPath)
	assert.True(t, os.IsNotExist(statErr), "processing file should be removed after success")

	// failed/ dir must not contain the signal.
	failedPath := filepath.Join(signalsDir, "failed", "planner-finished-planning-plan")
	_, statErr = os.Stat(failedPath)
	assert.True(t, os.IsNotExist(statErr), "successful signal must not appear in failed/")

	// Plan should have transitioned to ready.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("planning-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
}

// TestExecuteSignalProcess_InvalidTransition_MovesToFailed verifies that a
// signal whose FSM transition is invalid is dead-lettered into failed/.
func TestExecuteSignalProcess_InvalidTransition_MovesToFailed(t *testing.T) {
	store, root, project := setupSignalTestStore(t)
	signalsDir := filepath.Join(root, ".kasmos", "signals")
	require.NoError(t, taskfsm.EnsureSignalDirs(signalsDir))

	// implement-finished on a ready-plan is an invalid transition.
	sigFile := filepath.Join(signalsDir, "implement-finished-ready-plan")
	require.NoError(t, os.WriteFile(sigFile, nil, 0o644))

	opts := signalProcessOptions{
		repoRoot:   root,
		project:    project,
		signalsDir: signalsDir,
		store:      store,
	}
	processed, err := executeSignalProcess(opts)
	require.NoError(t, err)
	assert.Equal(t, 0, processed, "invalid transition should not count as processed")

	// failed/ must contain the signal and a reason file.
	failedPath := filepath.Join(signalsDir, "failed", "implement-finished-ready-plan")
	_, statErr := os.Stat(failedPath)
	assert.False(t, os.IsNotExist(statErr), "signal should be in failed/ after invalid transition")

	reasonPath := failedPath + ".reason"
	_, statErr = os.Stat(reasonPath)
	assert.False(t, os.IsNotExist(statErr), "reason file should exist in failed/")

	// Plan status must remain ready.
	ps, err := taskstate.Load(store, project, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("ready-plan")
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
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

// --- executeSignalEmit tests ---

func TestExecuteSignalEmit(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	err = executeSignalEmit(gw, "myproject", "planner_finished", "my-plan", "plan done")
	require.NoError(t, err)

	signals, err := gw.List("myproject", taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, "my-plan", signals[0].PlanFile)
	assert.Equal(t, "planner_finished", signals[0].SignalType)
	assert.Equal(t, `{"body":"plan done"}`, signals[0].Payload)
}

func TestExecuteSignalEmit_InvalidType(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	err = executeSignalEmit(gw, "proj", "invalid_type", "plan", "")
	assert.Error(t, err)
}

func TestExecuteSignalEmit_EmptyPayloadFSMSignal(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	err = executeSignalEmit(gw, "proj", "implement_finished", "my-plan", "")
	require.NoError(t, err)

	signals, err := gw.List("proj", taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, "", signals[0].Payload)
}

func TestExecuteSignalEmit_ElaboratorFinishedRejectsPayload(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	err = executeSignalEmit(gw, "proj", "elaborator_finished", "my-plan", "unexpected payload")
	assert.Error(t, err)
}

func TestExecuteSignalEmit_ImplementTaskFinished(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	payload := `{"wave_number":2,"task_number":3}`
	err = executeSignalEmit(gw, "proj", "implement_task_finished", "my-plan", payload)
	require.NoError(t, err)

	signals, err := gw.List("proj", taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, payload, signals[0].Payload)
}

func TestNormalizeSignalPayload_FSMSignalWrapsBody(t *testing.T) {
	payload, err := normalizeSignalPayload("planner_finished", "some message")
	require.NoError(t, err)
	assert.Equal(t, `{"body":"some message"}`, payload)
}

func TestNormalizeSignalPayload_FSMSignalPreservesJSON(t *testing.T) {
	json := `{"key":"value"}`
	payload, err := normalizeSignalPayload("review_approved", json)
	require.NoError(t, err)
	assert.Equal(t, json, payload)
}

func TestNormalizeSignalPayload_FSMSignalEmptyPayload(t *testing.T) {
	payload, err := normalizeSignalPayload("implement_finished", "")
	require.NoError(t, err)
	assert.Equal(t, "", payload)
}

func TestNormalizeSignalPayload_ImplementWaveRequiresJSON(t *testing.T) {
	_, err := normalizeSignalPayload("implement_wave", "not json")
	assert.Error(t, err)

	payload, err := normalizeSignalPayload("implement_wave", `{"wave_number":1}`)
	require.NoError(t, err)
	assert.Equal(t, `{"wave_number":1}`, payload)
}

func TestNormalizeSignalPayload_ElaboratorFinishedRejectsPayload(t *testing.T) {
	_, err := normalizeSignalPayload("elaborator_finished", "not empty")
	assert.Error(t, err)

	payload, err := normalizeSignalPayload("elaborator_finished", "")
	require.NoError(t, err)
	assert.Equal(t, "", payload)
}

func TestNormalizeSignalPayload_ImplementTaskFinishedRejectsNonNumeric(t *testing.T) {
	_, err := normalizeSignalPayload("implement_task_finished", `{"wave_number":"x","task_number":1}`)
	assert.Error(t, err)

	_, err = normalizeSignalPayload("implement_task_finished", `{"wave_number":1,"task_number":"y"}`)
	assert.Error(t, err)

	// Fractional values must also be rejected — they pass JSON float64 assertion
	// but would cause json.Unmarshal-into-int failure in the gateway scanner.
	_, err = normalizeSignalPayload("implement_task_finished", `{"wave_number":1.5,"task_number":1}`)
	assert.Error(t, err)

	_, err = normalizeSignalPayload("implement_task_finished", `{"wave_number":1,"task_number":2.5}`)
	assert.Error(t, err)

	payload, err := normalizeSignalPayload("implement_task_finished", `{"wave_number":2,"task_number":3}`)
	require.NoError(t, err)
	assert.Equal(t, `{"wave_number":2,"task_number":3}`, payload)
}

func TestNormalizeSignalPayload_ImplementWaveRejectsNonNumeric(t *testing.T) {
	_, err := normalizeSignalPayload("implement_wave", `{"wave_number":"not-a-number"}`)
	assert.Error(t, err)

	// Fractional value must also be rejected — it passes float64 assertion
	// but would cause json.Unmarshal-into-int failure in the gateway scanner.
	_, err = normalizeSignalPayload("implement_wave", `{"wave_number":1.5}`)
	assert.Error(t, err)

	payload, err := normalizeSignalPayload("implement_wave", `{"wave_number":1}`)
	require.NoError(t, err)
	assert.Equal(t, `{"wave_number":1}`, payload)
}

func TestNewSignalEmitCmd_Structure(t *testing.T) {
	cmd := newSignalEmitCmd()
	assert.Equal(t, "emit <signal-type> <plan-file>", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("payload"), "--payload flag should exist on emit command")
}
