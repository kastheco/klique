package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPanel() *NavigationPanel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return NewNavigationPanel(&sp)
}

func makeInst(title, planFile string, status session.Status) *session.Instance {
	return &session.Instance{
		Title:    title,
		PlanFile: planFile,
		Status:   status,
	}
}

// ---------- rebuildRows grouping ----------

func TestRebuildRows_EmptyPanel(t *testing.T) {
	n := newTestPanel()
	n.SetData(nil, nil, nil, nil, nil)
	assert.Empty(t, n.rows)
	assert.Equal(t, 0, n.selectedIdx)
}

func TestRebuildRows_PlansWithInstances(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{
		{Filename: "plan-a.md"},
		{Filename: "plan-b.md"},
	}
	instances := []*session.Instance{
		makeInst("a-impl", "plan-a.md", session.Running),
		makeInst("b-impl", "plan-b.md", session.Running),
	}
	statuses := map[string]TopicStatus{
		"plan-a.md": {HasRunning: true},
		"plan-b.md": {HasRunning: true},
	}
	n.SetData(plans, instances, nil, nil, statuses)

	// Both plans have running instances so they should be expanded.
	// Expected: plan-a header, a-impl, plan-b header, b-impl
	require.Len(t, n.rows, 4)
	assert.Equal(t, navRowPlanHeader, n.rows[0].Kind)
	assert.Equal(t, navRowInstance, n.rows[1].Kind)
	assert.Equal(t, "a-impl", n.rows[1].Label)
	assert.Equal(t, navRowPlanHeader, n.rows[2].Kind)
	assert.Equal(t, navRowInstance, n.rows[3].Kind)
}

func TestRebuildRows_SoloInstances(t *testing.T) {
	n := newTestPanel()
	instances := []*session.Instance{
		makeInst("solo-1", "", session.Running),
		makeInst("solo-2", "", session.Ready),
	}
	n.SetData(nil, instances, nil, nil, nil)

	// Solo instances: solo header + 2 instances
	require.Len(t, n.rows, 3)
	assert.Equal(t, navRowSoloHeader, n.rows[0].Kind)
	assert.Equal(t, navRowInstance, n.rows[1].Kind)
	assert.Equal(t, navRowInstance, n.rows[2].Kind)
}

func TestRebuildRows_MixedPlanAndSolo(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "plan.md"}}
	instances := []*session.Instance{
		makeInst("plan-impl", "plan.md", session.Running),
		makeInst("adhoc", "", session.Running),
	}
	statuses := map[string]TopicStatus{"plan.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// plan header + plan instance + solo header + solo instance
	require.Len(t, n.rows, 4)
	assert.Equal(t, navRowPlanHeader, n.rows[0].Kind)
	assert.Equal(t, navRowInstance, n.rows[1].Kind)
	assert.Equal(t, "plan-impl", n.rows[1].Label)
	assert.Equal(t, navRowSoloHeader, n.rows[2].Kind)
	assert.Equal(t, navRowInstance, n.rows[3].Kind)
	assert.Equal(t, "adhoc", n.rows[3].Label)
}

func TestRebuildRows_HistoryAndCancelled(t *testing.T) {
	n := newTestPanel()
	history := []PlanDisplay{{Filename: "old-plan.md"}}
	cancelled := []PlanDisplay{{Filename: "bad-plan.md"}}
	n.SetData(nil, nil, history, cancelled, nil)

	// history toggle + cancelled row
	require.Len(t, n.rows, 2)
	assert.Equal(t, navRowHistoryToggle, n.rows[0].Kind)
	assert.Equal(t, navRowCancelled, n.rows[1].Kind)
}

