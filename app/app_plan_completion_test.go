package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
	"github.com/kastheco/klique/ui/overlay"
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
	require.NoError(t, ps.SetStatus(planFile, planstate.StatusImplementing))

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

	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusPlanning))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusImplementing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusReviewing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusFinished))

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, planstate.StatusFinished, entry.Status)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
}
