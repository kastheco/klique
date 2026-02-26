package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/kastheco/kasmos/config/planstate"
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
	SidebarImportClickUp     = "__import_clickup__"
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
	rowKindCancelled                           // cancelled plan (strikethrough)
	rowKindImportAction                        // "+ import from clickup" action row
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
	Indent          int  // visual indent in characters (0, 2, or 4)
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

// Tree-mode styles

// stageCheckStyle is for completed stage indicators (✓).
var stageCheckStyle = lipgloss.NewStyle().Foreground(ColorFoam)

// stageActiveStyle is for the currently active stage indicator (▸).
var stageActiveStyle = lipgloss.NewStyle().Foreground(ColorIris)

// stageLockedStyle is for locked/unreachable stage indicators (○).
var stageLockedStyle = lipgloss.NewStyle().Foreground(ColorMuted)

// topicLabelStyle is for topic header labels in tree mode.
var topicLabelStyle = lipgloss.NewStyle().Foreground(ColorText).Bold(true)

// historyToggleStyle is for the history section divider.
var historyToggleStyle = lipgloss.NewStyle().Foreground(ColorMuted)

// importActionStyle is for the clickable import row.
var importActionStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Padding(0, 1)

var importActionSelectedStyle = lipgloss.NewStyle().
	Background(ColorFoam).
	Foreground(ColorBase).
	Padding(0, 1)

// Legend styles for the status indicator key at the bottom of the sidebar.
var legendLabelStyle = lipgloss.NewStyle().Foreground(ColorMuted)
var legendSepStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
var legendIdleStyle = lipgloss.NewStyle().Foreground(ColorMuted)

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
	items            []SidebarItem
	plans            []PlanDisplay
	selectedIdx      int
	height, width    int
	focused          bool
	clickUpAvailable bool // true when ClickUp MCP is detected

	searchActive bool
	searchQuery  string

	repoName    string // current repo name shown at bottom
	repoHovered bool   // true when mouse is hovering over the repo button

	// Three-level tree state (Task 3)
	rows            []sidebarRow
	collapsedTopics map[string]bool // tracks manually collapsed topics (default = expanded)
	expandedPlans   map[string]bool
	// stored data for rebuild
	treeTopics    []TopicDisplay
	treeUngrouped []PlanDisplay
	treeHistory   []PlanDisplay
	treeCancelled []PlanDisplay

	useTreeMode     bool // true when SetTopicsAndPlans has been called
	historyExpanded bool // whether the History section is expanded
	scrollOffset    int  // row index of first visible sidebar row
	planStatuses    map[string]TopicStatus
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
		selectedIdx:     0,
		focused:         true,
		collapsedTopics: make(map[string]bool),
		expandedPlans:   make(map[string]bool),
		planStatuses:    make(map[string]TopicStatus),
	}
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.clampSidebarScroll()
}

// availSidebarRows returns the number of tree-mode sidebar rows that fit in the panel.
// Header accounts for: search bar (3 lines with border) + 2 blank lines = 5 lines.
// borderAndPadding accounts for the outer border (2 lines: top + bottom) plus
// the empirical rendering overhead from the search bar lipgloss layout. Value 4
// produces correct results at all tested heights.
func (s *Sidebar) availSidebarRows() int {
	const borderAndPadding = 4
	const headerLines = 5
	avail := s.height - borderAndPadding - headerLines
	if avail < 1 {
		avail = 1
	}
	return avail
}

