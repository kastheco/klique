package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoloAgent_NoAutomaticPushPromptOnExit verifies that when a solo agent's
// tmux session exits, the automatic push-then-review flow does NOT trigger.
func TestSoloAgent_NoAutomaticPushPromptOnExit(t *testing.T) {
	const planFile = "solo-test.md"

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	_, ps, fsm := newSharedStoreForTest(t, plansDir)
	require.NoError(t, ps.Register(planFile, "solo test", "plan/solo-test", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     "solo-test-solo",
		Path:      t.TempDir(),
		Program:   "claude",
		TaskFile:  planFile,
		AgentType: session.AgentTypeCoder,
	})
	require.NoError(t, err)
	inst.SoloAgent = true

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	_ = list.AddInstance(inst)

	h := &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		nav:               list,
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&sp),
		overlays:          overlay.NewManager(),
		taskState:         ps,
		taskStateDir:      plansDir,
		fsm:               fsm,
		waveOrchestrators: make(map[string]*orchestration.WaveOrchestrator),
	}

	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: inst.Title, TmuxAlive: false},
		},
		PlanState: ps,
	}

	model, _ := h.Update(msg)
	updated, ok := model.(*home)
	require.True(t, ok)

	assert.NotEqual(t, stateConfirm, updated.state,
		"solo agent exit must NOT trigger confirmation overlay")
	assert.False(t, updated.overlays.IsActive(),
		"solo agent exit must NOT set confirmation overlay")
}
