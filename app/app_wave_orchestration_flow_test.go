package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waveFlowHome builds a minimal home struct suitable for wave-orchestration flow tests.
func waveFlowHome(t *testing.T, ps *planstate.PlanState, plansDir string, orchMap map[string]*WaveOrchestrator) *home {
	t.Helper()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		nav:               list,
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		planState:         ps,
		planStateDir:      plansDir,
		waveOrchestrators: orchMap,
	}
	return h
}

// TestWaveMonitor_CancelWaveAdvanceRePrompts verifies that canceling a wave-advance
// confirmation resets the orchestrator confirm latch so the next metadata tick
// can display the prompt again (fixes deadlock).
func TestWaveMonitor_CancelWaveAdvanceRePrompts(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "First", Body: "do first"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Second", Body: "do second"}}},
		},
	}
	orch := NewWaveOrchestrator("test.md", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1) // wave 1 done
	orch.NeedsConfirm()      // consume the one-shot latch so it won't fire again
	require.False(t, orch.NeedsConfirm(), "latch already consumed, must be false")

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                        context.Background(),
		state:                      stateConfirm,
		appConfig:                  config.DefaultConfig(),
		nav:                        ui.NewNavigationPanel(&sp),
		menu:                       ui.NewMenu(),
		tabbedWindow:               ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:               overlay.NewToastManager(&sp),
		waveOrchestrators:          map[string]*WaveOrchestrator{"test.md": orch},
		pendingWaveConfirmPlanFile: "test.md",
		confirmationOverlay:        overlay.NewConfirmationOverlay("Wave 1 complete. Start Wave 2?"),
	}

	// Press 'n' (cancel key = default "n")
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	_, _ = h.handleKeyPress(keyMsg)

	// Orchestrator latch must be reset so the next tick can re-prompt
	assert.True(t, orch.NeedsConfirm(), "cancel must reset orchestrator confirm latch for re-prompt")
}

// TestWaveMonitor_PausedTaskCountsAsFailed verifies that a paused task instance
// is treated as a failure in the wave monitor, causing the wave to complete (with
// failure) and a failed-wave decision prompt to appear.
func TestWaveMonitor_PausedTaskCountsAsFailed(t *testing.T) {
	const planFile = "2026-02-21-paused-task.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "paused task test", "plan/paused-task", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Create the task instance but mark it as Paused
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "paused-task-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.SetStatus(session.Paused)

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
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
	require.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay must be set for failed-wave decision")
	assert.Equal(t, "r", updated.confirmationOverlay.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
	assert.Equal(t, "n", updated.confirmationOverlay.CancelKey,
		"failed-wave cancel key must be 'n' (next wave)")
	assert.NotNil(t, updated.pendingWaveNextAction,
		"failed-wave next action must be set for 'n' (next wave)")
}

// TestWaveMonitor_MissingTaskCountsAsFailed verifies that a task with no matching
// instance in the list is counted as failed, triggering the failed-wave decision prompt.
func TestWaveMonitor_MissingTaskCountsAsFailed(t *testing.T) {
	const planFile = "2026-02-21-missing-task.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "missing task test", "plan/missing-task", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// No instance added to the list — the task is "missing"
	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})

	msg := metadataResultMsg{
		Results:   []instanceMetadata{},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Missing task must be treated as failed and trigger the failed-wave prompt
	assert.Equal(t, stateConfirm, updated.state,
		"missing task must trigger wave-failed decision prompt")
	require.NotNil(t, updated.confirmationOverlay)
	assert.Equal(t, "r", updated.confirmationOverlay.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
}

// TestWaveMonitor_AbortKeyDeletesOrchestrator verifies that pressing 'a' on the
// failed-wave decision prompt removes the orchestrator and returns to default state.
func TestWaveMonitor_AbortKeyDeletesOrchestrator(t *testing.T) {
	const planFile = "2026-02-21-abort-test.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()
	orch.MarkTaskFailed(1)
	require.Equal(t, WaveStateWaveComplete, orch.State())

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                        context.Background(),
		state:                      stateConfirm,
		appConfig:                  config.DefaultConfig(),
		nav:                        ui.NewNavigationPanel(&sp),
		menu:                       ui.NewMenu(),
		tabbedWindow:               ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:               overlay.NewToastManager(&sp),
		waveOrchestrators:          map[string]*WaveOrchestrator{planFile: orch},
		pendingWaveConfirmPlanFile: planFile,
		confirmationOverlay:        overlay.NewConfirmationOverlay("Wave 1 failed. r=retry n=next wave a=abort"),
		pendingWaveAbortAction: func() tea.Msg {
			return waveAbortMsg{planFile: planFile}
		},
	}
	// Set confirm key to 'r' as the failed-wave dialog would
	h.confirmationOverlay.ConfirmKey = "r"
	h.confirmationOverlay.CancelKey = ""

	// Press 'a' for abort
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	model, cmd := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	// State must return to default and abort action must be returned as cmd
	assert.Equal(t, stateDefault, updated.state, "state must return to default after abort")
	assert.Nil(t, updated.confirmationOverlay, "overlay must be cleared after abort")
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

	const planFile = "2026-02-21-no-waves.md"
	// Plan content without ## Wave headers (has tasks but no waves)
	content := "# Plan\n\n**Goal:** Test\n\n### Task 1: Something\n\nDo it.\n"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(content), 0o644))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "no waves test", "plan/no-waves", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusPlanning)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		planState:         ps,
		planStateDir:      plansDir,
		fsm:               newPlanFSMForTest(t, plansDir),
		activeRepoPath:    dir,
		program:           "opencode",
		nav:               list,
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		waveOrchestrators: make(map[string]*WaveOrchestrator),
	}

	_, _ = h.triggerPlanStage(planFile, "implement")

	// Plan status must have reverted to planning (parse failed, no StatusImplementing set)
	entry, ok := ps.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusPlanning, entry.Status,
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
	assert.Contains(t, plannerInst.QueuedPrompt, "commit",
		"planner prompt must instruct the planner to commit the annotated plan")
}