// clampSidebarScroll adjusts scrollOffset so selectedIdx stays within the visible window.
func (s *Sidebar) clampSidebarScroll() {
	if len(s.rows) == 0 {
		s.scrollOffset = 0
		return
	}
	avail := s.availSidebarRows()
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+avail {
		s.scrollOffset = s.selectedIdx - avail + 1
	}
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}
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
		// In tree mode, selectedIdx indexes into s.rows, not s.items.
		// The flat items list below is only used for non-tree rendering;
		// do NOT let its ID-restoration logic overwrite the tree selection.
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
				HasRunning:      !isCancelled && (planSt.HasRunning || p.Status == string(planstate.StatusImplementing)),
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

	// In tree mode, selectedIdx is an index into s.rows (managed by rebuildRows).
	// Only restore flat-mode selection when NOT in tree mode.
	if !s.useTreeMode {
		prevID := ""
		if s.selectedIdx >= 0 && s.selectedIdx < len(s.items) {
			prevID = s.items[s.selectedIdx].ID
		}

		s.items = items

		if prevID != "" {
			for i, item := range items {
				if item.ID == prevID {
					s.selectedIdx = i
					return
				}
			}
		}

		if s.selectedIdx >= len(items) {
			s.selectedIdx = len(items) - 1
		}
		if s.selectedIdx < 0 {
			s.selectedIdx = 0
		}
	} else {
		s.items = items
	}
}

// GetSelectedIdx returns the index of the currently selected item in the sidebar.
func (s *Sidebar) GetSelectedIdx() int {
	return s.selectedIdx
}

// GetScrollOffset returns the current scroll offset for the visible row window.
func (s *Sidebar) GetScrollOffset() int {
	return s.scrollOffset
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

// GetSelectedPlanFile returns the plan filename if a plan item or stage item is selected, or "".
func (s *Sidebar) GetSelectedPlanFile() string {
	id := s.GetSelectedID()
	if strings.HasPrefix(id, SidebarPlanPrefix) {
		return id[len(SidebarPlanPrefix):]
	}
	if strings.HasPrefix(id, SidebarPlanStagePrefix) {
		// Stage ID format: "__plan_stage__<planFile>::<stage>"
		rest := id[len(SidebarPlanStagePrefix):]
		if idx := strings.Index(rest, "::"); idx >= 0 {
			return rest[:idx]
		}
	}
	return ""
}

func (s *Sidebar) Up() {
	if s.useTreeMode {
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
		s.clampSidebarScroll()
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
		s.clampSidebarScroll()
		return
	}
	for i := s.selectedIdx + 1; i < len(s.items); i++ {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			return
		}
	}
}

// IsTreeMode returns true when the sidebar is rendering in tree mode.
func (s *Sidebar) IsTreeMode() bool {
	return s.useTreeMode
}

// Right expands the selected node if collapsed, or moves to its first child if already expanded.
// No-op on stage rows and in flat mode.
func (s *Sidebar) Right() {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic:
		if row.Collapsed {
			s.ToggleSelectedExpand()
		} else {
			if s.selectedIdx+1 < len(s.rows) {
				s.selectedIdx++
			}
		}
	case rowKindPlan:
		if row.Collapsed {
			s.ToggleSelectedExpand()
		} else {
			if s.selectedIdx+1 < len(s.rows) && s.rows[s.selectedIdx+1].Kind == rowKindStage {
				s.selectedIdx++
			}
		}
		// Stage, History, Cancelled: no-op
	}
}