func TestRebuildRows_HistoryExpanded(t *testing.T) {
	n := newTestPanel()
	history := []PlanDisplay{
		{Filename: "old-a.md"},
		{Filename: "old-b.md"},
	}
	n.SetData(nil, nil, history, nil, nil)
	require.Len(t, n.rows, 1) // just the toggle, collapsed

	// Select the history toggle and expand it
	n.selectedIdx = 0
	n.ToggleSelectedExpand()
	assert.True(t, n.historyExpanded)
	// toggle + 2 history plans
	require.Len(t, n.rows, 3)
	assert.Equal(t, navRowHistoryToggle, n.rows[0].Kind)
	assert.Equal(t, navRowHistoryPlan, n.rows[1].Kind)
	assert.Equal(t, navRowHistoryPlan, n.rows[2].Kind)
}

func TestRebuildRows_ClickUpAvailable(t *testing.T) {
	n := newTestPanel()
	n.SetClickUpAvailable(true)
	require.Len(t, n.rows, 1)
	assert.Equal(t, navRowImportAction, n.rows[0].Kind)
	assert.Equal(t, SidebarImportClickUp, n.rows[0].ID)
}

// ---------- sort ordering ----------

func TestSortOrder_NotificationsFirst(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{
		{Filename: "running.md"},
		{Filename: "notified.md"},
	}
	instances := []*session.Instance{
		makeInst("running-impl", "running.md", session.Running),
		makeInst("notified-impl", "notified.md", session.Running),
	}
	instances[1].Notified = true
	statuses := map[string]TopicStatus{
		"running.md":  {HasRunning: true},
		"notified.md": {HasNotification: true},
	}
	n.SetData(plans, instances, nil, nil, statuses)

	// Notified plan should sort before running-only plan.
	require.True(t, len(n.rows) >= 2)
	assert.Contains(t, n.rows[0].PlanFile, "notified.md")
}

func TestSortOrder_InstancesWithinPlan(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "plan.md"}}
	instances := []*session.Instance{
		makeInst("paused", "plan.md", session.Paused),
		makeInst("running", "plan.md", session.Running),
		{Title: "notified", PlanFile: "plan.md", Status: session.Running, Notified: true},
	}
	statuses := map[string]TopicStatus{"plan.md": {HasRunning: true, HasNotification: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Within plan: notified (0) < running (1) < paused (3)
	require.Len(t, n.rows, 4) // header + 3 instances
	assert.Equal(t, "notified", n.rows[1].Label)
	assert.Equal(t, "running", n.rows[2].Label)
	assert.Equal(t, "paused", n.rows[3].Label)
}

// ---------- navigation (Up/Down/Left/Right) ----------

func TestNavigation_UpDown(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)
	// rows: [header, instance]
	require.Len(t, n.rows, 2)

	assert.Equal(t, 0, n.selectedIdx)
	n.Down()
	assert.Equal(t, 1, n.selectedIdx)
	n.Down() // at end, should stay
	assert.Equal(t, 1, n.selectedIdx)
	n.Up()
	assert.Equal(t, 0, n.selectedIdx)
	n.Up() // at start, should stay
	assert.Equal(t, 0, n.selectedIdx)
}

func TestNavigation_LeftCollapsesAndJumpsToParent(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Select the instance (row 1)
	n.selectedIdx = 1
	// Left on instance should jump to parent plan header
	n.Left()
	assert.Equal(t, 0, n.selectedIdx)
	assert.Equal(t, navRowPlanHeader, n.rows[0].Kind)

	// Left on expanded plan header should collapse it
	assert.False(t, n.rows[0].Collapsed)
	n.Left()
	// After collapse, rebuild happens — only plan header remains
	require.Len(t, n.rows, 1)
	assert.True(t, n.rows[0].Collapsed)
}

func TestNavigation_RightExpandsAndDescends(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Ready), // Ready — not running
	}
	n.SetData(plans, instances, nil, nil, nil)
	// Plan has no running/notification → auto-collapsed
	require.Len(t, n.rows, 1)
	assert.True(t, n.rows[0].Collapsed)

	// Right on collapsed plan should expand
	n.Right()
	require.Len(t, n.rows, 2)
	assert.False(t, n.rows[0].Collapsed)
	assert.Equal(t, 0, n.selectedIdx) // still on header

	// Right on expanded plan should descend to first child
	n.Right()
	assert.Equal(t, 1, n.selectedIdx)
}

