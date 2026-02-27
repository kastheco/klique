package app

// Tests for the "canceled/renamed plans delay removal" bug:
//
//  1. cancel_plan: the confirm action goroutine was returning nil instead of
//     planRefreshMsg{}, so no Update() was triggered after the FSM transition.
//     The plan stayed visible in the sidebar until the next metadata tick (~2s).
//
//  2. rename_plan: after Rename() the nav selection was not updated to point at
//     the renamed plan, so the cursor silently jumped to an unrelated row.
//     Also, loadPlanState() was called redundantly — Rename() already updates
//     the in-memory PlanState, so the extra disk read was unnecessary.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCancelDelayHome builds a minimal *home for the cancel/rename delay tests.
func newCancelDelayHome(t *testing.T, ps *planstate.PlanState, plansDir, repoDir string) *home {
	t.Helper()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	storage, err := session.NewStorage(config.DefaultState())
	require.NoError(t, err)
	return &home{
		planState:      ps,
		planStateDir:   plansDir,
		fsm:            newFSMForTest(plansDir).PlanStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:   overlay.NewToastManager(&sp),
		storage:        storage,
		activeRepoPath: repoDir,
	}
}

// TestCancelPlan_ConfirmActionReturnsPlanRefreshMsg verifies that the cancel_plan
// confirm action returns planRefreshMsg (not nil) so that bubbletea triggers an
// Update() that removes the plan from the sidebar immediately — without waiting
// for the next metadata tick.
func TestCancelPlan_ConfirmActionReturnsPlanRefreshMsg(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	planFile := "2026-02-27-cancel-delay.md"
	require.NoError(t, ps.Register(planFile, "cancel delay test", "plan/cancel-delay", time.Now()))

	h := newCancelDelayHome(t, ps, plansDir, dir)
	h.updateSidebarPlans()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile), "plan must be selectable in sidebar")

	_, _ = h.executeContextAction("cancel_plan")

	require.NotNil(t, h.pendingConfirmAction, "cancel_plan must set pendingConfirmAction")

	msg := h.pendingConfirmAction()
	_, ok := msg.(planRefreshMsg)
	assert.True(t, ok,
		"cancel_plan confirm action must return planRefreshMsg so Update() fires immediately; got %T (nil means no re-render until next tick)", msg)
}

// TestRenamePlan_SelectionFollowsRenamedPlan verifies that after the rename
// handler runs (stateRenamePlan path in handleKeyPress), the nav panel selection
// points to the newly-named plan rather than an unrelated row.
//
// This is the core fix: without SelectByID after updateSidebarPlans, rebuildRows()
// cannot restore the selection (the old plan ID no longer exists in the rows) and
// the cursor silently jumps to whatever row is now at the same numeric index.
// Two plans are used so the cursor can actually land on the wrong one.
func TestRenamePlan_SelectionFollowsRenamedPlan(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	// "aardvark" sorts before "marmot" — so index 0 = aardvark, index 1 = marmot.
	// After renaming aardvark → zebra: "marmot" sorts before "zebra",
	// so index 0 = marmot, index 1 = zebra.
	// Without the fix, selectedIdx stays 0 → cursor lands on marmot, not zebra.
	aardvarkFile := "2026-02-27-aardvark.md"
	marmotFile := "2026-02-27-marmot.md"
	require.NoError(t, ps.Register(aardvarkFile, "aardvark", "plan/aardvark", time.Now()))
	require.NoError(t, ps.Register(marmotFile, "marmot", "plan/marmot", time.Now()))

	h := newCancelDelayHome(t, ps, plansDir, dir)
	h.updateSidebarPlans()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+aardvarkFile), "aardvark must be selectable")

	// Drive the actual stateRenamePlan handler via handleKeyPress with a
	// pre-submitted TextInputOverlay (value = "zebra", Submitted = true).
	// Pressing Enter on a submitted overlay triggers the rename code path.
	tio := overlay.NewTextInputOverlay("rename plan", "zebra")
	h.textInputOverlay = tio
	h.state = stateRenamePlan

	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	_, _ = h.handleKeyPress(enterKey)

	// The handler renamed aardvark → zebra and must have called SelectByID so
	// the cursor now points at the zebra plan, not marmot.
	// Derive the expected new filename the same way planstate.Rename does.
	expectedID := ui.SidebarPlanPrefix + "2026-02-27-zebra.md"
	assert.Equal(t, expectedID, h.nav.GetSelectedID(),
		"selection must follow the renamed plan; without the fix it jumps to marmot")
}
