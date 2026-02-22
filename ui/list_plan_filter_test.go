package ui

import (
	"testing"

	"github.com/kastheco/klique/session"
)

func mustInstance(t *testing.T, title, planFile string) *session.Instance {
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
	return inst
}

func TestListSetFilter_ByPlanAndUngrouped(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "alpha", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "beta", "2026-02-21-beta.md"))
	_ = l.AddInstance(mustInstance(t, "scratch", ""))

	l.SetFilter("2026-02-21-alpha.md")
	if len(l.items) != 1 || l.items[0].Title != "alpha" {
		t.Fatalf("plan filter mismatch: got %+v", l.items)
	}

	l.SetFilter(SidebarUngrouped)
	if len(l.items) != 1 || l.items[0].Title != "scratch" {
		t.Fatalf("ungrouped filter mismatch: got %+v", l.items)
	}
}

func TestListSetSearchFilter_MatchesPlanFileAcrossAllInstances(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "worker-a", "2026-02-21-auth-refactor.md"))
	_ = l.AddInstance(mustInstance(t, "worker-b", "2026-02-21-payments.md"))

	l.SetFilter("2026-02-21-auth-refactor.md") // selected plan should not scope search
	l.SetSearchFilter("payments")

	if len(l.items) != 1 || l.items[0].Title != "worker-b" {
		t.Fatalf("search should be global across plans, got %+v", l.items)
	}
}

func TestListKillInstancesByPlan(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "alpha-1", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "alpha-2", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "beta-1", "2026-02-21-beta.md"))

	l.KillInstancesByPlan("2026-02-21-alpha.md")

	if len(l.allItems) != 1 || l.allItems[0].Title != "beta-1" {
		t.Fatalf("KillInstancesByPlan() mismatch: got %+v", l.allItems)
	}
}
