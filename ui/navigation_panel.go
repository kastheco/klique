package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

const (
	SidebarPlanPrefix        = "__plan__"
	SidebarTopicPrefix       = "__topic__"
	SidebarPlanHistoryToggle = "__plan_history_toggle__"
	SidebarImportClickUp     = "__import_clickup__"
)

type PlanDisplay struct {
	Filename    string
	Status      string
	Description string
	Branch      string
	Topic       string
}

type TopicStatus struct {
	HasRunning      bool
	HasNotification bool
}

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

type navRow struct {
	Kind            navRowKind
	ID              string
	Label           string
	PlanFile        string
	PlanStatus      string // plan lifecycle status (e.g. "implementing", "reviewing")
	Instance        *session.Instance
	Collapsed       bool
	HasRunning      bool
	HasNotification bool
	Indent          int // indentation level in spaces (0 = top-level)
}

// Navigation panel styles
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

type NavigationPanel struct {
	spinner *spinner.Model

	rows         []navRow
	selectedIdx  int
	scrollOffset int

	plans          []PlanDisplay
	topics         []TopicDisplay
	instances      []*session.Instance
	deadPlans      []PlanDisplay
	historyPlans   []PlanDisplay
	promotedPlans  []PlanDisplay // finished plans promoted to active (have running instances)
	cancelled      []PlanDisplay
	planStatuses   map[string]TopicStatus
	collapsed      map[string]bool
	userOverrides  map[string]bool
	inspectedPlans map[string]bool

	deadExpanded    bool
	historyExpanded bool
	searchActive    bool
	searchQuery     string
	clickUpAvail    bool

	repoName    string
	repoHovered bool

	// Audit section rendered by the AuditPane and displayed inside the border.
	auditView   string
	auditHeight int

	width, height int
	focused       bool
}

type TopicDisplay struct {
	Name  string
	Plans []PlanDisplay
}

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

func (n *NavigationPanel) SetData(plans []PlanDisplay, instances []*session.Instance, history []PlanDisplay, cancelled []PlanDisplay, planStatuses map[string]TopicStatus) {
	n.plans = plans
	n.instances = instances
	n.cancelled = cancelled
	if planStatuses == nil {
		n.planStatuses = make(map[string]TopicStatus)
	} else {
		n.planStatuses = planStatuses
	}
	// Split history into dead (has instances or manually inspected) and history.
	n.splitDeadFromHistory(history)
	n.rebuildRows()
}

func (n *NavigationPanel) SetPlans(plans []PlanDisplay) {
	n.plans = plans
	n.rebuildRows()
}

func (n *NavigationPanel) SetTopicsAndPlans(topics []TopicDisplay, ungrouped []PlanDisplay, history []PlanDisplay, cancelled ...[]PlanDisplay) {
	n.topics = topics
	if len(cancelled) > 0 {
		n.cancelled = cancelled[0]
	}
	plans := make([]PlanDisplay, 0, len(ungrouped))
	plans = append(plans, ungrouped...)
	for _, t := range topics {
		plans = append(plans, t.Plans...)
	}
	n.plans = plans
	// Split history into dead (has instances or manually inspected) and history.
	n.splitDeadFromHistory(history)
	n.rebuildRows()
}

// SetPlanStatuses updates plan-level status flags (running/notification)
// without triggering a row rebuild. Call this before SetTopicsAndPlans so
// the subsequent rebuild uses correct statuses.
func (n *NavigationPanel) SetPlanStatuses(statuses map[string]TopicStatus) {
	if statuses != nil {
		n.planStatuses = statuses
	}
}

func (n *NavigationPanel) SetItems(_ []string, _ map[string]int, _ int, _ map[string]bool, _ map[string]TopicStatus, planStatuses map[string]TopicStatus) {
	if planStatuses != nil {
		n.planStatuses = planStatuses
	}
	n.rebuildRows()
}

