package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/mattn/go-runewidth"
)

// Sidebar ID prefixes used for row IDs and zone marking.
const (
	SidebarPlanPrefix        = "__plan__"
	SidebarTopicPrefix       = "__topic__"
	SidebarPlanHistoryToggle = "__plan_history_toggle__"
	SidebarImportClickUp     = "__import_clickup__"
)

// PlanDisplay holds display metadata for a single plan entry in the sidebar.
type PlanDisplay struct {
	Filename    string
	Status      string
	Description string
	Branch      string
	Topic       string
}

// TopicStatus captures aggregate run/notification state for a plan.
type TopicStatus struct {
	HasRunning      bool
	HasNotification bool
}

// TopicDisplay groups plans under a named topic header.
type TopicDisplay struct {
	Name  string
	Plans []PlanDisplay
}

// navRowKind enumerates the distinct row types rendered in the nav panel.
type navRowKind int

const (
	navRowPlanHeader navRowKind = iota
	navRowInstance
	navRowSoloHeader
	navRowTopicHeader
	navRowImportAction
	navRowDeadToggle
	navRowDeadPlan
	navRowHistoryToggle
	navRowHistoryPlan
	navRowCancelled
)

// navRow holds the data for a single rendered row in the navigation panel.
type navRow struct {
	Kind            navRowKind
	ID              string
	Label           string
	TaskFile        string
	PlanStatus      string
	Instance        *session.Instance
	Collapsed       bool
	HasRunning      bool
	HasNotification bool
	Indent          int
}

// ---------- styles ----------

var (
	navItemStyle          = lipgloss.NewStyle().Foreground(ColorText).Padding(0, 1)
	navSelectedRowStyle   = lipgloss.NewStyle().Background(ColorIris).Foreground(ColorBase).Padding(0, 1)
	navActiveRowStyle     = lipgloss.NewStyle().Background(ColorOverlay).Foreground(ColorText).Padding(0, 1)
	navSectionDivStyle    = lipgloss.NewStyle().Foreground(ColorMuted).Padding(0, 1)
	navPlanLabelStyle     = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	navInstanceLabelStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
	navRunningIconStyle   = lipgloss.NewStyle().Foreground(ColorFoam)
	navReadyIconStyle     = lipgloss.NewStyle().Foreground(ColorFoam)
	navNotifyIconStyle    = lipgloss.NewStyle().Foreground(ColorRose)
	navPausedIconStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	navCompletedIconStyle = lipgloss.NewStyle().Foreground(ColorFoam).Faint(true)
	navIdleIconStyle      = lipgloss.NewStyle().Foreground(ColorMuted)
	navCancelledLblStyle  = lipgloss.NewStyle().Foreground(ColorMuted).Strikethrough(true)
	navImportStyle        = lipgloss.NewStyle().Foreground(ColorFoam).Padding(0, 1)
	navHistoryDivStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	navLegendLabelStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	navSearchBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1)
	navSearchActiveStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorFoam).Padding(0, 1)
)

// ---------- NavigationPanel ----------

// NavigationPanel is the sidebar showing the plan/instance tree.
type NavigationPanel struct {
	spinner *spinner.Model

	rows         []navRow
	selectedIdx  int
	scrollOffset int

	// Data sources
	plans         []PlanDisplay
	topics        []TopicDisplay
	instances     []*session.Instance
	deadPlans     []PlanDisplay
	historyPlans  []PlanDisplay
	promotedPlans []PlanDisplay
	cancelled     []PlanDisplay
	planStatuses  map[string]TopicStatus

	// Collapse state: collapsed holds the current collapsed/expanded value;
	// userOverrides tracks which plan files have been manually toggled.
	collapsed      map[string]bool
	userOverrides  map[string]bool
	inspectedPlans map[string]bool

	deadExpanded    bool
	historyExpanded bool
	searchActive    bool
	searchQuery     string
	clickUpAvail    bool

	// Embedded audit view rendered below the legend.
	auditView         string
	auditContentLines int // actual rendered body lines (events + minute headers)

	width, height int
	focused       bool
}

// NewNavigationPanel creates a NavigationPanel with an attached spinner model.
func NewNavigationPanel(sp *spinner.Model) *NavigationPanel {
	return &NavigationPanel{
		spinner:        sp,
		planStatuses:   make(map[string]TopicStatus),
		collapsed:      make(map[string]bool),
		userOverrides:  make(map[string]bool),
		inspectedPlans: make(map[string]bool),
		deadExpanded:   true,
		focused:        true,
	}
}

// ---------- data setters ----------

// SetData is the primary data update path. It replaces all state and rebuilds rows.
func (n *NavigationPanel) SetData(
	plans []PlanDisplay,
	instances []*session.Instance,
	history []PlanDisplay,
	cancelled []PlanDisplay,
	planStatuses map[string]TopicStatus,
) {
	n.plans = plans
	n.instances = instances
	n.cancelled = cancelled
	if planStatuses == nil {
		n.planStatuses = make(map[string]TopicStatus)
	} else {
		n.planStatuses = planStatuses
	}
	n.splitDeadFromHistory(history)
	n.rebuildRows()
}

// SetPlans replaces the plan list and rebuilds rows.
func (n *NavigationPanel) SetPlans(plans []PlanDisplay) {
	n.plans = plans
	n.rebuildRows()
}

// SetTopicsAndPlans replaces topics, ungrouped plans, history, and optionally
// cancelled plans, then rebuilds rows.
func (n *NavigationPanel) SetTopicsAndPlans(
	topics []TopicDisplay,
	ungrouped []PlanDisplay,
	history []PlanDisplay,
	cancelled ...[]PlanDisplay,
) {
	n.topics = topics
	if len(cancelled) > 0 {
		n.cancelled = cancelled[0]
	}
	all := make([]PlanDisplay, 0, len(ungrouped))
	all = append(all, ungrouped...)
	for _, t := range topics {
		all = append(all, t.Plans...)
	}
	n.plans = all
	n.splitDeadFromHistory(history)
	n.rebuildRows()
}

// SetPlanStatuses stores plan-level status flags without triggering a rebuild.
func (n *NavigationPanel) SetPlanStatuses(statuses map[string]TopicStatus) {
	if statuses != nil {
		n.planStatuses = statuses
	}
}

