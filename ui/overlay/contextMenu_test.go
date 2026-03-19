package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ MouseHandler = NewContextMenu([]ContextMenuItem{{Label: "kill", Action: "kill"}})

func contextMenuMouseTarget(t *testing.T, view, needle string) (int, int) {
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

func TestContextMenu_ImplementsOverlay(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	var _ Overlay = NewContextMenu(items)
}

func TestContextMenu_HandleKey_Select(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "kill", result.Action)
}

func TestContextMenu_HandleKey_Navigate(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_NumberShortcut(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_Dismiss(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.Empty(t, result.Action)
}

func TestContextMenu_HandleKey_DisabledSkipped(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "disabled", Action: "disabled", Disabled: true},
		{Label: "enabled", Action: "enabled"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, "enabled", result.Action)
}

func TestContextMenu_HandleMouse_SelectRename(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}, {Label: "rename", Action: "rename"}}
	cm := NewContextMenu(items)
	x, y := contextMenuMouseTarget(t, cm.View(), "2 rename")

	result := cm.HandleMouse(x, y, tea.MouseLeft)

	assert.True(t, result.Dismissed)
	assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleMouse_DisabledIgnored(t *testing.T) {
	items := []ContextMenuItem{{Label: "kill", Action: "kill"}, {Label: "rename", Action: "rename", Disabled: true}}
	cm := NewContextMenu(items)
	x, y := contextMenuMouseTarget(t, cm.View(), "2 rename")

	result := cm.HandleMouse(x, y, tea.MouseLeft)

	assert.Equal(t, Result{}, result)
}

// --- Drill-down navigation tests ---

// TestContextMenu_FlatItemsStillWork verifies backward compatibility: menus without
// Children still select and dismiss normally.
func TestContextMenu_FlatItemsStillWork(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
		{Label: "rename", Action: "rename"},
	}
	cm := NewContextMenu(items)
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.Equal(t, "kill", result.Action)
}

// TestContextMenu_DrillIn_EnterOnParent verifies that pressing enter on an item that
// has Children drills into the sub-menu instead of dismissing the overlay.
func TestContextMenu_DrillIn_EnterOnParent(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
				{Label: "detach", Action: "detach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	// Enter on parent should NOT dismiss; it should drill in.
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, result.Dismissed, "drilling into a parent must not dismiss the overlay")
	assert.Empty(t, result.Action, "drilling into a parent must not return an action")

	// After drilling in, CurrentItems should show only the sub-menu items.
	current := cm.CurrentItems()
	require.Len(t, current, 2)
	assert.Equal(t, "attach", current[0].Label)
	assert.Equal(t, "detach", current[1].Label)
}

// TestContextMenu_DrillBack_Left verifies that pressing left pops back one level.
func TestContextMenu_DrillBack_Left(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	// Drill in.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Len(t, cm.CurrentItems(), 1, "should be in sub-menu")

	// Left should pop back.
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.False(t, result.Dismissed, "left at sub-menu level must not dismiss")

	current := cm.CurrentItems()
	require.Len(t, current, 2, "should be back at root level")
	assert.Equal(t, "session", current[0].Label)
}

// TestContextMenu_DrillBack_BackspaceWhenSearchEmpty verifies that pressing backspace
// when the search query is empty pops back one level (not dismissing).
func TestContextMenu_DrillBack_BackspaceWhenSearchEmpty(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
	}
	cm := NewContextMenu(items)

	// Drill in.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Len(t, cm.CurrentItems(), 1)

	// Backspace with empty search → pop back.
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.False(t, result.Dismissed)
	assert.Len(t, cm.CurrentItems(), 1, "root has one item (session)")
}

// TestContextMenu_AllItems_RecursiveFlattening verifies that AllItems() returns every
// item in the tree (root + all descendants), not just the current level.
func TestContextMenu_AllItems_RecursiveFlattening(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
				{Label: "detach", Action: "detach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	all := cm.AllItems()
	// Expected: session, attach, detach, kill — 4 items total (parents + children).
	require.Len(t, all, 4)
	labels := make([]string, len(all))
	for i, item := range all {
		labels[i] = item.Label
	}
	assert.Contains(t, labels, "session")
	assert.Contains(t, labels, "attach")
	assert.Contains(t, labels, "detach")
	assert.Contains(t, labels, "kill")
}

// TestContextMenu_AllItems_AfterDrillIn verifies that AllItems() still returns the
// full root tree even after drilling into a sub-menu.
func TestContextMenu_AllItems_AfterDrillIn(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	// Drill in — AllItems must still cover the whole tree.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	all := cm.AllItems()
	require.Len(t, all, 3, "session + attach + kill")

	current := cm.CurrentItems()
	require.Len(t, current, 1, "only sub-menu items at current level")
	assert.Equal(t, "attach", current[0].Label)
}

// TestContextMenu_EscAlwaysDismisses verifies that esc dismisses regardless of depth.
func TestContextMenu_EscAlwaysDismisses(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
	}
	cm := NewContextMenu(items)

	// Drill in first.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Len(t, cm.CurrentItems(), 1)

	// Esc should always dismiss.
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
}

// TestContextMenu_SelectInSubMenu verifies that selecting a leaf item inside a
// sub-menu dismisses the overlay and returns the correct action.
func TestContextMenu_SelectInSubMenu(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
				{Label: "detach", Action: "detach"},
			},
		},
	}
	cm := NewContextMenu(items)

	// Drill in.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Navigate down and select "detach".
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, result.Dismissed)
	assert.Equal(t, "detach", result.Action)
}

