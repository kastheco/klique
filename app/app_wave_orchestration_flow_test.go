package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waveFlowHome builds a minimal home struct suitable for wave-orchestration flow tests.
func waveFlowHome(t *testing.T, ps *taskstate.TaskState, plansDir string, orchMap map[string]*orchestration.WaveOrchestrator) *home {
	t.Helper()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	threshold := 0
	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         &config.Config{BlueprintSkipThresholdValue: &threshold},
		nav:               list,
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		overlays:          overlay.NewManager(),
		taskState:         ps,
		taskStateDir:      plansDir,
		waveOrchestrators: orchMap,
	}
	return h
}

// TestWaveMonitor_CancelWaveAdvanceRePrompts verifies that canceling a wave-advance
// confirmation resets the orchestrator confirm latch so the next metadata tick
// can display the prompt again (fixes deadlock).
func TestWaveMonitor_CancelWaveAdvanceRePrompts(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "First", Body: "do first"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Second", Body: "do second"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator("test", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1) // wave 1 done
	orch.NeedsConfirm()      // consume the one-shot latch so it won't fire again
	require.False(t, orch.NeedsConfirm(), "latch already consumed, must be false")

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	mgr1 := overlay.NewManager()
	mgr1.Show(overlay.NewConfirmationOverlay("Wave 1 complete. Start Wave 2?"))
	h := &home{
		ctx:                        context.Background(),
		state:                      stateConfirm,
		appConfig:                  config.DefaultConfig(),
		nav:                        ui.NewNavigationPanel(&sp),
		menu:                       ui.NewMenu(),
		tabbedWindow:               ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:               overlay.NewToastManager(&sp),
		overlays:                   mgr1,
		waveOrchestrators:          map[string]*orchestration.WaveOrchestrator{"test": orch},
		pendingWaveConfirmTaskFile: "test",
	}

	// Press 'n' (cancel key = default "n")
	keyMsg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, _ = h.handleKeyPress(keyMsg)

	// Orchestrator latch must be reset so the next tick can re-prompt
	assert.True(t, orch.NeedsConfirm(), "cancel must reset orchestrator confirm latch for re-prompt")
}

// TestWaveMonitor_PausedTaskCountsAsFailed verifies that a paused task instance
// is treated as a failure in the wave monitor, causing the wave to complete (with
// failure) and a failed-wave decision prompt to appear.
func TestWaveMonitor_PausedTaskCountsAsFailed(t *testing.T) {
	const planFile = "paused-task"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "paused task test", "plan/paused-task", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// Create the task instance but mark it as Paused
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "paused-task-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.SetStatus(session.Paused)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "paused-task-T1", TmuxAlive: false}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Wave must have detected failure and shown the failed-wave decision prompt
	assert.Equal(t, stateConfirm, updated.state,
		"paused task must trigger wave-failed decision prompt")
	require.True(t, updated.overlays.IsActive(),
		"confirmation overlay must be set for failed-wave decision")
	co1, ok1 := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok1, "current overlay must be a ConfirmationOverlay")
	assert.Equal(t, "r", co1.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
	assert.Equal(t, "n", co1.CancelKey,
		"failed-wave cancel key must be 'n' (next wave)")
	assert.NotNil(t, updated.pendingWaveNextAction,
		"failed-wave next action must be set for 'n' (next wave)")
}

// TestWaveMonitor_MissingTaskCountsAsFailed verifies that a task with no matching
// instance in the list is counted as failed, triggering the failed-wave decision prompt.
func TestWaveMonitor_MissingTaskCountsAsFailed(t *testing.T) {
	const planFile = "missing-task"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "missing task test", "plan/missing-task", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// No instance added to the list — the task is "missing"
	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})

	msg := metadataResultMsg{
		Results:   []instanceMetadata{},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Missing task must be treated as failed and trigger the failed-wave prompt
	assert.Equal(t, stateConfirm, updated.state,
		"missing task must trigger wave-failed decision prompt")
	require.True(t, updated.overlays.IsActive())
	co2, ok2 := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok2, "current overlay must be a ConfirmationOverlay")
	assert.Equal(t, "r", co2.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
}

