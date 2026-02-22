package ui

import (
	"testing"

	"github.com/kastheco/klique/config/planstate"
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
	s.SetPlans([]PlanDisplay{{Filename: "2026-02-20-plan-orchestration.md", Status: string(planstate.StatusInProgress)}})
	s.SetItems(map[string]int{"2026-02-20-plan-orchestration.md": 1}, 0, map[string]GroupStatus{})

	if len(s.items) < 3 {
		t.Fatalf("expected at least 3 sidebar items, got %d", len(s.items))
	}

	if s.items[1].Name != "Plans" || !s.items[1].IsSection {
		t.Fatalf("item[1] = %+v, want Plans section", s.items[1])
	}
	if s.items[2].ID != SidebarPlanPrefix+"2026-02-20-plan-orchestration.md" {
		t.Fatalf("item[2].ID = %q, want %q", s.items[2].ID, SidebarPlanPrefix+"2026-02-20-plan-orchestration.md")
	}
}

func TestGetSelectedPlanFile(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "plan.md", Status: string(planstate.StatusReady)}})
	s.SetItems(map[string]int{}, 0, map[string]GroupStatus{})

	if s.GetSelectedPlanFile() != "" {
		t.Fatalf("selected plan should be empty when All is selected")
	}

	s.ClickItem(2)
	if got := s.GetSelectedPlanFile(); got != "plan.md" {
		t.Fatalf("GetSelectedPlanFile() = %q, want %q", got, "plan.md")
	}
}

func TestSidebarTopicTree(t *testing.T) {
	s := NewSidebar()

	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "ui-refactor", Plans: []PlanDisplay{
				{Filename: "sidebar.md", Status: "in_progress"},
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

	// Topic starts collapsed — plan should not be visible
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"a.md"))

	// Expand topic
	s.SelectByID(SidebarTopicPrefix + "ui")
	s.ToggleSelectedExpand()

	// Now plan should be visible
	assert.True(t, s.HasRowID(SidebarPlanPrefix+"a.md"))
}

func TestSidebarExpandPlanStages(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
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
		[]PlanDisplay{{Filename: "old.md", Status: "completed"}},
	)

	assert.True(t, s.HasRowID(SidebarPlanHistoryToggle))
}