// splitDeadFromHistory partitions finished plans into three buckets:
//   - promoted (appended to n.plans): has running/loading instances
//   - dead: has only non-running instances or was manually inspected
//   - history: no instances at all
func (n *NavigationPanel) splitDeadFromHistory(finished []PlanDisplay) {
	type planInfo struct {
		hasInstances bool
		hasRunning   bool
	}
	infoByPlan := make(map[string]planInfo, len(n.instances))
	for _, inst := range n.instances {
		if inst.PlanFile != "" {
			info := infoByPlan[inst.PlanFile]
			info.hasInstances = true
			if inst.Status == session.Running || inst.Status == session.Loading {
				info.hasRunning = true
			}
			infoByPlan[inst.PlanFile] = info
		}
	}
	// Remove any previously promoted plans from n.plans before re-partitioning.
	if len(n.promotedPlans) > 0 {
		promoted := make(map[string]bool, len(n.promotedPlans))
		for _, p := range n.promotedPlans {
			promoted[p.Filename] = true
		}
		filtered := n.plans[:0]
		for _, p := range n.plans {
			if !promoted[p.Filename] {
				filtered = append(filtered, p)
			}
		}
		n.plans = filtered
	}
	n.deadPlans = nil
	n.historyPlans = nil
	n.promotedPlans = nil
	for _, p := range finished {
		info := infoByPlan[p.Filename]
		if info.hasRunning {
			// Running instances → promote to active plans list.
			n.promotedPlans = append(n.promotedPlans, p)
			n.plans = append(n.plans, p)
		} else if info.hasInstances || n.inspectedPlans[p.Filename] {
			n.deadPlans = append(n.deadPlans, p)
		} else {
			n.historyPlans = append(n.historyPlans, p)
		}
	}
}

// resplitDead re-partitions dead, promoted, and history plans based on current instances.
func (n *NavigationPanel) resplitDead() {
	all := make([]PlanDisplay, 0, len(n.deadPlans)+len(n.promotedPlans)+len(n.historyPlans))
	all = append(all, n.deadPlans...)
	all = append(all, n.promotedPlans...)
	all = append(all, n.historyPlans...)
	n.splitDeadFromHistory(all)
}

// InspectPlan moves a history plan into the dead section for manual inspection.
func (n *NavigationPanel) InspectPlan(planFile string) {
	if n.inspectedPlans == nil {
		n.inspectedPlans = make(map[string]bool)
	}
	n.inspectedPlans[planFile] = true
	n.resplitDead()
	n.deadExpanded = true
	n.rebuildRows()
}