// ---------- expand/collapse ----------

func TestToggleSelectedExpand_PlanHeader(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)
	require.Len(t, n.rows, 2) // expanded

	ok := n.ToggleSelectedExpand()
	assert.True(t, ok)
	require.Len(t, n.rows, 1) // collapsed

	ok = n.ToggleSelectedExpand()
	assert.True(t, ok)
	require.Len(t, n.rows, 2) // expanded again
}

func TestToggleSelectedExpand_Instance_ReturnsFalse(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)
	n.selectedIdx = 1 // instance row

	ok := n.ToggleSelectedExpand()
	assert.False(t, ok)
}

func TestAutoCollapse_NoRunningNoNotification(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Paused),
	}
	n.SetData(plans, instances, nil, nil, nil)

	// No running/notification → auto-collapsed, instance hidden
	require.Len(t, n.rows, 1)
	assert.True(t, n.rows[0].Collapsed)
}

func TestUserOverride_PreservesCollapsedState(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// User manually collapses (sets override)
	n.ToggleSelectedExpand()
	require.Len(t, n.rows, 1)

	// Refresh data — override should persist even though plan has running instances
	n.SetData(plans, instances, nil, nil, statuses)
	require.Len(t, n.rows, 1, "user override should keep plan collapsed")
}

// ---------- selection API ----------

func TestGetSelectedInstance(t *testing.T) {
	n := newTestPanel()
	inst := makeInst("solo", "", session.Running)
	n.SetData(nil, []*session.Instance{inst}, nil, nil, nil)

	// row 0 = solo header, row 1 = instance
	n.selectedIdx = 0
	assert.Nil(t, n.GetSelectedInstance())

	n.selectedIdx = 1
	assert.Equal(t, inst, n.GetSelectedInstance())
}

func TestGetSelectedPlanFile(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "plan.md"}}
	n.SetData(plans, nil, nil, nil, nil)

	// Plan header row
	n.selectedIdx = 0
	assert.Equal(t, "plan.md", n.GetSelectedPlanFile())
}

func TestIsSelectedPlanHeader(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "plan.md"}}
	instances := []*session.Instance{
		makeInst("inst", "plan.md", session.Running),
	}
	statuses := map[string]TopicStatus{"plan.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	n.selectedIdx = 0
	assert.True(t, n.IsSelectedPlanHeader())

	n.selectedIdx = 1
	assert.False(t, n.IsSelectedPlanHeader())
}

func TestSoloHeaderSkippedDuringNavigation(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "plan.md"}}
	instances := []*session.Instance{
		makeInst("plan-impl", "plan.md", session.Running),
		makeInst("adhoc", "", session.Running),
	}
	statuses := map[string]TopicStatus{"plan.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Layout: [0]=plan header, [1]=plan-impl, [2]=solo header, [3]=adhoc

	// Down from plan-impl should skip solo header, land on adhoc
	n.selectedIdx = 1
	n.Down()
	assert.Equal(t, 3, n.selectedIdx, "Down should skip solo header")

	// Up from adhoc should skip solo header, land on plan-impl
	n.Up()
	assert.Equal(t, 1, n.selectedIdx, "Up should skip solo header")
}

func TestSoloHeaderSkippedBySelectFirst(t *testing.T) {
	n := newTestPanel()
	instances := []*session.Instance{
		makeInst("adhoc", "", session.Running),
	}
	n.SetData(nil, instances, nil, nil, nil)

	// Layout: [0]=solo header, [1]=adhoc
	n.SelectFirst()
	assert.Equal(t, 1, n.selectedIdx, "SelectFirst should skip solo header")
}

func TestSelectByID(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{
		{Filename: "a.md"},
		{Filename: "b.md"},
	}
	n.SetData(plans, nil, nil, nil, nil)

	ok := n.SelectByID(SidebarPlanPrefix + "b.md")
	assert.True(t, ok)
	assert.Equal(t, "b.md", n.GetSelectedPlanFile())
}

