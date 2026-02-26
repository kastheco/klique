# Redesign Instance List Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Merge sidebar + instance list into a unified NavigationPanel, creating a 2-column layout with plans/solo as top-level grouping and compact two-line instance rows.

**Architecture:** A new `NavigationPanel` component in `ui/` replaces both `Sidebar` and `List`. It owns tree state (plans → instances, solo section, history), renders all row kinds, and exposes a unified selection API. The app `View()` drops the middle column, giving the preview pane ~30-40% more horizontal space. CPU/memory stats move from instance rows to the info tab. The info tab gains a plan summary view shown when a plan header is selected.

**Tech Stack:** Go, bubbletea v1.3.x, lipgloss v1.1.x, bubbles (spinner)

**Size:** Medium (estimated ~2.5 hours, 3 tasks, 2 waves)

**Design doc:** `docs/plans/2026-02-26-redesign-instance-list-design.md`

---

## Wave 1: New Components

> Two independent tasks that build the NavigationPanel and extend the InfoPane. No app/ changes yet — everything is tested in isolation.

### Task 1: NavigationPanel Component

Build the complete `NavigationPanel` type that replaces both `Sidebar` and `List`.

**Files:**
- Create: `ui/nav_panel.go` — type definition, row model, tree rebuild, sorting, expand/collapse, scroll
- Create: `ui/nav_panel_render.go` — row renderers, styles, `String()` method, keyboard nav, mouse handling, search, selection API
- Create: `ui/nav_panel_test.go` — tests for tree rebuild, sorting, navigation, rendering

**Step 1: Define the row model and NavigationPanel type in `ui/nav_panel.go`**

```go
package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
)

// navRowKind identifies the type of a navigation panel row.
type navRowKind int

const (
	navRowPlanHeader    navRowKind = iota // collapsible plan header
	navRowInstance                        // two-line instance row under a plan
	navRowSoloHeader                      // "── solo ──" divider
	navRowSoloInstance                    // instance row under solo section
	navRowImportAction                    // "+ import from clickup"
	navRowHistoryToggle                   // "── ▸ history ──"
	navRowHistoryPlan                     // plan in the history section
	navRowCancelled                       // cancelled plan with strikethrough
)

// navRow is a single row in the navigation panel.
type navRow struct {
	Kind     navRowKind
	ID       string // unique identifier for selection persistence
	Label    string // display text
	PlanFile string // plan filename (for plan headers and instances under plans)

	// Instance reference (nil for non-instance rows)
	Instance *session.Instance

	// Plan header state
	Collapsed bool
	// Status aggregation for plan headers
	HasRunning      bool
	HasNotification bool

	// Visual
	Indent int // indent in characters (0, 2, 4)
}

// NavigationPanel is the unified left panel replacing Sidebar + List.
type NavigationPanel struct {
	rows        []navRow
	selectedIdx int
	height      int
	width       int
	focused     bool
	spinner     *spinner.Model

	// Data sources — set by the app layer
	plans     []PlanDisplay
	instances []*session.Instance

	// Tree state
	collapsedPlans map[string]bool // manually collapsed plans (default: auto based on activity)
	userOverrides  map[string]bool // tracks plans the user has manually toggled

	// Sections
	historyPlans   []PlanDisplay
	cancelledPlans []PlanDisplay
	clickUpAvail   bool

	// Search
	searchActive bool
	searchQuery  string

	// Scroll
	scrollOffset int

	// Repo indicator
	repoName    string
	repoHovered bool

	// Plan statuses from app layer (for aggregating running/notification state)
	planStatuses map[string]TopicStatus
}

func NewNavigationPanel(spinner *spinner.Model) *NavigationPanel {
	return &NavigationPanel{
		spinner:        spinner,
		collapsedPlans: make(map[string]bool),
		userOverrides:  make(map[string]bool),
		planStatuses:   make(map[string]TopicStatus),
	}
}
```

**Step 2: Implement `SetData` and `rebuildRows` with sort ordering**

The `SetData` method receives plans and instances from the app layer and triggers a rebuild. The rebuild groups instances under their parent plan, creates a solo section for ungrouped instances, and sorts by the priority order: notified → running → idle.