func (n *NavigationPanel) rebuildRows() {
	prevID := ""
	prevIdx := n.selectedIdx
	if prevIdx >= 0 && prevIdx < len(n.rows) {
		prevID = n.rows[prevIdx].ID
	}

	instancesByPlan := make(map[string][]*session.Instance)
	var solo []*session.Instance
	for _, inst := range n.instances {
		if inst.PlanFile == "" {
			solo = append(solo, inst)
			continue
		}
		instancesByPlan[inst.PlanFile] = append(instancesByPlan[inst.PlanFile], inst)
	}

	sortInstances := func(list []*session.Instance) {
		sort.SliceStable(list, func(i, j int) bool {
			ki := navInstanceSortKey(list[i])
			kj := navInstanceSortKey(list[j])
			if ki != kj {
				return ki < kj
			}
			return strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
		})
	}
	for _, list := range instancesByPlan {
		sortInstances(list)
	}
	sortInstances(solo)

	plans := append([]PlanDisplay(nil), n.plans...)
	sort.SliceStable(plans, func(i, j int) bool {
		pi, pj := plans[i], plans[j]
		ki := navPlanSortKey(pi, instancesByPlan[pi.Filename], n.planStatuses[pi.Filename])
		kj := navPlanSortKey(pj, instancesByPlan[pj.Filename], n.planStatuses[pj.Filename])
		if ki != kj {
			return ki < kj
		}
		return strings.ToLower(planstate.DisplayName(pi.Filename)) < strings.ToLower(planstate.DisplayName(pj.Filename))
	})

	rows := make([]navRow, 0, len(plans)+len(n.instances)+len(n.deadPlans)+len(n.historyPlans)+len(n.cancelled)+6)

	// Helper to append a plan header + its child instances.
	// indent is the indentation level in spaces for the plan row.
	appendPlan := func(p PlanDisplay, indent int) {
		insts := instancesByPlan[p.Filename]
		hasRunning, hasNotification := aggregateNavPlanStatus(insts, n.planStatuses[p.Filename])
		collapsed := n.isPlanCollapsed(p.Filename, hasRunning, hasNotification)
		rows = append(rows, navRow{
			Kind:            navRowPlanHeader,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           planstate.DisplayName(p.Filename),
			PlanFile:        p.Filename,
			PlanStatus:      p.Status,
			Collapsed:       collapsed,
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
			Indent:          indent,
		})
		if !collapsed {
			for _, inst := range insts {
				rows = append(rows, navRow{
					Kind:     navRowInstance,
					ID:       "inst:" + inst.Title,
					Label:    inst.Title,
					PlanFile: inst.PlanFile,
					Instance: inst,
					Indent:   indent,
				})
			}
		}
	}

	// Import action pinned at the top of the list, below the search bar.
	if n.clickUpAvail {
		rows = append(rows, navRow{Kind: navRowImportAction, ID: SidebarImportClickUp, Label: "+ import from clickup"})
	}

	// Dead section: done plans with instances or manually inspected.
	// Shown above active plans so they're accessible for cleanup.
	if len(n.deadPlans) > 0 {
		rows = append(rows, navRow{Kind: navRowDeadToggle, ID: "__dead_toggle__", Label: "dead", Collapsed: !n.deadExpanded})
		if n.deadExpanded {
			for _, p := range n.deadPlans {
				appendPlan(p, 2)
			}
		}
	}

	// Split plans into active (sort key 0,1) and idle (sort key 2).
	var activePlans, idlePlans []PlanDisplay
	for _, p := range plans {
		sk := navPlanSortKey(p, instancesByPlan[p.Filename], n.planStatuses[p.Filename])
		if sk < 2 {
			activePlans = append(activePlans, p)
		} else {
			idlePlans = append(idlePlans, p)
		}
	}

	// Emit active plans flat.
	for _, p := range activePlans {
		appendPlan(p, 0)
	}

	// Solo instances between active and idle.
	if len(solo) > 0 {
		rows = append(rows, navRow{Kind: navRowSoloHeader, ID: "__solo__", Label: "agents"})
		for _, inst := range solo {
			rows = append(rows, navRow{Kind: navRowInstance, ID: "inst:" + inst.Title, Label: inst.Title, Instance: inst})
		}
	}

	// Idle plans grouped by topic.
	// Build a set of idle filenames for quick lookup.
	idleSet := make(map[string]bool, len(idlePlans))
	for _, p := range idlePlans {
		idleSet[p.Filename] = true
	}

	// Emit topics that contain idle plans.
	emitted := make(map[string]bool)
	for _, t := range n.topics {
		var topicIdlePlans []PlanDisplay
		for _, p := range t.Plans {
			if idleSet[p.Filename] {
				topicIdlePlans = append(topicIdlePlans, p)
			}
		}
		if len(topicIdlePlans) == 0 {
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
		if !collapsed {
			for _, p := range topicIdlePlans {
				appendPlan(p, 2)
				emitted[p.Filename] = true
			}
		} else {
			for _, p := range topicIdlePlans {
				emitted[p.Filename] = true
			}
		}
	}

	// Emit ungrouped idle plans (no topic).
	for _, p := range idlePlans {
		if !emitted[p.Filename] {
			appendPlan(p, 0)
		}
	}

	if len(n.historyPlans) > 0 {
		rows = append(rows, navRow{Kind: navRowHistoryToggle, ID: SidebarPlanHistoryToggle, Label: "history", Collapsed: !n.historyExpanded})
		if n.historyExpanded {
			for _, p := range n.historyPlans {
				rows = append(rows, navRow{Kind: navRowHistoryPlan, ID: SidebarPlanPrefix + p.Filename, Label: planstate.DisplayName(p.Filename), PlanFile: p.Filename})
			}
		}
	}

	for _, p := range n.cancelled {
		rows = append(rows, navRow{Kind: navRowCancelled, ID: SidebarPlanPrefix + p.Filename, Label: planstate.DisplayName(p.Filename), PlanFile: p.Filename})
	}

	n.rows = rows
	if len(rows) == 0 {
		n.selectedIdx = 0
		n.scrollOffset = 0
		return
	}
	if prevID != "" {
		for i, row := range rows {
			if row.ID == prevID {
				n.selectedIdx = i
				n.clampScroll()
				return
			}
		}
		// prevID not found — the selected plan may have been transiently
		// absent from an async remote-store reload. Clamp to the same
		// numeric position so the selection doesn't jump to a random row.
		// Skip non-selectable divider rows.
		if prevIdx >= len(rows) {
			prevIdx = len(rows) - 1
		}
		if prevIdx < 0 {
			prevIdx = 0
		}
		n.selectedIdx = prevIdx
		// If we landed on a non-selectable divider, nudge down then up.
		if n.selectedIdx < len(rows) && rows[n.selectedIdx].Kind == navRowSoloHeader {
			if n.selectedIdx+1 < len(rows) {
				n.selectedIdx++
			} else if n.selectedIdx > 0 {
				n.selectedIdx--
			}
		}
		n.clampScroll()
		return
	}
	if n.selectedIdx >= len(rows) {
		n.selectedIdx = len(rows) - 1
	}
	if n.selectedIdx < 0 {
		n.selectedIdx = 0
	}
	n.clampScroll()
}

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
	// Ready / unknown
	return 3
}