// Left collapses the selected node if expanded, or moves to its parent.
// On ungrouped plans with no parent topic, behaves like Up().
func (s *Sidebar) Left() {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic:
		if !row.Collapsed {
			s.ToggleSelectedExpand()
		} else if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	case rowKindPlan:
		if !row.Collapsed {
			s.ToggleSelectedExpand()
			return
		}
		// Move to parent topic
		for i := s.selectedIdx - 1; i >= 0; i-- {
			if s.rows[i].Kind == rowKindTopic {
				s.selectedIdx = i
				return
			}
		}
		// No parent topic — move up
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	case rowKindStage:
		// Move to parent plan
		for i := s.selectedIdx - 1; i >= 0; i-- {
			if s.rows[i].Kind == rowKindPlan {
				s.selectedIdx = i
				return
			}
		}
	case rowKindHistoryToggle, rowKindCancelled, rowKindImportAction:
		if s.selectedIdx > 0 {
			s.selectedIdx--
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

// SetTopicsAndPlans sets the three-level tree data and rebuilds rows.
func (s *Sidebar) SetTopicsAndPlans(topics []TopicDisplay, ungrouped []PlanDisplay, history []PlanDisplay, cancelled ...[]PlanDisplay) {
	s.treeTopics = topics
	s.treeUngrouped = ungrouped
	s.treeHistory = history
	if len(cancelled) > 0 {
		s.treeCancelled = cancelled[0]
	}
	s.useTreeMode = true
	s.rebuildRows()
}

// SetClickUpAvailable controls whether the import action row is visible.
func (s *Sidebar) SetClickUpAvailable(available bool) {
	s.clickUpAvailable = available
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
		return string(planstate.StatusImplementing)
	}
	return p.Status
}

// isPlanActive returns true if the status represents an active lifecycle stage
// (running instances or an in-progress lifecycle stage like planning/implementing).
func isPlanActive(status string) bool {
	return status == string(planstate.StatusPlanning) ||
		status == string(planstate.StatusImplementing)
}

// miscTopic is the virtual topic name for ungrouped plans.
const miscTopic = "miscellaneous"

// rebuildRows rebuilds the flat row list from the tree structure.
func (s *Sidebar) rebuildRows() {
	// Preserve selection by row ID across rebuilds so periodic ticks
	// don't reset the user's navigation position.
	prevID := ""
	if s.selectedIdx >= 0 && s.selectedIdx < len(s.rows) {
		prevID = s.rows[s.selectedIdx].ID
	}

	rows := []sidebarRow{}

	// Real topics first
	for _, t := range s.treeTopics {
		// Aggregate status from child plans
		hasRunning := false
		hasNotification := false
		for _, p := range t.Plans {
			eff := s.effectivePlanStatus(p)
			if isPlanActive(eff) {
				hasRunning = true
			}
			if eff == string(planstate.StatusReviewing) {
				hasNotification = true
			}
		}

		rows = append(rows, sidebarRow{
			Kind:            rowKindTopic,
			ID:              SidebarTopicPrefix + t.Name,
			Label:           t.Name,
			Collapsed:       s.collapsedTopics[t.Name],
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
			Indent:          0,
		})
		if !s.collapsedTopics[t.Name] {
			for _, p := range t.Plans {
				effective := p
				effective.Status = s.effectivePlanStatus(p)
				rows = append(rows, sidebarRow{
					Kind:            rowKindPlan,
					ID:              SidebarPlanPrefix + p.Filename,
					Label:           planstate.DisplayName(p.Filename),
					PlanFile:        p.Filename,
					Collapsed:       !s.expandedPlans[p.Filename],
					HasRunning:      isPlanActive(effective.Status),
					HasNotification: effective.Status == string(planstate.StatusReviewing),
					Indent:          2,
				})
				if s.expandedPlans[p.Filename] {
					rows = append(rows, planStageRows(effective, 3)...)
				}
			}
		}
	}

	// Virtual "Miscellaneous" topic for ungrouped plans (after real topics)
	if len(s.treeUngrouped) > 0 {
		hasRunning := false
		hasNotification := false
		for _, p := range s.treeUngrouped {
			eff := s.effectivePlanStatus(p)
			if isPlanActive(eff) {
				hasRunning = true
			}
			if eff == string(planstate.StatusReviewing) {
				hasNotification = true
			}
		}
		rows = append(rows, sidebarRow{
			Kind:            rowKindTopic,
			ID:              SidebarTopicPrefix + miscTopic,
			Label:           miscTopic,
			Collapsed:       s.collapsedTopics[miscTopic],
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
			Indent:          0,
		})
		if !s.collapsedTopics[miscTopic] {
			for _, p := range s.treeUngrouped {
				effective := p
				effective.Status = s.effectivePlanStatus(p)
				rows = append(rows, sidebarRow{
					Kind:            rowKindPlan,
					ID:              SidebarPlanPrefix + p.Filename,
					Label:           planstate.DisplayName(p.Filename),
					PlanFile:        p.Filename,
					Collapsed:       !s.expandedPlans[p.Filename],
					HasRunning:      isPlanActive(effective.Status),
					HasNotification: effective.Status == string(planstate.StatusReviewing),
					Indent:          2,
				})
				if s.expandedPlans[p.Filename] {
					rows = append(rows, planStageRows(effective, 3)...)
				}
			}
		}
	}

	// Import action (only when ClickUp MCP is detected)
	if s.clickUpAvailable {
		rows = append(rows, sidebarRow{
			Kind:   rowKindImportAction,
			ID:     SidebarImportClickUp,
			Label:  "+ import from clickup",
			Indent: 0,
		})
	}

	// History toggle (if there are finished plans)
	if len(s.treeHistory) > 0 {
		rows = append(rows, sidebarRow{
			Kind:      rowKindHistoryToggle,
			ID:        SidebarPlanHistoryToggle,
			Label:     "History",
			Collapsed: !s.historyExpanded,
			Indent:    0,
		})
		if s.historyExpanded {
			for _, p := range s.treeHistory {
				rows = append(rows, sidebarRow{
					Kind:      rowKindPlan,
					ID:        SidebarPlanPrefix + p.Filename,
					Label:     planstate.DisplayName(p.Filename),
					PlanFile:  p.Filename,
					Collapsed: !s.expandedPlans[p.Filename],
					Done:      true,
					Indent:    2,
				})
				if s.expandedPlans[p.Filename] {
					rows = append(rows, planStageRows(p, 3)...)
				}
			}
		}
	}

	// Cancelled plans (shown at bottom with strikethrough)
	for _, p := range s.treeCancelled {
		rows = append(rows, sidebarRow{
			Kind:     rowKindCancelled,
			ID:       SidebarPlanPrefix + p.Filename,
			Label:    planstate.DisplayName(p.Filename),
			PlanFile: p.Filename,
			Indent:   0,
		})
	}

	s.rows = rows
	s.scrollOffset = 0

	// Restore selection by ID if possible.
	if prevID != "" {
		for i, row := range rows {
			if row.ID == prevID {
				s.selectedIdx = i
				s.clampSidebarScroll()
				return
			}
		}
	}

	// Fallback: clamp selectedIdx
	if s.selectedIdx >= len(rows) {
		s.selectedIdx = len(rows) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
	s.clampSidebarScroll()
}

// planStageRows returns the four lifecycle stage rows for a plan.
func planStageRows(p PlanDisplay, indent int) []sidebarRow {
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
			Indent:   indent,
		})
	}
	return rows
}

