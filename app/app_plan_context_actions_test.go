package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFSMPlanStart_TransitionsReadyToPlanning verifies that the FSM correctly
// transitions a ready plan to planning via the PlanStart event (replacing the
// deleted setPlanStatus / modify_plan path).
func TestFSMPlanStart_TransitionsReadyToPlanning(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ps, err := planstate.Load(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "2026-02-21-auth-refactor.md"
	if err := ps.Register(planFile, "auth refactor", "plan/auth-refactor", time.Now()); err != nil {
		t.Fatal(err)
	}

	fsm := planfsm.New(plansDir)
	if err := fsm.Transition(planFile, planfsm.PlanStart); err != nil {
		t.Fatalf("Transition(PlanStart) error: %v", err)
	}

	reloaded, _ := planstate.Load(plansDir)
	entry, ok := reloaded.Entry(planFile)
	if !ok {
		t.Fatal("plan entry missing after PlanStart transition")
	}
	if entry.Status != planstate.StatusPlanning {
		t.Fatalf("status = %q, want %q", entry.Status, planstate.StatusPlanning)
	}
}

func TestExecuteContextAction_MarkPlanDoneFromReadyTransitionsToDone(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	planFile := "2026-02-24-clickup-import.md"
	require.NoError(t, ps.Register(planFile, "clickup import", "plan/clickup-import", time.Now()))

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.fsm = newFSMForTest(plansDir).PlanStateMachine
	h.updateSidebarPlans()
	h.updateSidebarItems()
	require.True(t, h.sidebar.SelectByID(ui.SidebarPlanPrefix+planFile))

	updatedModel, _ := h.executeContextAction("mark_plan_done")
	updated := updatedModel.(*home)

	reloaded, err := planstate.Load(plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusDone, entry.Status)

	updatedEntry, ok := updated.planState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusDone, updatedEntry.Status)
}