```go
// SetData updates the navigation panel with current plans and instances.
func (n *NavigationPanel) SetData(
	plans []PlanDisplay,
	instances []*session.Instance,
	history []PlanDisplay,
	cancelled []PlanDisplay,
	planStatuses map[string]TopicStatus,
) {
	n.plans = plans
	n.instances = instances
	n.historyPlans = history
	n.cancelledPlans = cancelled
	n.planStatuses = planStatuses
	n.rebuildRows()
}

func (n *NavigationPanel) rebuildRows() {
	prevID := ""
	if n.selectedIdx >= 0 && n.selectedIdx < len(n.rows) {
		prevID = n.rows[n.selectedIdx].ID
	}

	// Group instances by plan file
	instancesByPlan := make(map[string][]*session.Instance)
	var soloInstances []*session.Instance
	for _, inst := range n.instances {
		if inst.PlanFile != "" {
			instancesByPlan[inst.PlanFile] = append(instancesByPlan[inst.PlanFile], inst)
		} else {
			soloInstances = append(soloInstances, inst)
		}
	}

	// Sort instances within each group: notified → running → ready → paused → completed
	sortInstances := func(insts []*session.Instance) {
		sort.SliceStable(insts, func(i, j int) bool {
			return instanceSortKey(insts[i]) < instanceSortKey(insts[j])
		})
	}
	for _, insts := range instancesByPlan {
		sortInstances(insts)
	}
	sortInstances(soloInstances)

	// Sort plans: notified → running → idle, then by recent activity
	sortedPlans := make([]PlanDisplay, len(n.plans))
	copy(sortedPlans, n.plans)
	sort.SliceStable(sortedPlans, func(i, j int) bool {
		return planSortKey(sortedPlans[i], instancesByPlan, n.planStatuses) <
			planSortKey(sortedPlans[j], instancesByPlan, n.planStatuses)
	})

	var rows []navRow

	// Plans with their instances
	for _, p := range sortedPlans {
		planInsts := instancesByPlan[p.Filename]
		hasRunning, hasNotification := aggregatePlanStatus(planInsts, n.planStatuses[p.Filename])

		collapsed := n.isPlanCollapsed(p.Filename, hasRunning, hasNotification)

		rows = append(rows, navRow{
			Kind:            navRowPlanHeader,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           planstate.DisplayName(p.Filename),
			PlanFile:        p.Filename,
			Collapsed:       collapsed,
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
			Indent:          0,
		})

		if !collapsed {
			for _, inst := range planInsts {
				rows = append(rows, navRow{
					Kind:     navRowInstance,
					ID:       "inst:" + inst.Title,
					Label:    instanceTitle(inst),
					PlanFile: inst.PlanFile,
					Instance: inst,
					Indent:   2,
				})
			}
		}
	}

	// Solo section
	if len(soloInstances) > 0 {
		rows = append(rows, navRow{
			Kind:  navRowSoloHeader,
			ID:    "__solo__",
			Label: "solo",
		})
		for _, inst := range soloInstances {
			rows = append(rows, navRow{
				Kind:     navRowSoloInstance,
				ID:       "inst:" + inst.Title,
				Label:    inst.Title,
				Instance: inst,
				Indent:   2,
			})
		}
	}

	// Import action
	if n.clickUpAvail {
		rows = append(rows, navRow{
			Kind:  navRowImportAction,
			ID:    SidebarImportClickUp,
			Label: "+ import from clickup",
		})
	}

	// History
	if len(n.historyPlans) > 0 {
		rows = append(rows, navRow{
			Kind:      navRowHistoryToggle,
			ID:        SidebarPlanHistoryToggle,
			Label:     "History",
			Collapsed: !n.historyExpanded(),
		})
		// History plans shown when expanded (implement historyExpanded bool field)
	}

	// Cancelled
	for _, p := range n.cancelledPlans {
		rows = append(rows, navRow{
			Kind:     navRowCancelled,
			ID:       SidebarPlanPrefix + p.Filename,
			Label:    planstate.DisplayName(p.Filename),
			PlanFile: p.Filename,
		})
	}

	n.rows = rows

	// Restore selection
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
```

Add the helper functions for sorting and status aggregation:

```go
// instanceSortKey returns a sort key: notified=0, running=1, ready=2, paused=3, completed=4
func instanceSortKey(inst *session.Instance) int {
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
	}
	return 5
}

// planSortKey: plans with notifications first (0), then running (1), then idle (2)
func planSortKey(p PlanDisplay, instancesByPlan map[string][]*session.Instance, statuses map[string]TopicStatus) int {
	st := statuses[p.Filename]
	insts := instancesByPlan[p.Filename]
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

func aggregatePlanStatus(insts []*session.Instance, planSt TopicStatus) (hasRunning, hasNotification bool) {
	hasRunning = planSt.HasRunning
	hasNotification = planSt.HasNotification
	for _, inst := range insts {
		if inst.Notified {
			hasNotification = true
		}
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasRunning = true
		}
	}
	return
}

// isPlanCollapsed returns whether a plan should be collapsed.
// User overrides take precedence. Otherwise, plans with activity are expanded.
func (n *NavigationPanel) isPlanCollapsed(planFile string, hasRunning, hasNotification bool) bool {
	if _, ok := n.userOverrides[planFile]; ok {
		return n.collapsedPlans[planFile]
	}
	// Auto: expand if active, collapse if idle
	return !hasRunning && !hasNotification
}

// instanceTitle returns the display title for an instance row.
func instanceTitle(inst *session.Instance) string {
	switch {
	case inst.WaveNumber > 0 && inst.TaskNumber > 0:
		return fmt.Sprintf("wave %d · task %d", inst.WaveNumber, inst.TaskNumber)
	case inst.AgentType == session.AgentTypeReviewer && inst.PlanFile != "":
		return "review"
	case inst.SoloAgent && inst.PlanFile != "":
		return planstate.DisplayName(inst.PlanFile)
	case inst.AgentType == session.AgentTypeCoder && inst.PlanFile != "" && inst.WaveNumber == 0:
		return "applying fixes"
	default:
		return inst.Title
	}
}
```

