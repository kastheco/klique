package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldPromptPushAfterCoderExit(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeCoder}

	if !shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("expected push prompt for exited coder")
	}
}

func TestShouldPromptPushAfterCoderExit_NoPromptForReviewer(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeReviewer}

	if shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("did not expect push prompt for reviewer")
	}
}

// TestMetadataTickHandler_CoderExitTriggersPrompt verifies that when the metadata
// tick handler processes a coder instance with TmuxAlive=false and plan status
// StatusImplementing, it wires through to promptPushBranchThenAdvance and sets
// the confirmation overlay (proving the push-prompt lifecycle path is connected).
func TestMetadataTickHandler_CoderExitTriggersPrompt(t *testing.T) {
	const planFile = "2026-02-21-test-feature.md"

	// Build a planState with the plan in StatusImplementing.
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "test feature", "plan/test-feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Build a coder instance (not started — we inject metadata directly).
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     "test-feature-implement",
		Path:      t.TempDir(),
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(inst)

	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         list,
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager: overlay.NewToastManager(&sp),
		planState:    ps,
		planStateDir: plansDir,
		fsm:          planfsm.New(plansDir),
	}

	// Inject a metadataResultMsg with TmuxAlive=false for the coder instance.
	// This simulates the coder's tmux session having exited.
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{
				Title:     inst.Title,
				TmuxAlive: false,
			},
		},
		PlanState: ps,
	}

	model, _ := h.Update(msg)
	updated, ok := model.(*home)
	require.True(t, ok)

	// The push-prompt confirmation overlay must have been set.
	assert.Equal(t, stateConfirm, updated.state,
		"expected stateConfirm after coder exit with StatusImplementing")
	assert.NotNil(t, updated.confirmationOverlay,
		"expected confirmation overlay to be set for push-prompt")
}

// TestPromptPushBranchThenAdvance_SetStatusErrorPropagates verifies that when
// SetStatus fails inside the push-action closure, the error is returned as a
// tea.Msg rather than being silently swallowed with _ =.
//
// TestPromptPushBranchThenAdvance_ReturnsCoderCompleteMsg verifies that the
// confirm action returns a coderCompleteMsg so the Update handler can perform
// the FSM transition and spawn a reviewer.
func TestPromptPushBranchThenAdvance_ReturnsCoderCompleteMsg(t *testing.T) {
	const planFile = "2026-02-21-test-feature.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "test feature", "plan/test-feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	inst := &session.Instance{
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		planState:    ps,
		planStateDir: plansDir,
		fsm:          planfsm.New(plansDir),
		toastManager: overlay.NewToastManager(&sp),
	}

	// Call promptPushBranchThenAdvance — this sets pendingConfirmAction.
	_ = h.promptPushBranchThenAdvance(inst)

	require.NotNil(t, h.pendingConfirmAction,
		"pendingConfirmAction must be set after promptPushBranchThenAdvance")

	msg := h.pendingConfirmAction()

	ccMsg, ok := msg.(coderCompleteMsg)
	assert.True(t, ok,
		"push action must return coderCompleteMsg, got %T: %v", msg, msg)
	assert.Equal(t, planFile, ccMsg.planFile,
		"coderCompleteMsg must carry the correct plan file")
}

// TestMetadataTickHandler_NoRepromptWhenConfirmPending verifies that when the
// app is already in stateConfirm (a confirmation overlay is showing), a second
// metadata tick does NOT re-trigger promptPushBranchThenAdvance and overwrite
// the existing overlay. Without this guard the modal re-appears every tick.
func TestMetadataTickHandler_NoRepromptWhenConfirmPending(t *testing.T) {
	const planFile = "2026-02-21-test-feature.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "test feature", "plan/test-feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     "test-feature-implement",
		Path:      t.TempDir(),
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(inst)

	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         list,
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager: overlay.NewToastManager(&sp),
		planState:    ps,
		planStateDir: plansDir,
		fsm:          planfsm.New(plansDir),
	}

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: inst.Title, TmuxAlive: false},
		},
		PlanState: ps,
	}

	// First tick: should set stateConfirm and the overlay.
	model1, _ := h.Update(msg)
	updated1, ok := model1.(*home)
	require.True(t, ok)
	require.Equal(t, stateConfirm, updated1.state, "first tick must set stateConfirm")
	firstOverlay := updated1.confirmationOverlay
	require.NotNil(t, firstOverlay)

	// Second tick while stateConfirm is active: must NOT overwrite the overlay.
	model2, _ := updated1.Update(msg)
	updated2, ok := model2.(*home)
	require.True(t, ok)
	assert.Equal(t, stateConfirm, updated2.state, "state must remain stateConfirm")
	assert.Same(t, firstOverlay, updated2.confirmationOverlay,
		"second tick must not replace the existing confirmation overlay")
}

