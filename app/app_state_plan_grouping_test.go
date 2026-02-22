package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
)

func newHomeForPlanStateTests(t *testing.T) *home {
	t.Helper()
	sp := spinner.New()
	h := &home{
		ctx:     context.Background(),
		list:    ui.NewList(&sp, false),
		sidebar: ui.NewSidebar(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusInProgress},
			"2026-02-21-beta.md":  {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()
	return h
}

func addInstance(t *testing.T, h *home, title, planFile string) {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    title,
		Path:     ".",
		Program:  "claude",
		PlanFile: planFile,
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	_ = h.list.AddInstance(inst)
}

func TestFilterInstancesByPlan_SelectedPlanFiltersList(t *testing.T) {
	h := newHomeForPlanStateTests(t)
	addInstance(t, h, "alpha-worker", "2026-02-21-alpha.md")
	addInstance(t, h, "beta-worker", "2026-02-21-beta.md")
	h.updateSidebarItems()

	h.sidebar.ClickItem(2) // All, Plans(section), alpha(plan)
	h.filterInstancesByPlan()

	if h.list.NumInstances() != 1 {
		t.Fatalf("filtered instances = %d, want 1", h.list.NumInstances())
	}
	if got := h.list.GetSelectedInstance().Title; got != "alpha-worker" {
		t.Fatalf("selected title = %q, want %q", got, "alpha-worker")
	}
}

func TestFilterBySearch_IsGlobalAcrossPlans(t *testing.T) {
	h := newHomeForPlanStateTests(t)
	addInstance(t, h, "alpha-worker", "2026-02-21-alpha.md")
	addInstance(t, h, "beta-worker", "2026-02-21-beta.md")
	h.updateSidebarItems()

	h.sidebar.ClickItem(2) // select alpha plan first
	h.filterInstancesByPlan()
	h.sidebar.SetSearchQuery("beta-worker")

	h.filterBySearch()

	if h.list.NumInstances() != 1 {
		t.Fatalf("search result size = %d, want 1", h.list.NumInstances())
	}
	if got := h.list.GetSelectedInstance().Title; got != "beta-worker" {
		t.Fatalf("search selected = %q, want %q", got, "beta-worker")
	}
}

func TestGetAssignablePlanNames_IncludesUngroupedAndPlans(t *testing.T) {
	h := newHomeForPlanStateTests(t)

	items, mapping := h.getAssignablePlanNames()
	if len(items) != 3 {
		t.Fatalf("picker items len = %d, want 3", len(items))
	}
	if items[0] != "(Ungrouped)" {
		t.Fatalf("items[0] = %q, want (Ungrouped)", items[0])
	}
	if mapping["(Ungrouped)"] != "" {
		t.Fatalf("ungrouped mapping = %q, want empty", mapping["(Ungrouped)"])
	}
}