func TestSelectInstance(t *testing.T) {
	n := newTestPanel()
	inst1 := makeInst("s1", "", session.Running)
	inst2 := makeInst("s2", "", session.Running)
	n.SetData(nil, []*session.Instance{inst1, inst2}, nil, nil, nil)

	ok := n.SelectInstance(inst2)
	assert.True(t, ok)
	assert.Equal(t, inst2, n.GetSelectedInstance())
}

func TestGetSelectedID(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "my-plan.md"}}
	n.SetData(plans, nil, nil, nil, nil)

	assert.Equal(t, SidebarPlanPrefix+"my-plan.md", n.GetSelectedID())
}

// ---------- selection persistence ----------

func TestSelectionPersistence_AcrossRebuild(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{
		{Filename: "a.md"},
		{Filename: "b.md"},
	}
	n.SetData(plans, nil, nil, nil, nil)
	n.selectedIdx = 1 // select plan-b header
	prevID := n.rows[1].ID

	// Refresh data — same plans, selection should be preserved
	n.SetData(plans, nil, nil, nil, nil)
	assert.Equal(t, prevID, n.rows[n.selectedIdx].ID)
}

// ---------- Kill/Remove ----------

func TestRemoveByTitle(t *testing.T) {
	n := newTestPanel()
	inst1 := makeInst("keep", "", session.Running)
	inst2 := makeInst("remove-me", "", session.Running)
	n.SetData(nil, []*session.Instance{inst1, inst2}, nil, nil, nil)

	removed := n.RemoveByTitle("remove-me")
	assert.NotNil(t, removed)
	assert.Equal(t, "remove-me", removed.Title)
	assert.Equal(t, 1, n.TotalInstances())
}

func TestRemoveByTitle_NotFound(t *testing.T) {
	n := newTestPanel()
	inst := makeInst("x", "", session.Running)
	n.SetData(nil, []*session.Instance{inst}, nil, nil, nil)

	removed := n.RemoveByTitle("nonexistent")
	assert.Nil(t, removed)
	assert.Equal(t, 1, n.TotalInstances())
}

func TestRemove_SelectedInstance(t *testing.T) {
	n := newTestPanel()
	inst := makeInst("target", "", session.Running)
	n.SetData(nil, []*session.Instance{inst}, nil, nil, nil)
	n.selectedIdx = 1 // instance row

	n.Remove()
	assert.Equal(t, 0, n.TotalInstances())
}

// ---------- search ----------

func TestSearch_ActivateDeactivate(t *testing.T) {
	n := newTestPanel()
	assert.False(t, n.IsSearchActive())

	n.ActivateSearch()
	assert.True(t, n.IsSearchActive())
	assert.Equal(t, "", n.GetSearchQuery())

	n.SetSearchQuery("hello")
	assert.Equal(t, "hello", n.GetSearchQuery())

	n.DeactivateSearch()
	assert.False(t, n.IsSearchActive())
	assert.Equal(t, "", n.GetSearchQuery())
}

func TestSearch_FiltersVisibleRows(t *testing.T) {
	n := newTestPanel()
	n.SetSize(80, 40)
	plans := []PlanDisplay{
		{Filename: "auth-plan.md"},
		{Filename: "billing-plan.md"},
	}
	instances := []*session.Instance{
		makeInst("auth-impl", "auth-plan.md", session.Running),
		makeInst("billing-impl", "billing-plan.md", session.Running),
	}
	statuses := map[string]TopicStatus{
		"auth-plan.md":    {HasRunning: true},
		"billing-plan.md": {HasRunning: true},
	}
	n.SetData(plans, instances, nil, nil, statuses)

	n.ActivateSearch()
	n.SetSearchQuery("auth")
	output := n.String()
	assert.Contains(t, output, "auth")
	// billing should be filtered out
	assert.NotContains(t, output, "billing")
}

// ---------- rendering ----------