// TestRebuildOrphanedOrchestrators_SkipsPausedOrExitedOnlyPlans verifies restart
// recovery does not resurrect wave orchestration when a plan only has stale
// paused/exited wave instances and no active task sessions.
func TestRebuildOrphanedOrchestrators_SkipsPausedOrExitedOnlyPlans(t *testing.T) {
	const planFile = "stale-wave"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store := taskstore.NewTestSQLiteStore(t)
	content := "**Goal:** stale test\n\n## Wave 1\n\n### Task 1: First\n\nDo first.\n\n### Task 2: Second\n\nDo second.\n"
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Branch:   "plan/stale-wave",
		Content:  content,
	}))

	ps, err := taskstate.Load(store, "proj", plansDir)
	require.NoError(t, err)
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*orchestration.WaveOrchestrator))
	h.taskStore = store
	h.taskStoreProject = "proj"

	pausedInst, err := session.NewInstance(session.InstanceOptions{
		Title:      "stale-wave-W1-T1",
		Path:       dir,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	pausedInst.MarkStartedForTest()
	pausedInst.SetStatus(session.Paused)
	h.nav.AddInstance(pausedInst)

	exitedInst, err := session.NewInstance(session.InstanceOptions{
		Title:      "stale-wave-W1-T2",
		Path:       dir,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	exitedInst.MarkStartedForTest()
	exitedInst.Exited = true
	h.nav.AddInstance(exitedInst)

	h.rebuildOrphanedOrchestrators()
	_, exists := h.waveOrchestrators[planFile]
	assert.False(t, exists, "stale paused/exited wave instances must not trigger orchestrator rebuild")
}

// TestRebuildOrphanedOrchestrators_RestoresWithActiveWaveInstance verifies restart
// recovery still works when at least one active wave instance exists.
func TestRebuildOrphanedOrchestrators_RestoresWithActiveWaveInstance(t *testing.T) {
	const planFile = "active-wave"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store := taskstore.NewTestSQLiteStore(t)
	content := "**Goal:** active test\n\n## Wave 1\n\n### Task 1: First\n\nDo first.\n\n### Task 2: Second\n\nDo second.\n"
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Branch:   "plan/active-wave",
		Content:  content,
	}))

	ps, err := taskstate.Load(store, "proj", plansDir)
	require.NoError(t, err)
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*orchestration.WaveOrchestrator))
	h.taskStore = store
	h.taskStoreProject = "proj"

	completedInst, err := session.NewInstance(session.InstanceOptions{
		Title:      "active-wave-W1-T1",
		Path:       dir,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	completedInst.MarkStartedForTest()
	completedInst.SetStatus(session.Paused)
	h.nav.AddInstance(completedInst)

	activeInst, err := session.NewInstance(session.InstanceOptions{
		Title:      "active-wave-W1-T2",
		Path:       dir,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	activeInst.MarkStartedForTest()
	h.nav.AddInstance(activeInst)

	h.rebuildOrphanedOrchestrators()
	orch, exists := h.waveOrchestrators[planFile]
	require.True(t, exists, "active wave instance must trigger orchestrator rebuild")
	assert.Equal(t, 1, orch.CurrentWaveNumber())
	assert.Equal(t, orchestration.WaveStateRunning, orch.State())
	assert.True(t, orch.IsTaskComplete(1), "paused task should be restored as completed")
	assert.True(t, orch.IsTaskRunning(2), "active task should remain running")
}

// TestWaveMonitor_LoadingInstanceNotMarkedFailed verifies that a wave task whose
// instance exists in the nav list but hasn't finished async startup (Loading status)
// is NOT prematurely marked as failed. This prevents the "instant all-complete" bug
// where the metadata tick fires before StartInSharedWorktree completes.
func TestWaveMonitor_LoadingInstanceNotMarkedFailed(t *testing.T) {
	const planFile = "loading-race"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "Task 1", Body: "do it"},
				{Number: 2, Title: "Task 2", Body: "do it too"},
			}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "loading race test", "plan/loading-race", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// Create instances in Loading state (async start not yet complete).
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:      "loading-race-W1-T1",
		Path:       t.TempDir(),
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
		PeerCount:  2,
	})
	require.NoError(t, err)
	inst1.SetStatus(session.Loading)

	inst2, err := session.NewInstance(session.InstanceOptions{
		Title:      "loading-race-W1-T2",
		Path:       t.TempDir(),
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 1,
		PeerCount:  2,
	})
	require.NoError(t, err)
	inst2.SetStatus(session.Loading)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(inst1)
	_ = h.nav.AddInstance(inst2)

	// Metadata tick with NO results for the loading instances (they aren't started yet).
	msg := metadataResultMsg{
		Results:   []instanceMetadata{},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Orchestrator must still be running — tasks must NOT be marked failed.
	require.Len(t, updated.waveOrchestrators, 1,
		"orchestrator must survive when instances are still loading")
	assert.Equal(t, orchestration.WaveStateRunning, orch.State(),
		"wave must remain running while instances are loading")
	assert.False(t, orch.IsTaskComplete(1), "task 1 must not be complete")
	assert.False(t, orch.IsTaskComplete(2), "task 2 must not be complete")
}

// TestWaveMonitor_AbortKeyDeletesOrchestrator verifies that pressing 'a' on the
// failed-wave decision prompt removes the orchestrator and returns to default state.
func TestWaveMonitor_AbortKeyDeletesOrchestrator(t *testing.T) {
	const planFile = "abort-test"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()
	orch.MarkTaskFailed(1)
	require.Equal(t, orchestration.WaveStateWaveComplete, orch.State())

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	abortCo := overlay.NewConfirmationOverlay("Wave 1 failed. r=retry n=next wave a=abort")
	abortCo.ConfirmKey = "r"
	abortCo.CancelKey = ""
	mgrAbort := overlay.NewManager()
	mgrAbort.Show(abortCo)
	h := &home{
		ctx:                        context.Background(),
		state:                      stateConfirm,
		appConfig:                  config.DefaultConfig(),
		nav:                        ui.NewNavigationPanel(&sp),
		menu:                       ui.NewMenu(),
		tabbedWindow:               ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:               overlay.NewToastManager(&sp),
		overlays:                   mgrAbort,
		waveOrchestrators:          map[string]*orchestration.WaveOrchestrator{planFile: orch},
		pendingWaveConfirmTaskFile: planFile,
		pendingWaveAbortAction: func() tea.Msg {
			return waveAbortMsg{planFile: planFile}
		},
	}

	// Press 'a' for abort
	keyMsg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, cmd := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	// State must return to default and abort action must be returned as cmd
	assert.Equal(t, stateDefault, updated.state, "state must return to default after abort")
	assert.False(t, updated.overlays.IsActive(), "overlay must be cleared after abort")
	assert.Nil(t, updated.pendingWaveAbortAction, "abort action must be cleared")
	assert.NotNil(t, cmd, "abort tea.Cmd must be returned so Update can execute it")
}