Also implement the expand/collapse, scroll, size, focus, and selection methods:

```go
func (n *NavigationPanel) SetSize(width, height int) {
	n.width = width
	n.height = height
	n.clampScroll()
}

func (n *NavigationPanel) SetFocused(focused bool) { n.focused = focused }
func (n *NavigationPanel) IsFocused() bool          { return n.focused }
func (n *NavigationPanel) SetRepoName(name string)  { n.repoName = name }
func (n *NavigationPanel) SetRepoHovered(h bool)    { n.repoHovered = h }
func (n *NavigationPanel) SetClickUpAvailable(a bool) {
	n.clickUpAvail = a
	n.rebuildRows()
}

func (n *NavigationPanel) ActivateSearch()         { n.searchActive = true; n.searchQuery = "" }
func (n *NavigationPanel) DeactivateSearch()       { n.searchActive = false; n.searchQuery = "" }
func (n *NavigationPanel) IsSearchActive() bool    { return n.searchActive }
func (n *NavigationPanel) GetSearchQuery() string  { return n.searchQuery }
func (n *NavigationPanel) SetSearchQuery(q string) { n.searchQuery = q }

// ToggleSelectedExpand toggles expand/collapse on plan headers and history.
func (n *NavigationPanel) ToggleSelectedExpand() bool {
	if n.selectedIdx >= len(n.rows) {
		return false
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		n.collapsedPlans[row.PlanFile] = !n.collapsedPlans[row.PlanFile]
		n.userOverrides[row.PlanFile] = true
		n.rebuildRows()
		return true
	case navRowHistoryToggle:
		// toggle historyExpanded field
		n.rebuildRows()
		return true
	}
	return false
}

// Up moves selection up, skipping solo header dividers.
func (n *NavigationPanel) Up() {
	for i := n.selectedIdx - 1; i >= 0; i-- {
		if n.rows[i].Kind != navRowSoloHeader {
			n.selectedIdx = i
			n.clampScroll()
			return
		}
	}
}

// Down moves selection down, skipping solo header dividers.
func (n *NavigationPanel) Down() {
	for i := n.selectedIdx + 1; i < len(n.rows); i++ {
		if n.rows[i].Kind != navRowSoloHeader {
			n.selectedIdx = i
			n.clampScroll()
			return
		}
	}
}

// Left collapses or moves to parent.
func (n *NavigationPanel) Left() {
	if n.selectedIdx >= len(n.rows) {
		return
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		if !row.Collapsed {
			n.ToggleSelectedExpand()
		} else if n.selectedIdx > 0 {
			n.Up()
		}
	case navRowInstance:
		// Move to parent plan header
		for i := n.selectedIdx - 1; i >= 0; i-- {
			if n.rows[i].Kind == navRowPlanHeader {
				n.selectedIdx = i
				n.clampScroll()
				return
			}
		}
	case navRowSoloInstance:
		n.Up() // move up to solo header or above
	}
}

// Right expands or moves to first child.
func (n *NavigationPanel) Right() {
	if n.selectedIdx >= len(n.rows) {
		return
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader:
		if row.Collapsed {
			n.ToggleSelectedExpand()
		} else if n.selectedIdx+1 < len(n.rows) {
			n.Down()
		}
	case navRowHistoryToggle:
		if row.Collapsed {
			n.ToggleSelectedExpand()
		}
	}
}

// GetSelectedInstance returns the instance if an instance row is selected, or nil.
func (n *NavigationPanel) GetSelectedInstance() *session.Instance {
	if n.selectedIdx >= len(n.rows) {
		return nil
	}
	return n.rows[n.selectedIdx].Instance
}

// GetSelectedPlanFile returns the plan filename for the selected row.
// Works for plan headers and instance rows under a plan.
func (n *NavigationPanel) GetSelectedPlanFile() string {
	if n.selectedIdx >= len(n.rows) {
		return ""
	}
	return n.rows[n.selectedIdx].PlanFile
}

// IsSelectedPlanHeader returns true if a plan header is selected.
func (n *NavigationPanel) IsSelectedPlanHeader() bool {
	if n.selectedIdx >= len(n.rows) {
		return false
	}
	k := n.rows[n.selectedIdx].Kind
	return k == navRowPlanHeader || k == navRowHistoryPlan
}

// GetSelectedID returns the unique ID of the selected row.
func (n *NavigationPanel) GetSelectedID() string {
	if n.selectedIdx >= len(n.rows) {
		return ""
	}
	return n.rows[n.selectedIdx].ID
}

// SelectInstance finds and selects the given instance. Returns true if found.
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

// GetInstances returns all instances (for persistence, metadata updates, etc).
func (n *NavigationPanel) GetInstances() []*session.Instance {
	return n.instances
}

// TotalInstances returns total instance count.
func (n *NavigationPanel) TotalInstances() int {
	return len(n.instances)
}

// AddInstance adds an instance and rebuilds. Returns a finalizer for repo registration.
func (n *NavigationPanel) AddInstance(inst *session.Instance) func() {
	n.instances = append(n.instances, inst)
	n.rebuildRows()
	return func() {} // repo tracking handled by app layer
}

// RemoveByTitle removes an instance by title. Returns removed instance or nil.
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

// Kill removes and kills the currently selected instance.
func (n *NavigationPanel) Kill() {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return
	}
	inst.Kill()
	n.RemoveByTitle(inst.Title)
}

// Remove dismisses the selected instance without killing the tmux session.
func (n *NavigationPanel) Remove() {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return
	}
	n.RemoveByTitle(inst.Title)
}

// Attach attaches to the selected instance's tmux session.
func (n *NavigationPanel) Attach() (chan struct{}, error) {
	inst := n.GetSelectedInstance()
	if inst == nil {
		return nil, fmt.Errorf("no instance selected")
	}
	return inst.Attach()
}

func (n *NavigationPanel) availRows() int {
	const borderAndPadding = 4
	const headerLines = 5
	avail := n.height - borderAndPadding - headerLines
	if avail < 1 {
		avail = 1
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

// SelectedSpaceAction returns the context-sensitive action label for the space key.
func (n *NavigationPanel) SelectedSpaceAction() string {
	if n.selectedIdx >= len(n.rows) {
		return "toggle"
	}
	row := n.rows[n.selectedIdx]
	switch row.Kind {
	case navRowPlanHeader, navRowHistoryToggle:
		if row.Collapsed {
			return "expand"
		}
		return "collapse"
	default:
		return "toggle"
	}
}
```