func TestString_BasicOutput(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 30)
	plans := []PlanDisplay{{Filename: "my-plan.md"}}
	instances := []*session.Instance{
		makeInst("worker", "my-plan.md", session.Running),
	}
	statuses := map[string]TopicStatus{"my-plan.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	output := n.String()
	assert.Contains(t, output, "my-plan")
	assert.Contains(t, output, "worker")
}

func TestString_EmptyPanel(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 30)
	n.SetData(nil, nil, nil, nil, nil)
	output := n.String()
	assert.NotEmpty(t, output) // should still render search box + border
}

// ---------- CycleActive ----------

func TestCycleNextActive(t *testing.T) {
	n := newTestPanel()
	inst1 := makeInst("a", "", session.Running)
	inst2 := makeInst("b", "", session.Paused)
	inst3 := makeInst("c", "", session.Running)
	n.SetData(nil, []*session.Instance{inst1, inst2, inst3}, nil, nil, nil)

	// Select first instance
	n.SelectInstance(inst1)
	// CycleNextActive should skip paused inst2 and land on inst3
	n.CycleNextActive()
	assert.Equal(t, inst3, n.GetSelectedInstance())
}

func TestCyclePrevActive(t *testing.T) {
	n := newTestPanel()
	inst1 := makeInst("a", "", session.Running)
	inst2 := makeInst("b", "", session.Paused)
	inst3 := makeInst("c", "", session.Running)
	n.SetData(nil, []*session.Instance{inst1, inst2, inst3}, nil, nil, nil)

	// Select last instance
	n.SelectInstance(inst3)
	n.CyclePrevActive()
	assert.Equal(t, inst1, n.GetSelectedInstance())
}

// ---------- misc ----------

func TestSetSize(t *testing.T) {
	n := newTestPanel()
	n.SetSize(100, 50)
	assert.Equal(t, 100, n.width)
	assert.Equal(t, 50, n.height)
}

func TestSetFocused(t *testing.T) {
	n := newTestPanel()
	assert.True(t, n.IsFocused()) // default
	n.SetFocused(false)
	assert.False(t, n.IsFocused())
}

func TestSelectedSpaceAction(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	instances := []*session.Instance{
		makeInst("inst", "p.md", session.Running),
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, instances, nil, nil, statuses)

	// Plan header expanded → "collapse"
	n.selectedIdx = 0
	assert.Equal(t, "collapse", n.SelectedSpaceAction())

	// Collapse it
	n.ToggleSelectedExpand()
	assert.Equal(t, "expand", n.SelectedSpaceAction())

	// Instance → "toggle"
	n.ToggleSelectedExpand()
	n.selectedIdx = 1
	assert.Equal(t, "toggle", n.SelectedSpaceAction())
}

func TestAddInstance(t *testing.T) {
	n := newTestPanel()
	inst := makeInst("new", "", session.Loading)
	cleanup := n.AddInstance(inst)
	assert.NotNil(t, cleanup)
	assert.Equal(t, 1, n.TotalInstances())
	assert.Equal(t, inst, n.GetInstances()[0])
}

func TestClear(t *testing.T) {
	n := newTestPanel()
	n.SetData(nil, []*session.Instance{makeInst("x", "", session.Running)}, nil, nil, nil)
	assert.Equal(t, 1, n.TotalInstances())

	n.Clear()
	assert.Equal(t, 0, n.TotalInstances())
	assert.Empty(t, n.rows)
}

func TestSelectFirst(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "a.md"}, {Filename: "b.md"}}
	n.SetData(plans, nil, nil, nil, nil)
	n.selectedIdx = 1

	n.SelectFirst()
	assert.Equal(t, 0, n.selectedIdx)
}

func TestClickItem(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "a.md"}, {Filename: "b.md"}}
	n.SetData(plans, nil, nil, nil, nil)

	n.ClickItem(1)
	assert.Equal(t, 1, n.selectedIdx)
}

// ---------- SelectInstance auto-expand ----------