// TestContextMenu_NumberShortcut_DrillsIntoParent verifies that a numeric shortcut
// on a parent item drills into the sub-menu (zero Result) instead of returning an action.
func TestContextMenu_NumberShortcut_DrillsIntoParent(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label:  "session",
			Action: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	// "1" corresponds to "session" which has children → should drill, not dismiss.
	result := cm.HandleKey(tea.KeyPressMsg{Code: '1', Text: "1"})
	assert.False(t, result.Dismissed, "numeric shortcut on parent must not dismiss")
	assert.Empty(t, result.Action)

	// We should now be inside the sub-menu.
	current := cm.CurrentItems()
	require.Len(t, current, 1)
	assert.Equal(t, "attach", current[0].Label)
}

// TestContextMenu_LeftAtRootIsNoop verifies that pressing left at the root level
// does nothing (no dismiss, no state change).
func TestContextMenu_LeftAtRootIsNoop(t *testing.T) {
	items := []ContextMenuItem{
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	result := cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.False(t, result.Dismissed, "left at root must not dismiss")
	assert.Empty(t, result.Action)

	// Still at root.
	current := cm.CurrentItems()
	require.Len(t, current, 1)
	assert.Equal(t, "kill", current[0].Label)
}

// --- View / rendering tests added in Task 2 ---

// TestContextMenu_View_ParentShowsArrow verifies that items with Children are rendered
// with a "→" suffix to indicate drill-in navigation, while leaf items are not.
func TestContextMenu_View_ParentShowsArrow(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)
	view := cm.View()

	var sessionLine, killLine string
	for _, l := range strings.Split(view, "\n") {
		clean := stripANSI(l)
		if strings.Contains(clean, "1 session") {
			sessionLine = clean
		}
		if strings.Contains(clean, "2 kill") {
			killLine = clean
		}
	}
	require.NotEmpty(t, sessionLine, "should find session item line")
	require.NotEmpty(t, killLine, "should find kill item line")
	assert.Contains(t, sessionLine, "→", "parent item must have → suffix")
	assert.NotContains(t, killLine, "→", "leaf item must not have → suffix")
}

// TestContextMenu_View_SubMenuShowsTitle verifies that navigating into a sub-menu
// renders a "← title" header line, and that the root view does not show a header.
func TestContextMenu_View_SubMenuShowsTitle(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
	}
	cm := NewContextMenu(items)

	// Root view must not show a sub-menu header.
	viewBefore := cm.View()
	for _, line := range strings.Split(viewBefore, "\n") {
		assert.NotContains(t, stripANSI(line), "← session",
			"root view must not show sub-menu header")
	}

	// After drilling in the header must appear.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	viewAfter := cm.View()

	var found bool
	for _, line := range strings.Split(viewAfter, "\n") {
		if strings.Contains(stripANSI(line), "← session") {
			found = true
			break
		}
	}
	assert.True(t, found, "sub-menu view must contain ← session header")
}

// TestContextMenu_View_SubMenuHintShowsBack verifies that the hint inside a sub-menu
// includes "← back" and uses "space select" (matching the root hint wording).
func TestContextMenu_View_SubMenuHintShowsBack(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
	}
	cm := NewContextMenu(items)
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}) // drill in

	allText := stripANSI(strings.Join(strings.Split(cm.View(), "\n"), " "))
	assert.Contains(t, allText, "← back", "sub-menu hint must include ← back")
	assert.Contains(t, allText, "space select",
		"sub-menu hint must say 'space select' (not 'enter select')")
}

// TestContextMenu_HandleMouse_DrillInOnParent verifies that left-clicking a parent item
// (one that has Children) drills into the sub-menu without dismissing the overlay.
func TestContextMenu_HandleMouse_DrillInOnParent(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
			},
		},
		{Label: "kill", Action: "kill"},
	}
	cm := NewContextMenu(items)

	// The rendered row includes the "→" suffix; use it as the needle so we confirm
	// the view actually shows the parent indicator.
	view := cm.View()
	x, y := contextMenuMouseTarget(t, view, "1 session →")

	result := cm.HandleMouse(x, y, tea.MouseLeft)

	assert.Equal(t, Result{}, result, "clicking a parent item must not dismiss the overlay")
	current := cm.CurrentItems()
	require.Len(t, current, 1)
	assert.Equal(t, "attach", current[0].Label)
}

// TestContextMenu_HandleMouse_SelectInSubMenu verifies that clicking a leaf item
// inside a sub-menu dismisses the overlay and returns the correct action.
func TestContextMenu_HandleMouse_SelectInSubMenu(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "session",
			Children: []ContextMenuItem{
				{Label: "attach", Action: "attach"},
				{Label: "detach", Action: "detach"},
			},
		},
	}
	cm := NewContextMenu(items)

	// Drill into the sub-menu first.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Len(t, cm.CurrentItems(), 2)

	view := cm.View()
	x, y := contextMenuMouseTarget(t, view, "2 detach")

	result := cm.HandleMouse(x, y, tea.MouseLeft)

	assert.True(t, result.Dismissed, "clicking a leaf in a sub-menu must dismiss the overlay")
	assert.Equal(t, "detach", result.Action)
}

// TestContextMenu_CalculateWidth_IncludesChildren verifies that the menu width is
// determined by the widest label anywhere in the full item tree, including child items.
// Drilling into a sub-menu must not shrink the border.
func TestContextMenu_CalculateWidth_IncludesChildren(t *testing.T) {
	items := []ContextMenuItem{
		{
			Label: "cat",
			Children: []ContextMenuItem{
				// This long label must drive the menu width even before drilling in.
				{Label: "a very long child label wider than the parent", Action: "long"},
			},
		},
	}
	cm := NewContextMenu(items)
	widthBefore := cm.width

	// Drill in — width must remain the same because it was computed from the full tree.
	cm.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	widthAfter := cm.width

	assert.Equal(t, widthBefore, widthAfter,
		"width must be stable after drill-in; set by the widest label in the full tree")
}
