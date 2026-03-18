package overlay

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

// ContextMenuItem represents a single menu option.
// If Children is non-empty the item is a parent/category that drills into a sub-menu
// when selected rather than returning an action.
type ContextMenuItem struct {
	Label    string
	Action   string // identifier returned when selected (empty for parent items)
	Disabled bool
	Children []ContextMenuItem // non-nil → this is a parent item
}

// menuLevel is a snapshot of state saved on the navigation stack when drilling in.
type menuLevel struct {
	items       []ContextMenuItem
	title       string
	selectedIdx int
	searchQuery string
}

// ContextMenu displays a floating context menu with search, numbered shortcuts,
// and optional sub-menu navigation via Children.
type ContextMenu struct {
	items       []ContextMenuItem
	filtered    []filteredItem
	selectedIdx int
	width       int
	searchQuery string

	// rootItems holds the immutable copy of the original item tree so AllItems()
	// can always recurse from the root regardless of current drill depth.
	rootItems []ContextMenuItem

	// title is shown when inside a sub-menu.
	title string

	// stack holds the saved state of parent levels when drilled into sub-menus.
	stack []menuLevel
}

// filteredItem tracks the original index for number shortcuts.
type filteredItem struct {
	item    ContextMenuItem
	origIdx int // 1-based number for display and hotkey
}

// NewContextMenu creates a context menu with the given items.
// Position is managed by the OverlayManager via ShowPositioned.
func NewContextMenu(items []ContextMenuItem) *ContextMenu {
	c := &ContextMenu{
		items:     append([]ContextMenuItem(nil), items...),
		rootItems: append([]ContextMenuItem(nil), items...),
	}
	c.applyFilter()
	c.calculateWidth()
	return c
}

// displayLabel returns the canonical display text for a menu item: 1-based index
// number, a space, the item label, and (for parent items) a " →" suffix.
// Using this helper in calculateWidth, HandleMouse, and View keeps measured,
// matched, and rendered strings consistent.
func (c *ContextMenu) displayLabel(index int, item ContextMenuItem) string {
	label := fmt.Sprintf("%d %s", index, item.Label)
	if len(item.Children) > 0 {
		label += " →"
	}
	return label
}

func (c *ContextMenu) calculateWidth() {
	maxWidth := 0
	// Recurse over rootItems so width is stable across all drill levels.
	var measureItems func(items []ContextMenuItem)
	measureItems = func(items []ContextMenuItem) {
		for i, item := range items {
			label := fmt.Sprintf("%d %s", i+1, item.Label)
			if len(item.Children) > 0 {
				label += " →"
				// Account for the "← Label" header shown when inside this sub-menu.
				headerW := runewidth.StringWidth("← " + item.Label)
				if headerW > maxWidth {
					maxWidth = headerW
				}
				measureItems(item.Children)
			}
			if w := runewidth.StringWidth(label); w > maxWidth {
				maxWidth = w
			}
		}
	}
	measureItems(c.rootItems)
	placeholder := "\uf002 Type to filter..."
	if w := runewidth.StringWidth(placeholder); w > maxWidth {
		maxWidth = w
	}
	c.width = maxWidth + 4 // padding for item decorations
}

func (c *ContextMenu) applyFilter() {
	c.filtered = nil
	query := strings.ToLower(c.searchQuery)
	for i, item := range c.items {
		if query == "" || strings.Contains(strings.ToLower(item.Label), query) {
			c.filtered = append(c.filtered, filteredItem{
				item:    item,
				origIdx: i + 1,
			})
		}
	}
	if c.selectedIdx >= len(c.filtered) {
		c.selectedIdx = len(c.filtered) - 1
	}
	if c.selectedIdx < 0 {
		c.selectedIdx = 0
	}
	c.skipToNonDisabled(1)
}

func (c *ContextMenu) skipToNonDisabled(direction int) {
	if len(c.filtered) == 0 {
		return
	}
	start := c.selectedIdx
	for c.filtered[c.selectedIdx].item.Disabled {
		c.selectedIdx += direction
		if c.selectedIdx >= len(c.filtered) {
			c.selectedIdx = 0
		}
		if c.selectedIdx < 0 {
			c.selectedIdx = len(c.filtered) - 1
		}
		if c.selectedIdx == start {
			break
		}
	}
}

