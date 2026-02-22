package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/klique/config/planstate"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

// ZoneRepoSwitch is the bubblezone ID for the clickable repo indicator.
const ZoneRepoSwitch = "repo-switch"

var sidebarTitleStyle = lipgloss.NewStyle().
	Background(ColorIris).
	Foreground(ColorBase)

// sidebarBorderStyle wraps the entire sidebar content in a subtle rounded border
var sidebarBorderStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorOverlay).
	Padding(0, 1)

var topicItemStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(ColorText)

// selectedTopicStyle — focused: iris bg on dark base
var selectedTopicStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Background(ColorIris).
	Foreground(ColorBase)

// activeTopicStyle — unfocused: muted overlay bg
var activeTopicStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Background(ColorOverlay).
	Foreground(ColorText)

var sectionHeaderStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Padding(0, 1)

var searchBarStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorOverlay).
	Padding(0, 1)

var searchActiveBarStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorFoam).
	Padding(0, 1)

const (
	SidebarAll        = "__all__"
	SidebarUngrouped  = "__ungrouped__"
	SidebarPlanPrefix = "__plan__"
)

// PlanDisplay holds plan info for sidebar rendering.
type PlanDisplay struct {
	Filename string
	Status   string
}

// dimmedTopicStyle is for topics with no matching instances during search
var dimmedTopicStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(ColorMuted)

var sidebarRunningStyle = lipgloss.NewStyle().
	Foreground(ColorFoam)

var sidebarReadyStyle = lipgloss.NewStyle().
	Foreground(ColorFoam)

var sidebarNotifyStyle = lipgloss.NewStyle().
	Foreground(ColorRose)

// SidebarItem represents a selectable item in the sidebar.
type SidebarItem struct {
	Name            string
	ID              string
	IsSection       bool
	Count           int
	MatchCount      int  // search match count (-1 = not searching)
	SharedWorktree  bool // true if this topic has a shared worktree
	HasRunning      bool // true if this topic has running instances
	HasNotification bool // true if this topic has recently-finished instances
}

// Sidebar is the left-most panel showing topics and search.
type Sidebar struct {
	items         []SidebarItem
	plans         []PlanDisplay
	selectedIdx   int
	height, width int
	focused       bool

	searchActive bool
	searchQuery  string

	repoName    string // current repo name shown at bottom
	repoHovered bool   // true when mouse is hovering over the repo button
}

// SetPlans stores unfinished plans for sidebar display.
func (s *Sidebar) SetPlans(plans []PlanDisplay) {
	s.plans = plans
}

