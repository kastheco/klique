package overlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

// LauncherItem represents a single command in the launcher.
type LauncherItem struct {
	Label    string // display text (e.g. "new plan")
	Hint     string // right-aligned keybind hint (e.g. "n")
	Action   string // identifier returned on selection
	Disabled bool
}

// CommandLauncherOverlay is a dmenu-style searchable command palette overlay.
type CommandLauncherOverlay struct {
	title       string
	allItems    []LauncherItem
	filtered    []filteredLauncherItem
	selectedIdx int
	searchQuery string
	width       int
}

// filteredLauncherItem pairs a LauncherItem with its original index.
type filteredLauncherItem struct {
	item    LauncherItem
	origIdx int // 1-based, for display
}

// NewCommandLauncherOverlay creates a CommandLauncherOverlay with the given title and items.
func NewCommandLauncherOverlay(title string, items []LauncherItem) *CommandLauncherOverlay {
	c := &CommandLauncherOverlay{
		title:    title,
		allItems: make([]LauncherItem, len(items)),
	}
	copy(c.allItems, items)
	c.applyFilter()
	c.calculateWidth()
	return c
}

func (c *CommandLauncherOverlay) calculateWidth() {
	maxWidth := 0
	for _, item := range c.allItems {
		// label + some padding + hint
		w := runewidth.StringWidth(item.Label) + runewidth.StringWidth(item.Hint) + 6
		if w > maxWidth {
			maxWidth = w
		}
	}
	placeholder := "\uf002 type to filter..."
	if w := runewidth.StringWidth(placeholder) + 4; w > maxWidth {
		maxWidth = w
	}
	if maxWidth < 50 {
		maxWidth = 50
	}
	c.width = maxWidth
}

func (c *CommandLauncherOverlay) applyFilter() {
	c.filtered = nil
	query := strings.ToLower(c.searchQuery)
	for i, item := range c.allItems {
		if query == "" || strings.Contains(strings.ToLower(item.Label), query) {
			c.filtered = append(c.filtered, filteredLauncherItem{
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

func (c *CommandLauncherOverlay) skipToNonDisabled(direction int) {
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

// HandleKey implements Overlay. Processes key events and returns a Result.
func (c *CommandLauncherOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	switch msg.String() {
	case "esc":
		return Result{Dismissed: true}
	case "enter", " ", "space":
		// "space" is what msg.String() returns for the space key (KeySpace with Text=" ")
		// because the ultraviolet library falls back to Keystroke() when Text==" ".
		if len(c.filtered) == 0 {
			return Result{Dismissed: true, Submitted: false}
		}
		if c.selectedIdx < len(c.filtered) && !c.filtered[c.selectedIdx].item.Disabled {
			return Result{Dismissed: true, Submitted: true, Action: c.filtered[c.selectedIdx].item.Action}
		}
		return Result{Dismissed: true}
	case "up", "shift+tab":
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
	case "down", "tab":
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
		// Append typed text to the search query; skip space (handled by "space" case above).
		if len(msg.Text) > 0 && msg.Text != " " {
			c.searchQuery += msg.Text
			c.applyFilter()
		}
	}
	return Result{}
}

// HandleMouse implements MouseHandler. Handles mouse clicks on items.
func (c *CommandLauncherOverlay) HandleMouse(relX, relY int, button tea.MouseButton) Result {
	if button != tea.MouseLeft {
		return Result{}
	}

	lines := strings.Split(c.View(), "\n")
	if relY < 0 || relY >= len(lines) {
		return Result{}
	}

	line := stripANSI(lines[relY])
	for i, fi := range c.filtered {
		if strings.Contains(line, fi.item.Label) {
			c.selectedIdx = i
			if fi.item.Disabled {
				return Result{}
			}
			return Result{Dismissed: true, Action: fi.item.Action}
		}
	}

	return Result{}
}

// View implements Overlay. Renders the launcher using DefaultStyles.
func (c *CommandLauncherOverlay) View() string {
	st := DefaultStyles()
	var b strings.Builder

	// lipgloss v2: Width() sets total outer width including border+padding.
	// FloatingBorder frame = 6 (border 2 + padding 4), so content area = c.width - 6.
	innerW := c.width - 6
	if innerW < 6 {
		innerW = 6
	}

	// Title line
	b.WriteString(st.Title.Render(c.title))
	b.WriteString("\n")

	// Search bar
	searchText := c.searchQuery
	if searchText == "" {
		searchText = st.Muted.Render("\uf002 type to filter...")
	}
	b.WriteString(st.SearchBar.Width(innerW).Render(searchText))
	b.WriteString("\n")

	// Item list
	if len(c.filtered) == 0 {
		b.WriteString(st.DisabledItem.Width(innerW).Render("no matches"))
	} else {
		for i, fi := range c.filtered {
			label := fi.item.Label
			hint := fi.item.Hint

			// Calculate padding between label and hint
			// innerW accounts for border frame; prefix "▸ " = 2 chars, " " before hint = 1 char = 3 total extra
			padLen := innerW - runewidth.StringWidth(label) - runewidth.StringWidth(hint) - 4
			if padLen < 1 {
				padLen = 1
			}
			pad := strings.Repeat(" ", padLen)

			hintStr := st.Muted.Render(hint)

			var line string
			if fi.item.Disabled {
				line = st.DisabledItem.Width(innerW).Render(
					"  " + label + pad + hint)
			} else if i == c.selectedIdx {
				line = st.SelectedItem.Width(innerW).Render(
					"▸ " + label + pad + hint)
			} else {
				// Render hint inline since we need special styling
				line = st.Item.Width(innerW).Render(
					"  " + label + pad + hintStr)
			}
			b.WriteString(line)
			if i < len(c.filtered)-1 {
				b.WriteString("\n")
			}
		}
	}

	// Bottom hint
	b.WriteString("\n")
	b.WriteString(st.Hint.Render("↑↓ navigate • enter select • esc close"))

	return st.FloatingBorder.Width(c.width).Render(b.String())
}

// SetSize implements Overlay. Updates available dimensions.
func (c *CommandLauncherOverlay) SetSize(width, height int) {
	c.width = width
}
