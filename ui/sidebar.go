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
	SidebarAll               = "__all__"
	SidebarUngrouped         = "__ungrouped__"
	SidebarPlanPrefix        = "__plan__"
	SidebarTopicPrefix       = "__topic__"
	SidebarPlanStagePrefix   = "__plan_stage__"
	SidebarPlanHistoryToggle = "__plan_history_toggle__"
)

// PlanDisplay holds plan info for sidebar rendering.
type PlanDisplay struct {
	Filename    string
	Status      string
	Description string
	Branch      string
	Topic       string
}

// TopicDisplay holds a topic and its plans for sidebar rendering.
type TopicDisplay struct {
	Name  string
	Plans []PlanDisplay
}

// sidebarRowKind identifies the type of a sidebar row.
type sidebarRowKind int

const (
	rowKindItem          sidebarRowKind = iota // regular item (All, Ungrouped)
	rowKindSection                             // section header
	rowKindTopic                               // topic header
	rowKindPlan                                // plan header
	rowKindStage                               // plan lifecycle stage
	rowKindHistoryToggle                       // "History" toggle row

)

// sidebarRow is a single rendered row in the sidebar.
type sidebarRow struct {
	Kind      sidebarRowKind
	ID        string
	Label     string
	PlanFile  string
	Stage     string
	Collapsed bool
	// display flags
	HasRunning      bool
	HasNotification bool
	Count           int
	Done            bool // stage is completed
	Active          bool // stage is currently active
	Locked          bool // stage is not yet reachable
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

var sidebarCancelledStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Strikethrough(true)

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
	IsCancelled     bool // true if this plan was cancelled (render with strikethrough)
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

	// Three-level tree state (Task 3)
	rows           []sidebarRow
	expandedTopics map[string]bool
	expandedPlans  map[string]bool
	// stored data for rebuild
	treeTopics    []TopicDisplay
	treeUngrouped []PlanDisplay
	treeHistory   []PlanDisplay

	useTreeMode  bool // true when SetTopicsAndPlans has been called
	planStatuses map[string]TopicStatus
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
		selectedIdx:    0,
		focused:        true,
		expandedTopics: make(map[string]bool),
		expandedPlans:  make(map[string]bool),
		planStatuses:   make(map[string]TopicStatus),
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
func (s *Sidebar) SetItems(
	topicNames []string,
	instanceCountByTopic map[string]int,
	ungroupedCount int,
	sharedTopics map[string]bool,
	topicStatuses map[string]TopicStatus,
	planStatuses map[string]TopicStatus,
) {
	s.planStatuses = planStatuses
	if s.useTreeMode {
		s.rebuildRows()
	}

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
			isCancelled := p.Status == string(planstate.StatusCancelled)
			planSt := planStatuses[p.Filename]
			items = append(items, SidebarItem{
				Name:            planstate.DisplayName(p.Filename),
				ID:              SidebarPlanPrefix + p.Filename,
				HasRunning:      !isCancelled && (planSt.HasRunning || p.Status == string(planstate.StatusInProgress)),
				HasNotification: !isCancelled && (planSt.HasNotification || p.Status == string(planstate.StatusReviewing)),
				IsCancelled:     isCancelled,
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

	// Preserve the currently selected ID across rebuilds so the periodic
	// metadata tick doesn't reset the user's navigation position.
	prevID := ""
	if s.selectedIdx >= 0 && s.selectedIdx < len(s.items) {
		prevID = s.items[s.selectedIdx].ID
	}

	s.items = items

	// Try to restore selection by ID.
	if prevID != "" {
		for i, item := range items {
			if item.ID == prevID {
				s.selectedIdx = i
				return
			}
		}
	}

	// Fallback: clamp index if the previous ID no longer exists.
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
	if s.useTreeMode {
		if s.selectedIdx < len(s.rows) {
			return s.rows[s.selectedIdx].ID
		}
		return ""
	}
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
	if s.useTreeMode {
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
		return
	}
	for i := s.selectedIdx - 1; i >= 0; i-- {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			return
		}
	}
}

func (s *Sidebar) Down() {
	if s.useTreeMode {
		if s.selectedIdx < len(s.rows)-1 {
			s.selectedIdx++
		}
		return
	}
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
	if s.useTreeMode {
		if row >= 0 && row < len(s.rows) {
			s.selectedIdx = row
		}
		return
	}
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

// DisableTreeMode forces flat-mode navigation (using s.items) even if tree
// data has been loaded. Use this until String() is updated to render s.rows.
func (s *Sidebar) DisableTreeMode() {
	s.useTreeMode = false
}

// SetTopicsAndPlans sets the three-level tree data and rebuilds rows.
func (s *Sidebar) SetTopicsAndPlans(topics []TopicDisplay, ungrouped []PlanDisplay, history []PlanDisplay) {
	s.treeTopics = topics
	s.treeUngrouped = ungrouped
	s.treeHistory = history
	s.useTreeMode = true
	s.rebuildRows()
}

// effectivePlanStatus overlays live runtime status onto plan-state status for display.
func (s *Sidebar) effectivePlanStatus(p PlanDisplay) string {
	if p.Status == string(planstate.StatusCancelled) {
		return p.Status
	}
	st := s.planStatuses[p.Filename]
	if st.HasNotification {
		return string(planstate.StatusReviewing)
	}
	if st.HasRunning {
		return string(planstate.StatusInProgress)
	}
	return p.Status
}

// rebuildRows rebuilds the flat row list from the tree structure.
func (s *Sidebar) rebuildRows() {
	rows := []sidebarRow{}

	// Ungrouped plans (shown at top level, always visible)
	for _, p := range s.treeUngrouped {
		effective := p
		effective.Status = s.effectivePlanStatus(p)
		rows = append(rows, sidebarRow{
			Kind:            rowKindPlan,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           planstate.DisplayName(p.Filename),
			PlanFile:        p.Filename,
			Collapsed:       !s.expandedPlans[p.Filename],
			HasRunning:      effective.Status == string(planstate.StatusInProgress),
			HasNotification: effective.Status == string(planstate.StatusReviewing),
		})
		// If expanded, add stage rows
		if s.expandedPlans[p.Filename] {
			rows = append(rows, planStageRows(effective)...)
		}
	}

	// Topic headers (collapsed by default)
	for _, t := range s.treeTopics {
		rows = append(rows, sidebarRow{
			Kind:      rowKindTopic,
			ID:        SidebarTopicPrefix + t.Name,
			Label:     t.Name,
			Collapsed: !s.expandedTopics[t.Name],
		})
		// If expanded, add plan rows under topic
		if s.expandedTopics[t.Name] {
			for _, p := range t.Plans {
				effective := p
				effective.Status = s.effectivePlanStatus(p)
				rows = append(rows, sidebarRow{
					Kind:            rowKindPlan,
					ID:              SidebarPlanPrefix + p.Filename,
					Label:           planstate.DisplayName(p.Filename),
					PlanFile:        p.Filename,
					Collapsed:       !s.expandedPlans[p.Filename],
					HasRunning:      effective.Status == string(planstate.StatusInProgress),
					HasNotification: effective.Status == string(planstate.StatusReviewing),
				})
				if s.expandedPlans[p.Filename] {
					rows = append(rows, planStageRows(effective)...)
				}
			}
		}
	}

	// History toggle (if there are finished plans)
	if len(s.treeHistory) > 0 {
		rows = append(rows, sidebarRow{
			Kind:  rowKindHistoryToggle,
			ID:    SidebarPlanHistoryToggle,
			Label: "History",
		})
	}

	s.rows = rows
}

// planStageRows returns the four lifecycle stage rows for a plan.
func planStageRows(p PlanDisplay) []sidebarRow {
	stages := []struct{ name, label string }{
		{"plan", "Plan"},
		{"implement", "Implement"},
		{"review", "Review"},
		{"finished", "Finished"},
	}
	rows := make([]sidebarRow, 0, 4)
	for _, st := range stages {
		done, active, locked := stageState(p.Status, st.name)
		rows = append(rows, sidebarRow{
			Kind:     rowKindStage,
			ID:       SidebarPlanStagePrefix + p.Filename + "::" + st.name,
			Label:    st.label,
			PlanFile: p.Filename,
			Stage:    st.name,
			Done:     done,
			Active:   active,
			Locked:   locked,
		})
	}
	return rows
}

// stageState returns (done, active, locked) for a stage given the plan status.
func stageState(status, stage string) (done, active, locked bool) {
	switch stage {
	case "plan":
		done = status == "in_progress" || status == "reviewing" || status == "done" || status == "completed"
		active = status == "ready"
		locked = false
	case "implement":
		done = status == "reviewing" || status == "done" || status == "completed"
		active = status == "in_progress"
		locked = status == "ready"
	case "review":
		done = status == "done" || status == "completed"
		active = status == "reviewing"
		locked = status == "ready" || status == "in_progress"
	case "finished":
		done = status == "done" || status == "completed"
		active = false
		locked = !(status == "reviewing" || status == "done" || status == "completed")
	}
	return
}

// ToggleSelectedExpand toggles expand/collapse of the selected topic or plan row.
// Returns true if the toggle was handled (row was a topic or plan header).
func (s *Sidebar) ToggleSelectedExpand() bool {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return false
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic:
		topicName := row.ID[len(SidebarTopicPrefix):]
		s.expandedTopics[topicName] = !s.expandedTopics[topicName]
		s.rebuildRows()
		return true
	case rowKindPlan:
		s.expandedPlans[row.PlanFile] = !s.expandedPlans[row.PlanFile]
		s.rebuildRows()
		return true
	}
	return false
}

// GetSelectedPlanStage returns the plan file and stage if a stage row is selected.
func (s *Sidebar) GetSelectedPlanStage() (planFile, stage string, ok bool) {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return "", "", false
	}
	row := s.rows[s.selectedIdx]
	if row.Kind == rowKindStage {
		return row.PlanFile, row.Stage, true
	}
	return "", "", false
}

// GetSelectedTopicName returns the topic name if a topic row is selected, or "".
func (s *Sidebar) GetSelectedTopicName() string {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return ""
	}
	row := s.rows[s.selectedIdx]
	if row.Kind == rowKindTopic {
		return row.ID[len(SidebarTopicPrefix):]
	}
	return ""
}