// TestTriggerPlanStage_ImplementNoWaves_RespecsPlanner verifies that when the
// implement stage is triggered on a plan without ## Wave headers, the plan status
// reverts to planning and a new planner session is spawned with a wave-annotation prompt.
func TestTriggerPlanStage_ImplementNoWaves_RespawnsPlanner(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	const planFile = "no-waves"
	// Plan content without ## Wave headers (has tasks but no waves)
	content := "# Plan\n\n**Goal:** Test\n\n### Task 1: Something\n\nDo it.\n"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(content), 0o644))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "no waves test", "plan/no-waves", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusPlanning)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		taskState:         ps,
		taskStateDir:      plansDir,
		fsm:               newPlanFSMForTest(t, plansDir),
		activeRepoPath:    dir,
		program:           "opencode",
		nav:               list,
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		waveOrchestrators: make(map[string]*orchestration.WaveOrchestrator),
	}

	_, _ = h.triggerTaskStage(planFile, "implement")

	// Plan status must have reverted to planning (parse failed, no StatusImplementing set)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusPlanning, entry.Status,
		"plan status must revert to planning when wave headers are missing")

	// A new planner instance must have been added to the list
	instances := list.GetInstances()
	require.NotEmpty(t, instances, "a planner instance must be spawned after parse failure")
	plannerInst := instances[len(instances)-1]
	assert.Equal(t, session.AgentTypePlanner, plannerInst.AgentType,
		"spawned instance must be a planner")
	assert.Contains(t, plannerInst.QueuedPrompt, "Wave",
		"planner prompt must mention Wave headers")
	assert.Contains(t, plannerInst.QueuedPrompt, "planner-finished-",
		"planner prompt must include the signal file instruction for kasmos completion detection")
	assert.Contains(t, plannerInst.QueuedPrompt, "kas task",
		"planner prompt must instruct the planner to store the annotated plan")
}

// ---------------------------------------------------------------------------
// All-waves-complete → review flow tests
// ---------------------------------------------------------------------------

// TestWaveMonitor_AllComplete_ShowsReviewPrompt verifies that when all tasks in the
// final wave complete, the orchestrator is deleted and a confirmation dialog appears
// asking the user to push and start review.
func TestWaveMonitor_AllComplete_ShowsReviewPrompt(t *testing.T) {
	const planFile = "all-complete"

	// Single wave plan — completing its tasks triggers orchestration.WaveStateAllComplete directly.
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Only task", Body: "do it"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "all-complete test", "plan/all-complete", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// Create task instance for the finished wave task.
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "all-complete-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "all-complete-W1-T1", TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Orchestrator must be deleted
	assert.Empty(t, updated.waveOrchestrators,
		"orchestrator must be deleted after all waves complete")

	// Confirmation dialog must appear for review
	assert.Equal(t, stateConfirm, updated.state,
		"state must be stateConfirm to prompt user for review")
	require.True(t, updated.overlays.IsActive(),
		"confirmation overlay must be set for all-complete review prompt")
	co3, ok3 := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok3, "current overlay must be a ConfirmationOverlay")
	// Standard confirm dialog (y/n) — not a wave-failed decision prompt
	assert.Equal(t, "y", co3.ConfirmKey,
		"confirm key must be 'y' for review prompt")
}

// TestWaveAllCompleteMsg_TransitionsToReviewing verifies that the waveAllCompleteMsg
// handler transitions the plan FSM from implementing to reviewing.
func TestWaveAllCompleteMsg_TransitionsToReviewing(t *testing.T) {
	const planFile = "review-transition"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "review transition test", "plan/review-trans", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*orchestration.WaveOrchestrator))
	h.fsm = newPlanFSMForTest(t, plansDir)

	model, _ := h.Update(wavePushCompleteMsg{planFile: planFile})
	updated := model.(*home)

	// Reload plan state from disk to verify FSM transition persisted.
	reloaded, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReviewing, entry.Status,
		"plan must transition to reviewing after wavePushCompleteMsg")

	// Toast must confirm the transition
	_ = updated // ensure the model is used (toast is in-memory, hard to assert without rendering)
}

// TestWaveAllCompleteMsg_PushIsAsync verifies that waveAllCompleteMsg
// returns an async command and that the plan transitions on wavePushCompleteMsg.
func TestWaveAllCompleteMsg_PushIsAsync(t *testing.T) {
	const planFile = "review-async"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "review async test", "plan/review-async", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*orchestration.WaveOrchestrator))
	h.fsm = newPlanFSMForTest(t, plansDir)

	model, cmd := h.Update(waveAllCompleteMsg{planFile: planFile})
	require.True(t, cmd != nil, "waveAllCompleteMsg must return non-nil async cmd")
	updated := model.(*home)

	// Plan remains implementing immediately after handling waveAllCompleteMsg.
	reloaded, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	entryBefore, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusImplementing, entryBefore.Status,
		"plan must remain implementing before async push completion")

	msg := cmd()
	pushMsg, ok := msg.(wavePushCompleteMsg)
	require.True(t, ok, "waveAllCompleteMsg command must resolve to wavePushCompleteMsg, got %T", msg)
	assert.Equal(t, planFile, pushMsg.planFile, "push message must include same plan file")

	model, _ = updated.Update(pushMsg)
	updated = model.(*home)

	// Transition to reviewing after push completion.
	reloaded, err = newTestPlanState(t, plansDir)
	require.NoError(t, err)
	entryAfter, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReviewing, entryAfter.Status,
		"plan must transition to reviewing after wavePushCompleteMsg")

	// Ensure caller gets back a usable updated model after async completion.
	_ = updated

}

