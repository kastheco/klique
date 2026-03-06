package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TmuxBrowserItem holds metadata for a single tmux session (managed or orphaned).
type TmuxBrowserItem struct {
	Name      string
	Title     string
	Created   time.Time
	Windows   int
	Attached  bool
	Width     int
	Height    int
	Managed   bool   // true = tracked by a kasmos instance
	TaskFile  string // task filename (managed only)
	AgentType string // "planner"/"coder"/"reviewer" (managed only)
	Status    string // "running"/"ready"/"loading"/"paused" (managed only)
}

// TmuxBrowserOverlay shows orphaned tmux sessions with kill/adopt/attach actions.
type TmuxBrowserOverlay struct {
	sessions    []TmuxBrowserItem
	filtered    []int // indices into sessions
	selectedIdx int
	searchQuery string
	width       int
}

// NewTmuxBrowserOverlay creates a browser overlay from discovered orphan sessions.
func NewTmuxBrowserOverlay(items []TmuxBrowserItem) *TmuxBrowserOverlay {
	b := &TmuxBrowserOverlay{
		sessions: items,
		width:    56,
	}
	b.applyFilter()
	return b
}

func (b *TmuxBrowserOverlay) applyFilter() {
	b.filtered = nil
	query := strings.ToLower(b.searchQuery)
	for i, item := range b.sessions {
		if query == "" || strings.Contains(strings.ToLower(item.Title), query) {
			b.filtered = append(b.filtered, i)
		}
	}
	if b.selectedIdx >= len(b.filtered) {
		b.selectedIdx = len(b.filtered) - 1
	}
	if b.selectedIdx < 0 {
		b.selectedIdx = 0
	}
}

// SelectedItem returns the currently highlighted session, or a zero value if empty.
func (b *TmuxBrowserOverlay) SelectedItem() TmuxBrowserItem {
	if len(b.filtered) == 0 || b.selectedIdx >= len(b.filtered) {
		return TmuxBrowserItem{}
	}
	return b.sessions[b.filtered[b.selectedIdx]]
}

// RemoveSelected removes the currently selected item from the list.
func (b *TmuxBrowserOverlay) RemoveSelected() {
	if len(b.filtered) == 0 || b.selectedIdx >= len(b.filtered) {
		return
	}
	idx := b.filtered[b.selectedIdx]
	b.sessions = append(b.sessions[:idx], b.sessions[idx+1:]...)
	b.applyFilter()
}

// IsEmpty returns true if there are no sessions to display.
func (b *TmuxBrowserOverlay) IsEmpty() bool {
	return len(b.sessions) == 0
}

// render draws the browser overlay.
func (b *TmuxBrowserOverlay) render() string {
	st := DefaultStyles()
	var s strings.Builder

	s.WriteString(st.Title.Render("tmux sessions"))
	s.WriteString("\n")

	// Search bar
	// lipgloss v2: Width() = total outer width. FloatingBorder frame = 6.
	innerWidth := b.width - 6
	if innerWidth < 10 {
		innerWidth = 10
	}
	searchText := b.searchQuery
	if searchText == "" {
		searchText = st.Muted.Render("\uf002 type to filter...")
	}
	s.WriteString(st.SearchBar.Width(innerWidth).Render(searchText))
	s.WriteString("\n")

	// Items
	if len(b.filtered) == 0 {
		s.WriteString(st.Muted.Render("  no sessions"))
		s.WriteString("\n")
	} else {
		for i, idx := range b.filtered {
			item := b.sessions[idx]
			age := relativeTime(item.Created)
			dims := fmt.Sprintf("%d×%d", item.Width, item.Height)

			attachedIndicator := "  "
			if item.Attached {
				attachedIndicator = "● "
			}

			// Badge for managed items
			badge := ""
			if item.Managed {
				badgeText := "managed"
				if item.AgentType != "" {
					badgeText = item.AgentType
				}
				badge = st.Muted.Render(" [" + badgeText + "]")
			}

			label := fmt.Sprintf("%-28s %8s %s%s",
				truncateStr(item.Title, 28), age, attachedIndicator, dims) + badge

			if i == b.selectedIdx {
				s.WriteString(st.SelectedItem.Width(innerWidth).Render("▸ " + label))
			} else {
				s.WriteString(st.Item.Width(innerWidth).Render("  " + label))
			}
			s.WriteString("\n")
		}
	}

	hint := "↑↓ navigate · k kill · o attach · esc close"
	if len(b.filtered) > 0 && !b.SelectedItem().Managed {
		hint = "↑↓ navigate · k kill · a adopt · o attach · esc close"
	}
	s.WriteString(st.Hint.Render(hint))

	return st.FloatingBorder.Width(b.width).Render(s.String())
}