// IsSelectedTopicHeader returns true if a topic header row is selected.
func (s *Sidebar) IsSelectedTopicHeader() bool {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return false
	}
	return s.rows[s.selectedIdx].Kind == rowKindTopic
}

// IsSelectedPlanHeader returns true if a plan header row is selected.
func (s *Sidebar) IsSelectedPlanHeader() bool {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return false
	}
	return s.rows[s.selectedIdx].Kind == rowKindPlan
}

// HasRowID returns true if any row has the given ID. Used in tests.
func (s *Sidebar) HasRowID(id string) bool {
	for _, row := range s.rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

// SelectByID selects the row with the given ID. Returns true if found.
func (s *Sidebar) SelectByID(id string) bool {
	for i, row := range s.rows {
		if row.ID == id {
			s.selectedIdx = i
			return true
		}
	}
	return false
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

		// Content area = itemWidth - 2 (Padding(0,1) in item styles)
		contentWidth := itemWidth - 2
		isPlan := strings.HasPrefix(item.ID, SidebarPlanPrefix)

		var line string
		if isPlan {
			// ── Plan items: bubbles-style ──
			// Layout: [indent 1ch][cursor 1ch][name...][gap...][status 2ch]
			// Plans are indented under their section header with status
			// shown as a trailing glyph (right-aligned, always visible).
			cursor := " "
			if i == s.selectedIdx {
				cursor = "▸"
			}

			// Trailing status glyph — always visible, colored by state
			var statusGlyph string
			var statusStyle lipgloss.Style
			switch {
			case item.IsCancelled:
				statusGlyph = "✕"
				statusStyle = sidebarCancelledStyle
			case item.HasNotification: // reviewing
				statusGlyph = "◉"
				statusStyle = sidebarNotifyStyle
			case item.HasRunning: // in_progress
				statusGlyph = "●"
				statusStyle = sidebarRunningStyle
			default: // ready
				statusGlyph = "○"
				statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
			}

			const planIndent = 1  // extra indent under section header
			const cursorWidth = 1 // ▸ or space
			const trailWidth = 2  // " " + glyph

			// Truncate name to fit
			nameText := item.Name
			maxName := contentWidth - planIndent - cursorWidth - trailWidth
			if maxName < 3 {
				maxName = 3
			}
			if runewidth.StringWidth(nameText) > maxName {
				nameText = runewidth.Truncate(nameText, maxName-1, "…")
			}

			textPart := nameText
			if item.IsCancelled {
				textPart = sidebarCancelledStyle.Render(textPart)
			}

			usedWidth := planIndent + cursorWidth + runewidth.StringWidth(textPart) + trailWidth
			gap := contentWidth - usedWidth
			if gap < 0 {
				gap = 0
			}

			line = " " + cursor + textPart + strings.Repeat(" ", gap) + " " + statusStyle.Render(statusGlyph)
		} else {
			// ── Non-plan items: All, topics, ungrouped ──
			// Layout: [cursor 1ch][name+count...][gap...][trailing icons]

			// Build trailing icons
			trailingWidth := 0
			if item.SharedWorktree {
				trailingWidth += 2 // " \ue727"
			}
			if item.HasNotification || item.HasRunning {
				trailingWidth += 2 // " ●"
			}

			// Count suffix
			displayCount := item.Count
			if s.searchActive && item.MatchCount >= 0 {
				displayCount = item.MatchCount
			}
			countSuffix := ""
			if displayCount > 0 {
				countSuffix = fmt.Sprintf(" (%d)", displayCount)
			}

			// Truncate name
			nameText := item.Name
			maxName := contentWidth - 1 - runewidth.StringWidth(countSuffix) - trailingWidth
			if maxName < 3 {
				maxName = 3
			}
			if runewidth.StringWidth(nameText) > maxName {
				nameText = runewidth.Truncate(nameText, maxName-1, "…")
			}

			cursor := " "
			if i == s.selectedIdx {
				cursor = "▸"
			}

			textPart := nameText + countSuffix
			leftWidth := 1 + runewidth.StringWidth(textPart)
			gap := contentWidth - leftWidth - trailingWidth
			if gap < 0 {
				gap = 0
			}

			// Trailing icons
			var styledTrailing string
			if item.SharedWorktree {
				styledTrailing += " \ue727"
			}
			if item.HasNotification {
				if time.Now().UnixMilli()/500%2 == 0 {
					styledTrailing += " " + sidebarReadyStyle.Render("●")
				} else {
					styledTrailing += " " + sidebarNotifyStyle.Render("●")
				}
			} else if item.HasRunning {
				styledTrailing += " " + sidebarRunningStyle.Render("●")
			}

			line = cursor + textPart + strings.Repeat(" ", gap) + styledTrailing
		}

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
