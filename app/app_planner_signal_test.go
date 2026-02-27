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
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plannerSignalHome builds a minimal home with a planner instance registered
// in StatusPlanning (so the FSM transition PlannerFinished → StatusReady succeeds).
func plannerSignalHome(t *testing.T, planFile string) (*home, *planstate.PlanState, string, *session.Instance) {
	t.Helper()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "test plan", "plan/test", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusPlanning)

	plannerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "test-plan-planner",
		Path:      dir,
		Program:   "claude",
		PlanFile:  planFile,
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
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:          overlay.NewToastManager(&sp),
		planState:             ps,
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		plannerPrompted:       make(map[string]bool),
		pendingReviewFeedback: make(map[string]string),
		waveOrchestrators:     make(map[string]*WaveOrchestrator),
		instanceFinalizers:    make(map[*session.Instance]func()),
		activeRepoPath:        dir,
		program:               "claude",
	}

	return h, ps, plansDir, plannerInst
}

// TestPlannerFinishedSignal_ShowsConfirmDialog verifies that when a PlannerFinished
// signal is processed, the app enters stateConfirm with a confirmation overlay
// and pendingPlannerPlanFile is set.
func TestPlannerFinishedSignal_ShowsConfirmDialog(t *testing.T) {
	const planFile = "2026-02-27-feature.md"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	signal := planfsm.Signal{
		Event:    planfsm.PlannerFinished,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"PlannerFinished signal must set stateConfirm")
	assert.NotNil(t, updated.confirmationOverlay,
		"PlannerFinished signal must set confirmationOverlay")
	assert.Equal(t, planFile, updated.pendingPlannerPlanFile,
		"pendingPlannerPlanFile must be set to the plan file from the signal")
}

// TestPlannerFinishedSignal_ConfirmKillsPlannerAndTriggersImplement verifies that
// after the user confirms (plannerCompleteMsg), the planner instance is removed,
// plannerPrompted is set, and triggerPlanStage("implement") is called.
func TestPlannerFinishedSignal_ConfirmKillsPlannerAndTriggersImplement(t *testing.T) {
	const planFile = "2026-02-27-feature.md"
	h, _, plansDir, plannerInst := plannerSignalHome(t, planFile)

	// Set up the state as if the confirm dialog was shown after a PlannerFinished signal.
	h.state = stateConfirm
	h.pendingPlannerInstanceTitle = plannerInst.Title
	h.pendingPlannerPlanFile = planFile

	// Write a minimal plan file so triggerPlanStage can read it.
	planContent := "# Plan\n\n## Wave 1\n\n### Task 1: Something\n\nDo it.\n"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	// Advance the FSM to StatusReady so triggerPlanStage can proceed to "implement".
	require.NoError(t, h.fsm.Transition(planFile, planfsm.PlannerFinished))
	h.loadPlanState()

	// Send the confirm message.
	_, _ = h.Update(plannerCompleteMsg{planFile: planFile})

	assert.True(t, h.plannerPrompted[planFile],
		"plannerPrompted must be true after confirm")
	assert.Empty(t, h.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after confirm")
	assert.Empty(t, h.pendingPlannerPlanFile,
		"pendingPlannerPlanFile must be cleared after confirm")

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
	const planFile = "2026-02-27-feature.md"
	h, _, _, plannerInst := plannerSignalHome(t, planFile)

	// Advance FSM to StatusReady (as the signal handler would do).
	require.NoError(t, h.fsm.Transition(planFile, planfsm.PlannerFinished))
	h.loadPlanState()

	// Set up state as if the confirm dialog was shown.
	h.state = stateConfirm
	h.pendingPlannerInstanceTitle = plannerInst.Title
	h.pendingPlannerPlanFile = planFile
	h.confirmationOverlay = overlay.NewConfirmationOverlay("plan is ready. start implementation?")
	h.pendingConfirmAction = func() tea.Msg {
		return plannerCompleteMsg{planFile: planFile}
	}

	// Press "n" (cancel).
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	model, _ := h.handleKeyPress(keyMsg)
	updated := model.(*home)

	assert.True(t, updated.plannerPrompted[planFile],
		"plannerPrompted must be true after cancel")
	assert.Empty(t, updated.pendingPlannerInstanceTitle,
		"pendingPlannerInstanceTitle must be cleared after cancel")
	assert.Empty(t, updated.pendingPlannerPlanFile,
		"pendingPlannerPlanFile must be cleared after cancel")

	// Planner instance must be removed.
	for _, inst := range updated.nav.GetInstances() {
		assert.NotEqual(t, plannerInst.Title, inst.Title,
			"planner instance must be removed from nav after cancel")
	}

	// Plan must still be at StatusReady (not advanced to implementing).
	entry, ok := updated.planState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReady, entry.Status,
		"plan must remain at StatusReady after cancel — user declined implementation")
}

// TestPlannerFinishedSignal_SkipsWhenAlreadyPrompted verifies that when
// plannerPrompted[planFile] is already true, no dialog is shown.
func TestPlannerFinishedSignal_SkipsWhenAlreadyPrompted(t *testing.T) {
	const planFile = "2026-02-27-feature.md"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// Mark already prompted.
	h.plannerPrompted[planFile] = true

	signal := planfsm.Signal{
		Event:    planfsm.PlannerFinished,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.NotEqual(t, stateConfirm, updated.state,
		"no confirm dialog when plannerPrompted is already true")
	assert.Nil(t, updated.confirmationOverlay,
		"no confirmation overlay when plannerPrompted is already true")
}

// TestPlannerTmuxDeath_NoFallbackDialog verifies that when a planner pane dies
// but NO sentinel was written (plan still in StatusPlanning), NO confirmation
// dialog is shown. The plan must remain in StatusPlanning.
//
// This is the definitive regression test for the removed tmux-death fallback:
// spurious transitions must not occur just because the planner process died.
func TestPlannerTmuxDeath_NoFallbackDialog(t *testing.T) {
	const planFile = "2026-02-27-no-fallback.md"
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
	assert.Nil(t, updated.confirmationOverlay,
		"tmux death without sentinel must NOT set confirmationOverlay")

	// Plan must remain in StatusPlanning (not advanced).
	entry, ok := updated.planState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusPlanning, entry.Status,
		"plan must remain StatusPlanning when planner pane dies without a sentinel")
}

// TestPlannerFinishedSignal_SkipsWhenConfirmActive verifies that when
// state == stateConfirm, no new dialog is shown (avoids clobbering an active overlay).
func TestPlannerFinishedSignal_SkipsWhenConfirmActive(t *testing.T) {
	const planFile = "2026-02-27-feature.md"
	h, ps, _, _ := plannerSignalHome(t, planFile)

	// Pre-existing confirm overlay.
	existingOverlay := overlay.NewConfirmationOverlay("existing question?")
	h.state = stateConfirm
	h.confirmationOverlay = existingOverlay

	signal := planfsm.Signal{
		Event:    planfsm.PlannerFinished,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	model, _ := h.Update(msg)
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"state must remain stateConfirm")
	assert.Same(t, existingOverlay, updated.confirmationOverlay,
		"existing overlay must not be replaced when confirm is already active")
	assert.Empty(t, updated.pendingPlannerPlanFile,
		"pendingPlannerPlanFile must not be set when confirm is already active")
}