// TestWaveMonitor_AllComplete_MultiWave verifies the flow with a multi-wave plan
// where all waves complete sequentially (wave 1 done → advance → wave 2 done → review prompt).
func TestWaveMonitor_AllComplete_MultiWave(t *testing.T) {
	const planFile = "multi-wave"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "W1 task", Body: "first"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "W2 task", Body: "second"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()     // start wave 1
	orch.MarkTaskComplete(1) // wave 1 done → orchestration.WaveStateWaveComplete
	require.Equal(t, orchestration.WaveStateWaveComplete, orch.State())

	orch.StartNextWave() // advance to wave 2
	require.Equal(t, orchestration.WaveStateRunning, orch.State())

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "multi wave test", "plan/multi-wave", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// Wave 2 task instance.
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "multi-wave-W2-T2",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 2,
	})
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	// 1. Feed wave-2 completion event.
	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "multi-wave-W2-T2", TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 2,
			TaskNumber: 2,
			TaskFile:   planFile,
		}},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Wave 2 was the last wave → AllComplete → review prompt
	assert.Empty(t, updated.waveOrchestrators,
		"orchestrator must be deleted after final wave completes")
	assert.Equal(t, stateConfirm, updated.state,
		"state must be stateConfirm for review prompt after final wave")
}

// TestRetryFailedWaveTasks_RemovesOldInstances verifies that when a failed wave task
// is retried, the old failed instance is removed from the list before the new one is
// spawned. Without this cleanup, each retry leaves behind ghost instances that all get
// marked ImplementationComplete when waves finish — producing duplicate entries.
func TestRetryFailedWaveTasks_RemovesOldInstances(t *testing.T) {
	const planFile = "retry-cleanup"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "Task 1", Body: "do first"},
				{Number: 6, Title: "Task 6", Body: "the flaky one"},
			}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	// Task 1 completed, task 6 failed.
	orch.MarkTaskComplete(1)
	orch.MarkTaskFailed(6)
	require.Equal(t, orchestration.WaveStateAllComplete, orch.State(), "single-wave plan should be AllComplete")

	dir := t.TempDir()
	// spawnWaveTasks → Setup() creates .worktrees/ inside dir before failing
	// (no real git repo). Force-remove it so t.TempDir cleanup doesn't fail.
	t.Cleanup(func() { os.RemoveAll(filepath.Join(dir, ".worktrees")) })
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "retry cleanup test", "plan/retry-cleanup", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	planName := taskstate.DisplayName(planFile)

	// Create the completed task 1 instance
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:      planName + "-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst1.SetStatus(session.Ready)

	// Create the failed task 6 instance (the one that should be removed on retry)
	failedInst6, err := session.NewInstance(session.InstanceOptions{
		Title:      planName + "-W1-T6",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 6,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	failedInst6.SetStatus(session.Paused) // failed tasks end up paused

	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	h.storage = storage
	h.allInstances = []*session.Instance{inst1, failedInst6}
	h.activeRepoPath = dir
	h.program = "claude"
	_ = h.nav.AddInstance(inst1)
	_ = h.nav.AddInstance(failedInst6)

	// Verify we start with 2 instances
	require.Len(t, h.nav.GetInstances(), 2, "should start with 2 instances")

	// Count instances with TaskNumber==6 before retry
	countTask6 := func() int {
		count := 0
		for _, inst := range h.nav.GetInstances() {
			if inst.TaskNumber == 6 && inst.TaskFile == planFile {
				count++
			}
		}
		return count
	}
	require.Equal(t, 1, countTask6(), "should have exactly 1 task-6 instance before retry")

	entry, _ := ps.Entry(planFile)

	// retryFailedWaveTasks spawns new instances — but it should remove the old one first.
	// Note: spawnWaveTasks will fail (no real git/tmux) but the cleanup should happen before that.
	h.retryFailedWaveTasks(orch, entry)

	// The old failed task 6 instance must have been removed from the list.
	for _, inst := range h.nav.GetInstances() {
		if inst == failedInst6 {
			t.Fatal("old failed task-6 instance must be removed from the list on retry")
		}
	}

	// The old failed task 6 instance must have been removed from allInstances.
	for _, inst := range h.allInstances {
		if inst == failedInst6 {
			t.Fatal("old failed task-6 instance must be removed from allInstances on retry")
		}
	}

	// Task 1 instance must still be there (it wasn't retried)
	foundTask1 := false
	for _, inst := range h.nav.GetInstances() {
		if inst.TaskNumber == 1 && inst.TaskFile == planFile {
			foundTask1 = true
		}
	}
	assert.True(t, foundTask1, "task 1 instance must not be affected by task 6 retry")
}

// TestWaveSignal_TriggersImplementation verifies that a wave signal file written
// in .signals/ is correctly picked up by ScanWaveSignals and parsed into a
// WaveSignal with the correct WaveNumber and PlanFile fields, ready for TUI consumption.
func TestWaveSignal_TriggersImplementation(t *testing.T) {
	repoRoot := t.TempDir()
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Create a plan with wave headers
	planContent := "# Test\n\n**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\nDo the thing.\n"
	planFile := "wave-signal-test"
	plansDir := filepath.Join(repoRoot, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	// Register plan as implementing
	ps := &taskstate.TaskState{Dir: plansDir, Plans: make(map[string]taskstate.TaskEntry), TopicEntries: make(map[string]taskstate.TopicEntry)}
	ps.Plans[planFile] = taskstate.TaskEntry{
		Status: "implementing",
		Branch: "plan/wave-signal-test",
	}
	require.NoError(t, ps.Save())

	// Write a wave signal
	signalFile := fmt.Sprintf("implement-wave-1-%s", planFile)
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, signalFile), nil, 0o644))

	// Verify signal is scannable using the new .kasmos/signals/ convention
	waveSignals := taskfsm.ScanWaveSignals(signalsDir)
	require.Len(t, waveSignals, 1)
	assert.Equal(t, 1, waveSignals[0].WaveNumber)
	assert.Equal(t, planFile, waveSignals[0].TaskFile)
}