// SetItems is a legacy-compat shim — updates plan statuses and rebuilds.
func (n *NavigationPanel) SetItems(_ []string, _ map[string]int, _ int, _ map[string]bool, _ map[string]TopicStatus, planStatuses map[string]TopicStatus) {
	if planStatuses != nil {
		n.planStatuses = planStatuses
	}
	n.rebuildRows()
}

// ---------- dead/history partitioning ----------

// splitDeadFromHistory partitions a finished plan list into three buckets:
//   - promoted (added to n.plans): plan has at least one running/loading instance
//   - dead:     plan has only non-running instances, or was manually inspected
//   - history:  plan has no instances at all
//
// Previously promoted plans are removed from n.plans before re-partitioning.
func (n *NavigationPanel) splitDeadFromHistory(finished []PlanDisplay) {
	// Build per-plan instance info from current instance list.
	type info struct{ hasAny, hasRunning bool }
	byPlan := make(map[string]info, len(n.instances))
	for _, inst := range n.instances {
		if inst.TaskFile == "" {
			continue
		}
		entry := byPlan[inst.TaskFile]
		entry.hasAny = true
		if inst.Status == session.Running || inst.Status == session.Loading {
			entry.hasRunning = true
		}
		byPlan[inst.TaskFile] = entry
	}

	// Strip previously promoted plans from n.plans to avoid duplicates.
	if len(n.promotedPlans) > 0 {
		promoted := make(map[string]bool, len(n.promotedPlans))
		for _, p := range n.promotedPlans {
			promoted[p.Filename] = true
		}
		kept := n.plans[:0]
		for _, p := range n.plans {
			if !promoted[p.Filename] {
				kept = append(kept, p)
			}
		}
		n.plans = kept
	}

	n.deadPlans = nil
	n.historyPlans = nil
	n.promotedPlans = nil

	for _, p := range finished {
		inf := byPlan[p.Filename]
		switch {
		case inf.hasRunning:
			// Running instances → promote into active plans.
			n.promotedPlans = append(n.promotedPlans, p)
			n.plans = append(n.plans, p)
		case inf.hasAny || n.inspectedPlans[p.Filename]:
			// Non-running instances or manually inspected → dead section.
			n.deadPlans = append(n.deadPlans, p)
		default:
			// No instances → history.
			n.historyPlans = append(n.historyPlans, p)
		}
	}
}

// resplitDead recombines all finished plan buckets and re-partitions them.
func (n *NavigationPanel) resplitDead() {
	all := make([]PlanDisplay, 0, len(n.deadPlans)+len(n.promotedPlans)+len(n.historyPlans))
	all = append(all, n.deadPlans...)
	all = append(all, n.promotedPlans...)
	all = append(all, n.historyPlans...)
	n.splitDeadFromHistory(all)
}

// InspectPlan marks a history plan for manual inspection and moves it to dead.
func (n *NavigationPanel) InspectPlan(planFile string) {
	if n.inspectedPlans == nil {
		n.inspectedPlans = make(map[string]bool)
	}
	n.inspectedPlans[planFile] = true
	n.resplitDead()
	n.deadExpanded = true
	n.rebuildRows()
}

// ---------- row building ----------