func NewSidebar() *Sidebar {
	return &Sidebar{
		items: []SidebarItem{
			{Name: "All", ID: SidebarAll},
		},
		selectedIdx: 0,
		focused:     true,
	}
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

func (s *Sidebar) IsFocused() bool {
	return s.focused
}

// SetRepoName sets the current repo name displayed at the bottom of the sidebar.
func (s *Sidebar) SetRepoName(name string) {
	s.repoName = name
}

// SetRepoHovered sets whether the mouse is hovering over the repo button.
func (s *Sidebar) SetRepoHovered(hovered bool) {
	s.repoHovered = hovered
}

// TopicStatus holds status flags for a topic's instances.
type TopicStatus struct {
	HasRunning      bool
	HasNotification bool
}

// SetItems updates the sidebar items from the current topics.
// sharedTopics maps topic name → whether it has a shared worktree.
// topicStatuses maps topic name → running/notification status.
func (s *Sidebar) SetItems(topicNames []string, instanceCountByTopic map[string]int, ungroupedCount int, sharedTopics map[string]bool, topicStatuses map[string]TopicStatus) {
	totalCount := ungroupedCount
	for _, c := range instanceCountByTopic {
		totalCount += c
	}

	// Aggregate statuses for "All"
	anyRunning := false
	anyNotification := false
	for _, st := range topicStatuses {
		if st.HasRunning {
			anyRunning = true
		}
		if st.HasNotification {
			anyNotification = true
		}
	}

	items := []SidebarItem{
		{Name: "All", ID: SidebarAll, Count: totalCount, HasRunning: anyRunning, HasNotification: anyNotification},
	}

	if len(s.plans) > 0 {
		items = append(items, SidebarItem{Name: "Plans", IsSection: true})
		for _, p := range s.plans {
			items = append(items, SidebarItem{
				Name:            planstate.DisplayName(p.Filename),
				ID:              SidebarPlanPrefix + p.Filename,
				HasRunning:      p.Status == string(planstate.StatusInProgress),
				HasNotification: p.Status == string(planstate.StatusReviewing),
			})
		}
	}

	if len(topicNames) > 0 {
		items = append(items, SidebarItem{Name: "Topics", IsSection: true})
		for _, name := range topicNames {
			count := instanceCountByTopic[name]
			st := topicStatuses[name]
			items = append(items, SidebarItem{
				Name: name, ID: name, Count: count,
				SharedWorktree: sharedTopics[name],
				HasRunning:     st.HasRunning, HasNotification: st.HasNotification,
			})
		}
	}

	if ungroupedCount > 0 {
		ungroupedSt := topicStatuses[""]
		items = append(items, SidebarItem{Name: "Ungrouped", IsSection: true})
		items = append(items, SidebarItem{
			Name: "Ungrouped", ID: SidebarUngrouped, Count: ungroupedCount,
			HasRunning: ungroupedSt.HasRunning, HasNotification: ungroupedSt.HasNotification,
		})
	}

	s.items = items
	if s.selectedIdx >= len(items) {
		s.selectedIdx = len(items) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
}

// GetSelectedIdx returns the index of the currently selected item in the sidebar.
func (s *Sidebar) GetSelectedIdx() int {
	return s.selectedIdx
}

func (s *Sidebar) GetSelectedID() string {
	if len(s.items) == 0 {
		return SidebarAll
	}
	return s.items[s.selectedIdx].ID
}

// GetSelectedPlanFile returns the plan filename if a plan item is selected, or "".
func (s *Sidebar) GetSelectedPlanFile() string {
	id := s.GetSelectedID()
	if strings.HasPrefix(id, SidebarPlanPrefix) {
		return id[len(SidebarPlanPrefix):]
	}
	return ""
}

func (s *Sidebar) Up() {
	for i := s.selectedIdx - 1; i >= 0; i-- {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			return
		}
	}
}

func (s *Sidebar) Down() {
	for i := s.selectedIdx + 1; i < len(s.items); i++ {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			return
		}
	}
}

// ClickItem selects a sidebar item by its rendered row offset (0-indexed from the first item).
// Section headers count as a row but are skipped for selection.
func (s *Sidebar) ClickItem(row int) {
	currentRow := 0
	for i, item := range s.items {
		if currentRow == row {
			if !item.IsSection {
				s.selectedIdx = i
			}
			return
		}
		currentRow++
	}
}

// UpdateMatchCounts sets the search match counts for each topic item.
// Pass nil to clear search highlighting.
func (s *Sidebar) UpdateMatchCounts(matchesByTopic map[string]int, totalMatches int) {
	for i := range s.items {
		if s.items[i].IsSection {
			continue
		}
		if matchesByTopic == nil {
			s.items[i].MatchCount = -1 // not searching
			continue
		}
		switch s.items[i].ID {
		case SidebarAll:
			s.items[i].MatchCount = totalMatches
		case SidebarUngrouped:
			s.items[i].MatchCount = matchesByTopic[""]
		default:
			s.items[i].MatchCount = matchesByTopic[s.items[i].ID]
		}
	}
}

// SelectFirst selects the first non-section item (typically "All").
func (s *Sidebar) SelectFirst() {
	for i, item := range s.items {
		if !item.IsSection {
			s.selectedIdx = i
			return
		}
	}
}

func (s *Sidebar) ActivateSearch()         { s.searchActive = true; s.searchQuery = "" }
func (s *Sidebar) DeactivateSearch()       { s.searchActive = false; s.searchQuery = "" }
func (s *Sidebar) IsSearchActive() bool    { return s.searchActive }
func (s *Sidebar) GetSearchQuery() string  { return s.searchQuery }
func (s *Sidebar) SetSearchQuery(q string) { s.searchQuery = q }

// GetSelectedTopicName returns the topic name if a topic row is selected, or "".
// Full implementation in Task 3 (three-level sidebar tree).
func (s *Sidebar) GetSelectedTopicName() string {
	return "" // stub — implemented in Task 3
}

// IsSelectedTopicHeader returns true if a topic header row is selected.
// Full implementation in Task 3.
func (s *Sidebar) IsSelectedTopicHeader() bool {
	return false // stub — implemented in Task 3
}