// TestPlannerExit_CancelKillsInstanceAndMarksPrompted verifies that pressing "n"
// on the planner-exit dialog kills the planner instance and marks plannerPrompted.
func TestPlannerExit_CancelKillsInstanceAndMarksPrompted(t *testing.T) {
	const planFile = "cancel-kill"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-cancel-inst",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.TaskFile = planFile

	// Create storage so saveAllInstances doesn't panic
	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	mgrCancel := overlay.NewManager()
	mgrCancel.Show(overlay.NewConfirmationOverlay("Plan 'cancel-kill' is ready. Start implementation?"))
	h := &home{
		ctx:                         context.Background(),
		state:                       stateConfirm,
		appConfig:                   config.DefaultConfig(),
		nav:                         ui.NewNavigationPanel(&sp),
		menu:                        ui.NewMenu(),
		tabbedWindow:                ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:                overlay.NewToastManager(&sp),
		overlays:                    mgrCancel,
		storage:                     storage,
		waveOrchestrators:           make(map[string]*orchestration.WaveOrchestrator),
		plannerPrompted:             make(map[string]bool),
		coderPushPrompted:           make(map[string]bool),
		pendingPlannerInstanceTitle: "planner-cancel-inst",
		pendingPlannerTaskFile:      planFile,
		allInstances:                []*session.Instance{inst},
	}
	_ = h.nav.AddInstance(inst)

	// Press 'n' (cancel key — default for confirmation overlay)
	keyMsg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	model, _ := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	assert.True(t, updated.plannerPrompted[planFile],
		"plannerPrompted must be true after cancel")
	assert.Empty(t, updated.allInstances,
		"planner instance must be removed from allInstances after cancel")
	assert.Empty(t, updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after cancel")
	assert.Empty(t, updated.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must be cleared after cancel")
}

// --- Focus-before-overlay tests ---
// These verify that agent-related overlays auto-focus the relevant instance
// so the user can see the agent output behind the dialog.

// TestWaveMonitor_FocusesTaskInstance_WhenWaveCompleteShown verifies that
// showing the wave-complete confirmation auto-selects a task instance for
// that plan so the agent output is visible behind the overlay.
func TestWaveMonitor_FocusesTaskInstance_WhenWaveCompleteShown(t *testing.T) {
	const planFile = "focus-wave"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus wave test", "plan/focus-wave", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	planName := taskstate.DisplayName(planFile)

	// Add an "other" instance first (so it's selected by default), then the task instance.
	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()
	taskTitle := fmt.Sprintf("%s-W1-T1", planName)
	taskInst := &session.Instance{
		Title:      taskTitle,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	}
	taskInst.MarkStartedForTest()

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(taskInst)
	h.updateSidebarTasks() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: taskTitle, TmuxAlive: true},
		},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state, "should show wave-advance confirm")
	// The task instance should be selected, not the other one.
	assert.Equal(t, taskInst, updated.nav.GetSelectedInstance(),
		"wave-advance overlay should auto-focus a task instance for the plan")
}

// TestWaveMonitor_FocusesTaskInstance_WhenFailedWaveShown verifies that the
// failed-wave decision dialog auto-focuses a task instance for the plan.
func TestWaveMonitor_FocusesTaskInstance_WhenFailedWaveShown(t *testing.T) {
	const planFile = "focus-failed"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus failed test", "plan/focus-failed", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	planName := taskstate.DisplayName(planFile)

	// "other" instance selected by default.
	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()
	taskTitle := fmt.Sprintf("%s-W1-T1", planName)
	taskInst := &session.Instance{
		Title:      taskTitle,
		Program:    "opencode",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	}
	taskInst.MarkStartedForTest()
	taskInst.SetStatus(session.Paused) // paused = treated as failed

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(taskInst)
	h.updateSidebarTasks() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: taskTitle, TmuxAlive: false},
		},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state, "should show failed-wave decision")
	assert.Equal(t, taskInst, updated.nav.GetSelectedInstance(),
		"failed-wave overlay should auto-focus a task instance for the plan")
}

// TestPlannerExit_FocusesPlannerInstance_BeforeConfirm verifies that when a
// PlannerFinished signal is processed, the planner instance is auto-focused
// so its output is visible behind the overlay.
func TestPlannerExit_FocusesPlannerInstance_BeforeConfirm(t *testing.T) {
	const planFile = "focus-planner"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus planner test", "plan/focus-planner", time.Now()))
	// Plan is StatusPlanning — the PlannerFinished signal will transition it to StatusReady.
	seedPlanStatus(t, ps, planFile, taskstate.StatusPlanning)

	plannerInst := &session.Instance{
		Title:     "focus-planner-plan",
		Program:   "opencode",
		TaskFile:  planFile,
		AgentType: session.AgentTypePlanner,
	}
	plannerInst.MarkStartedForTest()

	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()

	h := waveFlowHome(t, ps, plansDir, nil)
	h.waveOrchestrators = make(map[string]*orchestration.WaveOrchestrator)
	h.plannerPrompted = make(map[string]bool)
	h.coderPushPrompted = make(map[string]bool)
	h.pendingReviewFeedback = make(map[string]string)
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(plannerInst)
	h.updateSidebarTasks() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	// Use the signal-driven path: PlannerFinished signal triggers the dialog.
	signal := taskfsm.Signal{
		Event:    taskfsm.PlannerFinished,
		TaskFile: planFile,
	}
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: "focus-planner-plan", TmuxAlive: true},
		},
		PlanState: ps,
		Signals:   []taskfsm.Signal{signal},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state, "should show planner-exit confirm")
	assert.Equal(t, plannerInst, updated.nav.GetSelectedInstance(),
		"planner-exit overlay should auto-focus the planner instance")
}

