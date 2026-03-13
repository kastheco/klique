package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plannerSignalHome builds a minimal home with a planner instance registered
// in StatusPlanning (so the FSM transition PlannerFinished → StatusReady succeeds).
func plannerSignalHome(t *testing.T, planFile string) (*home, *taskstate.TaskState, string, *session.Instance) {
	t.Helper()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	require.NoError(t, ps.Register(planFile, "test plan", "plan/test", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusPlanning)

	plannerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "test-plan-planner",
		Path:      dir,
		Program:   "claude",
		TaskFile:  planFile,
		AgentType: session.AgentTypePlanner,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	_ = list.AddInstance(plannerInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		nav:                   list,
		allInstances:          []*session.Instance{plannerInst},
		menu:                  ui.NewMenu(),
		toastManager:          overlay.NewToastManager(&sp),
		overlays:              overlay.NewManager(),
		taskState:             ps,
		taskStateDir:          plansDir,
		taskStore:             store,
		taskStoreProject:      "test",
		fsm:                   fsm,
		plannerPrompted:       make(map[string]bool),
		coderPushPrompted:     make(map[string]bool),
		pendingReviewFeedback: make(map[string]string),
		waveOrchestrators:     make(map[string]*orchestration.WaveOrchestrator),
		instanceFinalizers:    make(map[*session.Instance]func()),
		activeRepoPath:        dir,
		program:               "claude",
	}

	return h, ps, plansDir, plannerInst
}

// TestPlannerFinishedSignal_ShowsConfirmDialog verifies that when a PlannerFinished
// signal is processed, the app enters stateConfirm with a confirmation overlay
// and pendingPlannerTaskFile is set.
func TestPlannerFinishedSignal_ShowsConfirmDialog(t *testing.T) {
	const planFile = "feature"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	signal := taskfsm.Signal{
		Event:    taskfsm.PlannerFinished,
		TaskFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []taskfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"PlannerFinished signal must set stateConfirm")
	assert.True(t, updated.overlays.IsActive(),
		"PlannerFinished signal must show confirmation overlay")
	assert.Equal(t, planFile, updated.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must be set to the plan file from the signal")
}

// TestPlannerFinishedSignal_ConfirmKillsPlannerAndTriggersImplement verifies that
// after the user confirms (plannerCompleteMsg), the planner instance is removed,
// plannerPrompted is set, and triggerTaskStage("implement") is called.
func TestPlannerFinishedSignal_ConfirmKillsPlannerAndTriggersImplement(t *testing.T) {
	const planFile = "feature"
	h, _, plansDir, plannerInst := plannerSignalHome(t, planFile)

	// Set up the state as if the confirm dialog was shown after a PlannerFinished signal.
	h.state = stateConfirm
	h.pendingPlannerInstanceTitle = plannerInst.Title
	h.pendingPlannerTaskFile = planFile

	// Write a minimal plan file so triggerTaskStage can read it.
	planContent := "# Plan\n\n## Wave 1\n\n### Task 1: Something\n\nDo it.\n"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	// Advance the FSM to StatusReady so triggerTaskStage can proceed to "implement".
	require.NoError(t, h.fsm.Transition(planFile, taskfsm.PlannerFinished))
	h.loadTaskState()

	// Send the confirm message.
	_, _ = h.Update(plannerCompleteMsg{planFile: planFile})

	assert.True(t, h.plannerPrompted[planFile],
		"plannerPrompted must be true after confirm")
	assert.Empty(t, h.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after confirm")
	assert.Empty(t, h.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must be cleared after confirm")

	// Planner instance must be removed from nav and allInstances.
	for _, inst := range h.nav.GetInstances() {
		assert.NotEqual(t, plannerInst.Title, inst.Title,
			"planner instance must be removed from nav after confirm")
	}
	for _, inst := range h.allInstances {
		assert.NotEqual(t, plannerInst.Title, inst.Title,
			"planner instance must be removed from allInstances after confirm")
	}
}

// TestPlannerFinishedSignal_CancelKillsPlannerAndLeavesReady verifies that after
// the user cancels (no), the planner instance is removed, plannerPrompted is set,
// and the plan stays at StatusReady.
func TestPlannerFinishedSignal_CancelKillsPlannerAndLeavesReady(t *testing.T) {
	const planFile = "feature"
	h, _, _, plannerInst := plannerSignalHome(t, planFile)

	// Advance FSM to StatusReady (as the signal handler would do).
	require.NoError(t, h.fsm.Transition(planFile, taskfsm.PlannerFinished))
	h.loadTaskState()

	// Set up state as if the confirm dialog was shown.
	h.state = stateConfirm
	h.pendingPlannerInstanceTitle = plannerInst.Title
	h.pendingPlannerTaskFile = planFile
	h.overlays.Show(overlay.NewConfirmationOverlay("plan is ready. start implementation?"))
	h.pendingConfirmAction = func() tea.Msg {
		return plannerCompleteMsg{planFile: planFile}
	}

	// Press "n" (cancel).
	keyMsg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	model, _ := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	assert.True(t, updated.plannerPrompted[planFile],
		"plannerPrompted must be true after cancel")
	assert.Empty(t, updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after cancel")
	assert.Empty(t, updated.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must be cleared after cancel")

	// Planner instance must be removed.
	for _, inst := range updated.nav.GetInstances() {
		assert.NotEqual(t, plannerInst.Title, inst.Title,
			"planner instance must be removed from nav after cancel")
	}

	// Plan must still be at StatusReady (not advanced to implementing).
	entry, ok := updated.taskState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status,
		"plan must remain at StatusReady after cancel — user declined implementation")
}

// TestPlannerFinishedSignal_SkipsWhenAlreadyPrompted verifies that when
// plannerPrompted[planFile] is already true, no dialog is shown.
func TestPlannerFinishedSignal_SkipsWhenAlreadyPrompted(t *testing.T) {
	const planFile = "feature"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// Mark already prompted.
	h.plannerPrompted[planFile] = true

	signal := taskfsm.Signal{
		Event:    taskfsm.PlannerFinished,
		TaskFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []taskfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.NotEqual(t, stateConfirm, updated.state,
		"no confirm dialog when plannerPrompted is already true")
	assert.False(t, updated.overlays.IsActive(),
		"no confirmation overlay when plannerPrompted is already true")
}

// TestPlannerTmuxDeath_NoFallbackDialog verifies that when a planner pane dies
// but NO sentinel was written (plan still in StatusPlanning), NO confirmation
// dialog is shown. The plan must remain in StatusPlanning.
//
// This is the definitive regression test for the removed tmux-death fallback:
// spurious transitions must not occur just because the planner process died.
func TestPlannerTmuxDeath_NoFallbackDialog(t *testing.T) {
	const planFile = "no-fallback"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// No sentinel written — plan stays in StatusPlanning.
	// The planner pane dies (TmuxAlive: false), but there are no signals.
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-plan-planner", TmuxAlive: false},
		},
		PlanState: ps,
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.NotEqual(t, stateConfirm, updated.state,
		"tmux death without sentinel must NOT show a confirm dialog")
	assert.False(t, updated.overlays.IsActive(),
		"tmux death without sentinel must NOT show confirmation overlay")

	// Plan must remain in StatusPlanning (not advanced).
	entry, ok := updated.taskState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusPlanning, entry.Status,
		"plan must remain StatusPlanning when planner pane dies without a sentinel")
}

// TestPlannerFinishedSignal_DeferredWhenOverlayActive verifies that when the
// PlannerFinished signal arrives while an overlay is active, the dialog is NOT
// lost — it is deferred and shown on the next metadata tick once the overlay clears.
func TestPlannerFinishedSignal_DeferredWhenOverlayActive(t *testing.T) {
	const planFile = "feature"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// Simulate an active overlay (e.g. new-plan form is open).
	existingOverlay := overlay.NewConfirmationOverlay("unrelated question?")
	h.state = stateConfirm
	h.overlays.Show(existingOverlay)

	signal := taskfsm.Signal{
		Event:    taskfsm.PlannerFinished,
		TaskFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []taskfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	// Overlay must be untouched — we must NOT clobber it.
	assert.Same(t, existingOverlay, updated.overlays.Current(),
		"existing overlay must not be replaced while active")

	// The plan file must be queued for deferred dialog.
	assert.Contains(t, updated.deferredPlannerDialogs, planFile,
		"plan file must be queued in deferredPlannerDialogs when overlay was active")

	// Now simulate the overlay clearing (state returns to default).
	updated.state = stateDefault
	updated.overlays.Dismiss()

	// Send an empty metadata tick — deferred dialog should fire.
	emptyMsg := metadataResultMsg{PlanState: ps}
	model2, _ := updated.Update(emptyMsg)
	updated2 := model2.(*home)

	assert.Equal(t, stateConfirm, updated2.state,
		"deferred PlannerFinished dialog must show on next tick after overlay clears")
	assert.True(t, updated2.overlays.IsActive(),
		"confirmation overlay must be set for deferred dialog")
	assert.Empty(t, updated2.deferredPlannerDialogs,
		"deferredPlannerDialogs must be cleared after showing the dialog")
	assert.Equal(t, planFile, updated2.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must be set for the deferred plan")
}

// TestPlannerFinishedSignal_SkipsWhenConfirmActive verifies that when
// state == stateConfirm, no new dialog is shown (avoids clobbering an active overlay).
func TestPlannerFinishedSignal_SkipsWhenConfirmActive(t *testing.T) {
	const planFile = "feature"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// Pre-existing confirm overlay.
	existingOverlay := overlay.NewConfirmationOverlay("existing question?")
	h.state = stateConfirm
	h.overlays.Show(existingOverlay)

	signal := taskfsm.Signal{
		Event:    taskfsm.PlannerFinished,
		TaskFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []taskfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"state must remain stateConfirm")
	assert.Same(t, existingOverlay, updated.overlays.Current(),
		"existing overlay must not be replaced when confirm is already active")
	assert.Empty(t, updated.pendingPlannerTaskFile,
		"pendingPlannerTaskFile must not be set when confirm is already active")
}