**Step 3: Implement rendering in `ui/nav_panel_render.go`**

Create styles and row renderers. Reuse the existing Rosé Pine Moon palette from `ui/theme.go`. Instance rows render two lines: title line with status icon, branch line with diff stats and activity.

Carry over from current sidebar: search bar, legend, repo button, border. Carry over from current list renderer: spinner integration, diff stats, activity text, branch truncation.

The `String()` method follows the same pattern as the current `Sidebar.String()`:
1. Render search bar
2. Render visible rows (scroll windowed)
3. Gap fill
4. Legend + repo button
5. Wrap in border

Instance rows are 2 lines (title + branch). All other rows are 1 line. Account for this in scroll calculations — instance rows consume 2 of the available row budget.

Key styles to define (reuse existing palette colors):
- `navPlanHeaderStyle` — bold text for plan names
- `navInstanceTitleStyle` — normal text, indented
- `navInstanceBranchStyle` — muted/subtle text, further indented
- `navSoloDividerStyle` — muted, like current historyToggleStyle
- Carry over: `sidebarRunningStyle`, `sidebarNotifyStyle`, `sidebarCancelledStyle`, etc.

**Step 4: Write tests in `ui/nav_panel_test.go`**

Test the following:
- `rebuildRows` groups instances under correct plans
- Solo instances appear under solo header
- Sort ordering: notified plans → running plans → idle plans
- Instance sort within plan: notified → running → ready → paused → completed
- Auto-expand: plans with running instances are expanded, idle plans collapsed
- User override: manually collapsed plan stays collapsed even with running instances
- `Up()`/`Down()` skip solo header dividers
- `Left()` from instance moves to parent plan header
- `Right()` on collapsed plan expands it
- `GetSelectedInstance()` returns nil for plan header rows
- `GetSelectedPlanFile()` works for both plan headers and instances under plans
- `SelectInstance()` finds and selects the correct row
- `Kill()`/`Remove()` remove the instance and rebuild
- Search filtering reduces visible rows