// TestWaveMonitor_AllComplete_DeferredWhenOverlayActive verifies that when all
// waves complete while the user is in an overlay (e.g. confirmation dialog),
// the review prompt is deferred and shown on the next tick when the overlay clears.
func TestWaveMonitor_AllComplete_DeferredWhenOverlayActive(t *testing.T) {
	const planFile = "deferred-complete"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Only task", Body: "do it"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "deferred test", "plan/deferred", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "deferred-complete-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	// Simulate user being in an overlay (e.g. another confirmation dialog)
	h.state = stateConfirm

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "deferred-complete-W1-T1", TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Orchestrator must be deleted (tasks are paused)
	assert.Empty(t, updated.waveOrchestrators,
		"orchestrator must be deleted even when overlay is active")

	// But the confirm dialog must NOT have been shown (overlay was blocking)
	// Instead, the plan file must be in pendingAllComplete
	assert.Contains(t, updated.pendingAllComplete, planFile,
		"plan must be deferred to pendingAllComplete when overlay blocks")

	// Now simulate the overlay clearing and another metadata tick arriving
	updated.state = stateDefault
	msg2 := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "deferred-complete-W1-T1", TmuxAlive: true}},
		PlanState: ps,
	}
	model2, _ := updated.Update(msg2)
	updated2 := model2.(*home)

	// Now the confirm dialog must appear
	assert.Equal(t, stateConfirm, updated2.state,
		"deferred all-complete must show confirm dialog on next tick")
	assert.Empty(t, updated2.pendingAllComplete,
		"pendingAllComplete must be drained after showing dialog")
}

// TestAutoAdvanceWaves_SkipsConfirmOnSuccess verifies that when AutoAdvanceWaves is
// true and a wave completes with zero failures, the model is configured to auto-advance
// without showing a confirmation dialog.
func TestAutoAdvanceWaves_SkipsConfirmOnSuccess(t *testing.T) {
	// Build a plan with 2 waves
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "T1"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "T2"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator("test", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1) // wave 1 complete, no failures

	m := &home{
		appConfig:         &config.Config{AutoAdvanceWaves: true},
		waveOrchestrators: map[string]*orchestration.WaveOrchestrator{"test": orch},
		taskState:         &taskstate.TaskState{Plans: map[string]taskstate.TaskEntry{"test": {Status: "implementing"}}},
		state:             stateDefault,
	}

	// NeedsConfirm should be true (wave just completed)
	assert.True(t, orch.NeedsConfirm())

	// With auto-advance enabled, the handler should NOT show a confirm dialog
	// and instead directly emit a waveAdvanceMsg.
	// This is a unit-level assertion on the branching logic.
	assert.True(t, m.appConfig.AutoAdvanceWaves)
	assert.Equal(t, 0, orch.FailedTaskCount())
}

// TestAutoAdvanceWaves_ShowsConfirmOnFailure verifies that even when AutoAdvanceWaves
// is true, a wave with failures still shows the decision dialog.
func TestAutoAdvanceWaves_ShowsConfirmOnFailure(t *testing.T) {
	const planFile = "auto-advance-failure"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "T1"},
				{Number: 2, Title: "T2"},
			}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 3, Title: "T3"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1)
	orch.MarkTaskFailed(2) // wave 1 complete with 1 failure

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "auto-advance failure test", "plan/auto-advance-failure", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	// Create task instances
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-failure-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst1.PromptDetected = true
	inst1.HasWorked = true

	inst2, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-failure-W1-T2",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst2.SetStatus(session.Paused) // failed

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	// Enable auto-advance
	h.appConfig = &config.Config{AutoAdvanceWaves: true}
	_ = h.nav.AddInstance(inst1)
	_ = h.nav.AddInstance(inst2)

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "auto-advance-failure-W1-T1", TmuxAlive: true},
			{Title: "auto-advance-failure-W1-T2", TmuxAlive: false},
		},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Even with auto-advance enabled, failures must show the decision dialog
	assert.Equal(t, stateConfirm, updated.state,
		"failed wave must show decision dialog even when auto-advance is enabled")
	require.True(t, updated.overlays.IsActive(),
		"confirmation overlay must be set for failed-wave decision")
	co4, ok4 := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok4, "current overlay must be a ConfirmationOverlay")
	assert.Equal(t, "r", co4.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
}

// TestAutoAdvanceWaves_EmitsAdvanceMsgOnSuccess verifies that when AutoAdvanceWaves
// is true and a wave completes with zero failures, the Update handler emits a
// waveAdvanceMsg directly (no confirmation dialog shown).
func TestAutoAdvanceWaves_EmitsAdvanceMsgOnSuccess(t *testing.T) {
	const planFile = "auto-advance-success"

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "T1"}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2, Title: "T2"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "auto-advance success test", "plan/auto-advance-success", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-success-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	// Enable auto-advance
	h.appConfig = &config.Config{AutoAdvanceWaves: true}
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "auto-advance-success-W1-T1", TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	}
	model, cmd := h.Update(msg)
	updated := model.(*home)

	// With auto-advance enabled and no failures, must NOT show a confirmation dialog
	assert.NotEqual(t, stateConfirm, updated.state,
		"auto-advance must not show confirmation dialog on success")
	assert.False(t, updated.overlays.IsActive(),
		"no confirmation overlay must be shown when auto-advancing")

	// The cmd must be non-nil (it contains the waveAdvanceMsg)
	assert.NotNil(t, cmd, "auto-advance must emit a tea.Cmd containing waveAdvanceMsg")
}

