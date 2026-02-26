package ui

import (
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanDisplayName(t *testing.T) {
	if got := planstate.DisplayName("2026-02-20-my-feature.md"); got != "my-feature" {
		t.Fatalf("planstate.DisplayName() = %q, want %q", got, "my-feature")
	}
	if got := planstate.DisplayName("plain-plan.md"); got != "plain-plan" {
		t.Fatalf("planstate.DisplayName() = %q, want %q", got, "plain-plan")
	}
}

func TestSidebarSetItems_IncludesPlansSectionBeforeTopics(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "2026-02-20-plan-orchestration.md", Status: string(planstate.StatusImplementing)}})
	s.SetItems([]string{"alpha"}, map[string]int{"alpha": 1}, 0, map[string]bool{"alpha": false}, map[string]TopicStatus{"alpha": {}}, map[string]TopicStatus{})

	if len(s.items) < 5 {
		t.Fatalf("expected at least 5 sidebar items, got %d", len(s.items))
	}

	if s.items[1].Name != "Plans" || !s.items[1].IsSection {
		t.Fatalf("item[1] = %+v, want Plans section", s.items[1])
	}
	if s.items[2].ID != SidebarPlanPrefix+"2026-02-20-plan-orchestration.md" {
		t.Fatalf("item[2].ID = %q, want %q", s.items[2].ID, SidebarPlanPrefix+"2026-02-20-plan-orchestration.md")
	}
	if s.items[3].Name != "Topics" || !s.items[3].IsSection {
		t.Fatalf("item[3] = %+v, want Topics section", s.items[3])
	}
}

func TestGetSelectedPlanFile(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "plan.md", Status: string(planstate.StatusReady)}})
	s.SetItems(nil, map[string]int{}, 0, map[string]bool{}, map[string]TopicStatus{}, map[string]TopicStatus{})

	if s.GetSelectedPlanFile() != "" {
		t.Fatalf("selected plan should be empty when All is selected")
	}

	s.ClickItem(2)
	if got := s.GetSelectedPlanFile(); got != "plan.md" {
		t.Fatalf("GetSelectedPlanFile() = %q, want %q", got, "plan.md")
	}
}

func TestGetSelectedPlanFile_StageRow(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: string(planstate.StatusReady)}},
		nil,
	)

	// Expand the plan to reveal stage rows
	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()

	// Select a stage row
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::implement")

	if got := s.GetSelectedPlanFile(); got != "fix.md" {
		t.Fatalf("GetSelectedPlanFile() with stage selected = %q, want %q", got, "fix.md")
	}
}

func TestSidebarTopicTree(t *testing.T) {
	s := NewSidebar()

	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "ui-refactor", Plans: []PlanDisplay{
				{Filename: "sidebar.md", Status: "implementing"},
				{Filename: "menu.md", Status: "ready"},
			}},
		},
		[]PlanDisplay{{Filename: "bugfix.md", Status: "ready"}}, // ungrouped
		nil, // history
	)

	// Topic header should exist
	require.True(t, s.HasRowID(SidebarTopicPrefix+"ui-refactor"))
	// Ungrouped plan should exist at top level
	require.True(t, s.HasRowID(SidebarPlanPrefix+"bugfix.md"))
}

func TestSidebarExpandTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "ui", Plans: []PlanDisplay{
				{Filename: "a.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Topic starts expanded — plan should be visible
	assert.True(t, s.HasRowID(SidebarPlanPrefix+"a.md"))

	// Collapse topic
	s.SelectByID(SidebarTopicPrefix + "ui")
	s.ToggleSelectedExpand()

	// Now plan should be hidden
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"a.md"))
}

func TestSidebarExpandPlanStages(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "implementing"}},
		nil,
	)

	// Expand ungrouped plan
	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()

	// Stage rows should appear
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::implement"))
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::review"))
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::finished"))
}