// rebuildRows reconstructs the rows slice from current state. Selection is
// preserved by ID where possible, or clamped to the previous numeric position.
func (n *NavigationPanel) rebuildRows() {
	// Capture previous selection ID for restore.
	prevID := ""
	prevIdx := n.selectedIdx
	if prevIdx >= 0 && prevIdx < len(n.rows) {
		prevID = n.rows[prevIdx].ID
	}

	// Partition instances into plan-attached and solo.
	byPlan := make(map[string][]*session.Instance)
	var solo []*session.Instance
	for _, inst := range n.instances {
		if inst.TaskFile == "" {
			solo = append(solo, inst)
		} else {
			byPlan[inst.TaskFile] = append(byPlan[inst.TaskFile], inst)
		}
	}

	// Sort helpers
	sortInsts := func(list []*session.Instance) {
		sort.SliceStable(list, func(i, j int) bool {
			ki, kj := navInstanceSortKey(list[i]), navInstanceSortKey(list[j])
			if ki != kj {
				return ki < kj
			}
			return strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
		})
	}
	for _, list := range byPlan {
		sortInsts(list)
	}
	sortInsts(solo)

	// Sort plans: notification (0) < running/active-status (1) < idle (2), then alpha.
	sorted := append([]PlanDisplay(nil), n.plans...)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, pj := sorted[i], sorted[j]
		ki := navPlanSortKey(pi, byPlan[pi.Filename], n.planStatuses[pi.Filename])
		kj := navPlanSortKey(pj, byPlan[pj.Filename], n.planStatuses[pj.Filename])
		if ki != kj {
			return ki < kj
		}
		return strings.ToLower(taskstate.DisplayName(pi.Filename)) < strings.ToLower(taskstate.DisplayName(pj.Filename))
	})

	capacity := len(sorted) + len(n.instances) + len(n.deadPlans) + len(n.historyPlans) + len(n.cancelled) + 8
	rows := make([]navRow, 0, capacity)

	// appendPlan emits a plan header row and optionally its child instance rows.
	appendPlan := func(p PlanDisplay, indent int) {
		insts := byPlan[p.Filename]
		hasRunning, hasNotif := aggregateNavPlanStatus(insts, n.planStatuses[p.Filename])
		collapsed := n.isPlanCollapsed(p.Filename, hasRunning, hasNotif)
		rows = append(rows, navRow{
			Kind:            navRowPlanHeader,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           taskstate.DisplayName(p.Filename),
			TaskFile:        p.Filename,
			PlanStatus:      p.Status,
			Collapsed:       collapsed,
			HasRunning:      hasRunning,
			HasNotification: hasNotif,
			Indent:          indent,
		})
		if !collapsed {
			for _, inst := range insts {
				rows = append(rows, navRow{
					Kind:     navRowInstance,
					ID:       "inst:" + inst.Title,
					Label:    inst.Title,
					TaskFile: inst.TaskFile,
					Instance: inst,
					Indent:   indent,
				})
			}
		}
	}

	// ClickUp import action (pinned at top when available).
	if n.clickUpAvail {
		rows = append(rows, navRow{
			Kind:  navRowImportAction,
			ID:    SidebarImportClickUp,
			Label: "+ import from clickup",
		})
	}

	// Dead section: plans with non-running instances or manually inspected.
	if len(n.deadPlans) > 0 {
		rows = append(rows, navRow{
			Kind:      navRowDeadToggle,
			ID:        "__dead_toggle__",
			Label:     "dead",
			Collapsed: !n.deadExpanded,
		})
		if n.deadExpanded {
			for _, p := range n.deadPlans {
				appendPlan(p, 2)
			}
		}
	}

	// Split sorted plans into active (key < 2) and idle (key == 2).
	var activePlans, idlePlans []PlanDisplay
	for _, p := range sorted {
		if navPlanSortKey(p, byPlan[p.Filename], n.planStatuses[p.Filename]) < 2 {
			activePlans = append(activePlans, p)
		} else {
			idlePlans = append(idlePlans, p)
		}
	}

	// Active plans (running, notified, implementing, reviewing).
	for _, p := range activePlans {
		appendPlan(p, 0)
	}

	// Solo instances section (between active plans and idle plans).
	if len(solo) > 0 {
		rows = append(rows, navRow{Kind: navRowSoloHeader, ID: "__solo__", Label: "agents"})
		for _, inst := range solo {
			rows = append(rows, navRow{
				Kind:     navRowInstance,
				ID:       "inst:" + inst.Title,
				Label:    inst.Title,
				Instance: inst,
			})
		}
	}

	// Idle plans grouped by topic, then ungrouped remainder.
	idleSet := make(map[string]bool, len(idlePlans))
	for _, p := range idlePlans {
		idleSet[p.Filename] = true
	}

	emitted := make(map[string]bool)
	for _, t := range n.topics {
		var planGroup []PlanDisplay
		for _, p := range t.Plans {
			if idleSet[p.Filename] {
				planGroup = append(planGroup, p)
			}
		}
		if len(planGroup) == 0 {
			continue
		}
		topicID := SidebarTopicPrefix + t.Name
		collapsed := n.collapsed[topicID]
		rows = append(rows, navRow{
			Kind:      navRowTopicHeader,
			ID:        topicID,
			Label:     t.Name,
			Collapsed: collapsed,
		})
		for _, p := range planGroup {
			emitted[p.Filename] = true
			if !collapsed {
				appendPlan(p, 2)
			}
		}
	}

	// Ungrouped idle plans.
	for _, p := range idlePlans {
		if !emitted[p.Filename] {
			appendPlan(p, 0)
		}
	}

	// History section (collapsed toggle, expands to list).
	if len(n.historyPlans) > 0 {
		rows = append(rows, navRow{
			Kind:      navRowHistoryToggle,
			ID:        SidebarPlanHistoryToggle,
			Label:     "history",
			Collapsed: !n.historyExpanded,
		})
		if n.historyExpanded {
			for _, p := range n.historyPlans {
				rows = append(rows, navRow{
					Kind:     navRowHistoryPlan,
					ID:       SidebarPlanPrefix + p.Filename,
					Label:    taskstate.DisplayName(p.Filename),
					TaskFile: p.Filename,
				})
			}
		}
	}

	// Cancelled plans (always shown, no toggle).
	for _, p := range n.cancelled {
		rows = append(rows, navRow{
			Kind:     navRowCancelled,
			ID:       SidebarPlanPrefix + p.Filename,
			Label:    taskstate.DisplayName(p.Filename),
			TaskFile: p.Filename,
		})
	}

	n.rows = rows

	if len(rows) == 0 {
		n.selectedIdx = 0
		n.scrollOffset = 0
		return
	}

	// Restore selection by ID.
	if prevID != "" {
		for i, row := range rows {
			if row.ID == prevID {
				n.selectedIdx = i
				n.clampScroll()
				return
			}
		}
	}

	// ID not found — clamp to previous numeric position, skipping dividers.
	if prevIdx >= len(rows) {
		prevIdx = len(rows) - 1
	}
	if prevIdx < 0 {
		prevIdx = 0
	}
	n.selectedIdx = prevIdx
	if n.selectedIdx < len(rows) && rows[n.selectedIdx].Kind == navRowSoloHeader {
		if n.selectedIdx+1 < len(rows) {
			n.selectedIdx++
		} else if n.selectedIdx > 0 {
			n.selectedIdx--
		}
	}
	n.clampScroll()
}

// ---------- sort key helpers ----------

// navInstanceSortKey returns the sort priority for an instance within a plan.
// Lower values sort first: running (0) < notified (1) < paused (2) < done (3).
func navInstanceSortKey(inst *session.Instance) int {
	if inst.ImplementationComplete {
		return 3
	}
	switch inst.Status {
	case session.Running, session.Loading:
		return 0
	case session.Paused:
		return 2
	}
	if inst.Notified {
		return 1
	}
	return 3
}

// navPlanSortKey returns the sort priority for a plan.
// 0 = has notification, 1 = running or active lifecycle, 2 = idle.
func navPlanSortKey(p PlanDisplay, insts []*session.Instance, st TopicStatus) int {
	hasNotif := st.HasNotification
	hasRunning := st.HasRunning
	for _, inst := range insts {
		if inst.Notified {
			hasNotif = true
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasRunning = true
		}
	}
	switch {
	case hasNotif:
		return 0
	case hasRunning, p.Status == "implementing", p.Status == "reviewing":
		return 1
	default:
		return 2
	}
}

// aggregateNavPlanStatus derives combined running/notification flags from
// instance state and stored plan status flags.
func aggregateNavPlanStatus(insts []*session.Instance, st TopicStatus) (hasRunning, hasNotif bool) {
	hasRunning = st.HasRunning
	hasNotif = st.HasNotification
	for _, inst := range insts {
		if inst.Notified {
			hasNotif = true
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasRunning = true
		}
	}
	return
}

// isPlanCollapsed returns whether a plan should be shown collapsed.
// User overrides always win; otherwise collapse unless running or notified.
func (n *NavigationPanel) isPlanCollapsed(planFile string, hasRunning, hasNotif bool) bool {
	if _, ok := n.userOverrides[planFile]; ok {
		return n.collapsed[planFile]
	}
	return !hasRunning && !hasNotif
}