func TestFullPlanLifecycle_StateTransitions(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(
		"2026-02-21-auth-refactor.md",
		"Refactor JWT auth",
		"plan/auth-refactor",
		time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
	))

	seedPlanStatus(t, ps, "2026-02-21-auth-refactor.md", planstate.StatusPlanning)
	seedPlanStatus(t, ps, "2026-02-21-auth-refactor.md", planstate.StatusImplementing)
	seedPlanStatus(t, ps, "2026-02-21-auth-refactor.md", planstate.StatusReviewing)
	seedPlanStatus(t, ps, "2026-02-21-auth-refactor.md", planstate.StatusDone)

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, planstate.StatusDone, entry.Status)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
}

// TestMetadataResultMsg_SignalDoesNotClobberFreshPlanState verifies that when
// signals are present in a metadataResultMsg, the stale msg.PlanState (loaded
// by the goroutine before signals were scanned) does not overwrite the fresh
// planState that loadPlanState() sets after FSM transitions are applied.
//
// Regression test for: sentinel processed → disk updated → loadPlanState() →
// m.planState="ready", then m.planState=msg.PlanState → m.planState="planning"
// (stale), causing the sidebar to show the wrong status for ~500ms.
func TestMetadataResultMsg_SignalDoesNotClobberFreshPlanState(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	const planFile = "2026-02-23-feature.md"
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "feature", "plan/feature", time.Now()))
	// Planner is running — status is "planning"
	seedPlanStatus(t, ps, planFile, planstate.StatusPlanning)

	// Simulate the goroutine snapshot: loaded "planning" before sentinel was seen.
	stalePlanState, err := planstate.Load(plansDir)
	require.NoError(t, err)
	assert.Equal(t, planstate.StatusPlanning, stalePlanState.Plans[planFile].Status)

	// Build a minimal home with FSM wired up.
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		planState:             stalePlanState, // starts with stale state
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		pendingReviewFeedback: make(map[string]string),
		plannerPrompted:       make(map[string]bool),
		menu:                  ui.NewMenu(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager:          overlay.NewToastManager(&sp),
		list:                  ui.NewList(&sp, false),
		sidebar:               ui.NewSidebar(),
	}

	// Construct a metadataResultMsg as the goroutine would: stale PlanState +
	// a PlannerFinished signal (sentinel written after the goroutine loaded state).
	signal := planfsm.Signal{
		Event:    planfsm.PlannerFinished,
		PlanFile: planFile,
	}
	// We can't set the private filePath, so we pre-delete the sentinel file
	// (ConsumeSignal would normally delete it — safe to skip deletion here).

	msg := metadataResultMsg{
		PlanState: stalePlanState, // goroutine's stale snapshot
		Signals:   []planfsm.Signal{signal},
	}

	// Feed the message through Update.
	_, _ = h.Update(msg)

	// After Update, h.planState must reflect the FSM transition (planning→ready),
	// NOT the stale msg.PlanState snapshot.
	require.NotNil(t, h.planState)
	entry, ok := h.planState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReady, entry.Status,
		"planState must show 'ready' after PlannerFinished signal — stale msg.PlanState must not overwrite it")
}

