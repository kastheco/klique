package app

import (
	"context"
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
		nav:         list,
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
		nav:         ui.NewNavigationPanel(&sp),
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
	ps, err := planstate.Load(plansDir)
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
	ps, err := planstate.Load(plansDir)
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
		nav:         ui.NewNavigationPanel(&sp),
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

	ps, err := planstate.Load(plansDir)
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
		fsm:               planfsm.New(plansDir),
		activeRepoPath:    dir,
		program:           "opencode",
		nav:         list,
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
}

// ---------------------------------------------------------------------------
// Planner-exit detection and confirmation flow tests
// ---------------------------------------------------------------------------

// TestPlannerExit_ShowsImplementConfirm verifies that when a planner session's
// tmux pane dies and the plan status is StatusPlanning, a confirmation dialog appears.
func TestPlannerExit_ShowsImplementConfirm(t *testing.T) {
	const planFile = "2026-02-22-planner-exit.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "planner exit test", "plan/planner-exit", time.Now()))
	// The planner was launched so status is StatusPlanning — prompt fires when pane dies.
	seedPlanStatus(t, ps, planFile, planstate.StatusPlanning)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-exit-inst",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = make(map[string]bool)
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "planner-exit-inst", TmuxAlive: false}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"state must be stateConfirm when planner dies and plan is ready")
	require.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay must be set for planner-exit prompt")
	assert.Equal(t, "planner-exit-inst", updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be set to the planner instance title")
}

// TestPlannerExit_NoRePromptAfterAnswer verifies that once the user has answered
// the planner-exit prompt (plannerPrompted[planFile] = true), the dialog doesn't reappear.
func TestPlannerExit_NoRePromptAfterAnswer(t *testing.T) {
	const planFile = "2026-02-22-no-reprompt.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "no reprompt test", "plan/no-reprompt", time.Now()))

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-no-reprompt",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = map[string]bool{planFile: true} // already answered
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "planner-no-reprompt", TmuxAlive: false}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"state must remain stateDefault when plannerPrompted is already true")
}

// TestPlannerExit_NoPromptWhileAlive verifies that while the planner tmux pane
// is still alive, no confirmation prompt appears.
func TestPlannerExit_NoPromptWhileAlive(t *testing.T) {
	const planFile = "2026-02-22-still-alive.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "still alive test", "plan/still-alive", time.Now()))

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-alive",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.plannerPrompted = make(map[string]bool)
	_ = h.nav.AddInstance(inst)

	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "planner-alive", TmuxAlive: true}},
		PlanState: ps,
	}
	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"state must remain stateDefault while planner tmux pane is alive")
}

// TestPlannerExit_EscPreservesForRePrompt verifies that pressing esc on the
// planner-exit dialog does NOT mark plannerPrompted, allowing re-prompt next tick.
func TestPlannerExit_EscPreservesForRePrompt(t *testing.T) {
	const planFile = "2026-02-22-esc-reprompt.md"

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "planner-esc-inst",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.AgentType = session.AgentTypePlanner
	inst.PlanFile = planFile

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                         context.Background(),
		state:                       stateConfirm,
		appConfig:                   config.DefaultConfig(),
		nav:         ui.NewNavigationPanel(&sp),
		menu:                        ui.NewMenu(),
		tabbedWindow:                ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:                overlay.NewToastManager(&sp),
		waveOrchestrators:           make(map[string]*WaveOrchestrator),
		plannerPrompted:             make(map[string]bool),
		pendingPlannerInstanceTitle: "planner-esc-inst",
		confirmationOverlay:         overlay.NewConfirmationOverlay("Plan 'esc-reprompt' is ready. Start implementation?"),
	}
	_ = h.nav.AddInstance(inst)

	// Press esc
	keyMsg := tea.KeyMsg{Type: tea.KeyEscape}
	model, _ := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"state must return to default after esc")
	assert.Empty(t, updated.plannerPrompted,
		"plannerPrompted must NOT be set after esc — allows re-prompt")
	assert.Empty(t, updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after esc")
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
	ps, err := planstate.Load(plansDir)
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
	h.fsm = planfsm.New(plansDir)
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
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "review transition test", "plan/review-trans", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.fsm = planfsm.New(plansDir)

	model, _ := h.Update(waveAllCompleteMsg{planFile: planFile})
	updated := model.(*home)

	// Reload plan state from disk to verify FSM transition persisted.
	reloaded, err := planstate.Load(plansDir)
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
	ps, err := planstate.Load(plansDir)
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
	h.fsm = planfsm.New(plansDir)
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
	ps, err := planstate.Load(plansDir)
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
		nav:         ui.NewNavigationPanel(&sp),
		menu:                        ui.NewMenu(),
		tabbedWindow:                ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:                overlay.NewToastManager(&sp),
		storage:                     storage,
		waveOrchestrators:           make(map[string]*WaveOrchestrator),
		plannerPrompted:             make(map[string]bool),
		pendingPlannerInstanceTitle: "planner-cancel-inst",
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
}