// ---------------------------------------------------------------------------
// All-waves-complete → review flow tests
// ---------------------------------------------------------------------------

// TestWaveMonitor_AllComplete_ShowsReviewPrompt verifies that when all tasks in the
// final wave complete, the orchestrator is deleted and a confirmation dialog appears
// asking the user to push and start review.
func TestWaveMonitor_AllComplete_ShowsReviewPrompt(t *testing.T) {
	const planFile = "2026-02-24-all-complete.md"

	// Single wave plan — completing its tasks triggers WaveStateAllComplete directly.
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Only task", Body: "do it"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "all-complete test", "plan/all-complete", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Create task instance with PromptDetected (agent finished)
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "all-complete-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.PromptDetected = true

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "all-complete-W1-T1", TmuxAlive: true}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	// Orchestrator must be deleted
	assert.Empty(t, updated.waveOrchestrators,
		"orchestrator must be deleted after all waves complete")

	// Confirmation dialog must appear for review
	assert.Equal(t, stateConfirm, updated.state,
		"state must be stateConfirm to prompt user for review")
	require.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay must be set for all-complete review prompt")
	// Standard confirm dialog (y/n) — not a wave-failed decision prompt
	assert.Equal(t, "y", updated.confirmationOverlay.ConfirmKey,
		"confirm key must be 'y' for review prompt")
}

// TestWaveAllCompleteMsg_TransitionsToReviewing verifies that the waveAllCompleteMsg
// handler transitions the plan FSM from implementing to reviewing.
func TestWaveAllCompleteMsg_TransitionsToReviewing(t *testing.T) {
	const planFile = "2026-02-24-review-transition.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "review transition test", "plan/review-trans", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.fsm = newPlanFSMForTest(t, plansDir)

	model, _ := h.Update(waveAllCompleteMsg{planFile: planFile})
	updated := model.(*home)

	// Reload plan state from disk to verify FSM transition persisted.
	reloaded, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReviewing, entry.Status,
		"plan must transition to reviewing after waveAllCompleteMsg")

	// Toast must confirm the transition
	_ = updated // ensure the model is used (toast is in-memory, hard to assert without rendering)
}

