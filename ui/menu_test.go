package ui

import (
	"strings"
	"testing"

	"github.com/kastheco/kasmos/session"
)

func stripMenuANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
		}
		if !inEsc {
			b.WriteRune(r)
		}
		if inEsc && r == 'm' {
			inEsc = false
		}
	}
	return b.String()
}

func TestMenu_SidebarEmptyHidesNewPlanAndUsesUpdatedLabels(t *testing.T) {
	m := NewMenu()
	m.SetSize(140, 1)
	m.SetState(StateEmpty)
	m.SetFocusSlot(MenuSlotSidebar)

	out := stripMenuANSI(m.String())

	if strings.Contains(out, "new plan") {
		t.Fatalf("menu should hide 'new plan' hint when fallback logo is showing; got: %q", out)
	}
	if !strings.Contains(out, "↵/o select") {
		t.Fatalf("menu should label enter as select; got: %q", out)
	}
	if !strings.Contains(out, "space toggle") {
		t.Fatalf("menu should label space as toggle; got: %q", out)
	}
	if !strings.Contains(out, "v preview") {
		t.Fatalf("menu should label v as preview; got: %q", out)
	}
}

func TestMenu_SidebarSpaceActionLabelOverridesToggle(t *testing.T) {
	m := NewMenu()
	m.SetSize(140, 1)
	m.SetState(StateDefault)
	m.SetFocusSlot(MenuSlotSidebar)

	m.SetSidebarSpaceAction("expand")
	out := stripMenuANSI(m.String())
	if !strings.Contains(out, "space expand") {
		t.Fatalf("menu should render dynamic sidebar space label; got: %q", out)
	}

	m.SetSidebarSpaceAction("collapse")
	out = stripMenuANSI(m.String())
	if !strings.Contains(out, "space collapse") {
		t.Fatalf("menu should update sidebar space label to collapse; got: %q", out)
	}
}

func TestMenu_InstancePromptDetectedShowsYesKeybind(t *testing.T) {
	m := NewMenu()
	m.SetSize(140, 1)
	m.SetFocusSlot(MenuSlotList)
	m.SetInstance(&session.Instance{Status: session.Running, PromptDetected: true})
	m.SetState(StateDefault)

	out := stripMenuANSI(m.String())
	if !strings.Contains(out, "y yes") {
		t.Fatalf("menu should show yes keybind for prompted instance; got: %q", out)
	}
}

func TestMenu_InstanceWithoutPromptHidesYesKeybind(t *testing.T) {
	m := NewMenu()
	m.SetSize(140, 1)
	m.SetFocusSlot(MenuSlotList)
	m.SetInstance(&session.Instance{Status: session.Running, PromptDetected: false})
	m.SetState(StateDefault)

	out := stripMenuANSI(m.String())
	if strings.Contains(out, "y yes") {
		t.Fatalf("menu should hide yes keybind when instance has no prompt; got: %q", out)
	}
}

func TestMenu_TmuxSessionCountRightAligned(t *testing.T) {
	m := NewMenu()
	m.SetSize(120, 1)
	m.SetState(StateEmpty)
	m.SetTmuxSessionCount(7)

	out := stripMenuANSI(m.String())
	if !strings.Contains(out, "tmux:7") {
		t.Fatalf("menu should contain tmux:7; got: %q", out)
	}
	// tmux label should appear near the right edge (after the centered menu content)
	idx := strings.Index(out, "tmux:7")
	if idx < 80 {
		t.Fatalf("tmux:7 should be right-aligned (index >= 80), got index %d in: %q", idx, out)
	}
}

func TestMenu_TmuxSessionCountZeroHidden(t *testing.T) {
	m := NewMenu()
	m.SetSize(120, 1)
	m.SetState(StateEmpty)
	m.SetTmuxSessionCount(0)

	out := stripMenuANSI(m.String())
	if strings.Contains(out, "tmux:") {
		t.Fatalf("menu should hide tmux count when zero; got: %q", out)
	}
}