// IsSelectedPlanHeader returns true if a plan header row is selected.
// Full implementation in Task 3.
func (s *Sidebar) IsSelectedPlanHeader() bool {
	return false // stub — implemented in Task 3
}

// ToggleSelectedExpand toggles expand/collapse of a topic or plan row.
// Full implementation in Task 3.
func (s *Sidebar) ToggleSelectedExpand() bool {
	return false // stub — implemented in Task 3
}

// GetSelectedPlanStage returns the plan file and stage if a stage row is selected.
// Full implementation in Task 3.
func (s *Sidebar) GetSelectedPlanStage() (planFile, stage string, ok bool) {
	return "", "", false // stub — implemented in Task 3
}

func (s *Sidebar) String() string {
	borderStyle := sidebarBorderStyle
	if s.focused {
		borderStyle = borderStyle.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	} else {
		borderStyle = borderStyle.BorderForeground(ColorOverlay)
	}

	// Inner width accounts for border (2) + border padding (2)
	innerWidth := s.width - 6
	if innerWidth < 8 {
		innerWidth = 8
	}

	var b strings.Builder

	// Search bar
	searchWidth := innerWidth - 4 // search bar has its own border+padding
	if searchWidth < 4 {
		searchWidth = 4
	}
	if s.searchActive {
		searchText := s.searchQuery
		if searchText == "" {
			searchText = " "
		}
		b.WriteString(searchActiveBarStyle.Width(searchWidth).Render(searchText))
	} else {
		b.WriteString(searchBarStyle.Width(searchWidth).Render("\uf002 search"))
	}
	b.WriteString("\n\n")

	// Items
	itemWidth := innerWidth - 2 // item padding
	if itemWidth < 4 {
		itemWidth = 4
	}
	for i, item := range s.items {
		// During search, hide section headers and topics with 0 matches
		if s.searchActive && s.searchQuery != "" {
			if item.IsSection {
				continue // hide section headers during search
			}
			if item.ID != SidebarAll && item.MatchCount == 0 {
				continue // hide topics with no matches
			}
		}

		if item.IsSection {
			b.WriteString(sectionHeaderStyle.Render("── " + item.Name + " ──"))
			b.WriteString("\n")
			continue
		}

		// Fixed-slot layout: [prefix 1ch] [name+count flexible] [icons fixed right]
		// Content area = itemWidth - 2 (Padding(0,1) in item styles)
		contentWidth := itemWidth - 2

		// Build trailing icons and measure their fixed width
		trailingWidth := 0
		if item.SharedWorktree {
			trailingWidth += 2 // " \ue727"
		}
		if item.HasNotification || item.HasRunning {
			trailingWidth += 2 // " ●"
		}

		// Build count suffix
		displayCount := item.Count
		if s.searchActive && item.MatchCount >= 0 {
			displayCount = item.MatchCount
		}
		countSuffix := ""
		if displayCount > 0 {
			countSuffix = fmt.Sprintf(" (%d)", displayCount)
		}

		// Truncate name to fit: contentWidth - prefix(1) - countSuffix - trailing
		nameText := item.Name
		maxNameWidth := contentWidth - 1 - runewidth.StringWidth(countSuffix) - trailingWidth
		if maxNameWidth < 3 {
			maxNameWidth = 3
		}
		if runewidth.StringWidth(nameText) > maxNameWidth {
			nameText = runewidth.Truncate(nameText, maxNameWidth-1, "…")
		}

		// Left part: prefix + name + count
		// Plan items use a status glyph as prefix (○/●/◉); selected items show ▸.
		// Selection takes priority over the status glyph.
		isPlan := strings.HasPrefix(item.ID, SidebarPlanPrefix)
		// Build prefix glyph. For plan items, use a colored status glyph;
		// for selected items, use ▸; otherwise a space.
		// The glyph is always 1 cell wide.
		prefixGlyph := " "
		var prefixStyle lipgloss.Style
		hasPrefixStyle := false
		if i == s.selectedIdx {
			prefixGlyph = "▸"
		} else if isPlan {
			hasPrefixStyle = true
			switch {
			case item.HasNotification:
				prefixGlyph = "◉"
				prefixStyle = sidebarNotifyStyle
			case item.HasRunning:
				prefixGlyph = "●"
				prefixStyle = sidebarRunningStyle
			default:
				prefixGlyph = "○"
				prefixStyle = sectionHeaderStyle
			}
		}
		// Build the text portion (name + count) without the prefix so the
		// outer item style applies a consistent background across it.
		textPart := nameText + countSuffix
		leftWidth := 1 + runewidth.StringWidth(textPart) // 1 for prefix glyph

		// Pad between left and right to push icons to the right edge
		gap := contentWidth - leftWidth - trailingWidth
		if gap < 0 {
			gap = 0
		}
		// Render prefix with its own style (colored glyph) then append
		// the rest as plain text so the outer item style's background
		// covers everything uniformly.
		var styledPrefix string
		if hasPrefixStyle {
			styledPrefix = prefixStyle.Render(prefixGlyph)
		} else {
			styledPrefix = prefixGlyph
		}
		paddedLeft := styledPrefix + textPart + strings.Repeat(" ", gap)

		// Style the trailing icons. Plan items use a prefix glyph instead of a
		// trailing dot, so skip the trailing icon for them.
		var styledTrailing string
		if item.SharedWorktree {
			styledTrailing += " \ue727"
		}
		if !isPlan {
			if item.HasNotification {
				if time.Now().UnixMilli()/500%2 == 0 {
					styledTrailing += " " + sidebarReadyStyle.Render("●")
				} else {
					styledTrailing += " " + sidebarNotifyStyle.Render("●")
				}
			} else if item.HasRunning {
				styledTrailing += " " + sidebarRunningStyle.Render("●")
			}
		}

		line := paddedLeft + styledTrailing
		if i == s.selectedIdx && s.focused {
			b.WriteString(selectedTopicStyle.Width(itemWidth).Render(line))
		} else if i == s.selectedIdx && !s.focused {
			b.WriteString(activeTopicStyle.Width(itemWidth).Render(line))
		} else {
			b.WriteString(topicItemStyle.Width(itemWidth).Render(line))
		}
		b.WriteString("\n")
	}

	// Build repo indicator as a clickable dropdown button at the bottom.
	// lipgloss .Width(w) includes padding but EXCLUDES borders.
	// So total rendered = w + 2(border). To fit in sidebar content area
	// (innerWidth - 2), we need: w + 2 <= innerWidth - 2 => w <= innerWidth - 4.
	var repoSection string
	if s.repoName != "" {
		btnWidth := innerWidth - 4 // same as search bar: border excluded from Width
		if btnWidth < 4 {
			btnWidth = 4
		}

		borderColor := lipgloss.TerminalColor(lipgloss.AdaptiveColor{Light: string(ColorSubtle), Dark: string(ColorOverlay)})
		textColor := lipgloss.TerminalColor(lipgloss.AdaptiveColor{Light: string(ColorMuted), Dark: string(ColorSubtle)})
		if s.repoHovered {
			borderColor = lipgloss.AdaptiveColor{Light: string(ColorMuted), Dark: string(ColorSubtle)}
			textColor = lipgloss.AdaptiveColor{Light: string(ColorBase), Dark: string(ColorText)}
		}

		// Truncate repo name to fit: btnWidth - padding(2) - arrow
		arrowStr := " ▾"
		contentWidth := btnWidth - 2 // subtract padding
		maxNameLen := contentWidth - runewidth.StringWidth(arrowStr)
		displayName := s.repoName
		if runewidth.StringWidth(displayName) > maxNameLen {
			displayName = runewidth.Truncate(displayName, maxNameLen-1, "…")
		}

		btnStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Foreground(textColor).
			Width(btnWidth).
			Padding(0, 1)

		repoButton := btnStyle.Render(displayName + arrowStr)
		repoSection = zone.Mark(ZoneRepoSwitch, repoButton)
	}

	topContent := b.String()

	// Wrap content in the subtle rounded border — use full available height
	borderHeight := s.height - 2 // account for top border + bottom border
	if borderHeight < 4 {
		borderHeight = 4
	}

	innerContent := topContent
	if repoSection != "" {
		topLines := strings.Count(topContent, "\n") + 1
		repoLines := strings.Count(repoSection, "\n") + 1
		// +1 because topContent's trailing \n merges with the gap,
		// producing 1 fewer line than topLines + gap + repoLines.
		gap := borderHeight - topLines - repoLines + 1
		if gap < 1 {
			gap = 1
		}
		innerContent = topContent + strings.Repeat("\n", gap) + repoSection
	}

	bordered := borderStyle.Width(innerWidth).Height(borderHeight).Render(innerContent)
	return lipgloss.Place(s.width, s.height, lipgloss.Left, lipgloss.Top, bordered)
}