```go
func TestNavigationPanel_RebuildGrouping(t *testing.T) {
	s := spinner.New()
	nav := NewNavigationPanel(&s)

	plans := []PlanDisplay{
		{Filename: "plan-a.md", Status: "implementing"},
		{Filename: "plan-b.md", Status: "ready"},
	}
	instances := []*session.Instance{
		{Title: "task1", PlanFile: "plan-a.md", Status: session.Running},
		{Title: "task2", PlanFile: "plan-a.md", Status: session.Ready},
		{Title: "solo1", Status: session.Running},
	}

	nav.SetData(plans, instances, nil, nil, nil)

	// Verify plan-a is expanded (has running instance), plan-b collapsed (idle)
	var planAHeader, planBHeader *navRow
	for i := range nav.rows {
		if nav.rows[i].ID == SidebarPlanPrefix+"plan-a.md" {
			planAHeader = &nav.rows[i]
		}
		if nav.rows[i].ID == SidebarPlanPrefix+"plan-b.md" {
			planBHeader = &nav.rows[i]
		}
	}
	assert.NotNil(t, planAHeader)
	assert.False(t, planAHeader.Collapsed, "plan with running instances should be expanded")
	assert.NotNil(t, planBHeader)
	assert.True(t, planBHeader.Collapsed, "idle plan should be collapsed")

	// Verify solo section exists
	hasSolo := false
	for _, row := range nav.rows {
		if row.Kind == navRowSoloHeader {
			hasSolo = true
		}
	}
	assert.True(t, hasSolo)
}

func TestNavigationPanel_SortOrder(t *testing.T) {
	s := spinner.New()
	nav := NewNavigationPanel(&s)

	plans := []PlanDisplay{
		{Filename: "idle.md", Status: "ready"},
		{Filename: "running.md", Status: "implementing"},
		{Filename: "notified.md", Status: "reviewing"},
	}
	instances := []*session.Instance{
		{Title: "t1", PlanFile: "running.md", Status: session.Running},
		{Title: "t2", PlanFile: "notified.md", Status: session.Ready, Notified: true},
	}
	statuses := map[string]TopicStatus{
		"notified.md": {HasNotification: true},
		"running.md":  {HasRunning: true},
	}

	nav.SetData(plans, instances, nil, nil, statuses)

	// First plan header should be the notified one
	assert.Equal(t, SidebarPlanPrefix+"notified.md", nav.rows[0].ID)
}

func TestNavigationPanel_InstanceSortWithinPlan(t *testing.T) {
	s := spinner.New()
	nav := NewNavigationPanel(&s)

	plans := []PlanDisplay{{Filename: "p.md", Status: "implementing"}}
	instances := []*session.Instance{
		{Title: "completed", PlanFile: "p.md", Status: session.Paused, ImplementationComplete: true},
		{Title: "running", PlanFile: "p.md", Status: session.Running},
		{Title: "notified", PlanFile: "p.md", Status: session.Ready, Notified: true},
		{Title: "ready", PlanFile: "p.md", Status: session.Ready},
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	nav.SetData(plans, instances, nil, nil, statuses)

	// Instances under the plan header should be: notified, running, ready, completed
	var instTitles []string
	for _, row := range nav.rows {
		if row.Instance != nil {
			instTitles = append(instTitles, row.Instance.Title)
		}
	}
	assert.Equal(t, []string{"notified", "running", "ready", "completed"}, instTitles)
}

func TestNavigationPanel_Navigation(t *testing.T) {
	s := spinner.New()
	nav := NewNavigationPanel(&s)
	nav.SetSize(40, 30)

	plans := []PlanDisplay{{Filename: "p.md", Status: "implementing"}}
	instances := []*session.Instance{
		{Title: "t1", PlanFile: "p.md", Status: session.Running},
		{Title: "solo1", Status: session.Running},
	}
	statuses := map[string]TopicStatus{"p.md": {HasRunning: true}}
	nav.SetData(plans, instances, nil, nil, statuses)

	// Start at row 0 (plan header)
	assert.Nil(t, nav.GetSelectedInstance())
	assert.Equal(t, "p.md", nav.GetSelectedPlanFile())

	// Down to instance
	nav.Down()
	assert.Equal(t, "t1", nav.GetSelectedInstance().Title)

	// Down should skip solo header
	nav.Down()
	assert.Equal(t, "solo1", nav.GetSelectedInstance().Title)
}
```

**Step 5: Run tests**

Run: `go test ./ui/ -run TestNavigationPanel -v`
Expected: all tests pass

**Step 6: Commit**

```bash
git add ui/nav_panel.go ui/nav_panel_render.go ui/nav_panel_test.go
git commit -m "feat(ui): add NavigationPanel component

Unified tree panel replacing Sidebar + List. Groups instances under
plans with compact two-line rows, sorts by activity priority."
```

---

### Task 2: InfoPane Plan Summary + Resource Relocation

Extend the InfoPane to show a plan summary when a plan header is selected, and add CPU/memory to the instance info section.

**Files:**
- Modify: `ui/info_pane.go` — add plan summary fields, render plan summary, add CPU/mem to instance section, add "view plan doc" indicator
- Modify: `ui/info_pane_test.go` — test plan summary rendering

**Step 1: Extend InfoData with plan summary fields**

Add these fields to `InfoData` in `ui/info_pane.go`:

```go
// Plan summary fields (shown when plan header is selected, no instance)
PlanInstanceCount   int
PlanRunningCount    int
PlanReadyCount      int
PlanPausedCount     int
PlanAddedLines      int
PlanRemovedLines    int

// Resource fields (shown when instance is selected)
CPUPercent float64
MemMB      float64

// IsPlanHeaderSelected distinguishes plan header vs instance selection.
IsPlanHeaderSelected bool
```