// drillIn pushes the current level onto the navigation stack and replaces the
// active items with item.Children, resetting search and selection.
func (c *ContextMenu) drillIn(item ContextMenuItem) {
	c.stack = append(c.stack, menuLevel{
		items:       c.items,
		title:       c.title,
		selectedIdx: c.selectedIdx,
		searchQuery: c.searchQuery,
	})
	c.items = item.Children
	c.title = item.Label
	c.searchQuery = ""
	c.selectedIdx = 0
	c.applyFilter()
	c.calculateWidth()
}

// drillBack pops one level from the navigation stack and restores the saved state.
// It returns false when already at the root level (nothing was popped).
func (c *ContextMenu) drillBack() bool {
	if len(c.stack) == 0 {
		return false
	}
	top := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	c.items = top.items
	c.title = top.title
	c.selectedIdx = top.selectedIdx
	c.searchQuery = top.searchQuery
	c.applyFilter()
	c.calculateWidth()
	return true
}

// CurrentItems returns the items at the current navigation level (not the full tree).
func (c *ContextMenu) CurrentItems() []ContextMenuItem {
	return c.items
}

// AllItems recursively returns every item in the root tree (parents and all
// descendants), regardless of the current drill depth. This is always derived
// from the immutable rootItems copy stored at construction time.
func (c *ContextMenu) AllItems() []ContextMenuItem {
	return flattenItems(c.rootItems)
}

// flattenItems recursively walks an item tree and returns a flat slice containing
// every item (parents first, then their children in depth-first order).
func flattenItems(items []ContextMenuItem) []ContextMenuItem {
	var result []ContextMenuItem
	for _, item := range items {
		result = append(result, item)
		if len(item.Children) > 0 {
			result = append(result, flattenItems(item.Children)...)
		}
	}
	return result
}

// HandleKey implements Overlay. It processes a key event and returns a Result
// indicating whether the menu should close and which action was selected.
func (c *ContextMenu) HandleKey(msg tea.KeyPressMsg) Result {
	switch msg.String() {
	case "esc":
		return Result{Dismissed: true}
	case " ", "enter":
		if c.selectedIdx < len(c.filtered) && !c.filtered[c.selectedIdx].item.Disabled {
			selected := c.filtered[c.selectedIdx].item
			if len(selected.Children) > 0 {
				// Parent item: drill into sub-menu, do not dismiss.
				c.drillIn(selected)
				return Result{}
			}
			return Result{Dismissed: true, Action: selected.Action}
		}
		return Result{}
	case "left":
		// Pop back one level; no-op at root (returns false).
		c.drillBack()
		return Result{}
	case "up":
		if len(c.filtered) > 0 {
			start := c.selectedIdx
			for {
				c.selectedIdx--
				if c.selectedIdx < 0 {
					c.selectedIdx = len(c.filtered) - 1
				}
				if !c.filtered[c.selectedIdx].item.Disabled || c.selectedIdx == start {
					break
				}
			}
		}
	case "down":
		if len(c.filtered) > 0 {
			start := c.selectedIdx
			for {
				c.selectedIdx++
				if c.selectedIdx >= len(c.filtered) {
					c.selectedIdx = 0
				}
				if !c.filtered[c.selectedIdx].item.Disabled || c.selectedIdx == start {
					break
				}
			}
		}
	case "backspace":
		if len(c.searchQuery) > 0 {
			runes := []rune(c.searchQuery)
			c.searchQuery = string(runes[:len(runes)-1])
			c.applyFilter()
		} else {
			// Search is empty: pop back instead of deleting (no-op at root).
			c.drillBack()
		}
	default:
		if len(msg.Text) > 0 {
			r := rune(msg.Text[0])
			// Number shortcut (1-9) when search is empty
			if r >= '1' && r <= '9' && c.searchQuery == "" {
				num := int(r - '0')
				for i, fi := range c.filtered {
					if fi.origIdx == num && !fi.item.Disabled {
						c.selectedIdx = i
						if len(fi.item.Children) > 0 {
							// Parent item: drill in, do not dismiss.
							c.drillIn(fi.item)
							return Result{}
						}
						return Result{Dismissed: true, Action: fi.item.Action}
					}
				}
				return Result{}
			}
			c.searchQuery += msg.Text
			c.applyFilter()
		}
	}
	return Result{}
}