// ---------- layout ----------

func (n *NavigationPanel) SetSize(width, height int) {
	n.width, n.height = width, height
	n.clampScroll()
}

func (n *NavigationPanel) SetAuditView(view string, contentLines int) {
	n.auditView = view
	n.auditContentLines = contentLines
}

func (n *NavigationPanel) SetFocused(focused bool)    { n.focused = focused }
func (n *NavigationPanel) IsFocused() bool            { return n.focused }
func (n *NavigationPanel) SetClickUpAvailable(a bool) { n.clickUpAvail = a; n.rebuildRows() }

// availRows returns the number of rows the scroll window can display.
// Overhead accounts for border (2), search box (3), blank line (1),
// legend (1), and gap above legend (1) = 8.  When the audit pane is
// active, additional lines are reserved dynamically based on actual
// content lines (events + minute headers) so the log height stays
// stable regardless of window size.
func (n *NavigationPanel) availRows() int {
	const (
		baseOverhead = 8 // border(2) + search(3) + blank(1) + legend(1) + gap(1)
		minNavRows   = 5 // always keep at least 5 nav item rows
	)
	overhead := baseOverhead
	if n.auditView != "" && n.auditContentLines > 0 {
		// Reserve: 1 gap below legend + 1 audit header + contentLines body
		auditReserve := 2 + n.auditContentLines
		// Cap at 50% of inner height so the task list isn't squished.
		innerHeight := n.height - baseOverhead
		halfPanel := innerHeight / 2
		if halfPanel < 3 {
			halfPanel = 3 // minimum viable audit (header + 2 lines)
		}
		if auditReserve > halfPanel {
			auditReserve = halfPanel
		}
		overhead += auditReserve
	}
	v := n.height - overhead
	if v < 1 {
		return 1
	}
	return v
}

// clampScroll ensures scrollOffset keeps selectedIdx visible.
func (n *NavigationPanel) clampScroll() {
	if len(n.rows) == 0 {
		n.scrollOffset = 0
		return
	}
	avail := n.availRows()
	if n.selectedIdx < n.scrollOffset {
		n.scrollOffset = n.selectedIdx
	}
	if n.selectedIdx >= n.scrollOffset+avail {
		n.scrollOffset = n.selectedIdx - avail + 1
	}
	if n.scrollOffset < 0 {
		n.scrollOffset = 0
	}
}

// ---------- search ----------

func (n *NavigationPanel) ActivateSearch()        { n.searchActive = true; n.searchQuery = "" }
func (n *NavigationPanel) DeactivateSearch()      { n.searchActive = false; n.searchQuery = "" }
func (n *NavigationPanel) IsSearchActive() bool   { return n.searchActive }
func (n *NavigationPanel) GetSearchQuery() string { return n.searchQuery }

// SetSearchQuery updates the search filter and snaps selection to the first matching row.
func (n *NavigationPanel) SetSearchQuery(q string) {
	n.searchQuery = q
	if q != "" && len(n.rows) > 0 && !n.rowMatchesSearch(n.selectedIdx) {
		for i := range n.rows {
			if n.rows[i].Kind != navRowSoloHeader && n.rowMatchesSearch(i) {
				n.selectedIdx = i
				n.clampScroll()
				return
			}
		}
	}
}

// rowMatchesSearch returns true if the row at idx passes the current search filter.
func (n *NavigationPanel) rowMatchesSearch(idx int) bool {
	if !n.searchActive || n.searchQuery == "" {
		return true
	}
	q := strings.ToLower(n.searchQuery)
	row := n.rows[idx]
	return strings.Contains(strings.ToLower(row.Label), q) ||
		strings.Contains(strings.ToLower(row.TaskFile), q)
}

// ---------- expand/collapse ----------

// ToggleSelectedExpand toggles the expand/collapse state of the selected row.
// Returns true if the row supports toggling.
func (n *NavigationPanel) ToggleSelectedExpand() bool {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return false
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		n.collapsed[row.TaskFile] = !row.Collapsed
		n.userOverrides[row.TaskFile] = true
		n.rebuildRows()
		return true
	case navRowTopicHeader:
		n.collapsed[row.ID] = !row.Collapsed
		n.rebuildRows()
		return true
	case navRowDeadToggle:
		n.deadExpanded = !n.deadExpanded
		n.rebuildRows()
		return true
	case navRowHistoryToggle:
		n.historyExpanded = !n.historyExpanded
		n.rebuildRows()
		return true
	default:
		return false
	}
}

// ---------- navigation ----------

// Up moves the selection up one visible, selectable row.
func (n *NavigationPanel) Up() {
	orig := n.selectedIdx
	for n.selectedIdx > 0 {
		n.selectedIdx--
		if n.rows[n.selectedIdx].Kind == navRowSoloHeader {
			continue
		}
		if !n.rowMatchesSearch(n.selectedIdx) {
			continue
		}
		n.clampScroll()
		return
	}
	n.selectedIdx = orig
}

// Down moves the selection down one visible, selectable row.
func (n *NavigationPanel) Down() {
	orig := n.selectedIdx
	for n.selectedIdx+1 < len(n.rows) {
		n.selectedIdx++
		if n.rows[n.selectedIdx].Kind == navRowSoloHeader {
			continue
		}
		if !n.rowMatchesSearch(n.selectedIdx) {
			continue
		}
		n.clampScroll()
		return
	}
	n.selectedIdx = orig
}

// Left collapses an expanded plan header, or jumps to the parent header when on an instance.
func (n *NavigationPanel) Left() {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		if !row.Collapsed {
			n.ToggleSelectedExpand()
		}
	case navRowInstance:
		for i := n.selectedIdx - 1; i >= 0; i-- {
			if n.rows[i].Kind == navRowPlanHeader {
				n.selectedIdx = i
				n.clampScroll()
				return
			}
		}
	}
}