**Step 2: Add plan summary rendering**

Add a `renderPlanSummary` method that shows plan metadata, wave progress, aggregated stats, and a "view plan doc" action indicator:

```go
func (p *InfoPane) renderPlanSummary() string {
	lines := []string{
		infoSectionStyle.Render("plan"),
		p.renderDivider(),
	}
	if p.data.PlanName != "" {
		lines = append(lines, p.renderRow("name", p.data.PlanName))
	}
	if p.data.PlanStatus != "" {
		lines = append(lines, p.renderStatusRow("status", p.data.PlanStatus))
	}
	if p.data.PlanTopic != "" {
		lines = append(lines, p.renderRow("topic", p.data.PlanTopic))
	}
	if p.data.PlanBranch != "" {
		lines = append(lines, p.renderRow("branch", p.data.PlanBranch))
	}
	if p.data.PlanCreated != "" {
		lines = append(lines, p.renderRow("created", p.data.PlanCreated))
	}

	// Instance counts
	if p.data.PlanInstanceCount > 0 {
		summary := fmt.Sprintf("%d", p.data.PlanInstanceCount)
		parts := []string{}
		if p.data.PlanRunningCount > 0 {
			parts = append(parts, fmt.Sprintf("%d running", p.data.PlanRunningCount))
		}
		if p.data.PlanReadyCount > 0 {
			parts = append(parts, fmt.Sprintf("%d ready", p.data.PlanReadyCount))
		}
		if p.data.PlanPausedCount > 0 {
			parts = append(parts, fmt.Sprintf("%d paused", p.data.PlanPausedCount))
		}
		if len(parts) > 0 {
			summary += " (" + strings.Join(parts, ", ") + ")"
		}
		lines = append(lines, p.renderRow("instances", summary))
	}

	// Aggregated diff stats
	if p.data.PlanAddedLines > 0 || p.data.PlanRemovedLines > 0 {
		diff := fmt.Sprintf("+%d -%d", p.data.PlanAddedLines, p.data.PlanRemovedLines)
		lines = append(lines, p.renderRow("lines changed", diff))
	}

	// Wave progress (reuse existing renderWaveSection if WaveTasks is populated)

	return strings.Join(lines, "\n")
}
```

**Step 3: Add CPU/memory to instance info section**

In `renderInstanceSection()`, after the existing fields, add:

```go
if p.data.CPUPercent > 0 || p.data.MemMB > 0 {
	lines = append(lines, p.renderRow("cpu", fmt.Sprintf("%.0f%%", p.data.CPUPercent)))
	lines = append(lines, p.renderRow("memory", fmt.Sprintf("%.0fM", p.data.MemMB)))
}
```

**Step 4: Add "view plan doc" indicator**

At the bottom of the plan summary, render a styled action hint:

```go
// In renderPlanSummary, after all fields:
lines = append(lines, "")
viewDocStyle := lipgloss.NewStyle().
	Foreground(ColorFoam).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorOverlay).
	Padding(0, 2)
lines = append(lines, viewDocStyle.Render("enter: view plan doc"))
```

**Step 5: Update the `render()` method to use plan summary**

```go
func (p *InfoPane) render() string {
	if !p.data.HasInstance && !p.data.IsPlanHeaderSelected {
		return "no instance selected"
	}

	var sections []string
	if p.data.IsPlanHeaderSelected {
		sections = append(sections, p.renderPlanSummary())
		if len(p.data.WaveTasks) > 0 {
			sections = append(sections, p.renderWaveSection())
		}
	} else {
		if p.data.HasPlan {
			sections = append(sections, p.renderPlanSection())
		}
		sections = append(sections, p.renderInstanceSection())
		if len(p.data.WaveTasks) > 0 {
			sections = append(sections, p.renderWaveSection())
		}
	}

	return strings.Join(sections, "\n\n")
}
```

**Step 6: Write tests**

```go
func TestInfoPane_PlanSummary(t *testing.T) {
	pane := NewInfoPane()
	pane.SetSize(60, 30)
	pane.SetData(InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             "my-feature",
		PlanStatus:           "implementing",
		PlanInstanceCount:    3,
		PlanRunningCount:     2,
		PlanReadyCount:       1,
	})

	output := pane.String()
	assert.Contains(t, output, "my-feature")
	assert.Contains(t, output, "implementing")
	assert.Contains(t, output, "3 (2 running, 1 ready)")
	assert.Contains(t, output, "view plan doc")
}

func TestInfoPane_InstanceWithResources(t *testing.T) {
	pane := NewInfoPane()
	pane.SetSize(60, 30)
	pane.SetData(InfoData{
		HasInstance: true,
		Title:       "task 1",
		Status:      "running",
		CPUPercent:  12.5,
		MemMB:       340,
	})

	output := pane.String()
	assert.Contains(t, output, "13%")  // rounded
	assert.Contains(t, output, "340M")
}
```

