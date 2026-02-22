package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
	"github.com/kastheco/klique/ui/overlay"
)

func newHomeForInputTests(t *testing.T) *home {
	t.Helper()
	sp := spinner.New()
	h := &home{
		ctx:            context.Background(),
		state:          stateDefault,
		program:        "claude",
		activeRepoPath: ".",
		list:           ui.NewList(&sp, false),
		menu:           ui.NewMenu(),
		sidebar:        ui.NewSidebar(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()
	h.updateSidebarItems()
	return h
}

func TestKeyMoveTo_OpensAssignPlanPicker(t *testing.T) {
	h := newHomeForInputTests(t)
	inst, _ := session.NewInstance(session.InstanceOptions{Title: "w", Path: ".", Program: "claude"})
	_ = h.list.AddInstance(inst)
	h.list.SetSelectedInstance(0)

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if h.state != stateMoveTo {
		t.Fatalf("state = %v, want stateMoveTo", h.state)
	}
	if h.pickerOverlay == nil {
		t.Fatalf("pickerOverlay should be initialized")
	}
	if got := h.pickerOverlay.Render(); got == "" {
		t.Fatalf("picker render should not be empty")
	}
}

func TestStateMoveTo_SubmitAssignsPlanFile(t *testing.T) {
	h := newHomeForInputTests(t)
	inst, _ := session.NewInstance(session.InstanceOptions{Title: "w", Path: ".", Program: "claude"})
	_ = h.list.AddInstance(inst)
	h.list.SetSelectedInstance(0)

	h.state = stateMoveTo
	h.planPickerMap = map[string]string{
		"(Ungrouped)": "",
		"alpha":       "2026-02-21-alpha.md",
	}
	h.pickerOverlay = overlay.NewPickerOverlay("Assign to plan", []string{"(Ungrouped)", "alpha"})

	// move selection to "alpha", then submit
	h.pickerOverlay.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	if inst.PlanFile != "2026-02-21-alpha.md" {
		t.Fatalf("PlanFile = %q, want 2026-02-21-alpha.md", inst.PlanFile)
	}
}