// TestImplementFinishedSignal_SpawnsReviewer verifies that when an
// implement-finished sentinel is processed, a reviewer instance is added to the
// list and a start cmd is returned. This is the sentinel-driven equivalent of
// the old checkPlanCompletion → transitionToReview path.
func TestImplementFinishedSignal_SpawnsReviewer(t *testing.T) {
	const planFile = "2026-02-23-feature.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "feature", "plan/feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	// Create a coder instance bound to this plan.
	coderInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "feature-implement",
		Path:      dir,
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(coderInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		list:                  list,
		menu:                  ui.NewMenu(),
		sidebar:               ui.NewSidebar(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager:          overlay.NewToastManager(&sp),
		planState:             ps,
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		pendingReviewFeedback: make(map[string]string),
		plannerPrompted:       make(map[string]bool),
		activeRepoPath:        dir,
		program:               "claude",
	}

	signal := planfsm.Signal{
		Event:    planfsm.ImplementFinished,
		PlanFile: planFile,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	_, _ = h.Update(msg)

	// A reviewer instance must have been added to the list.
	var foundReviewer bool
	for _, inst := range h.list.GetInstances() {
		if inst.PlanFile == planFile && inst.IsReviewer {
			foundReviewer = true
			break
		}
	}
	assert.True(t, foundReviewer,
		"implement-finished signal must spawn a reviewer instance")

	// Plan status must be "reviewing" on disk.
	reloaded, _ := planstate.Load(plansDir)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusReviewing, entry.Status)
}

// TestReviewChangesSignal_RespawnsCoder verifies that when a review-changes
// sentinel is processed, the plan transitions back to implementing and a new
// coder instance is added with the reviewer's feedback in its prompt.
func TestReviewChangesSignal_RespawnsCoder(t *testing.T) {
	const planFile = "2026-02-23-feature.md"
	const feedback = "Fix the error handling in auth.go"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	// Write a minimal plan file so spawnPlanAgent can read it.
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte("# Plan\n## Wave 1\n- Task 1\n"), 0o644))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(planFile, "feature", "plan/feature", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusReviewing)

	// Create a reviewer instance bound to this plan.
	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:     "feature-review",
		Path:      dir,
		Program:   "claude",
		PlanFile:  planFile,
		AgentType: session.AgentTypeReviewer,
	})
	require.NoError(t, err)
	reviewerInst.IsReviewer = true

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&sp, false)
	_ = list.AddInstance(reviewerInst)

	h := &home{
		ctx:                   context.Background(),
		state:                 stateDefault,
		appConfig:             config.DefaultConfig(),
		list:                  list,
		menu:                  ui.NewMenu(),
		sidebar:               ui.NewSidebar(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		toastManager:          overlay.NewToastManager(&sp),
		planState:             ps,
		planStateDir:          plansDir,
		fsm:                   planfsm.New(plansDir),
		pendingReviewFeedback: make(map[string]string),
		plannerPrompted:       make(map[string]bool),
		activeRepoPath:        dir,
		program:               "claude",
	}

	signal := planfsm.Signal{
		Event:    planfsm.ReviewChangesRequested,
		PlanFile: planFile,
		Body:     feedback,
	}
	msg := metadataResultMsg{
		PlanState: ps,
		Signals:   []planfsm.Signal{signal},
	}

	_, _ = h.Update(msg)

	// A coder instance must have been added with feedback in its prompt.
	var foundCoder bool
	for _, inst := range h.list.GetInstances() {
		if inst.PlanFile == planFile && inst.AgentType == session.AgentTypeCoder {
			foundCoder = true
			assert.Contains(t, inst.QueuedPrompt, feedback,
				"coder prompt must contain reviewer feedback")
			break
		}
	}
	assert.True(t, foundCoder,
		"review-changes signal must spawn a coder instance")

	// Plan status must be "implementing" on disk.
	reloaded, _ := planstate.Load(plansDir)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusImplementing, entry.Status)
}

// TestIsLocked_FinishedLockedWhenDone verifies that the "finished" stage is
// locked when the plan is already done, preventing a spurious FSM error.
func TestIsLocked_FinishedLockedWhenDone(t *testing.T) {
	assert.True(t, isLocked(planstate.StatusDone, "finished"),
		"finished stage must be locked when plan is already done")
	// Still unlocked for reviewing (the valid trigger).
	assert.False(t, isLocked(planstate.StatusReviewing, "finished"),
		"finished stage must be unlocked when plan is reviewing")
}