func TestSidebarSelectedSpaceAction(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{{
			Name: "ui",
			Plans: []PlanDisplay{{
				Filename: "a.md",
				Status:   "ready",
			}},
		}},
		nil,
		nil,
	)

	// Topic headers start expanded; selecting the topic should suggest collapse.
	require.True(t, s.SelectByID(SidebarTopicPrefix+"ui"))
	assert.Equal(t, "collapse", s.SelectedSpaceAction())

	// Collapse topic, then selected topic should suggest expand.
	s.ToggleSelectedExpand()
	assert.Equal(t, "expand", s.SelectedSpaceAction())

	// Select stage row (non-expandable) should fall back to toggle.
	require.True(t, s.SelectByID(SidebarTopicPrefix+"ui"))
	s.ToggleSelectedExpand() // expand topic again
	require.True(t, s.SelectByID(SidebarPlanPrefix+"a.md"))
	s.ToggleSelectedExpand() // expand plan stages
	require.True(t, s.SelectByID(SidebarPlanStagePrefix+"a.md::implement"))
	assert.Equal(t, "toggle", s.SelectedSpaceAction())
}

func TestSidebarGetSelectedPlanStage(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "reviewing"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::review")

	planFile, stage, ok := s.GetSelectedPlanStage()
	require.True(t, ok)
	assert.Equal(t, "fix.md", planFile)
	assert.Equal(t, "review", stage)
}

func TestSidebarGetSelectedTopicName(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{{Name: "auth", Plans: nil}},
		nil, nil,
	)

	s.SelectByID(SidebarTopicPrefix + "auth")
	name := s.GetSelectedTopicName()
	assert.Equal(t, "auth", name)
}

func TestSidebarPlanHistory(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil, nil,
		[]PlanDisplay{{Filename: "old.md", Status: "done"}},
	)

	assert.True(t, s.HasRowID(SidebarPlanHistoryToggle))
}

func TestSidebarSetItems_PlanRuntimeStatusOverridesReadyState(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "plan.md", Status: string(planstate.StatusReady)}})

	s.SetItems(
		nil,
		map[string]int{},
		0,
		map[string]bool{},
		map[string]TopicStatus{},
		map[string]TopicStatus{"plan.md": {HasRunning: true}},
	)

	require.Len(t, s.items, 3)
	assert.True(t, s.items[2].HasRunning)
	assert.False(t, s.items[2].HasNotification)
}

func findRowByID(rows []sidebarRow, id string) (sidebarRow, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return sidebarRow{}, false
}

func TestSidebarTreeRows_ApplyRuntimePlanStatusOverlay(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: string(planstate.StatusReady)}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()

	implementID := SidebarPlanStagePrefix + "fix.md::implement"
	implementRow, ok := findRowByID(s.rows, implementID)
	require.True(t, ok)
	assert.False(t, implementRow.Locked) // implement is never locked; triggerPlanStage validates
	assert.False(t, implementRow.Active)

	s.SetItems(nil, nil, 0, nil, map[string]TopicStatus{}, map[string]TopicStatus{"fix.md": {HasRunning: true}})

	implementRow, ok = findRowByID(s.rows, implementID)
	require.True(t, ok)
	assert.True(t, implementRow.Active)
	assert.False(t, implementRow.Locked)

	planRow, ok := findRowByID(s.rows, SidebarPlanPrefix+"fix.md")
	require.True(t, ok)
	assert.True(t, planRow.HasRunning)
	assert.False(t, planRow.HasNotification)
}

func TestSidebarTreeRender_UngroupedPlan(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "bugfix.md", Status: "ready"}},
		nil,
	)

	output := s.String()
	assert.Contains(t, output, "bugfix")
	assert.Contains(t, output, "○")
}

func TestSidebarTreeRender_TopicWithChevron(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "implementing"},
			}},
		},
		nil, nil,
	)

	output := s.String()
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "▸")
}

func TestSidebarTreeRender_ExpandedTopicShowsPlans(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "implementing"},
			}},
		},
		nil, nil,
	)

	// Topics are expanded by default — plans should be visible immediately
	output := s.String()
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "tokens")
	assert.Contains(t, output, "●")
}

func TestSidebarTreeRender_ExpandedPlanStages(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "implementing"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()

	output := s.String()
	assert.Contains(t, output, "Plan")
	assert.Contains(t, output, "Implement")
	assert.Contains(t, output, "Review")
	assert.Contains(t, output, "Finished")
}