func TestSelectInstance_ExpandsCollapsedPlan(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{{Filename: "p.md"}}
	inst := makeInst("worker", "p.md", session.Ready)
	// No running/notification → plan auto-collapses, instance hidden
	n.SetData(plans, []*session.Instance{inst}, nil, nil, nil)
	require.Len(t, n.rows, 1, "plan should be auto-collapsed")
	assert.True(t, n.rows[0].Collapsed)

	// SelectInstance should auto-expand the plan to reveal the instance
	ok := n.SelectInstance(inst)
	assert.True(t, ok, "SelectInstance should succeed by expanding the plan")
	assert.Equal(t, inst, n.GetSelectedInstance())
	// Plan should now be expanded
	require.True(t, len(n.rows) >= 2)
	assert.False(t, n.rows[0].Collapsed)
}

func TestCycleActive_ExpandsCollapsedPlan(t *testing.T) {
	n := newTestPanel()
	plans := []PlanDisplay{
		{Filename: "a.md"},
		{Filename: "b.md"},
	}
	instA := makeInst("a-worker", "a.md", session.Running)
	instB := makeInst("b-worker", "b.md", session.Running)
	statuses := map[string]TopicStatus{
		"a.md": {HasRunning: true},
		"b.md": {HasRunning: true},
	}
	n.SetData(plans, []*session.Instance{instA, instB}, nil, nil, statuses)

	// Select A, then manually collapse B
	n.SelectInstance(instA)
	// Find B's plan header and collapse it
	for i, row := range n.rows {
		if row.Kind == navRowPlanHeader && row.PlanFile == "b.md" {
			n.selectedIdx = i
			n.ToggleSelectedExpand()
			break
		}
	}
	// Go back to instA
	n.SelectInstance(instA)

	// CycleNextActive should find instB even though its plan is collapsed
	n.CycleNextActive()
	assert.Equal(t, instB, n.GetSelectedInstance(), "cycle should auto-expand collapsed plan")
}

// ---------- FindPlanInstance ----------

func TestFindPlanInstance_ReturnsRunning(t *testing.T) {
	n := newTestPanel()
	ready := &session.Instance{Title: "ready", PlanFile: "p.md", Status: session.Ready}
	running := &session.Instance{Title: "running", PlanFile: "p.md", Status: session.Running}
	n.SetData(nil, []*session.Instance{ready, running}, nil, nil, nil)

	result := n.FindPlanInstance("p.md")
	assert.Equal(t, running, result, "should prefer running instance")
}

func TestFindPlanInstance_NoneStarted(t *testing.T) {
	n := newTestPanel()
	paused := makeInst("paused", "p.md", session.Paused)
	n.SetData(nil, []*session.Instance{paused}, nil, nil, nil)

	result := n.FindPlanInstance("p.md")
	assert.Nil(t, result, "paused-only plan should return nil")
}

// ---------- rendering ----------

func TestString_SectionHeaders(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 40)
	plans := []PlanDisplay{
		{Filename: "active-plan.md"},
		{Filename: "idle-plan.md"},
	}
	instances := []*session.Instance{
		makeInst("worker", "active-plan.md", session.Running),
	}
	statuses := map[string]TopicStatus{
		"active-plan.md": {HasRunning: true},
	}
	n.SetData(plans, instances, nil, nil, statuses)
	output := n.String()
	assert.Contains(t, output, "active")
	assert.Contains(t, output, "idle")
}

func TestString_InstanceDisplayTitle(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 30)
	plans := []PlanDisplay{{Filename: "p.md"}}
	inst := &session.Instance{
		Title:      "p-W2-T5",
		PlanFile:   "p.md",
		Status:     session.Running,
		WaveNumber: 2,
		TaskNumber: 5,
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	n.SetData(plans, []*session.Instance{inst}, nil, nil, statuses)

	output := n.String()
	assert.Contains(t, output, "wave 2")
	assert.Contains(t, output, "task 5")
	assert.NotContains(t, output, "W2-T5", "raw wave/task format should not appear")
}

func TestString_Legend(t *testing.T) {
	n := newTestPanel()
	n.SetSize(60, 30)
	n.SetData(nil, nil, nil, nil, nil)
	output := n.String()
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "review")
	assert.Contains(t, output, "idle")
}