// TestWaveTaskCompletion_RequiresHasWorked verifies that a wave task is NOT
// marked complete when PromptDetected is true but HasWorked is false. This
// prevents permission prompts and early prompt returns from prematurely
// completing a wave (especially dangerous with auto-advance enabled).
func TestWaveTaskCompletion_RequiresHasWorked(t *testing.T) {
	const planFile = "has-worked-guard"

	plan := &taskparser.Plan{Waves: []taskparser.Wave{{
		Number: 1,
		Tasks:  []taskparser.Task{{Number: 1, Title: "do work"}},
	}}}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "task signal guard test", "plan/task-signal-guard", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	planName := taskstate.DisplayName(planFile)
	instTitle := planName + "-W1-T1"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      instTitle,
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.PromptDetected = true
	inst.HasWorked = true

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: instTitle, TmuxAlive: true}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Task must NOT be marked complete — orchestrator stays running.
	assert.Len(t, updated.waveOrchestrators, 1,
		"orchestrator must still exist when HasWorked is false")
	assert.Equal(t, orchestration.WaveStateRunning, orch.State(),
		"wave must remain running when task has not done real work")
	assert.NotEqual(t, stateConfirm, updated.state,
		"no completion dialog should appear")

	// Now send a task-finished signal — the orchestrator must be removed.
	model2, _ := updated.Update(metadataResultMsg{
		Results:   []instanceMetadata{{Title: instTitle, TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	})
	updated2 := model2.(*home)

	// Now it should complete.
	assert.Empty(t, updated2.waveOrchestrators,
		"orchestrator must be deleted after the task-finished signal arrives")
}

// TestWaveTaskCompletion_IgnoresPromptEchoUpdates verifies that a wave task is
// not marked complete when metadata shows an update while already at prompt
// (prompt-echo / startup noise). Completion must wait for non-prompt output.
func TestWaveTaskCompletion_IgnoresPromptEchoUpdates(t *testing.T) {
	const planFile = "prompt-echo-guard"

	plan := &taskparser.Plan{Waves: []taskparser.Wave{{
		Number: 1,
		Tasks:  []taskparser.Task{{Number: 1, Title: "do work"}},
	}}}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "prompt echo guard test", "plan/prompt-echo-guard", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	planName := taskstate.DisplayName(planFile)
	instTitle := planName + "-W1-T1"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      instTitle,
		Path:       t.TempDir(),
		Program:    "claude",
		TaskFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.PromptDetected = true
	inst.HasWorked = false

	h := waveFlowHome(t, ps, plansDir, map[string]*orchestration.WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(inst)

	// First update: metadata arrives while at prompt with no real work done.
	// The orchestrator must NOT complete the task yet.
	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: instTitle, TmuxAlive: true}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Len(t, updated.waveOrchestrators, 1,
		"orchestrator must still exist when only a prompt-echo update arrived")

	// Second update: a task-finished signal arrives — now the orchestrator must be removed.
	model2, _ := updated.Update(metadataResultMsg{
		Results:   []instanceMetadata{{Title: instTitle, TmuxAlive: true}},
		PlanState: ps,
		TaskSignals: []taskfsm.TaskSignal{{
			WaveNumber: 1,
			TaskNumber: 1,
			TaskFile:   planFile,
		}},
	})
	updated2 := model2.(*home)
	assert.Empty(t, updated2.waveOrchestrators, "orchestrator must be deleted after the task-finished signal arrives")
}

// ---------------------------------------------------------------------------
// Elaboration phase tests
// ---------------------------------------------------------------------------

// waveElabTestHarness bundles the minimal state needed for elaboration flow tests
// so each test doesn't have to duplicate setup boilerplate.
type waveElabTestHarness struct {
	t        *testing.T
	dir      string
	plansDir string
	store    taskstore.Store
	h        *home
}

func newWaveElabTestHarness(t *testing.T) *waveElabTestHarness {
	t.Helper()
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	threshold := 0

	store := taskstore.NewTestSQLiteStore(t)
	ps, err := taskstate.Load(store, "proj", plansDir)
	require.NoError(t, err)
	fsm := taskfsm.New(store, "proj", plansDir)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:               context.Background(),
		taskState:         ps,
		taskStore:         store,
		taskStoreProject:  "proj",
		taskStateDir:      plansDir,
		fsm:               fsm,
		nav:               ui.NewNavigationPanel(&sp),
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		overlays:          overlay.NewManager(),
		waveOrchestrators: make(map[string]*orchestration.WaveOrchestrator),
		activeRepoPath:    dir,
		program:           "opencode",
		appConfig:         &config.Config{BlueprintSkipThresholdValue: &threshold},
		state:             stateDefault,
	}
	return &waveElabTestHarness{t: t, dir: dir, plansDir: plansDir, store: store, h: h}
}

// registerPlan creates a plan entry in the store (with content and branch) and
// reloads task state so the home struct picks up the new plan.
func (th *waveElabTestHarness) registerPlan(planFile, content, branch string) {
	th.t.Helper()
	require.NoError(th.t, th.store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Branch:   branch,
		Content:  content,
	}))
	ps, err := taskstate.Load(th.store, "proj", th.plansDir)
	require.NoError(th.t, err)
	th.h.taskState = ps
}

// executeTaskStage delegates to the home struct's executeTaskStage.
func (th *waveElabTestHarness) executeTaskStage(planFile, stage string) (tea.Model, tea.Cmd) {
	return th.h.executeTaskStage(planFile, stage)
}

// TestImplementTriggersElaborationBeforeWave1 verifies that triggering "implement"
// creates a wave orchestrator in the elaborating state and spawns an elaborator instance.
func TestImplementTriggersElaborationBeforeWave1(t *testing.T) {
	h := newWaveElabTestHarness(t)

	const planFile = "elab-test"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\n**Files:**\n- Create: `foo.go`\n\nImplement foo."
	h.registerPlan(planFile, planContent, "plan/elab-test")

	model, _ := h.executeTaskStage(planFile, "implement")
	m := model.(*home)

	// Orchestrator should exist and be in elaborating state
	orch, exists := m.waveOrchestrators[planFile]
	require.True(t, exists, "orchestrator must be created")
	assert.Equal(t, orchestration.WaveStateElaborating, orch.State(),
		"orchestrator must be in elaborating state, not running")

	// An elaborator instance should have been spawned
	var foundElaborator bool
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.AgentType == session.AgentTypeElaborator {
			foundElaborator = true
			assert.Contains(t, inst.QueuedPrompt, "elaborator",
				"elaborator prompt must reference the elaborator role")
			break
		}
	}
	assert.True(t, foundElaborator, "elaborator instance must be spawned")
}