func navPlanSortKey(p PlanDisplay, insts []*session.Instance, st TopicStatus) int {
	hasNotification := st.HasNotification
	hasRunning := st.HasRunning
	for _, inst := range insts {
		if inst.Notified {
			hasNotification = true
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasRunning = true
		}
	}
	if hasNotification {
		return 0
	}
	if hasRunning {
		return 1
	}
	// Plans in active lifecycle states (implementing, reviewing) should
	// appear in the "active" section even without running instances —
	// e.g. after a restart when the agent's tmux session is gone.
	if p.Status == "implementing" || p.Status == "reviewing" {
		return 1
	}
	return 2
}

func aggregateNavPlanStatus(insts []*session.Instance, st TopicStatus) (bool, bool) {
	hasRunning := st.HasRunning
	hasNotification := st.HasNotification
	for _, inst := range insts {
		if inst.Notified {
			hasNotification = true
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasRunning = true
		}
	}
	return hasRunning, hasNotification
}

func (n *NavigationPanel) isPlanCollapsed(planFile string, hasRunning, hasNotification bool) bool {
	if _, ok := n.userOverrides[planFile]; ok {
		return n.collapsed[planFile]
	}
	return !hasRunning && !hasNotification
}

func (n *NavigationPanel) SetSize(width, height int) {
	n.width, n.height = width, height
	n.clampScroll()
}

// SetAuditView sets pre-rendered audit pane content to display inside the border.
func (n *NavigationPanel) SetAuditView(view string, h int) {
	n.auditView = view
	n.auditHeight = h
}

func (n *NavigationPanel) SetFocused(focused bool)    { n.focused = focused }
func (n *NavigationPanel) IsFocused() bool            { return n.focused }
func (n *NavigationPanel) SetRepoName(name string)    { n.repoName = name }
func (n *NavigationPanel) SetRepoHovered(h bool)      { n.repoHovered = h }
func (n *NavigationPanel) SetClickUpAvailable(a bool) { n.clickUpAvail = a; n.rebuildRows() }

