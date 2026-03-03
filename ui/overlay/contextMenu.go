package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// ContextMenuItem represents a single menu option.
type ContextMenuItem struct {
	Label    string
	Action   string // identifier returned when selected
	Disabled bool
}

// ContextMenu displays a floating context menu with search and numbered shortcuts.
type ContextMenu struct {
	items       []ContextMenuItem
	filtered    []filteredItem
	selectedIdx int
	width       int
	searchQuery string
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
		items: items,
	}
	c.applyFilter()
	c.calculateWidth()
	return c
}

func (c *ContextMenu) calculateWidth() {
	maxWidth := 0
	for i, item := range c.items {
		label := fmt.Sprintf("%d %s", i+1, item.Label)
		if w := runewidth.StringWidth(label); w > maxWidth {
			maxWidth = w
		}
	}
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

// HandleKey implements Overlay. It processes a key event and returns a Result
// indicating whether the menu should close and which action was selected.
func (c *ContextMenu) HandleKey(msg tea.KeyMsg) Result {
	switch msg.String() {
	case "esc":
		return Result{Dismissed: true}
	case " ", "enter":
		if c.selectedIdx < len(c.filtered) && !c.filtered[c.selectedIdx].item.Disabled {
			return Result{Dismissed: true, Action: c.filtered[c.selectedIdx].item.Action}
		}
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
		}
	default:
		if msg.Type == tea.KeyRunes {
			r := msg.Runes[0]
			// Number shortcut (1-9) when search is empty
			if r >= '1' && r <= '9' && c.searchQuery == "" {
				num := int(r - '0')
				for i, fi := range c.filtered {
					if fi.origIdx == num && !fi.item.Disabled {
						c.selectedIdx = i
						return Result{Dismissed: true, Action: fi.item.Action}
					}
				}
				return Result{}
			}
			c.searchQuery += string(msg.Runes)
			c.applyFilter()
		}
	}
	return Result{}
}

// View implements Overlay using DefaultStyles for rendering.
func (c *ContextMenu) View() string {
	st := DefaultStyles()
	var b strings.Builder

	// Width() sets content width — subtract each component's own
	// horizontal decorations so the rendered output fits in c.width.
	// SearchBar: border(2) + padding(2) = 4
	// Items: padding(2) = 2
	searchW := c.width - 4
	if searchW < 6 {
		searchW = 6
	}
	itemW := c.width - 2
	if itemW < 6 {
		itemW = 6
	}

	searchText := c.searchQuery
	if searchText == "" {
		searchText = st.Muted.Render("\uf002 Type to filter...")
	}
	b.WriteString(st.SearchBar.Width(searchW).Render(searchText))
	b.WriteString("\n")

	if len(c.filtered) == 0 {
		b.WriteString(st.DisabledItem.Width(itemW).Render("No matches"))
	} else {
		for i, fi := range c.filtered {
			numPrefix := st.NumberPrefix.Render(fmt.Sprintf("%d", fi.origIdx))
			label := fmt.Sprintf(" %s", fi.item.Label)

			var line string
			if fi.item.Disabled {
				line = st.DisabledItem.Width(itemW).Render(
					fmt.Sprintf("%d %s", fi.origIdx, fi.item.Label))
			} else if i == c.selectedIdx {
				line = st.SelectedItem.Width(itemW).Render(
					fmt.Sprintf("%d %s", fi.origIdx, fi.item.Label))
			} else {
				line = st.Item.Width(itemW).Render(numPrefix + label)
			}
			b.WriteString(line)
			if i < len(c.filtered)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(st.Hint.Render("↑↓ nav • space select • esc close"))

	return st.FloatingBorder.Width(c.width).Render(b.String())
}

// SetSize implements Overlay. Updates available dimensions; width is used for menu layout.
func (c *ContextMenu) SetSize(width, height int) {
	c.width = width
}

// Items returns all menu items (including disabled ones), in original order.
func (c *ContextMenu) Items() []ContextMenuItem {
	return c.items
}