// stageState returns (done, active, locked) for a stage given the plan status.
func stageState(status, stage string) (done, active, locked bool) {
	implementing := status == "implementing"
	postPlan := implementing || status == "reviewing" || status == "done"
	postImpl := status == "reviewing" || status == "done"
	switch stage {
	case "plan":
		done = postPlan
		active = status == "ready" || status == "planning"
		locked = false
	case "implement":
		done = postImpl
		active = implementing
		locked = false // triggerPlanStage validates wave headers and reverts if needed
	case "review":
		done = status == "done"
		active = status == "reviewing"
		locked = status == "ready" || status == "planning" || implementing
	case "finished":
		done = status == "done"
		active = false
		locked = !postImpl
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
		s.collapsedTopics[topicName] = !s.collapsedTopics[topicName]
		s.rebuildRows()
		return true
	case rowKindPlan:
		s.expandedPlans[row.PlanFile] = !s.expandedPlans[row.PlanFile]
		s.rebuildRows()
		return true
	case rowKindHistoryToggle:
		s.historyExpanded = !s.historyExpanded
		s.rebuildRows()
		return true
	}
	return false
}

// SelectedSpaceAction returns the context-sensitive action label for the space
// key in tree mode: "expand", "collapse", or fallback "toggle".
func (s *Sidebar) SelectedSpaceAction() string {
	if !s.useTreeMode || s.selectedIdx < 0 || s.selectedIdx >= len(s.rows) {
		return "toggle"
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic, rowKindPlan, rowKindHistoryToggle:
		if row.Collapsed {
			return "expand"
		}
		return "collapse"
	default:
		return "toggle"
	}
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

// renderTreeRows writes the tree-mode sidebar content into b.
// Each row is rendered according to its Kind via per-kind render functions.
// Only the visible window [scrollOffset, scrollOffset+avail) is rendered.
func (s *Sidebar) renderTreeRows(b *strings.Builder, itemWidth int) {
	contentWidth := itemWidth - 2 // account for Padding(0,1) in item styles

	avail := s.availSidebarRows()
	startRow := s.scrollOffset
	endRow := startRow + avail
	if endRow > len(s.rows) {
		endRow = len(s.rows)
	}
	if startRow > len(s.rows) {
		startRow = len(s.rows)
	}

	for relIdx, row := range s.rows[startRow:endRow] {
		absIdx := relIdx + startRow
		var line string
		rawLine := false

		switch row.Kind {
		case rowKindTopic:
			line = s.renderTopicRow(row, absIdx, contentWidth)
		case rowKindPlan:
			line = s.renderPlanRow(row, absIdx, contentWidth)
		case rowKindStage:
			line = s.renderStageRow(row)
		case rowKindHistoryToggle:
			line = s.renderHistoryToggleRow(contentWidth)
		case rowKindCancelled:
			line = s.renderCancelledRow(row, absIdx, contentWidth)
		case rowKindImportAction:
			line = s.renderImportRow(row, absIdx, contentWidth)
			rawLine = true
		}

		if rawLine {
			b.WriteString(line)
			b.WriteString("\n")
			continue
		}

		// Apply selection styling — strip inner ANSI so the selection
		// background covers the full row without gaps.
		if absIdx == s.selectedIdx && s.focused {
			b.WriteString(selectedTopicStyle.Width(itemWidth).Render(ansi.Strip(line)))
		} else if absIdx == s.selectedIdx && !s.focused {
			b.WriteString(activeTopicStyle.Width(itemWidth).Render(ansi.Strip(line)))
		} else {
			b.WriteString(topicItemStyle.Width(itemWidth).Render(line))
		}
		b.WriteString("\n")
	}
}

// renderTopicRow renders a topic header row.
// Layout: [cursor 1][chevron 1][space 1][label...][gap...][status 2]
func (s *Sidebar) renderTopicRow(row sidebarRow, idx, contentWidth int) string {
	chevron := "▸"
	if !row.Collapsed {
		chevron = "▾"
	}

	var statusGlyph string
	var statusStyle lipgloss.Style
	if row.HasNotification {
		statusGlyph = "◉"
		statusStyle = sidebarNotifyStyle
	} else if row.HasRunning {
		statusGlyph = "●"
		statusStyle = sidebarRunningStyle
	}

	label := row.Label
	const fixedW = 2 // chevron(1) + space(1)
	trailW := 0
	if statusGlyph != "" {
		trailW = 2 // " ●"
	}
	maxLabel := contentWidth - fixedW - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	text := topicLabelStyle.Render(label)
	usedW := fixedW + runewidth.StringWidth(label) + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	trail := ""
	if statusGlyph != "" {
		trail = " " + statusStyle.Render(statusGlyph)
	}
	return chevron + " " + text + strings.Repeat(" ", gap) + trail
}

// renderPlanRow renders a plan row.
// Layout: [indent][cursor 1][label...][gap...][chevron 1][space 1][status 1]
func (s *Sidebar) renderPlanRow(row sidebarRow, idx, contentWidth int) string {
	indent := strings.Repeat(" ", row.Indent)

	chevron := "▸"
	if !row.Collapsed {
		chevron = "▾"
	}

	cursor := " "
	if idx == s.selectedIdx {
		cursor = "▸"
	}

	var statusGlyph string
	var statusStyle lipgloss.Style
	switch {
	case row.HasNotification:
		statusGlyph = "◉"
		statusStyle = sidebarNotifyStyle
	case row.HasRunning:
		statusGlyph = "●"
		statusStyle = sidebarRunningStyle
	default:
		statusGlyph = "○"
		statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}

	label := row.Label
	indentW := row.Indent
	const cursorW = 1
	const chevronW = 2 // chevron + space
	const trailW = 1   // status glyph
	maxLabel := contentWidth - indentW - cursorW - chevronW - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	usedW := indentW + cursorW + runewidth.StringWidth(label) + chevronW + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	return indent + cursor + label + strings.Repeat(" ", gap) + chevron + " " + statusStyle.Render(statusGlyph)
}

// renderStageRow renders a plan lifecycle stage row.
// Layout: [indent][indicator 1][space 1][label]
func (s *Sidebar) renderStageRow(row sidebarRow) string {
	indent := strings.Repeat(" ", row.Indent)

	var indicator string
	var indStyle lipgloss.Style
	switch {
	case row.Done:
		indicator = "✓"
		indStyle = stageCheckStyle
	case row.Active:
		indicator = "▸"
		indStyle = stageActiveStyle
	default:
		indicator = "○"
		indStyle = stageLockedStyle
	}

	return indent + indStyle.Render(indicator) + " " + row.Label
}

// renderHistoryToggleRow renders the history section divider.
func (s *Sidebar) renderHistoryToggleRow(_ int) string {
	chevron := "▸"
	if s.historyExpanded {
		chevron = "▾"
	}
	return historyToggleStyle.Render("── " + chevron + " History ──")
}

func (s *Sidebar) renderImportRow(row sidebarRow, idx, width int) string {
	if idx == s.selectedIdx {
		return importActionSelectedStyle.Width(width).Render(row.Label)
	}
	return importActionStyle.Width(width).Render(row.Label)
}

// renderCancelledRow renders a cancelled plan with strikethrough.
// Layout: [cursor 1][label...][gap...][space 1][✕ 1]
func (s *Sidebar) renderCancelledRow(row sidebarRow, idx, contentWidth int) string {
	cursor := " "
	if idx == s.selectedIdx {
		cursor = "▸"
	}

	label := row.Label
	const trailW = 2 // " ✕"
	maxLabel := contentWidth - 1 - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	usedW := 1 + runewidth.StringWidth(label) + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	return cursor + sidebarCancelledStyle.Render(label) + strings.Repeat(" ", gap) + " " + sidebarCancelledStyle.Render("✕")
}

func (s *Sidebar) String() string {
	borderStyle := sidebarBorderStyle
	if s.focused {
		borderStyle = borderStyle.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	} else {
		borderStyle = borderStyle.BorderForeground(ColorOverlay)
	}

	// Inner width accounts for border (2) + border padding (2) = 4
	innerWidth := s.width - 4
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
	if s.useTreeMode {
		s.renderTreeRows(&b, itemWidth)
	} else {
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
	} // end else (flat mode)

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

	// Status legend pinned above the repo button at the bottom.
	legend := sidebarRunningStyle.Render("●") + legendLabelStyle.Render(" running") +
		legendSepStyle.Render("  ") +
		sidebarNotifyStyle.Render("◉") + legendLabelStyle.Render(" review") +
		legendSepStyle.Render("  ") +
		legendIdleStyle.Render("○") + legendLabelStyle.Render(" idle")

	// Wrap content in the subtle rounded border — use full available height
	borderHeight := s.height - 2 // account for top border + bottom border
	if borderHeight < 4 {
		borderHeight = 4
	}

	// Build bottom section: legend + optional repo button
	var bottomSection string
	bottomSection = legend
	if repoSection != "" {
		bottomSection = legend + "\n" + repoSection
	}

	topLines := strings.Count(topContent, "\n") + 1
	bottomLines := strings.Count(bottomSection, "\n") + 1
	gap := borderHeight - topLines - bottomLines + 1
	if gap < 1 {
		gap = 1
	}
	innerContent := topContent + strings.Repeat("\n", gap) + bottomSection

	bordered := borderStyle.Width(innerWidth).Height(borderHeight).Render(innerContent)
	placed := lipgloss.Place(s.width, s.height, lipgloss.Left, lipgloss.Top, bordered)

	// Clamp output to s.height lines so content never overflows the panel.
	if s.height > 0 {
		placedLines := strings.Split(placed, "\n")
		if len(placedLines) > s.height {
			placedLines = placedLines[:s.height]
			placed = strings.Join(placedLines, "\n")
		}
	}
	return placed
}