// Right expands a collapsed header, or descends into the first child row.
func (n *NavigationPanel) Right() {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		if row.Collapsed {
			n.ToggleSelectedExpand()
		} else {
			n.Down()
		}
	case navRowDeadToggle:
		if row.Collapsed {
			n.ToggleSelectedExpand()
		} else {
			n.Down()
		}
	case navRowHistoryToggle:
		if row.Collapsed {
			n.ToggleSelectedExpand()
		}
	}
}

// ---------- selection API ----------

func (n *NavigationPanel) GetSelectedInstance() *session.Instance {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return nil
	}
	return n.rows[n.selectedIdx].Instance
}

func (n *NavigationPanel) GetSelectedPlanFile() string {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return ""
	}
	return n.rows[n.selectedIdx].TaskFile
}

func (n *NavigationPanel) IsSelectedPlanHeader() bool {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return false
	}
	k := n.rows[n.selectedIdx].Kind
	return k == navRowPlanHeader || k == navRowHistoryPlan || k == navRowCancelled
}

func (n *NavigationPanel) IsSelectedHistoryPlan() bool {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return false
	}
	return n.rows[n.selectedIdx].Kind == navRowHistoryPlan
}

func (n *NavigationPanel) GetSelectedID() string {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return ""
	}
	return n.rows[n.selectedIdx].ID
}

func (n *NavigationPanel) GetSelectedIdx() int  { return n.selectedIdx }
func (n *NavigationPanel) GetScrollOffset() int { return n.scrollOffset }

// ClickItem selects a row by its raw index.
func (n *NavigationPanel) ClickItem(row int) {
	if row >= 0 && row < len(n.rows) {
		n.selectedIdx = row
		n.clampScroll()
	}
}

// SelectByID moves selection to the row with the given ID. Returns false if not found.
func (n *NavigationPanel) SelectByID(id string) bool {
	for i, row := range n.rows {
		if row.ID == id {
			n.selectedIdx = i
			n.clampScroll()
			return true
		}
	}
	return false
}

// SelectInstance moves selection to the given instance, auto-expanding its plan if needed.
func (n *NavigationPanel) SelectInstance(inst *session.Instance) bool {
	for i, row := range n.rows {
		if row.Instance == inst {
			n.selectedIdx = i
			n.clampScroll()
			return true
		}
	}
	// Instance is under a collapsed plan — expand and retry.
	if inst.TaskFile != "" {
		n.collapsed[inst.TaskFile] = false
		n.userOverrides[inst.TaskFile] = true
		n.rebuildRows()
		for i, row := range n.rows {
			if row.Instance == inst {
				n.selectedIdx = i
				n.clampScroll()
				return true
			}
		}
	}
	return false
}

func (n *NavigationPanel) SetSelectedInstance(idx int) {
	if idx < 0 || idx >= len(n.instances) {
		return
	}
	_ = n.SelectInstance(n.instances[idx])
}

func (n *NavigationPanel) SelectedIndex() int {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return 0
	}
	for i, item := range n.instances {
		if item == inst {
			return i
		}
	}
	return 0
}

// SelectFirst selects the first selectable row (skips solo-header dividers).
func (n *NavigationPanel) SelectFirst() {
	if len(n.rows) == 0 {
		return
	}
	n.selectedIdx = 0
	if n.rows[0].Kind == navRowSoloHeader && len(n.rows) > 1 {
		n.selectedIdx = 1
	}
	n.clampScroll()
}

// SelectedSpaceAction returns the label for the space-key action on the current row.
func (n *NavigationPanel) SelectedSpaceAction() string {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return "toggle"
	}
	switch n.rows[n.selectedIdx].Kind {
	case navRowPlanHeader, navRowDeadToggle, navRowHistoryToggle:
		if n.rows[n.selectedIdx].Collapsed {
			return "expand"
		}
		return "collapse"
	default:
		return "toggle"
	}
}

// ---------- cycle ----------

// CycleNextActive moves selection to the next non-paused instance in visual order.
func (n *NavigationPanel) CycleNextActive() { n.cycleActive(1) }

// CyclePrevActive moves selection to the previous non-paused instance in visual order.
func (n *NavigationPanel) CyclePrevActive() { n.cycleActive(-1) }

// cycleActive builds an ordered list of non-paused instances (including those
// hidden under collapsed plans at their header position), then advances by step.
func (n *NavigationPanel) cycleActive(step int) {
	var ordered []*session.Instance
	for _, row := range n.rows {
		switch {
		case row.Kind == navRowInstance && row.Instance != nil && !row.Instance.Paused():
			ordered = append(ordered, row.Instance)
		case row.Kind == navRowPlanHeader && row.Collapsed:
			for _, inst := range n.instances {
				if inst.TaskFile == row.TaskFile && !inst.Paused() {
					ordered = append(ordered, inst)
				}
			}
		}
	}
	if len(ordered) == 0 {
		return
	}

	cur := -1
	if sel := n.GetSelectedInstance(); sel != nil {
		for i, inst := range ordered {
			if inst == sel {
				cur = i
				break
			}
		}
	}
	if cur < 0 {
		cur = 0
	}
	next := (cur + step + len(ordered)) % len(ordered)
	n.SelectInstance(ordered[next])
}

// ---------- instance management ----------

func (n *NavigationPanel) GetInstances() []*session.Instance { return n.instances }
func (n *NavigationPanel) TotalInstances() int               { return len(n.instances) }
func (n *NavigationPanel) NumInstances() int                 { return len(n.instances) }

// AddInstance appends an instance and returns a no-op cleanup function.
func (n *NavigationPanel) AddInstance(inst *session.Instance) func() {
	n.instances = append(n.instances, inst)
	n.resplitDead()
	n.rebuildRows()
	return func() {}
}

// RemoveByTitle removes the instance with the given title and returns it (nil if not found).
func (n *NavigationPanel) RemoveByTitle(title string) *session.Instance {
	for i, inst := range n.instances {
		if inst.Title == title {
			n.instances = append(n.instances[:i], n.instances[i+1:]...)
			n.resplitDead()
			n.rebuildRows()
			return inst
		}
	}
	return nil
}

// Remove removes the currently selected instance.
func (n *NavigationPanel) Remove() {
	if inst := n.GetSelectedInstance(); inst != nil {
		n.RemoveByTitle(inst.Title)
	}
}

