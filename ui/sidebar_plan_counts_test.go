package ui

import (
	"testing"

	"github.com/kastheco/klique/config/planstate"
)

func findItem(t *testing.T, items []SidebarItem, id string) SidebarItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("sidebar item %q not found", id)
	return SidebarItem{}
}

func TestSidebarSetItems_PlanHeadersIncludeInstanceCounts(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{
		{Filename: "2026-02-21-alpha.md", Status: string(planstate.StatusInProgress)},
		{Filename: "2026-02-21-beta.md", Status: string(planstate.StatusReady)},
	})

	counts := map[string]int{
		"2026-02-21-alpha.md": 3,
		"2026-02-21-beta.md":  1,
	}
	statuses := map[string]GroupStatus{}

	s.SetItems(counts, 2, statuses)

	alpha := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-alpha.md")
	beta := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-beta.md")
	if alpha.Count != 3 || beta.Count != 1 {
		t.Fatalf("plan counts mismatch: alpha=%d beta=%d", alpha.Count, beta.Count)
	}

	ungrouped := findItem(t, s.items, SidebarUngrouped)
	if ungrouped.Count != 2 {
		t.Fatalf("ungrouped count mismatch: got %d want 2", ungrouped.Count)
	}
}

func TestSidebarUpdateMatchCounts_UsesPlanFileKey(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "2026-02-21-alpha.md", Status: string(planstate.StatusReady)}})
	s.SetItems(map[string]int{"2026-02-21-alpha.md": 1}, 1, map[string]GroupStatus{})

	s.UpdateMatchCounts(map[string]int{"2026-02-21-alpha.md": 4, "": 2}, 6)

	plan := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-alpha.md")
	if plan.MatchCount != 4 {
		t.Fatalf("plan match count mismatch: got %d want 4", plan.MatchCount)
	}
	ungrouped := findItem(t, s.items, SidebarUngrouped)
	if ungrouped.MatchCount != 2 {
		t.Fatalf("ungrouped match count mismatch: got %d want 2", ungrouped.MatchCount)
	}
}