func (n *NavigationPanel) ActivateSearch()        { n.searchActive = true; n.searchQuery = "" }
func (n *NavigationPanel) DeactivateSearch()      { n.searchActive = false; n.searchQuery = "" }
func (n *NavigationPanel) IsSearchActive() bool   { return n.searchActive }
func (n *NavigationPanel) GetSearchQuery() string { return n.searchQuery }
func (n *NavigationPanel) SetSearchQuery(q string) {
	n.searchQuery = q
	// If the current selection is hidden by the new filter, snap to the first visible row.
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

func (n *NavigationPanel) ToggleSelectedExpand() bool {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return false
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		n.collapsed[row.PlanFile] = !row.Collapsed
		n.userOverrides[row.PlanFile] = true
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

func (n *NavigationPanel) Up() {
	orig := n.selectedIdx
	for n.selectedIdx > 0 {
		n.selectedIdx--
		if n.rows[n.selectedIdx].Kind == navRowSoloHeader {
			continue // skip non-selectable divider
		}
		if !n.rowMatchesSearch(n.selectedIdx) {
			continue // skip rows hidden by search filter
		}
		n.clampScroll()
		return
	}
	// No valid row found above — revert.
	n.selectedIdx = orig
}

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

// rowMatchesSearch returns true if the row at idx is visible under the current
// search filter. Always true when search is inactive or query is empty.
func (n *NavigationPanel) rowMatchesSearch(idx int) bool {
	if !n.searchActive || n.searchQuery == "" {
		return true
	}
	row := n.rows[idx]
	q := strings.ToLower(n.searchQuery)
	return strings.Contains(strings.ToLower(row.Label), q) ||
		strings.Contains(strings.ToLower(row.PlanFile), q)
}

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
	return n.rows[n.selectedIdx].PlanFile
}

func (n *NavigationPanel) IsSelectedPlanHeader() bool {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return false
	}
	k := n.rows[n.selectedIdx].Kind
	return k == navRowPlanHeader || k == navRowHistoryPlan || k == navRowCancelled
}

// IsSelectedHistoryPlan returns true if the selected row is a history plan entry.
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

func (n *NavigationPanel) ClickItem(row int) {
	if row >= 0 && row < len(n.rows) {
		n.selectedIdx = row
		n.clampScroll()
	}
}

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