// SetSize updates the overlay width.
func (b *TmuxBrowserOverlay) SetSize(width, height int) {
	b.width = width
}

// HandleKey implements Overlay. Processes a key event and returns a Result.
//
// "kill" returns Result{Action: "kill"} without Dismissed so the browser stays
// open and the user can kill multiple sessions. The app layer must handle
// non-dismissed action results. "adopt" and "attach" do dismiss the overlay.
func (b *TmuxBrowserOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	switch msg.Code {
	case tea.KeyEscape:
		if b.searchQuery != "" {
			b.searchQuery = ""
			b.applyFilter()
			return Result{}
		}
		return Result{Dismissed: true}
	case tea.KeyEnter:
		if len(b.filtered) > 0 {
			return Result{Dismissed: true, Action: "attach"}
		}
		return Result{}
	case tea.KeyUp:
		if b.selectedIdx > 0 {
			b.selectedIdx--
		}
		return Result{}
	case tea.KeyDown:
		if b.selectedIdx < len(b.filtered)-1 {
			b.selectedIdx++
		}
		return Result{}
	case tea.KeyBackspace:
		if len(b.searchQuery) > 0 {
			runes := []rune(b.searchQuery)
			b.searchQuery = string(runes[:len(runes)-1])
			b.applyFilter()
		}
		return Result{}
	default:
		if len(msg.Text) > 0 {
			r := msg.Text
			// Action keys only fire when search is empty
			if b.searchQuery == "" {
				switch r {
				case "k":
					if len(b.filtered) > 0 {
						// Do NOT dismiss — browser stays open for multi-kill workflow.
						// The app layer handles the action via the non-dismissed path.
						return Result{Action: "kill"}
					}
					return Result{}
				case "a":
					if len(b.filtered) > 0 && !b.SelectedItem().Managed {
						return Result{Dismissed: true, Action: "adopt"}
					}
					return Result{}
				case "o":
					if len(b.filtered) > 0 {
						return Result{Dismissed: true, Action: "attach"}
					}
					return Result{}
				}
			}
			// All other runes type into search
			b.searchQuery += r
			b.applyFilter()
			return Result{}
		}
	}
	return Result{}
}

// HandleMouse implements MouseHandler. A left click on a visible session row
// selects it and mirrors the Enter key by attaching to that session.
func (b *TmuxBrowserOverlay) HandleMouse(relX, relY int, button tea.MouseButton) Result {
	if button != tea.MouseLeft {
		return Result{}
	}
	_ = relX

	lines := strings.Split(b.View(), "\n")
	if relY < 0 || relY >= len(lines) {
		return Result{}
	}

	line := stripANSI(lines[relY])
	for i, idx := range b.filtered {
		item := b.sessions[idx]
		titleField := fmt.Sprintf("%-28s", truncateStr(item.Title, 28))
		if lineContainsTextBoundary(line, titleField) {
			b.selectedIdx = i
			return Result{Dismissed: true, Action: "attach"}
		}
	}

	return Result{}
}

// View implements Overlay. Returns the rendered overlay string.
func (b *TmuxBrowserOverlay) View() string {
	return b.render()
}

// truncateStr truncates s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// relativeTime returns a human-readable relative time string.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
