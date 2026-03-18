package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandLauncherOverlay_ImplementsOverlay(t *testing.T) {
	items := []LauncherItem{{Label: "quit", Hint: "q", Action: "quit"}}
	var _ Overlay = NewCommandLauncherOverlay("commands", items)
}

func TestCommandLauncherOverlay_ImplementsMouseHandler(t *testing.T) {
	items := []LauncherItem{{Label: "quit", Hint: "q", Action: "quit"}}
	var _ MouseHandler = NewCommandLauncherOverlay("commands", items)
}

func TestCommandLauncherOverlay_SelectFirstItem(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "view keybinds", Hint: "?", Action: "view_keybinds"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "new_plan", result.Action)
}

func TestCommandLauncherOverlay_NavigateAndSelect(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "view keybinds", Hint: "?", Action: "view_keybinds"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	o.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "view_keybinds", result.Action)
}

func TestCommandLauncherOverlay_FilterAndSelect(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "view keybinds", Hint: "?", Action: "view_keybinds"},
		{Label: "quit", Hint: "q", Action: "quit"},
	}
	o := NewCommandLauncherOverlay("commands", items)

	// Type "view" to filter
	o.HandleKey(tea.KeyPressMsg{Text: "v"})
	o.HandleKey(tea.KeyPressMsg{Text: "i"})
	o.HandleKey(tea.KeyPressMsg{Text: "e"})
	o.HandleKey(tea.KeyPressMsg{Text: "w"})

	// First (and only) match should be selected
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "view_keybinds", result.Action)
}

func TestCommandLauncherOverlay_EscDismisses(t *testing.T) {
	items := []LauncherItem{{Label: "quit", Hint: "q", Action: "quit"}}
	o := NewCommandLauncherOverlay("commands", items)
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
	assert.Empty(t, result.Action)
}

func TestCommandLauncherOverlay_SpaceSelects(t *testing.T) {
	items := []LauncherItem{{Label: "quit", Hint: "q", Action: "quit"}}
	o := NewCommandLauncherOverlay("commands", items)
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "quit", result.Action)
}

func TestCommandLauncherOverlay_DisabledItemSkipped(t *testing.T) {
	items := []LauncherItem{
		{Label: "disabled", Hint: "-", Action: "disabled", Disabled: true},
		{Label: "enabled", Hint: "e", Action: "enabled"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	// First non-disabled item should be auto-selected
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "enabled", result.Action)
}

func TestCommandLauncherOverlay_BackspaceRemovesFilter(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "quit", Hint: "q", Action: "quit"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	o.HandleKey(tea.KeyPressMsg{Text: "z"})
	// "z" matches nothing
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action) // no match
	// Reset and verify backspace restores items
	o2 := NewCommandLauncherOverlay("commands", items)
	o2.HandleKey(tea.KeyPressMsg{Text: "z"})
	o2.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	result2 := o2.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "new_plan", result2.Action)
}

func TestCommandLauncherOverlay_ViewRendersHints(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "view keybinds", Hint: "?", Action: "view_keybinds"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	view := stripANSI(o.View())
	assert.Contains(t, view, "commands")
	assert.Contains(t, view, "new plan")
	assert.Contains(t, view, "n")
	assert.Contains(t, view, "view keybinds")
	assert.Contains(t, view, "?")
}

func TestCommandLauncherOverlay_HandleMouse_SelectItem(t *testing.T) {
	items := []LauncherItem{
		{Label: "new plan", Hint: "n", Action: "new_plan"},
		{Label: "quit", Hint: "q", Action: "quit"},
	}
	o := NewCommandLauncherOverlay("commands", items)
	view := o.View()
	// Find the "quit" item line
	x, y := launcherMouseTarget(t, view, "quit")
	result := o.HandleMouse(x, y, tea.MouseLeft)
	assert.True(t, result.Dismissed)
	assert.Equal(t, "quit", result.Action)
}

// launcherMouseTarget finds the position of needle in the rendered overlay view.
func launcherMouseTarget(t *testing.T, view, needle string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		x := strings.Index(clean, needle)
		if x >= 0 {
			return x, y
		}
	}
	require.FailNowf(t, "missing target", "could not find %q in view", needle)
	return 0, 0
}