func TestSidebarTreeRender_SelectedRowHighlighted(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetFocused(true)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{
			{Filename: "a.md", Status: "ready"},
			{Filename: "b.md", Status: "ready"},
		},
		nil,
	)

	s.Down()
	output := s.String()
	assert.Contains(t, output, "a")
	assert.Contains(t, output, "b")
}

func TestSidebarTreeRender_HistoryToggle(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "active.md", Status: "ready"}},
		[]PlanDisplay{{Filename: "old.md", Status: "done"}},
	)

	output := s.String()
	assert.Contains(t, output, "History")
}

func TestSidebar_ImportRowVisible(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetClickUpAvailable(true)
	s.SetTopicsAndPlans(nil, nil, nil)

	rendered := s.String()
	assert.Contains(t, rendered, "import from clickup")
}

func TestSidebar_ImportRowHidden(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetClickUpAvailable(false)
	s.SetTopicsAndPlans(nil, nil, nil)

	rendered := s.String()
	assert.NotContains(t, rendered, "import from clickup")
}

func TestSidebar_GetSelectedID_ImportRow(t *testing.T) {
	s := NewSidebar()
	s.SetClickUpAvailable(true)
	s.SetTopicsAndPlans(nil, nil, nil)

	for i := 0; i < len(s.rows); i++ {
		if s.rows[i].ID == SidebarImportClickUp {
			s.selectedIdx = i
			break
		}
	}

	assert.Equal(t, SidebarImportClickUp, s.GetSelectedID())
}

func TestSidebarTreeRender_CancelledPlan(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil, nil, nil,
		[]PlanDisplay{{Filename: "dropped.md", Status: "cancelled"}},
	)

	output := s.String()
	assert.Contains(t, output, "dropped")
	assert.Contains(t, output, "✕")
}

func TestSidebarTreeRender_TopicAggregateStatus(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "implementing"},
				{Filename: "session.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	output := s.String()
	assert.Contains(t, output, "●")
}

func TestSidebarRight_ExpandsCollapsedTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Collapse topic first (topics start expanded by default)
	s.SelectByID(SidebarTopicPrefix + "auth")
	s.ToggleSelectedExpand()
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))

	// Right should expand it
	s.Right()
	assert.True(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))
}

func TestSidebarRight_MovesToFirstChildWhenExpanded(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Topic starts expanded — Right moves to first child
	s.SelectByID(SidebarTopicPrefix + "auth")
	s.Right()
	assert.Equal(t, SidebarPlanPrefix+"tokens.md", s.GetSelectedID())
}

func TestSidebarLeft_CollapsesExpandedTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Topic starts expanded — Left should collapse it
	s.SelectByID(SidebarTopicPrefix + "auth")
	assert.True(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))

	s.Left()
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))
}

func TestSidebarLeft_MovesToParentFromPlan(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Topic starts expanded — select child plan directly
	s.SelectByID(SidebarPlanPrefix + "tokens.md")

	s.Left()
	assert.Equal(t, SidebarTopicPrefix+"auth", s.GetSelectedID())
}

func TestSidebarLeft_MovesToParentPlanFromStage(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "implementing"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::implement")

	s.Left()
	assert.Equal(t, SidebarPlanPrefix+"fix.md", s.GetSelectedID())
}

func TestSidebarLeft_UngroupedPlanMovesToMiscTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{
			{Filename: "a.md", Status: "ready"},
			{Filename: "b.md", Status: "ready"},
		},
		nil,
	)

	// First row is Miscellaneous topic, Down goes to a.md
	s.Down()
	assert.Equal(t, SidebarPlanPrefix+"a.md", s.GetSelectedID())
	// Left from child plan moves to parent miscellaneous topic
	s.Left()
	assert.Equal(t, SidebarTopicPrefix+"miscellaneous", s.GetSelectedID())
}

func TestSidebarRight_NoopOnStage(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "implementing"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::plan")
	before := s.GetSelectedID()

	s.Right()
	assert.Equal(t, before, s.GetSelectedID())
}

func TestSidebarRight_ExpandsPlan(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "implementing"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	assert.False(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))

	s.Right()
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))
}