func (n *NavigationPanel) SelectInstance(inst *session.Instance) bool {
	for i, row := range n.rows {
		if row.Instance == inst {
			n.selectedIdx = i
			n.clampScroll()
			return true
		}
	}
	// Instance not visible — may be under a collapsed plan. Expand it.
	if inst.PlanFile != "" {
		n.collapsed[inst.PlanFile] = false
		n.userOverrides[inst.PlanFile] = true
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

func (n *NavigationPanel) CycleNextActive() {
	n.cycleActive(1)
}

func (n *NavigationPanel) CyclePrevActive() {
	n.cycleActive(-1)
}

func (n *NavigationPanel) cycleActive(step int) {
	// Build the cycle list in visual (top-to-bottom) order so Ctrl+Up/Down
	// follows the on-screen layout across attention/active/solo sections.
	// For collapsed plan headers, insert their hidden instances at the
	// header's position so cycling can auto-expand them.
	var ordered []*session.Instance
	for _, row := range n.rows {
		switch {
		case row.Kind == navRowInstance && row.Instance != nil:
			if !row.Instance.Paused() {
				ordered = append(ordered, row.Instance)
			}
		case row.Kind == navRowPlanHeader && row.Collapsed:
			// Plan is collapsed — its instances aren't in rows.
			// Insert them here so they appear at the right visual position.
			for _, inst := range n.instances {
				if inst.PlanFile == row.PlanFile && !inst.Paused() {
					ordered = append(ordered, inst)
				}
			}
		}
	}
	if len(ordered) == 0 {
		return
	}

	// Find current position. Match by instance pointer from the selected row.
	cur := -1
	sel := n.GetSelectedInstance()
	if sel != nil {
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

func (n *NavigationPanel) GetInstances() []*session.Instance { return n.instances }
func (n *NavigationPanel) TotalInstances() int               { return len(n.instances) }
func (n *NavigationPanel) NumInstances() int                 { return len(n.instances) }

func (n *NavigationPanel) AddInstance(inst *session.Instance) func() {
	n.instances = append(n.instances, inst)
	n.resplitDead()
	n.rebuildRows()
	return func() {}
}

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

func (n *NavigationPanel) Remove() {
	inst := n.GetSelectedInstance()
	if inst != nil {
		n.RemoveByTitle(inst.Title)
	}
}

func (n *NavigationPanel) Kill() {
	inst := n.GetSelectedInstance()
	if inst != nil {
		_ = inst.Kill()
		n.RemoveByTitle(inst.Title)
	}
}

func (n *NavigationPanel) Attach() (chan struct{}, error) {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return nil, fmt.Errorf("no instance selected")
	}
	return inst.Attach()
}

func (n *NavigationPanel) Clear() {
	n.instances = nil
	n.rows = nil
	n.selectedIdx = 0
	n.scrollOffset = 0
}

func (n *NavigationPanel) SetSessionPreviewSize(width, height int) error {
	var err error
	for _, item := range n.instances {
		if !item.Started() || item.Paused() {
			continue
		}
		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = innerErr
		}
	}
	return err
}

func (n *NavigationPanel) SelectFirst() {
	if len(n.rows) > 0 {
		n.selectedIdx = 0
		// Skip non-selectable divider rows.
		if n.rows[0].Kind == navRowSoloHeader && len(n.rows) > 1 {
			n.selectedIdx = 1
		}
		n.clampScroll()
	}
}

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

func (n *NavigationPanel) availRows() int {
	avail := n.height - 8
	if avail < 1 {
		return 1
	}
	return avail
}

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

// FindPlanInstance returns the best interactive candidate for a plan.
// Priority: running > any non-paused instance. The caller should check
// Started()/Paused() on the returned instance before entering focus mode.
func (n *NavigationPanel) FindPlanInstance(planFile string) *session.Instance {
	var best *session.Instance
	for _, inst := range n.instances {
		if inst.PlanFile != planFile || inst.Paused() {
			continue
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			return inst
		}
		if best == nil {
			best = inst
		}
	}
	return best
}

// navInstanceTitle returns a clean display title for an instance in the sidebar.
func navInstanceTitle(inst *session.Instance) string {
	switch {
	case inst.WaveNumber > 0 && inst.TaskNumber > 0:
		return fmt.Sprintf("wave %d · task %d", inst.WaveNumber, inst.TaskNumber)
	case inst.AgentType == session.AgentTypeReviewer && inst.PlanFile != "":
		return "review"
	case inst.AgentType == session.AgentTypePlanner && inst.PlanFile != "":
		return "planning"
	case inst.SoloAgent && inst.PlanFile != "":
		return planstate.DisplayName(inst.PlanFile)
	case inst.AgentType == session.AgentTypeCoder && inst.PlanFile != "" && inst.WaveNumber == 0:
		return "applying fixes"
	default:
		return inst.Title
	}
}

// navInstanceStatusIcon returns a styled status glyph for an instance.
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
	if row.HasNotification {
		return navNotifyIconStyle.Render("◉")
	}
	if row.HasRunning {
		return navRunningIconStyle.Render("●")
	}
	return navIdleIconStyle.Render("○")
}

// navSectionLabel returns a lowercase section label for a plan sort key.
func navSectionLabel(key int) string {
	switch key {
	case 0:
		return "attention"
	case 1:
		return "active"
	default:
		return "plans"
	}
}

// navDividerLine builds a full-width rule like "──── label ────" spanning w cells.
func navDividerLine(label string, w int) string {
	inner := " " + label + " "
	innerW := runewidth.StringWidth(inner)
	remaining := w - innerW
	if remaining < 2 {
		return navHistoryDivStyle.Render(inner)
	}
	left := remaining / 2
	right := remaining - left
	return navHistoryDivStyle.Render(strings.Repeat("─", left) + inner + strings.Repeat("─", right))
}

// renderNavRow renders a single row's content (without selection styling).
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
		// Layout: indent + chevron(1) + space(1) + label + gap + space(1) + status
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
		// Plans under topics use normal weight; top-level plans use bold.
		labelStyle := navPlanLabelStyle
		if row.Indent > 0 {
			labelStyle = navPlanLabelStyle.Bold(false)
		}
		return indent + chevron + " " + labelStyle.Render(label) + strings.Repeat(" ", gap) + " " + statusIcon

	case navRowInstance:
		inst := row.Instance
		isSolo := row.PlanFile == ""

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
		// Layout: indent + title + gap + space(1) + status
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
		labelStyle := navInstanceLabelStyle
		if isSolo {
			indent = ""
			labelStyle = navPlanLabelStyle
		}
		// Dead instances: grey + strikethrough.
		if inst.Exited {
			labelStyle = navCancelledLblStyle
			statusIcon = navCancelledLblStyle.Render("✕")
			statusW = lipgloss.Width(statusIcon)
			// Recompute gap with updated status width.
			usedW = indentW + runewidth.StringWidth(title) + 1 + statusW
			gap = contentWidth - usedW
			if gap < 0 {
				gap = 0
			}
		}
		return indent + labelStyle.Render(title) + strings.Repeat(" ", gap) + " " + statusIcon

	case navRowSoloHeader:
		return navDividerLine("agents", contentWidth)

	case navRowTopicHeader:
		chevron := "▸"
		if !row.Collapsed {
			chevron = "▾"
		}
		label := row.Label
		maxLabel := contentWidth - 2 // chevron + space
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

func (n *NavigationPanel) String() string {
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1)
	if n.focused {
		border = border.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	}
	innerWidth := n.width - 4
	if innerWidth < 8 {
		innerWidth = 8
	}
	height := n.height - 2
	if height < 4 {
		height = 4
	}

	itemWidth := innerWidth - 2   // border has Padding(0,1) → content area is 2 chars narrower
	contentWidth := itemWidth - 2 // account for Padding(0,1) in row styles
	if contentWidth < 4 {
		contentWidth = 4
	}

	// Search bar
	searchWidth := innerWidth - 4
	if searchWidth < 4 {
		searchWidth = 4
	}
	var searchBox string
	if n.searchActive {
		searchText := n.searchQuery
		if searchText == "" {
			searchText = " "
		}
		searchBox = zone.Mark(ZoneNavSearch, navSearchActiveStyle.Width(searchWidth).Render(searchText))
	} else {
		searchBox = zone.Mark(ZoneNavSearch, navSearchBoxStyle.Width(searchWidth).Render("\uf002 search"))
	}

	// Build visible items, including section dividers between plan sort-key groups.
	type visItem struct {
		line   string
		rowIdx int // -1 for section dividers
	}
	items := make([]visItem, 0, len(n.rows)+4)
	selectedDisplayIdx := 0
	lastPlanKey := -1
	inDeadSection := false

	for i, row := range n.rows {
		// Search filter
		if n.searchActive && n.searchQuery != "" {
			q := strings.ToLower(n.searchQuery)
			if !strings.Contains(strings.ToLower(row.Label), q) &&
				!strings.Contains(strings.ToLower(row.PlanFile), q) {
				continue
			}
		}

		// Track dead section boundaries to suppress plan-group dividers.
		if row.Kind == navRowDeadToggle {
			inDeadSection = true
		} else if row.Kind != navRowPlanHeader && row.Kind != navRowInstance {
			inDeadSection = false
		}

		// Insert section dividers between plan sort-key groups
		if row.Kind == navRowPlanHeader && !inDeadSection {
			sk := 2
			if row.HasNotification {
				sk = 0
			} else if row.HasRunning {
				sk = 1
			} else if row.PlanStatus == "implementing" || row.PlanStatus == "reviewing" {
				sk = 1
			}
			if sk != lastPlanKey {
				label := navSectionLabel(sk)
				divLine := navDividerLine(label, itemWidth)
				items = append(items, visItem{line: divLine, rowIdx: -1})
				lastPlanKey = sk
			}
		}
		// Topic headers are always idle — ensure the idle divider appears.
		if row.Kind == navRowTopicHeader && lastPlanKey != 2 {
			divLine := navDividerLine(navSectionLabel(2), itemWidth)
			items = append(items, visItem{line: divLine, rowIdx: -1})
			lastPlanKey = 2
		}

		if i == n.selectedIdx {
			selectedDisplayIdx = len(items)
		}

		// Render the row content
		rawLine := n.renderNavRow(row, contentWidth)

		// Apply selection highlighting
		isSelected := i == n.selectedIdx
		var styledLine string
		if isSelected && n.focused {
			styledLine = navSelectedRowStyle.Width(itemWidth).Render(ansi.Strip(rawLine))
		} else if isSelected {
			styledLine = navActiveRowStyle.Width(itemWidth).Render(ansi.Strip(rawLine))
		} else if row.Kind == navRowImportAction {
			styledLine = navImportStyle.Width(itemWidth).Render(rawLine)
		} else {
			styledLine = navItemStyle.Width(itemWidth).Render(rawLine)
		}

		items = append(items, visItem{line: styledLine, rowIdx: i})
	}

	// Scroll window — keep the selected item visible
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

	// Legend — status icon key, centered in the pane
	legendContent := navIdleIconStyle.Render("○") + navLegendLabelStyle.Render(" idle") +
		"  " + navRunningIconStyle.Render("●") + navLegendLabelStyle.Render(" running") +
		"  " + navNotifyIconStyle.Render("◉") + navLegendLabelStyle.Render(" review") +
		"  " + navCompletedIconStyle.Render("●") + navLegendLabelStyle.Render(" done")
	legend := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(legendContent)

	// Repo switcher
	var repoSection string
	if n.repoName != "" {
		btnWidth := innerWidth - 4
		if btnWidth < 4 {
			btnWidth = 4
		}
		repoBtn := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorOverlay).
			Padding(0, 1).
			Width(btnWidth).
			Render(n.repoName + " ▾")
		repoSection = zone.Mark(ZoneNavRepo, repoBtn)
	}

	// Assemble content: list on top, legend + log pinned to bottom.
	topContent := searchBox + "\n" + body

	// Legend sits between history and the log divider.
	var legendSection string
	if repoSection != "" {
		legendSection = legend + "\n" + repoSection
	} else {
		legendSection = legend
	}

	topLines := strings.Count(topContent, "\n") + 1
	legendLines := strings.Count(legendSection, "\n") + 1

	// Determine how many lines the audit section can use without overflowing.
	// lipgloss .Height() doesn't truncate overflow, so we must never exceed height.
	maxAudit := height - topLines - legendLines // leaves gap=1 at this limit
	actualAudit := 0
	auditSection := ""
	if n.auditView != "" && n.auditHeight > 0 && maxAudit >= 3 {
		actualAudit = n.auditHeight
		if actualAudit > maxAudit {
			actualAudit = maxAudit
		}
		// Keep the last actualAudit lines (newest events are at the bottom).
		vlines := strings.Split(n.auditView, "\n")
		if len(vlines) > actualAudit {
			vlines = vlines[len(vlines)-actualAudit:]
		}
		auditSection = strings.Join(vlines, "\n")
	}

	gap := height - topLines - legendLines - actualAudit + 1
	if gap < 1 {
		gap = 1
	}

	// Order: list … gap … legend … log
	innerContent := topContent + strings.Repeat("\n", gap) + legendSection
	if auditSection != "" {
		innerContent += "\n" + auditSection
	}

	bordered := border.Width(innerWidth).Height(height).Render(innerContent)
	placed := lipgloss.Place(n.width, n.height, lipgloss.Left, lipgloss.Top, bordered)
	return zone.Mark(ZoneNavPanel, placed)
}

// RowCount returns the number of rows in the navigation panel rows slice.
// Used by zone-based mouse hit detection to iterate row zones.
func (n *NavigationPanel) RowCount() int { return len(n.rows) }