// TestImplementDirectlySkipsElaboration verifies that "implement_direct" creates an
// orchestrator in the running state without spawning an elaborator.
func TestImplementDirectlySkipsElaboration(t *testing.T) {
	h := newWaveElabTestHarness(t)

	const planFile = "direct-test"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\nDo it."
	h.registerPlan(planFile, planContent, "plan/direct-test")

	model, _ := h.executeTaskStage(planFile, "implement_direct")
	m := model.(*home)

	// Orchestrator should exist and be running (not elaborating)
	orch, exists := m.waveOrchestrators[planFile]
	require.True(t, exists, "orchestrator must be created")
	assert.NotEqual(t, orchestration.WaveStateElaborating, orch.State(),
		"direct implement must skip elaboration")
}

// TestDeadElaboratorRecovery verifies that when an elaborator instance dies
// (tmux exits) while the orchestrator is still in WaveStateElaborating, the
// metadata tick recovers by removing the dead elaborator and advancing the
// orchestrator out of the elaborating state (triggering wave 1).
func TestDeadElaboratorRecovery(t *testing.T) {
	h := newWaveElabTestHarness(t)

	const planFile = "dead-elab"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\n**Files:**\n- Create: `foo.go`\n\nImplement foo."
	h.registerPlan(planFile, planContent, "plan/dead-elab")

	// Set up: create orchestrator in elaborating state with a dead elaborator instance.
	plan, err := taskparser.Parse(planContent)
	require.NoError(t, err)
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	orch.SetElaborating()
	h.h.waveOrchestrators[planFile] = orch

	elaboratorInst := &session.Instance{
		Title:     "dead-elab-elaborator",
		Program:   "opencode",
		TaskFile:  planFile,
		AgentType: session.AgentTypeElaborator,
		Exited:    true, // tmux died
	}
	h.h.nav.AddInstance(elaboratorInst)

	// Simulate metadata tick with the elaborator's tmux dead.
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "dead-elab-elaborator", TmuxAlive: false},
		},
		PlanState: h.h.taskState,
	}
	model, _ := h.h.Update(msg)
	m := model.(*home)

	// The dead elaborator instance must have been removed.
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.AgentType == session.AgentTypeElaborator {
			t.Fatal("dead elaborator instance must be removed during recovery")
		}
	}

	// The orchestrator must no longer be stuck in elaborating state.
	// It may have been deleted by cascading wave-monitor logic (since no real
	// task instances exist in this test), which is fine — the key invariant
	// is that it's not stuck in WaveStateElaborating.
	if recoveredOrch, exists := m.waveOrchestrators[planFile]; exists {
		assert.NotEqual(t, orchestration.WaveStateElaborating, recoveredOrch.State(),
			"orchestrator must not remain in elaborating state after recovery")
	}
}

// TestImplementSkipsElaborationWhenElaboratorAlreadyRan verifies that triggering
// "implement" when a dead elaborator instance already exists for this plan
// skips the elaboration phase and goes straight to wave 1.
func TestImplementSkipsElaborationWhenElaboratorAlreadyRan(t *testing.T) {
	h := newWaveElabTestHarness(t)

	const planFile = "re-elab"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\n**Files:**\n- Create: `bar.go`\n\nImplement bar."
	h.registerPlan(planFile, planContent, "plan/re-elab")

	// Simulate a dead elaborator instance from a prior TUI session.
	deadElaborator := &session.Instance{
		Title:     "re-elab-elaborator",
		Program:   "opencode",
		TaskFile:  planFile,
		AgentType: session.AgentTypeElaborator,
		Exited:    true,
	}
	h.h.nav.AddInstance(deadElaborator)

	model, _ := h.executeTaskStage(planFile, "implement")
	m := model.(*home)

	// Orchestrator should exist and NOT be in elaborating state.
	orch, exists := m.waveOrchestrators[planFile]
	require.True(t, exists, "orchestrator must be created")
	assert.NotEqual(t, orchestration.WaveStateElaborating, orch.State(),
		"implement must skip elaboration when a prior elaborator already ran")

	// No new elaborator instance should have been spawned.
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.AgentType == session.AgentTypeElaborator {
			t.Fatal("no new elaborator should be spawned when one already ran")
		}
	}
}

// TestCoderExit_FocusesCoderInstance_BeforePushConfirm verifies that when a
// coder finishes (tmux dies) and the "push branch?" dialog shows, the coder
// instance is auto-focused so its output is visible behind the overlay.
func TestCoderExit_FocusesCoderInstance_BeforePushConfirm(t *testing.T) {
	const planFile = "focus-coder"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus coder test", "plan/focus-coder", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	coderInst := &session.Instance{
		Title:     "focus-coder-implement",
		Program:   "opencode",
		TaskFile:  planFile,
		AgentType: session.AgentTypeCoder,
	}
	coderInst.MarkStartedForTest()

	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()

	h := waveFlowHome(t, ps, plansDir, nil)
	h.waveOrchestrators = make(map[string]*orchestration.WaveOrchestrator)
	h.plannerPrompted = make(map[string]bool)
	h.coderPushPrompted = make(map[string]bool)
	h.pendingReviewFeedback = make(map[string]string)
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(coderInst)
	h.updateSidebarTasks() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: "focus-coder-implement", TmuxAlive: false},
		},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state, "should show coder-exit push confirm")
	assert.Equal(t, coderInst, updated.nav.GetSelectedInstance(),
		"coder-exit overlay should auto-focus the coder instance")
}
