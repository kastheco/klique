package app

import (
	"testing"

	"github.com/kastheco/klique/config/planstate"
)

func TestModifyPlanActionSetsPlanning(t *testing.T) {
	h := &home{
		planState: &planstate.PlanState{
			Dir: "/tmp",
			Plans: map[string]planstate.PlanEntry{
				"2026-02-21-auth-refactor.md": {Status: planstate.StatusImplementing},
			},
		},
	}

	err := h.setPlanStatus("2026-02-21-auth-refactor.md", planstate.StatusPlanning)
	if err != nil {
		t.Fatalf("setPlanStatus error: %v", err)
	}
	entry, _ := h.planState.Entry("2026-02-21-auth-refactor.md")
	if entry.Status != planstate.StatusPlanning {
		t.Fatalf("status = %q, want %q", entry.Status, planstate.StatusPlanning)
	}
}