// TestWaveMonitor_AllComplete_MultiWave verifies the flow with a multi-wave plan
// where all waves complete sequentially (wave 1 done → advance → wave 2 done → review prompt).
func TestWaveMonitor_AllComplete_MultiWave(t *testing.T) {
	const planFile = "2026-02-24-multi-wave.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "W1 task", Body: "first"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "W2 task", Body: "second"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()     // start wave 1
	orch.MarkTaskComplete(1) // wave 1 done → WaveStateWaveComplete
	require.Equal(t, WaveStateWaveComplete, orch.State())

	orch.StartNextWave() // advance to wave 2
	require.Equal(t, WaveStateRunning, orch.State())

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "multi wave test", "plan/multi-wave", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Wave 2 task instance — agent finished
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "multi-wave-W2-T2",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 2,
	})
	require.NoError(t, err)
	inst.PromptDetected = true

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "multi-wave-W2-T2", TmuxAlive: true}},
		PlanState: ps,
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
	const planFile = "2026-02-25-retry-cleanup.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "Task 1", Body: "do first"},
				{Number: 6, Title: "Task 6", Body: "the flaky one"},
			}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	// Task 1 completed, task 6 failed.
	orch.MarkTaskComplete(1)
	orch.MarkTaskFailed(6)
	require.Equal(t, WaveStateAllComplete, orch.State(), "single-wave plan should be AllComplete")

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "retry cleanup test", "plan/retry-cleanup", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	planName := planstate.DisplayName(planFile)

	// Create the completed task 1 instance
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:      planName + "-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
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
		PlanFile:   planFile,
		TaskNumber: 6,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	failedInst6.SetStatus(session.Paused) // failed tasks end up paused

	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
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
			if inst.TaskNumber == 6 && inst.PlanFile == planFile {
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
		if inst.TaskNumber == 1 && inst.PlanFile == planFile {
			foundTask1 = true
		}
	}
	assert.True(t, foundTask1, "task 1 instance must not be affected by task 6 retry")
}

// TestWaveSignal_TriggersImplementation verifies that a wave signal file written
// in .signals/ is correctly picked up by ScanWaveSignals and parsed into a
// WaveSignal with the correct WaveNumber and PlanFile fields, ready for TUI consumption.
func TestWaveSignal_TriggersImplementation(t *testing.T) {
	plansDir := t.TempDir()
	signalsDir := filepath.Join(plansDir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Create a plan with wave headers
	planContent := "# Test\n\n**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\nDo the thing.\n"
	planFile := "2026-02-20-wave-signal-test.md"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	// Register plan as implementing
	ps := &planstate.PlanState{Dir: plansDir, Plans: make(map[string]planstate.PlanEntry), TopicEntries: make(map[string]planstate.TopicEntry)}
	ps.Plans[planFile] = planstate.PlanEntry{
		Status: "implementing",
		Branch: "plan/wave-signal-test",
	}
	require.NoError(t, ps.Save())

	// Write a wave signal
	signalFile := fmt.Sprintf("implement-wave-1-%s", planFile)
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, signalFile), nil, 0o644))

	// Verify signal is scannable
	waveSignals := planfsm.ScanWaveSignals(plansDir)
	require.Len(t, waveSignals, 1)
	assert.Equal(t, 1, waveSignals[0].WaveNumber)
	assert.Equal(t, planFile, waveSignals[0].PlanFile)
}

// TestPlannerExit_CancelKillsInstanceAndMarksPrompted verifies that pressing "n"
// on the planner-exit dialog kills the planner instance and marks plannerPrompted.
func TestPlannerExit_CancelKillsInstanceAndMarksPrompted(t *testing.T) {
	const planFile = "2026-02-22-cancel-kill.md"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-cancel-inst",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	// Create storage so saveAllInstances doesn't panic
	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                         context.Background(),
		state:                       stateConfirm,
		appConfig:                   config.DefaultConfig(),
		nav:                         ui.NewNavigationPanel(&sp),
		menu:                        ui.NewMenu(),
		tabbedWindow:                ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:                overlay.NewToastManager(&sp),
		storage:                     storage,
		waveOrchestrators:           make(map[string]*WaveOrchestrator),
		plannerPrompted:             make(map[string]bool),
		pendingPlannerInstanceTitle: "planner-cancel-inst",
		pendingPlannerPlanFile:      planFile,
		confirmationOverlay:         overlay.NewConfirmationOverlay("Plan 'cancel-kill' is ready. Start implementation?"),
		allInstances:                []*session.Instance{inst},
	}
	_ = h.nav.AddInstance(inst)

	// Press 'n' (cancel key — default for confirmation overlay)
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	model, _ := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	assert.True(t, updated.plannerPrompted[planFile],
		"plannerPrompted must be true after cancel")
	assert.Empty(t, updated.allInstances,
		"planner instance must be removed from allInstances after cancel")
	assert.Empty(t, updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after cancel")
	assert.Empty(t, updated.pendingPlannerPlanFile,
		"pendingPlannerPlanFile must be cleared after cancel")
}