// HandleMouse handles mouse clicks and translates them into menu selection actions.
func (c *ContextMenu) HandleMouse(relX, relY int, button tea.MouseButton) Result {
	if button != tea.MouseLeft {
		return Result{}
	}

	lines := strings.Split(c.View(), "\n")
	if relY < 0 || relY >= len(lines) {
		return Result{}
	}

	line := stripANSI(lines[relY])
	for i, fi := range c.filtered {
		// Use displayLabel so the matched text always equals what is rendered
		// (including the " →" suffix for parent items). lineContainsTextBoundary
		// prevents false matches on shared prefixes (e.g. "1 x" inside "1 xy").
		itemText := c.displayLabel(fi.origIdx, fi.item)
		if lineContainsTextBoundary(line, itemText) {
			c.selectedIdx = i
			if fi.item.Disabled {
				return Result{}
			}
			if len(fi.item.Children) > 0 {
				c.drillIn(fi.item)
				return Result{}
			}
			return Result{Dismissed: true, Action: fi.item.Action}
		}
	}

	return Result{}
}

// View implements Overlay using DefaultStyles for rendering.
func (c *ContextMenu) View() string {
	st := DefaultStyles()
	var b strings.Builder

	// lipgloss v2: Width() sets total outer width including border+padding.
	// FloatingBorder frame = 6 (border 2 + padding 4), so content area = c.width - 6.
	// Inner components' total outer width must equal that content area.
	innerW := c.width - 6
	if innerW < 6 {
		innerW = 6
	}

	// Show a muted back-navigation header when inside a sub-menu.
	if c.title != "" {
		b.WriteString(st.Muted.Render("← " + c.title))
		b.WriteString("\n")
	}

	searchText := c.searchQuery
	if searchText == "" {
		searchText = st.Muted.Render("\uf002 Type to filter...")
	}
	b.WriteString(st.SearchBar.Width(innerW).Render(searchText))
	b.WriteString("\n")

	if len(c.filtered) == 0 {
		b.WriteString(st.DisabledItem.Width(innerW).Render("No matches"))
	} else {
		for i, fi := range c.filtered {
			fullLabel := c.displayLabel(fi.origIdx, fi.item)

			var line string
			if fi.item.Disabled {
				line = st.DisabledItem.Width(innerW).Render(fullLabel)
			} else if i == c.selectedIdx {
				line = st.SelectedItem.Width(innerW).Render(fullLabel)
			} else {
				numPrefix := st.NumberPrefix.Render(fmt.Sprintf("%d", fi.origIdx))
				itemLabel := " " + fi.item.Label
				if len(fi.item.Children) > 0 {
					// Render the arrow suffix in muted colour for normal (unselected) items.
					itemLabel += " " + st.Muted.Render("→")
				}
				line = st.Item.Width(innerW).Render(numPrefix + itemLabel)
			}
			b.WriteString(line)
			if i < len(c.filtered)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	if len(c.stack) > 0 {
		b.WriteString(st.Hint.Render("← back • ↑↓ nav • space select • esc close"))
	} else {
		b.WriteString(st.Hint.Render("↑↓ nav • space select • esc close"))
	}

	return st.FloatingBorder.Width(c.width).Render(b.String())
}

// SetSize implements Overlay. Updates available dimensions; width is used for menu layout.
func (c *ContextMenu) SetSize(width, height int) {
	c.width = width
}

// Items returns all menu items at the current level (including disabled ones), in original order.
// Wave 3 uses this for top-level category labels; use AllItems() for recursive access.
func (c *ContextMenu) Items() []ContextMenuItem {
	return c.items
}
