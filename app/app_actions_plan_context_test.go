package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
)

func TestExecuteContextAction_KillRunningInstancesInPlan(t *testing.T) {
	sp := spinner.New()
	h := &home{
		ctx:     context.Background(),
		list:    ui.NewList(&sp, false),
		sidebar: ui.NewSidebar(),
		menu:    ui.NewMenu(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusInProgress},
			"2026-02-21-beta.md":  {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()

	mk := func(title, planFile string) *session.Instance {
		inst, _ := session.NewInstance(session.InstanceOptions{Title: title, Path: ".", Program: "claude", PlanFile: planFile})
		return inst
	}
	alpha1 := mk("alpha-1", "2026-02-21-alpha.md")
	alpha2 := mk("alpha-2", "2026-02-21-alpha.md")
	beta := mk("beta", "2026-02-21-beta.md")
	h.allInstances = []*session.Instance{alpha1, alpha2, beta}
	_ = h.list.AddInstance(alpha1)
	_ = h.list.AddInstance(alpha2)
	_ = h.list.AddInstance(beta)

	h.updateSidebarItems()
	h.sidebar.ClickItem(2) // alpha plan

	_, _ = h.executeContextAction("kill_running_instances_in_plan")
	if h.confirmationOverlay == nil {
		t.Fatalf("expected confirmation overlay")
	}

	h.confirmationOverlay.OnConfirm()

	if len(h.allInstances) != 1 || h.allInstances[0].Title != "beta" {
		t.Fatalf("remaining instances mismatch: %+v", h.allInstances)
	}
}