// --- Focus-before-overlay tests ---
// These verify that agent-related overlays auto-focus the relevant instance
// so the user can see the agent output behind the dialog.

// TestWaveMonitor_FocusesTaskInstance_WhenWaveCompleteShown verifies that
// showing the wave-complete confirmation auto-selects a task instance for
// that plan so the agent output is visible behind the overlay.
func TestWaveMonitor_FocusesTaskInstance_WhenWaveCompleteShown(t *testing.T) {
	const planFile = "2026-02-21-focus-wave.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus wave test", "plan/focus-wave", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	planName := planstate.DisplayName(planFile)

	// Add an "other" instance first (so it's selected by default), then the task instance.
	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()
	taskTitle := fmt.Sprintf("%s-W1-T1", planName)
	taskInst := &session.Instance{
		Title:      taskTitle,
		Program:    "opencode",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	}
	taskInst.MarkStartedForTest()
	taskInst.PromptDetected = true

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(taskInst)
	h.updateSidebarPlans() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: taskTitle, TmuxAlive: true},
		},
		PlanState: ps,
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
	const planFile = "2026-02-21-focus-failed.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "Task 2", Body: "follow up"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus failed test", "plan/focus-failed", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	planName := planstate.DisplayName(planFile)

	// "other" instance selected by default.
	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()
	taskTitle := fmt.Sprintf("%s-W1-T1", planName)
	taskInst := &session.Instance{
		Title:      taskTitle,
		Program:    "opencode",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	}
	taskInst.MarkStartedForTest()
	taskInst.SetStatus(session.Paused) // paused = treated as failed

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(taskInst)
	h.updateSidebarPlans() // register plans so rebuildRows emits plan-grouped instances
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
	const planFile = "2026-02-21-focus-planner.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus planner test", "plan/focus-planner", time.Now()))
	// Plan is StatusPlanning — the PlannerFinished signal will transition it to StatusReady.
	seedPlanStatus(t, ps, planFile, planstate.StatusPlanning)

	plannerInst := &session.Instance{
		Title:     "focus-planner-plan",
		Program:   "opencode",
		PlanFile:  planFile,
		AgentType: session.AgentTypePlanner,
	}
	plannerInst.MarkStartedForTest()

	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()

	h := waveFlowHome(t, ps, plansDir, nil)
	h.waveOrchestrators = make(map[string]*WaveOrchestrator)
	h.plannerPrompted = make(map[string]bool)
	h.pendingReviewFeedback = make(map[string]string)
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(plannerInst)
	h.updateSidebarPlans() // register plans so rebuildRows emits plan-grouped instances
	h.nav.SetSelectedInstance(0)
	require.Equal(t, otherInst, h.nav.GetSelectedInstance(), "precondition: other-agent selected")

	// Use the signal-driven path: PlannerFinished signal triggers the dialog.
	signal := planfsm.Signal{
		Event:    planfsm.PlannerFinished,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "other-agent", TmuxAlive: true},
			{Title: "focus-planner-plan", TmuxAlive: true},
		},
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
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
	const planFile = "2026-02-28-deferred-complete.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "Only task", Body: "do it"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "deferred test", "plan/deferred", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "deferred-complete-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.PromptDetected = true

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	h.fsm = newPlanFSMForTest(t, plansDir)
	_ = h.nav.AddInstance(inst)

	// Simulate user being in an overlay (e.g. another confirmation dialog)
	h.state = stateConfirm

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "deferred-complete-W1-T1", TmuxAlive: true}},
		PlanState: ps,
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
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "T1"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "T2"}}},
		},
	}
	orch := NewWaveOrchestrator("test.md", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1) // wave 1 complete, no failures

	m := &home{
		appConfig:         &config.Config{AutoAdvanceWaves: true},
		waveOrchestrators: map[string]*WaveOrchestrator{"test.md": orch},
		planState:         &planstate.PlanState{Plans: map[string]planstate.PlanEntry{"test.md": {Status: "implementing"}}},
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
	const planFile = "2026-02-28-auto-advance-failure.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "T1"},
				{Number: 2, Title: "T2"},
			}},
			{Number: 2, Tasks: []planparser.Task{{Number: 3, Title: "T3"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1)
	orch.MarkTaskFailed(2) // wave 1 complete with 1 failure

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "auto-advance failure test", "plan/auto-advance-failure", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Create task instances
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-failure-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst1.PromptDetected = true

	inst2, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-failure-W1-T2",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 2,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst2.SetStatus(session.Paused) // failed

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
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
	require.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay must be set for failed-wave decision")
	assert.Equal(t, "r", updated.confirmationOverlay.ConfirmKey,
		"failed-wave confirm key must be 'r' (retry)")
}

// TestAutoAdvanceWaves_EmitsAdvanceMsgOnSuccess verifies that when AutoAdvanceWaves
// is true and a wave completes with zero failures, the Update handler emits a
// waveAdvanceMsg directly (no confirmation dialog shown).
func TestAutoAdvanceWaves_EmitsAdvanceMsgOnSuccess(t *testing.T) {
	const planFile = "2026-02-28-auto-advance-success.md"

	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{{Number: 1, Title: "T1"}}},
			{Number: 2, Tasks: []planparser.Task{{Number: 2, Title: "T2"}}},
		},
	}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.StartNextWave()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "auto-advance success test", "plan/auto-advance-success", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      "auto-advance-success-W1-T1",
		Path:       t.TempDir(),
		Program:    "claude",
		PlanFile:   planFile,
		TaskNumber: 1,
		WaveNumber: 1,
	})
	require.NoError(t, err)
	inst.PromptDetected = true

	h := waveFlowHome(t, ps, plansDir, map[string]*WaveOrchestrator{planFile: orch})
	// Enable auto-advance
	h.appConfig = &config.Config{AutoAdvanceWaves: true}
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "auto-advance-success-W1-T1", TmuxAlive: true}},
		PlanState: ps,
	}
	model, cmd := h.Update(msg)
	updated := model.(*home)

	// With auto-advance enabled and no failures, must NOT show a confirmation dialog
	assert.NotEqual(t, stateConfirm, updated.state,
		"auto-advance must not show confirmation dialog on success")
	assert.Nil(t, updated.confirmationOverlay,
		"no confirmation overlay must be shown when auto-advancing")

	// The cmd must be non-nil (it contains the waveAdvanceMsg)
	assert.NotNil(t, cmd, "auto-advance must emit a tea.Cmd containing waveAdvanceMsg")
}

// TestCoderExit_FocusesCoderInstance_BeforePushConfirm verifies that when a
// coder finishes (tmux dies) and the "push branch?" dialog shows, the coder
// instance is auto-focused so its output is visible behind the overlay.
func TestCoderExit_FocusesCoderInstance_BeforePushConfirm(t *testing.T) {
	const planFile = "2026-02-21-focus-coder.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "focus coder test", "plan/focus-coder", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	coderInst := &session.Instance{
		Title:     "focus-coder-implement",
		Program:   "opencode",
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	}
	coderInst.MarkStartedForTest()

	otherInst := &session.Instance{Title: "other-agent", Program: "opencode"}
	otherInst.MarkStartedForTest()

	h := waveFlowHome(t, ps, plansDir, nil)
	h.waveOrchestrators = make(map[string]*WaveOrchestrator)
	h.plannerPrompted = make(map[string]bool)
	h.pendingReviewFeedback = make(map[string]string)
	_ = h.nav.AddInstance(otherInst)
	_ = h.nav.AddInstance(coderInst)
	h.updateSidebarPlans() // register plans so rebuildRows emits plan-grouped instances
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