// Kill sends SIGKILL to the selected instance and removes it.
func (n *NavigationPanel) Kill() {
	if inst := n.GetSelectedInstance(); inst != nil {
		_ = inst.Kill()
		n.RemoveByTitle(inst.Title)
	}
}

// Attach opens an interactive terminal session for the selected instance.
func (n *NavigationPanel) Attach() (chan struct{}, error) {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return nil, fmt.Errorf("no instance selected")
	}
	return inst.Attach()
}

// Clear removes all instances and resets the row list.
func (n *NavigationPanel) Clear() {
	n.instances = nil
	n.rows = nil
	n.selectedIdx = 0
	n.scrollOffset = 0
}

// SetSessionPreviewSize resizes all active (non-paused) instance preview panes.
func (n *NavigationPanel) SetSessionPreviewSize(width, height int) error {
	var firstErr error
	for _, inst := range n.instances {
		if !inst.Started() || inst.Paused() {
			continue
		}
		if err := inst.SetPreviewSize(width, height); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// FindPlanInstance returns the best candidate instance for a plan: prefers
// running/loading over ready. Returns nil if only paused instances exist.
func (n *NavigationPanel) FindPlanInstance(planFile string) *session.Instance {
	var candidate *session.Instance
	for _, inst := range n.instances {
		if inst.TaskFile != planFile || inst.Paused() {
			continue
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			return inst // running wins immediately
		}
		if candidate == nil {
			candidate = inst
		}
	}
	return candidate
}

// RowCount returns the total number of rows in the panel.
func (n *NavigationPanel) RowCount() int { return len(n.rows) }

// ---------- display helpers ----------

// navInstanceTitle returns the human-readable display label for an instance.
func navInstanceTitle(inst *session.Instance) string {
	switch {
	case inst.WaveNumber > 0 && inst.TaskNumber > 0:
		return fmt.Sprintf("wave %d · task %d", inst.WaveNumber, inst.TaskNumber)
	case inst.AgentType == session.AgentTypeReviewer && inst.TaskFile != "":
		if inst.ReviewCycle > 0 {
			return fmt.Sprintf("review #%d", inst.ReviewCycle)
		}
		return "review"
	case inst.AgentType == session.AgentTypePlanner && inst.TaskFile != "":
		return "planning"
	case inst.SoloAgent && inst.TaskFile != "":
		return taskstate.DisplayName(inst.TaskFile)
	case inst.AgentType == session.AgentTypeCoder && inst.TaskFile != "" && inst.WaveNumber == 0:
		if inst.ReviewCycle > 0 {
			return fmt.Sprintf("applying fixes #%d", inst.ReviewCycle)
		}
		return "applying fixes"
	default:
		return inst.Title
	}
}

// navInstanceStatusIcon returns a styled status glyph for an instance row.
func (n *NavigationPanel) navInstanceStatusIcon(inst *session.Instance) string {
	if inst.Exited {
		return navCancelledLblStyle.Render("✕")
	}
	if inst.ImplementationComplete {
		return navCompletedIconStyle.Render("✓")
	}
	switch inst.Status {
	case session.Running, session.Loading:
		if n.spinner != nil {
			return strings.TrimRight(n.spinner.View(), " ")
		}
		return navRunningIconStyle.Render("●")
	case session.Ready:
		if inst.Notified {
			return navNotifyIconStyle.Render("◉")
		}
		return navReadyIconStyle.Render("●")
	case session.Paused:
		return navPausedIconStyle.Render("\uf04c")
	default:
		return navIdleIconStyle.Render("○")
	}
}

// navPlanStatusIcon returns a styled status glyph for a plan header row.
func navPlanStatusIcon(row navRow) string {
	switch {
	case row.HasNotification:
		return navNotifyIconStyle.Render("◉")
	case row.HasRunning:
		return navRunningIconStyle.Render("●")
	case row.PlanStatus == "planning":
		return navRunningIconStyle.Render("●")
	case row.PlanStatus == "reviewing":
		return navNotifyIconStyle.Render("◉")
	default:
		return navIdleIconStyle.Render("○")
	}
}

// navSectionLabel maps a plan sort key to a section label string.
func navSectionLabel(key int) string {
	if key < 2 {
		return "active"
	}
	return "plans"
}

// navDividerLine renders a full-width divider: "──── label ────".
func navDividerLine(label string, w int) string {
	inner := " " + label + " "
	innerW := runewidth.StringWidth(inner)
	remaining := w - innerW
	if remaining < 2 {
		return navHistoryDivStyle.Render(inner)
	}
	left := remaining / 2
	right := remaining - left
	line := strings.Repeat("─", left) + inner + strings.Repeat("─", right)
	return navHistoryDivStyle.Render(line)
}

// ---------- row rendering ----------

// renderNavRow produces the unstyled (pre-selection) text for a single row.
func (n *NavigationPanel) renderNavRow(row navRow, contentWidth int) string {
	switch row.Kind {

	case navRowPlanHeader:
		chevron := "▸"
		if !row.Collapsed {
			chevron = "▾"
		}
		statusIcon := navPlanStatusIcon(row)
		statusW := lipgloss.Width(statusIcon)
		indent := strings.Repeat(" ", row.Indent)
		indentW := row.Indent

		label := row.Label
		maxLabel := contentWidth - indentW - 3 - statusW
		if maxLabel < 3 {
			maxLabel = 3
		}
		if runewidth.StringWidth(label) > maxLabel {
			label = runewidth.Truncate(label, maxLabel-1, "…")
		}
		usedW := indentW + 2 + runewidth.StringWidth(label) + 1 + statusW
		gap := contentWidth - usedW
		if gap < 0 {
			gap = 0
		}
		lblStyle := navPlanLabelStyle
		if row.Indent > 0 {
			lblStyle = navPlanLabelStyle.Bold(false)
		}
		return indent + chevron + " " + lblStyle.Render(label) + strings.Repeat(" ", gap) + " " + statusIcon

	case navRowInstance:
		inst := row.Instance
		isSolo := row.TaskFile == ""
		if inst == nil {
			if isSolo {
				return row.Label
			}
			return "    " + row.Label
		}

		title := navInstanceTitle(inst)
		statusIcon := n.navInstanceStatusIcon(inst)
		statusW := lipgloss.Width(statusIcon)

		indentW := row.Indent + 4
		if isSolo {
			indentW = 0
		}
		maxLabel := contentWidth - indentW - 1 - statusW
		if maxLabel < 3 {
			maxLabel = 3
		}
		if runewidth.StringWidth(title) > maxLabel {
			title = runewidth.Truncate(title, maxLabel-1, "…")
		}
		usedW := indentW + runewidth.StringWidth(title) + 1 + statusW
		gap := contentWidth - usedW
		if gap < 0 {
			gap = 0
		}
		indent := strings.Repeat(" ", indentW)
		lblStyle := navInstanceLabelStyle
		if isSolo {
			indent = ""
			lblStyle = navPlanLabelStyle
		}
		if inst.Exited {
			lblStyle = navCancelledLblStyle
			statusIcon = navCancelledLblStyle.Render("✕")
			statusW = lipgloss.Width(statusIcon)
			usedW = indentW + runewidth.StringWidth(title) + 1 + statusW
			gap = contentWidth - usedW
			if gap < 0 {
				gap = 0
			}
		}
		return indent + lblStyle.Render(title) + strings.Repeat(" ", gap) + " " + statusIcon

	case navRowSoloHeader:
		return navDividerLine("agents", contentWidth)

	case navRowTopicHeader:
		chevron := "▸"
		if !row.Collapsed {
			chevron = "▾"
		}
		label := row.Label
		maxLabel := contentWidth - 2
		if maxLabel < 3 {
			maxLabel = 3
		}
		if runewidth.StringWidth(label) > maxLabel {
			label = runewidth.Truncate(label, maxLabel-1, "…")
		}
		return chevron + " " + navPlanLabelStyle.Render(label)

	case navRowDeadToggle:
		return navDividerLine("dead", contentWidth)

	case navRowHistoryToggle:
		chevron := "▸"
		if !row.Collapsed {
			chevron = "▾"
		}
		return navDividerLine(chevron+" history", contentWidth)

	case navRowHistoryPlan:
		label := row.Label
		doneIcon := navCompletedIconStyle.Render("●")
		doneW := lipgloss.Width(doneIcon)
		maxLabel := contentWidth - 1 - doneW
		if maxLabel < 3 {
			maxLabel = 3
		}
		if runewidth.StringWidth(label) > maxLabel {
			label = runewidth.Truncate(label, maxLabel-1, "…")
		}
		usedW := runewidth.StringWidth(label) + 1 + doneW
		gap := contentWidth - usedW
		if gap < 0 {
			gap = 0
		}
		return navIdleIconStyle.Render(label) + strings.Repeat(" ", gap) + " " + doneIcon

	case navRowCancelled:
		label := row.Label
		const trailW = 2
		maxLabel := contentWidth - trailW
		if maxLabel < 3 {
			maxLabel = 3
		}
		if runewidth.StringWidth(label) > maxLabel {
			label = runewidth.Truncate(label, maxLabel-1, "…")
		}
		usedW := runewidth.StringWidth(label) + trailW
		gap := contentWidth - usedW
		if gap < 0 {
			gap = 0
		}
		return navCancelledLblStyle.Render(label) + strings.Repeat(" ", gap) + " " + navCancelledLblStyle.Render("✕")

	case navRowImportAction:
		labelW := runewidth.StringWidth(row.Label)
		pad := contentWidth - labelW
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + row.Label

	default:
		return row.Label
	}
}

// ---------- String ----------

// String renders the full navigation panel as a bordered, scrollable string.
func (n *NavigationPanel) String() string {
	// Border style changes based on focus.
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1)
	if n.focused {
		border = border.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	}

	// In lipgloss v2, Width/Height specify total outer dimensions (border + padding + content).
	// The border style has border(2h,2v) + padding(0v,1h each side = 2h) = 4h, 2v frame.
	// innerWidth is the content area inside the border.
	innerWidth := n.width - 4
	if innerWidth < 8 {
		innerWidth = 8
	}
	innerHeight := n.height - 2
	if innerHeight < 4 {
		innerHeight = 4
	}

	// Row styles (navItemStyle etc.) have Padding(0,1) = 2h frame.
	// In v2, Width(itemWidth) means total = itemWidth, content = itemWidth - 2.
	// We want the total styled row to be innerWidth wide, so itemWidth = innerWidth.
	itemWidth := innerWidth
	contentWidth := itemWidth - 2 // content inside the row padding
	if contentWidth < 4 {
		contentWidth = 4
	}

	// Search bar styles have border(2) + padding(2) = 4h frame.
	// We want the search box total to be innerWidth wide.
	searchWidth := innerWidth
	if searchWidth < 8 {
		searchWidth = 8
	}
	var searchBox string
	if n.searchActive {
		text := n.searchQuery
		if text == "" {
			text = " "
		}
		searchBox = zone.Mark(ZoneNavSearch, navSearchActiveStyle.Width(searchWidth).Render(text))
	} else {
		searchBox = zone.Mark(ZoneNavSearch, navSearchBoxStyle.Width(searchWidth).Render("\uf002 search"))
	}

	// Build visible items, tracking which row each item maps to.
	type visItem struct {
		line   string
		rowIdx int // -1 for injected section dividers
	}
	items := make([]visItem, 0, len(n.rows)+4)
	selectedDisplayIdx := 0
	lastPlanKey := -1
	inDeadSection := false

	for i, row := range n.rows {
		// Apply search filter.
		if n.searchActive && n.searchQuery != "" {
			q := strings.ToLower(n.searchQuery)
			if !strings.Contains(strings.ToLower(row.Label), q) &&
				!strings.Contains(strings.ToLower(row.TaskFile), q) {
				continue
			}
		}

		// Track dead section to suppress plan-group dividers inside it.
		if row.Kind == navRowDeadToggle {
			inDeadSection = true
		} else if row.Kind != navRowPlanHeader && row.Kind != navRowInstance {
			inDeadSection = false
		}

		// Inject section dividers before plan header groups.
		if row.Kind == navRowPlanHeader && !inDeadSection {
			sk := 2
			if row.HasNotification || row.HasRunning ||
				row.PlanStatus == "implementing" || row.PlanStatus == "reviewing" {
				sk = 0 // unified "active" group
			}
			if sk != lastPlanKey {
				items = append(items, visItem{line: navDividerLine(navSectionLabel(sk), itemWidth), rowIdx: -1})
				lastPlanKey = sk
			}
		}
		// Topic headers always belong to the idle group.
		if row.Kind == navRowTopicHeader && lastPlanKey != 2 {
			items = append(items, visItem{line: navDividerLine(navSectionLabel(2), itemWidth), rowIdx: -1})
			lastPlanKey = 2
		}

		if i == n.selectedIdx {
			selectedDisplayIdx = len(items)
		}

		rawLine := n.renderNavRow(row, contentWidth)

		// Apply selection / import styling.
		var styledLine string
		switch {
		case i == n.selectedIdx && n.focused:
			styledLine = navSelectedRowStyle.Width(itemWidth).Render(ansi.Strip(rawLine))
		case i == n.selectedIdx:
			styledLine = navActiveRowStyle.Width(itemWidth).Render(ansi.Strip(rawLine))
		case row.Kind == navRowImportAction:
			styledLine = navImportStyle.Width(itemWidth).Render(rawLine)
		default:
			styledLine = navItemStyle.Width(itemWidth).Render(rawLine)
		}
		items = append(items, visItem{line: styledLine, rowIdx: i})
	}

	// Scroll window centred on the selected item.
	avail := n.availRows()
	start := selectedDisplayIdx - avail/2
	if start < 0 {
		start = 0
	}
	end := start + avail
	if end > len(items) {
		end = len(items)
		start = end - avail
		if start < 0 {
			start = 0
		}
	}

	var bodyLines []string
	for _, item := range items[start:end] {
		line := item.line
		if item.rowIdx >= 0 {
			line = zone.Mark(NavRowZoneID(item.rowIdx), line)
		}
		bodyLines = append(bodyLines, line)
	}
	body := strings.Join(bodyLines, "\n")
	if body != "" {
		body += "\n"
	}

	// Legend — icon key centred in the pane.
	legendContent := navIdleIconStyle.Render("○") + navLegendLabelStyle.Render(" idle") +
		"  " + navRunningIconStyle.Render("●") + navLegendLabelStyle.Render(" planning") +
		"  " + navRunningIconStyle.Render("●") + navLegendLabelStyle.Render(" running") +
		"  " + navNotifyIconStyle.Render("◉") + navLegendLabelStyle.Render(" review")
	legend := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(legendContent)

	topContent := searchBox + "\n" + body

	topLines := strings.Count(topContent, "\n") + 1
	legendLines := strings.Count(legend, "\n") + 1

	// Compute optional audit section (bottom-pinned).
	// Size audit FIRST based on actual content, then gaps absorb remainder.
	// This prevents the audit height from changing by ±1 per terminal resize row.
	auditSection := ""
	auditLines := 0
	if n.auditView != "" && n.auditContentLines > 0 {
		// Desired audit: 1 header + all content body lines.
		desiredAudit := 1 + n.auditContentLines
		// Available space: total height minus nav items, search, legend, and minimum gaps (2).
		availForAudit := innerHeight - topLines - legendLines - 2
		// Cap at 50% of inner height so the task list isn't squished.
		halfPanel := innerHeight / 2
		if availForAudit > halfPanel {
			availForAudit = halfPanel
		}
		if availForAudit < 3 {
			availForAudit = 0 // too small, hide audit entirely
		}
		maxAudit := desiredAudit
		if maxAudit > availForAudit {
			maxAudit = availForAudit
		}
		if maxAudit >= 3 {
			vlines := strings.Split(n.auditView, "\n")
			header := ""
			var bodyAudit []string
			if len(vlines) > 0 {
				header = vlines[0]
				bodyAudit = vlines[1:]
			}
			// Strip trailing blank lines.
			for len(bodyAudit) > 0 && strings.TrimSpace(bodyAudit[len(bodyAudit)-1]) == "" {
				bodyAudit = bodyAudit[:len(bodyAudit)-1]
			}
			maxBody := maxAudit - 1
			if maxBody < 0 {
				maxBody = 0
			}
			if len(bodyAudit) > maxBody {
				bodyAudit = bodyAudit[len(bodyAudit)-maxBody:]
				for len(bodyAudit) > 0 && isAuditContinuationLine(bodyAudit[0]) {
					bodyAudit = bodyAudit[1:]
				}
			}
			// No blank padding — show exactly what content exists.
			lines := make([]string, 0, 1+len(bodyAudit))
			lines = append(lines, header)
			lines = append(lines, bodyAudit...)
			auditSection = strings.Join(lines, "\n")
			auditLines = len(lines)
		}
	}

	// Fixed 1-line gaps around the legend; all leftover space goes above
	// (between nav items and legend) to keep legend pinned near the bottom.
	const legendGapBelow = 1
	gapAbove := innerHeight - topLines - legendLines - auditLines - legendGapBelow + 1
	if gapAbove < 1 {
		gapAbove = 1
	}

	innerContent := topContent + strings.Repeat("\n", gapAbove) + legend
	if auditSection != "" {
		innerContent += strings.Repeat("\n", legendGapBelow) + auditSection
	}

	// In lipgloss v2, Width/Height are total outer dimensions.
	// Pass the full nav panel dimensions so the border fits exactly.
	bordered := border.Width(n.width).Height(n.height).Render(innerContent)
	placed := lipgloss.Place(n.width, n.height, lipgloss.Left, lipgloss.Top, bordered)
	return zone.Mark(ZoneNavPanel, placed)
}

// isAuditContinuationLine returns true if the line is a word-wrap
// continuation fragment rather than a primary event or minute header line.
// Primary lines: " ◆  text" (1 leading space + icon) or "── 15:06 ──" (0 spaces).
// Continuations: "      text" (5+ leading spaces for alignment).
func isAuditContinuationLine(line string) bool {
	plain := ansi.Strip(line)
	if len(plain) == 0 || strings.TrimSpace(plain) == "" {
		return true
	}
	// Count leading spaces — continuation lines have 2+ (pad + overhead).
	spaces := 0
	for _, r := range plain {
		if r == ' ' {
			spaces++
		} else {
			break
		}
	}
	return spaces >= 2
}