**Step 7: Run tests**

Run: `go test ./ui/ -run TestInfoPane -v`
Expected: all pass

**Step 8: Commit**

```bash
git add ui/info_pane.go ui/info_pane_test.go
git commit -m "feat(ui): add plan summary to info tab, relocate CPU/mem

Plan header selection shows summary with instance counts and
aggregated stats. CPU and memory moved from instance list rows
to the info tab's instance section."
```

---

## Wave 2: App Integration

> **Depends on Wave 1:** NavigationPanel and InfoPane extensions must exist before wiring the app.

### Task 3: Wire NavigationPanel Into App + Delete Old Code

Replace `m.sidebar` + `m.list` with `m.nav`, switch to 2-column layout, rewire all call sites, and delete the old Sidebar/List code.

**Files:**
- Modify: `app/app.go` — replace field declarations, `newHome()`, `View()` layout, `WindowSizeMsg` handling
- Modify: `app/app_input.go` — rewire all input handling from `m.list.*`/`m.sidebar.*` to `m.nav.*`
- Modify: `app/app_state.go` — rewire state management, remove `slotList`, simplify focus slots
- Modify: `app/app_actions.go` — rewire action handlers
- Delete: `ui/list.go`, `ui/list_renderer.go`, `ui/list_styles.go`, `ui/sidebar.go`
- Delete: `ui/list_renderer_alignment_test.go`, `ui/list_styles_test.go`, `ui/list_scroll_test.go`, `ui/list_cycle_test.go`, `ui/list_highlight_test.go`, `ui/sidebar_test.go`, `ui/sidebar_scroll_test.go`
- Modify: all test files in `app/` that reference `m.list` or `m.sidebar` or `slotList`

**Step 1: Replace struct fields in `app/app.go`**

In the `home` struct, replace:
```go
// OLD
list    *ui.List
sidebar *ui.Sidebar
```
with:
```go
// NEW
nav *ui.NavigationPanel
```

Remove `listWidth` and `sidebarWidth` fields — replace with single `navWidth`.

In `newHome()`, replace:
```go
// OLD
list:    ui.NewList(&s, autoYes),
sidebar: ui.NewSidebar(),
```
with:
```go
// NEW
nav: ui.NewNavigationPanel(&s),
```

**Step 2: Update `View()` to 2-column layout**

```go
func (m *home) View() string {
	colStyle := lipgloss.NewStyle().Height(m.contentHeight)
	previewWithPadding := colStyle.Render(m.tabbedWindow.String())

	var cols []string
	if !m.sidebarHidden {
		cols = append(cols, colStyle.Render(m.nav.String()))
	}
	cols = append(cols, previewWithPadding)
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	// ... rest unchanged
}
```

**Step 3: Update `WindowSizeMsg` handler**

Replace the 3-column width calculation with 2-column. The nav panel gets the former sidebar width (or slightly wider — ~30% of terminal):

```go
case tea.WindowSizeMsg:
	navWidth := msg.Width * 30 / 100
	if navWidth < 25 { navWidth = 25 }
	previewWidth := msg.Width - navWidth
	m.navWidth = navWidth
	m.nav.SetSize(navWidth, contentHeight)
	m.tabbedWindow.SetSize(previewWidth, contentHeight)
```

**Step 4: Simplify focus slots**

Remove `slotList` (was 4). The focus ring becomes:
- 0 = nav panel (was sidebar)
- 1 = agent tab
- 2 = diff tab
- 3 = info tab

In `app_state.go`, update:
```go
const (
	slotNav   = 0 // was slotSidebar
	slotAgent = 1
	slotDiff  = 2
	slotInfo  = 3
)

func (m *home) setFocusSlot(slot int) {
	m.focusSlot = slot
	m.nav.SetFocused(slot == slotNav)
	// ... rest of tab focus logic
}
```

**Step 5: Rewire call sites systematically**

This is the bulk of the work — replace ~125 `m.list.*` and ~69 `m.sidebar.*` calls. Use the NavigationPanel's unified API. Key mappings:

