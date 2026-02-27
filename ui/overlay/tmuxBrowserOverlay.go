package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BrowserAction represents what the user chose in the tmux browser.
type BrowserAction int

const (
	BrowserNone    BrowserAction = iota
	BrowserDismiss               // esc
	BrowserKill                  // k (search empty)
	BrowserAdopt                 // a (search empty)
	BrowserAttach                // enter or o (search empty)
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
	PlanFile  string // plan filename (managed only)
	AgentType string // "planner"/"coder"/"reviewer" (managed only)
	Status    string // "running"/"ready"/"loading"/"paused" (managed only)
}

var browserBorderStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorIris).
	Padding(1, 2)

var browserTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorIris).
	MarginBottom(1)

var browserSearchStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorFoam).
	Padding(0, 1).
	MarginBottom(1)

var browserItemStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(colorText)

var browserSelectedStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Background(colorFoam).
	Foreground(colorBase)

var browserMutedStyle = lipgloss.NewStyle().
	Foreground(colorMuted)

var browserHintStyle = lipgloss.NewStyle().
	Foreground(colorMuted).
	MarginTop(1)

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

// HandleKeyPress processes input and returns the action to take.
func (b *TmuxBrowserOverlay) HandleKeyPress(msg tea.KeyMsg) BrowserAction {
	switch msg.Type {
	case tea.KeyEsc:
		if b.searchQuery != "" {
			b.searchQuery = ""
			b.applyFilter()
			return BrowserNone
		}
		return BrowserDismiss
	case tea.KeyEnter:
		if len(b.filtered) > 0 {
			return BrowserAttach
		}
		return BrowserNone
	case tea.KeyUp:
		if b.selectedIdx > 0 {
			b.selectedIdx--
		}
		return BrowserNone
	case tea.KeyDown:
		if b.selectedIdx < len(b.filtered)-1 {
			b.selectedIdx++
		}
		return BrowserNone
	case tea.KeyBackspace:
		if len(b.searchQuery) > 0 {
			runes := []rune(b.searchQuery)
			b.searchQuery = string(runes[:len(runes)-1])
			b.applyFilter()
		}
		return BrowserNone
	case tea.KeyRunes:
		r := string(msg.Runes)
		// Action keys only fire when search is empty
		if b.searchQuery == "" {
			switch r {
			case "k":
				if len(b.filtered) > 0 {
					return BrowserKill
				}
				return BrowserNone
			case "a":
				if len(b.filtered) > 0 && !b.SelectedItem().Managed {
					return BrowserAdopt
				}
				return BrowserNone
			case "o":
				if len(b.filtered) > 0 {
					return BrowserAttach
				}
				return BrowserNone
			}
		}
		// All other runes type into search
		b.searchQuery += r
		b.applyFilter()
		return BrowserNone
	}
	return BrowserNone
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

// Render draws the browser overlay.
func (b *TmuxBrowserOverlay) Render() string {
	var s strings.Builder

	s.WriteString(browserTitleStyle.Render("tmux sessions"))
	s.WriteString("\n")

	// Search bar
	innerWidth := b.width - 8
	if innerWidth < 10 {
		innerWidth = 10
	}
	searchText := b.searchQuery
	if searchText == "" {
		searchText = browserMutedStyle.Render("\uf002 type to filter...")
	}
	s.WriteString(browserSearchStyle.Width(innerWidth).Render(searchText))
	s.WriteString("\n")

	// Items
	if len(b.filtered) == 0 {
		s.WriteString(browserMutedStyle.Render("  no sessions"))
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
				badge = browserMutedStyle.Render(" [" + badgeText + "]")
			}

			label := fmt.Sprintf("%-28s %8s %s%s",
				truncateStr(item.Title, 28), age, attachedIndicator, dims) + badge

			if i == b.selectedIdx {
				s.WriteString(browserSelectedStyle.Width(innerWidth).Render("▸ " + label))
			} else {
				s.WriteString(browserItemStyle.Width(innerWidth).Render("  " + label))
			}
			s.WriteString("\n")
		}
	}

	hint := "↑↓ navigate · k kill · o attach · esc close"
	if len(b.filtered) > 0 && !b.SelectedItem().Managed {
		hint = "↑↓ navigate · k kill · a adopt · o attach · esc close"
	}
	s.WriteString(browserHintStyle.Render(hint))

	return browserBorderStyle.Width(b.width).Render(s.String())
}

// SetSize updates the overlay width.
func (b *TmuxBrowserOverlay) SetSize(width, height int) {
	b.width = width
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
