package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone"
)

const ZoneRepoSwitch = "repo-switch"

const (
	SidebarAll               = "__all__"
	SidebarUngrouped         = "__ungrouped__"
	SidebarPlanPrefix        = "__plan__"
	SidebarTopicPrefix       = "__topic__"
	SidebarPlanStagePrefix   = "__plan_stage__"
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

type StatusFilter int

const (
	StatusFilterAll StatusFilter = iota
	StatusFilterActive
)

type SortMode int

const (
	SortNewest SortMode = iota
	SortOldest
	SortName
	SortStatus
)

type navRowKind int

const (
	navRowPlanHeader navRowKind = iota
	navRowInstance
	navRowSoloHeader
	navRowImportAction
	navRowHistoryToggle
	navRowHistoryPlan
	navRowCancelled
)

type navRow struct {
	Kind            navRowKind
	ID              string
	Label           string
	PlanFile        string
	Instance        *session.Instance
	Collapsed       bool
	HasRunning      bool
	HasNotification bool
}

type NavigationPanel struct {
	spinner *spinner.Model

	rows         []navRow
	selectedIdx  int
	scrollOffset int

	plans         []PlanDisplay
	topics        []TopicDisplay
	ungrouped     []PlanDisplay
	instances     []*session.Instance
	historyPlans  []PlanDisplay
	cancelled     []PlanDisplay
	planStatuses  map[string]TopicStatus
	collapsed     map[string]bool
	userOverrides map[string]bool

	historyExpanded bool
	searchActive    bool
	searchQuery     string
	clickUpAvail    bool

	repoName    string
	repoHovered bool

	width, height int
	focused       bool

	statusFilter StatusFilter
	sortMode     SortMode
}

type TopicDisplay struct {
	Name  string
	Plans []PlanDisplay
}

func NewNavigationPanel(sp *spinner.Model) *NavigationPanel {
	return &NavigationPanel{
		spinner:       sp,
		planStatuses:  make(map[string]TopicStatus),
		collapsed:     make(map[string]bool),
		userOverrides: make(map[string]bool),
		focused:       true,
	}
}

func (n *NavigationPanel) SetData(plans []PlanDisplay, instances []*session.Instance, history []PlanDisplay, cancelled []PlanDisplay, planStatuses map[string]TopicStatus) {
	n.plans = plans
	n.instances = instances
	n.historyPlans = history
	n.cancelled = cancelled
	if planStatuses == nil {
		n.planStatuses = make(map[string]TopicStatus)
	} else {
		n.planStatuses = planStatuses
	}
	n.rebuildRows()
}

func (n *NavigationPanel) SetPlans(plans []PlanDisplay) {
	n.plans = plans
	n.rebuildRows()
}

func (n *NavigationPanel) SetTopicsAndPlans(topics []TopicDisplay, ungrouped []PlanDisplay, history []PlanDisplay, cancelled ...[]PlanDisplay) {
	n.topics = topics
	n.ungrouped = ungrouped
	n.historyPlans = history
	if len(cancelled) > 0 {
		n.cancelled = cancelled[0]
	}
	plans := make([]PlanDisplay, 0, len(ungrouped))
	plans = append(plans, ungrouped...)
	for _, t := range topics {
		plans = append(plans, t.Plans...)
	}
	n.plans = plans
	n.rebuildRows()
}

func (n *NavigationPanel) SetItems(_ []string, _ map[string]int, _ int, _ map[string]bool, _ map[string]TopicStatus, planStatuses map[string]TopicStatus) {
	if planStatuses != nil {
		n.planStatuses = planStatuses
	}
	n.rebuildRows()
}

func (n *NavigationPanel) rebuildRows() {
	prevID := ""
	if n.selectedIdx >= 0 && n.selectedIdx < len(n.rows) {
		prevID = n.rows[n.selectedIdx].ID
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

	rows := make([]navRow, 0, len(plans)+len(n.instances)+len(n.historyPlans)+len(n.cancelled)+4)
	for _, p := range plans {
		insts := instancesByPlan[p.Filename]
		hasRunning, hasNotification := aggregateNavPlanStatus(insts, n.planStatuses[p.Filename])
		collapsed := n.isPlanCollapsed(p.Filename, hasRunning, hasNotification)
		rows = append(rows, navRow{
			Kind:            navRowPlanHeader,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           planstate.DisplayName(p.Filename),
			PlanFile:        p.Filename,
			Collapsed:       collapsed,
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
		})
		if !collapsed {
			for _, inst := range insts {
				rows = append(rows, navRow{
					Kind:     navRowInstance,
					ID:       "inst:" + inst.Title,
					Label:    inst.Title,
					PlanFile: inst.PlanFile,
					Instance: inst,
				})
			}
		}
	}

	if len(solo) > 0 {
		rows = append(rows, navRow{Kind: navRowSoloHeader, ID: "__solo__", Label: "solo"})
		for _, inst := range solo {
			rows = append(rows, navRow{Kind: navRowInstance, ID: "inst:" + inst.Title, Label: inst.Title, Instance: inst})
		}
	}

	if n.clickUpAvail {
		rows = append(rows, navRow{Kind: navRowImportAction, ID: SidebarImportClickUp, Label: "+ import from clickup"})
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
	if inst.Notified {
		return 0
	}
	if inst.ImplementationComplete {
		return 4
	}
	switch inst.Status {
	case session.Running, session.Loading:
		return 1
	case session.Ready:
		return 2
	case session.Paused:
		return 3
	default:
		return 5
	}
}

func navPlanSortKey(_ PlanDisplay, insts []*session.Instance, st TopicStatus) int {
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
func (n *NavigationPanel) SetFocused(focused bool)    { n.focused = focused }
func (n *NavigationPanel) IsFocused() bool            { return n.focused }
func (n *NavigationPanel) SetRepoName(name string)    { n.repoName = name }
func (n *NavigationPanel) SetRepoHovered(h bool)      { n.repoHovered = h }
func (n *NavigationPanel) SetClickUpAvailable(a bool) { n.clickUpAvail = a; n.rebuildRows() }

func (n *NavigationPanel) ActivateSearch()         { n.searchActive = true; n.searchQuery = "" }
func (n *NavigationPanel) DeactivateSearch()       { n.searchActive = false; n.searchQuery = "" }
func (n *NavigationPanel) IsSearchActive() bool    { return n.searchActive }
func (n *NavigationPanel) GetSearchQuery() string  { return n.searchQuery }
func (n *NavigationPanel) SetSearchQuery(q string) { n.searchQuery = q }

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
	case navRowHistoryToggle:
		n.historyExpanded = !n.historyExpanded
		n.rebuildRows()
		return true
	default:
		return false
	}
}

func (n *NavigationPanel) Up() {
	if n.selectedIdx > 0 {
		n.selectedIdx--
		n.clampScroll()
	}
}

func (n *NavigationPanel) Down() {
	if n.selectedIdx+1 < len(n.rows) {
		n.selectedIdx++
		n.clampScroll()
	}
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
	if len(n.instances) == 0 {
		return
	}
	start := n.SelectedIndex()
	for i := 1; i <= len(n.instances); i++ {
		idx := (start + i*step + len(n.instances)*2) % len(n.instances)
		if !n.instances[idx].Paused() {
			n.SelectInstance(n.instances[idx])
			return
		}
	}
}

func (n *NavigationPanel) GetInstances() []*session.Instance { return n.instances }
func (n *NavigationPanel) TotalInstances() int               { return len(n.instances) }
func (n *NavigationPanel) NumInstances() int                 { return len(n.instances) }

func (n *NavigationPanel) AddInstance(inst *session.Instance) func() {
	n.instances = append(n.instances, inst)
	n.rebuildRows()
	return func() {}
}

func (n *NavigationPanel) RemoveByTitle(title string) *session.Instance {
	for i, inst := range n.instances {
		if inst.Title == title {
			n.instances = append(n.instances[:i], n.instances[i+1:]...)
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

func (n *NavigationPanel) GetSelectedPlanStage() (planFile, stage string, ok bool) {
	return "", "", false
}

func (n *NavigationPanel) IsSelectedTopicHeader() bool               { return false }
func (n *NavigationPanel) GetSelectedTopicName() string              { return "" }
func (n *NavigationPanel) UpdateMatchCounts(_ map[string]int, _ int) {}
func (n *NavigationPanel) SelectFirst() {
	if len(n.rows) > 0 {
		n.selectedIdx = 0
		n.clampScroll()
	}
}
func (n *NavigationPanel) IsTreeMode() bool { return true }

func (n *NavigationPanel) SetFilter(_ string)                   {}
func (n *NavigationPanel) SetHighlightFilter(_, _ string)       {}
func (n *NavigationPanel) SetSearchFilter(_ string)             {}
func (n *NavigationPanel) SetSearchFilterWithTopic(_, _ string) {}
func (n *NavigationPanel) HandleTabClick(_, _ int) (StatusFilter, bool) {
	return 0, false
}
func (n *NavigationPanel) SetStatusFilter(filter StatusFilter) { n.statusFilter = filter }
func (n *NavigationPanel) GetStatusFilter() StatusFilter       { return n.statusFilter }
func (n *NavigationPanel) CycleSortMode()                      { n.sortMode = (n.sortMode + 1) % 4 }
func (n *NavigationPanel) GetSortMode() SortMode               { return n.sortMode }
func (n *NavigationPanel) GetItemAtRow(row int) int            { return row + n.scrollOffset }

func (n *NavigationPanel) SelectedSpaceAction() string {
	if n.selectedIdx < 0 || n.selectedIdx >= len(n.rows) {
		return "toggle"
	}
	switch n.rows[n.selectedIdx].Kind {
	case navRowPlanHeader, navRowHistoryToggle:
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

	search := "search"
	if n.searchActive {
		search = n.searchQuery
		if search == "" {
			search = " "
		}
	}
	searchBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1).Width(innerWidth - 4).Render(search)

	visible := make([]string, 0, len(n.rows))
	for i, row := range n.rows {
		if n.searchActive && n.searchQuery != "" {
			q := strings.ToLower(n.searchQuery)
			if !strings.Contains(strings.ToLower(row.Label), q) && !strings.Contains(strings.ToLower(row.PlanFile), q) {
				continue
			}
		}
		prefix := "  "
		if i == n.selectedIdx {
			prefix = "▸ "
		}
		line := row.Label
		switch row.Kind {
		case navRowPlanHeader:
			ch := "▸"
			if !row.Collapsed {
				ch = "▾"
			}
			line = ch + " " + row.Label
		case navRowInstance:
			line = "  " + row.Label
		case navRowSoloHeader:
			line = "-- solo --"
		case navRowHistoryToggle:
			ch := "▸"
			if !row.Collapsed {
				ch = "▾"
			}
			line = "-- " + ch + " history --"
		}
		visible = append(visible, prefix+line)
	}

	start := n.scrollOffset
	if start > len(visible) {
		start = len(visible)
	}
	end := start + n.availRows()
	if end > len(visible) {
		end = len(visible)
	}
	body := strings.Join(visible[start:end], "\n")
	if body != "" {
		body += "\n"
	}

	repoLabel := n.repoName
	if repoLabel != "" {
		repoLabel = zone.Mark(ZoneRepoSwitch, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1).Width(innerWidth-4).Render(repoLabel+" ▾"))
	}

	content := searchBox + "\n\n" + body + "\n" + repoLabel
	return lipgloss.Place(n.width, n.height, lipgloss.Left, lipgloss.Top, border.Width(innerWidth).Height(height).Render(content))
}