| Old call | New call |
|----------|----------|
| `m.list.GetSelectedInstance()` | `m.nav.GetSelectedInstance()` |
| `m.list.Kill()` | `m.nav.Kill()` |
| `m.list.Remove()` | `m.nav.Remove()` |
| `m.list.Attach()` | `m.nav.Attach()` |
| `m.list.Up()` / `m.list.Down()` | `m.nav.Up()` / `m.nav.Down()` |
| `m.list.AddInstance(inst)` | `m.nav.AddInstance(inst)` |
| `m.list.SelectInstance(inst)` | `m.nav.SelectInstance(inst)` |
| `m.list.GetInstances()` | `m.nav.GetInstances()` |
| `m.list.TotalInstances()` | `m.nav.TotalInstances()` |
| `m.list.RemoveByTitle(t)` | `m.nav.RemoveByTitle(t)` |
| `m.list.NumInstances()` | `m.nav.TotalInstances()` |
| `m.list.SetFocused(b)` | handled by `setFocusSlot` |
| `m.list.SetStatusFilter(f)` | removed (sort ordering replaces filters) |
| `m.list.CycleSortMode()` | removed |
| `m.list.CycleNextActive()` / `CyclePrevActive()` | keep as nav methods if needed |
| `m.list.SetFilter(f)` | removed |
| `m.list.SetHighlightFilter(k,v)` | removed (tree grouping replaces highlighting) |
| `m.list.SetSearchFilter(q)` | search handled within nav panel |
| `m.list.HandleTabClick(x,y)` | removed (no more filter tabs) |
| `m.list.GetItemAtRow(r)` | removed (click handling via nav.ClickItem) |
| `m.list.SetSessionPreviewSize(w,h)` | move to NavigationPanel or keep in app directly |
| `m.sidebar.GetSelectedPlanFile()` | `m.nav.GetSelectedPlanFile()` |
| `m.sidebar.GetSelectedID()` | `m.nav.GetSelectedID()` |
| `m.sidebar.IsSelectedPlanHeader()` | `m.nav.IsSelectedPlanHeader()` |
| `m.sidebar.IsSelectedTopicHeader()` | removed (topics no longer separate rows) |
| `m.sidebar.GetSelectedTopicName()` | removed |
| `m.sidebar.Up()` / `Down()` | `m.nav.Up()` / `m.nav.Down()` |
| `m.sidebar.Left()` / `Right()` | `m.nav.Left()` / `m.nav.Right()` |
| `m.sidebar.ToggleSelectedExpand()` | `m.nav.ToggleSelectedExpand()` |
| `m.sidebar.SelectedSpaceAction()` | `m.nav.SelectedSpaceAction()` |
| `m.sidebar.SetItems(...)` | `m.nav.SetData(...)` |
| `m.sidebar.SetTopicsAndPlans(...)` | `m.nav.SetData(...)` |
| `m.sidebar.SetPlans(...)` | folded into `SetData` |
| `m.sidebar.ActivateSearch()` etc | `m.nav.ActivateSearch()` etc |
| `m.sidebar.SetRepoName(n)` | `m.nav.SetRepoName(n)` |
| `m.sidebar.SetClickUpAvailable(b)` | `m.nav.SetClickUpAvailable(b)` |

**Step 6: Update `syncSidebar` to use `m.nav.SetData()`**

The current `syncSidebar()` in `app_state.go` builds `PlanDisplay` arrays and calls `sidebar.SetTopicsAndPlans()` and `sidebar.SetItems()`. Consolidate into a single `m.nav.SetData()` call that passes plans, instances, history, cancelled, and status maps.

**Step 7: Update selection change handler to switch tabs**

When the selected row changes, detect whether it's a plan header or instance and switch tabs accordingly:

```go
func (m *home) onSelectionChanged() {
	inst := m.nav.GetSelectedInstance()
	if inst != nil {
		// Instance selected — show agent preview
		m.tabbedWindow.SetActiveTab(PreviewTab)
		m.updateInfoForInstance(inst)
	} else if m.nav.IsSelectedPlanHeader() {
		// Plan header selected — show info tab with plan summary
		m.tabbedWindow.SetActiveTab(InfoTab)
		m.updateInfoForPlan(m.nav.GetSelectedPlanFile())
	}
}
```

**Step 8: Delete old files**

```bash
rm ui/list.go ui/list_renderer.go ui/list_styles.go ui/sidebar.go
rm ui/list_renderer_alignment_test.go ui/list_styles_test.go
rm ui/list_scroll_test.go ui/list_cycle_test.go ui/list_highlight_test.go
rm ui/sidebar_test.go ui/sidebar_scroll_test.go
```

**Step 9: Fix compilation and tests**

Run: `go build ./...`

Fix any remaining references to deleted types. Common issues:
- `ui.StatusFilter*` constants referenced in app/ — remove those code paths
- `ui.SortMode*` constants — remove
- `ui.SidebarAll`, `ui.SidebarUngrouped` — remove (no longer needed)
- Test files in `app/` referencing `slotList` — update to use `slotNav`
- Test helpers that construct `ui.List` or `ui.Sidebar` — update to use `ui.NavigationPanel`

Run: `go test ./... -count=1`
Fix any failing tests.

**Step 10: Commit**

```bash
git add -A
git commit -m "feat: merge sidebar and instance list into NavigationPanel

Two-column layout: navigation panel (30%) + preview/tabs (70%).
Plans are top-level with compact instance rows grouped underneath.
Solo instances in dedicated section. CPU/mem moved to info tab.

Removes: ui/list.go, ui/list_renderer.go, ui/list_styles.go,
ui/sidebar.go and associated test files (~1200 lines deleted)."
```
