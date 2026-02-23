# Instance Grouping & Filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace topic-based instance grouping with plan-based grouping — instances associate with plans via PlanFile, sidebar selection filters the instance list, and context menu actions operate on plan-grouped instances.

**Architecture:** Instance.PlanFile becomes the grouping key (replacing TopicName). Sidebar plan selection drives list filtering. The `m` keybind shows a plan picker. Plan context menu includes instance management actions.

**Tech Stack:** Go, bubbletea, lipgloss

**Important — Recent Codebase Changes (post-plan-authoring):**

1. **Sidebar toggle** — `sidebarHidden bool` on `home` struct, `KeyToggleSidebar` (ctrl+s). When sidebar is hidden (`sidebarWidth = 0`), filtering still works but sidebar isn't rendered. The `s` key does a two-step reveal. Keep this intact — it's orthogonal to grouping work.
2. **Global background fill** — `ui.FillBackground()` in `View()`, all styles use `.Background(ColorBase)`. When adding new styles, include `.Background(ColorBase)`.
3. **`TopicName` removal** — Plan 1 removes `TopicName` from Instance and all topic references. This plan assumes that's done. If `TopicName` still exists in any form, replace it with `PlanFile`-based logic.

---

### Task 1: List Filtering Core (PlanFile + Ungrouped)

**Files:**
- Create: `ui/list_plan_filter_test.go`
- Modify: `ui/list.go`
- Test: `ui/list_plan_filter_test.go`

1. **Write failing tests for plan-based list behavior**

```go
package ui

import (
	"testing"

	"github.com/kastheco/kasmos/session"
)

func mustInstance(t *testing.T, title, planFile string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    title,
		Path:     ".",
		Program:  "claude",
		PlanFile: planFile,
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	return inst
}

func TestListSetFilter_ByPlanAndUngrouped(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "alpha", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "beta", "2026-02-21-beta.md"))
	_ = l.AddInstance(mustInstance(t, "scratch", ""))

	l.SetFilter("2026-02-21-alpha.md")
	if len(l.items) != 1 || l.items[0].Title != "alpha" {
		t.Fatalf("plan filter mismatch: got %+v", l.items)
	}

	l.SetFilter(SidebarUngrouped)
	if len(l.items) != 1 || l.items[0].Title != "scratch" {
		t.Fatalf("ungrouped filter mismatch: got %+v", l.items)
	}
}

func TestListSetSearchFilter_MatchesPlanFileAcrossAllInstances(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "worker-a", "2026-02-21-auth-refactor.md"))
	_ = l.AddInstance(mustInstance(t, "worker-b", "2026-02-21-payments.md"))

	l.SetFilter("2026-02-21-auth-refactor.md") // selected plan should not scope search
	l.SetSearchFilter("payments")

	if len(l.items) != 1 || l.items[0].Title != "worker-b" {
		t.Fatalf("search should be global across plans, got %+v", l.items)
	}
}

func TestListKillInstancesByPlan(t *testing.T) {
	l := NewList(nil, false)
	_ = l.AddInstance(mustInstance(t, "alpha-1", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "alpha-2", "2026-02-21-alpha.md"))
	_ = l.AddInstance(mustInstance(t, "beta-1", "2026-02-21-beta.md"))

	l.KillInstancesByPlan("2026-02-21-alpha.md")

	if len(l.allItems) != 1 || l.allItems[0].Title != "beta-1" {
		t.Fatalf("KillInstancesByPlan() mismatch: got %+v", l.allItems)
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./ui -run 'TestListSetFilter_ByPlanAndUngrouped|TestListSetSearchFilter_MatchesPlanFileAcrossAllInstances|TestListKillInstancesByPlan' -count=1`

Expected: FAIL with errors like `l.KillInstancesByPlan undefined` and topic-based filter/search assertions failing.

3. **Implement list filtering and plan kill methods**

```go
// ui/list.go

// KillInstancesByPlan kills and removes all instances belonging to the given plan file.
func (l *List) KillInstancesByPlan(planFile string) {
	var remaining []*session.Instance
	for _, inst := range l.allItems {
		if inst.PlanFile == planFile {
			if err := inst.Kill(); err != nil {
				log.ErrorLog.Printf("could not kill instance %s: %v", inst.Title, err)
			}
			repoName, err := inst.RepoName()
			if err == nil {
				l.rmRepo(repoName)
			}
		} else {
			remaining = append(remaining, inst)
		}
	}
	l.allItems = remaining
	l.rebuildFilteredItems()
}

// SetSearchFilter filters instances by title and plan filename across all instances.
func (l *List) SetSearchFilter(query string) {
	l.filter = ""
	q := strings.ToLower(query)
	filtered := make([]*session.Instance, 0, len(l.allItems))
	for _, inst := range l.allItems {
		if l.statusFilter == StatusFilterActive && inst.Paused() {
			continue
		}
		if q == "" ||
			strings.Contains(strings.ToLower(inst.Title), q) ||
			strings.Contains(strings.ToLower(inst.PlanFile), q) {
			filtered = append(filtered, inst)
		}
	}
	l.items = filtered
	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
}

func (l *List) rebuildFilteredItems() {
	var grouped []*session.Instance
	if l.filter == "" {
		grouped = l.allItems
	} else if l.filter == SidebarUngrouped {
		for _, inst := range l.allItems {
			if inst.PlanFile == "" {
				grouped = append(grouped, inst)
			}
		}
	} else {
		for _, inst := range l.allItems {
			if inst.PlanFile == l.filter {
				grouped = append(grouped, inst)
			}
		}
	}

	if l.statusFilter == StatusFilterActive {
		filtered := make([]*session.Instance, 0, len(grouped))
		for _, inst := range grouped {
			if !inst.Paused() {
				filtered = append(filtered, inst)
			}
		}
		l.items = filtered
	} else {
		l.items = grouped
	}

	l.sortItems()
	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
}
```

4. **Run test to verify it passes**

Run: `go test ./ui -run 'TestListSetFilter_ByPlanAndUngrouped|TestListSetSearchFilter_MatchesPlanFileAcrossAllInstances|TestListKillInstancesByPlan' -count=1`

Expected: PASS.

5. **Commit**

```bash
git add ui/list.go ui/list_plan_filter_test.go
git commit -m "refactor(ui): switch list grouping and filtering to plan files"
```

### Task 2: Sidebar Plan Counts + Match Mapping

**Files:**
- Create: `ui/sidebar_plan_counts_test.go`
- Modify: `ui/sidebar.go`
- Test: `ui/sidebar_plan_counts_test.go`, `ui/sidebar_test.go`

1. **Write failing sidebar tests for plan counts and plan-keyed search matches**

```go
package ui

import (
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
)

func findItem(t *testing.T, items []SidebarItem, id string) SidebarItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("sidebar item %q not found", id)
	return SidebarItem{}
}

func TestSidebarSetItems_PlanHeadersIncludeInstanceCounts(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{
		{Filename: "2026-02-21-alpha.md", Status: string(planstate.StatusInProgress)},
		{Filename: "2026-02-21-beta.md", Status: string(planstate.StatusReady)},
	})

	counts := map[string]int{
		"2026-02-21-alpha.md": 3,
		"2026-02-21-beta.md":  1,
	}
	statuses := map[string]GroupStatus{}

	s.SetItems(counts, 2, statuses)

	alpha := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-alpha.md")
	beta := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-beta.md")
	if alpha.Count != 3 || beta.Count != 1 {
		t.Fatalf("plan counts mismatch: alpha=%d beta=%d", alpha.Count, beta.Count)
	}

	ungrouped := findItem(t, s.items, SidebarUngrouped)
	if ungrouped.Count != 2 {
		t.Fatalf("ungrouped count mismatch: got %d want 2", ungrouped.Count)
	}
}

func TestSidebarUpdateMatchCounts_UsesPlanFileKey(t *testing.T) {
	s := NewSidebar()
	s.SetPlans([]PlanDisplay{{Filename: "2026-02-21-alpha.md", Status: string(planstate.StatusReady)}})
	s.SetItems(map[string]int{"2026-02-21-alpha.md": 1}, 1, map[string]GroupStatus{})

	s.UpdateMatchCounts(map[string]int{"2026-02-21-alpha.md": 4, "": 2}, 6)

	plan := findItem(t, s.items, SidebarPlanPrefix+"2026-02-21-alpha.md")
	if plan.MatchCount != 4 {
		t.Fatalf("plan match count mismatch: got %d want 4", plan.MatchCount)
	}
	ungrouped := findItem(t, s.items, SidebarUngrouped)
	if ungrouped.MatchCount != 2 {
		t.Fatalf("ungrouped match count mismatch: got %d want 2", ungrouped.MatchCount)
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./ui -run 'TestSidebarSetItems_PlanHeadersIncludeInstanceCounts|TestSidebarUpdateMatchCounts_UsesPlanFileKey' -count=1`

Expected: FAIL due to `SetItems` signature mismatch and plan match-count lookup using sidebar IDs instead of plan filenames.

3. **Implement sidebar item model changes for plan grouping**

```go
// ui/sidebar.go

// GroupStatus holds status flags for a plan-grouped set of instances.
type GroupStatus struct {
	HasRunning      bool
	HasNotification bool
}

// SetItems updates sidebar items from plan-grouped instance counts.
func (s *Sidebar) SetItems(instanceCountByPlan map[string]int, ungroupedCount int, groupStatuses map[string]GroupStatus) {
	totalCount := ungroupedCount
	for _, c := range instanceCountByPlan {
		totalCount += c
	}

	anyRunning := false
	anyNotification := false
	for _, st := range groupStatuses {
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
			st := groupStatuses[p.Filename]
			items = append(items, SidebarItem{
				Name:            planstate.DisplayName(p.Filename),
				ID:              SidebarPlanPrefix + p.Filename,
				Count:           instanceCountByPlan[p.Filename],
				HasRunning:      st.HasRunning,
				HasNotification: st.HasNotification,
			})
		}
	}

	if ungroupedCount > 0 {
		ungroupedSt := groupStatuses[""]
		items = append(items, SidebarItem{Name: "Ungrouped", IsSection: true})
		items = append(items, SidebarItem{
			Name:            "Ungrouped",
			ID:              SidebarUngrouped,
			Count:           ungroupedCount,
			HasRunning:      ungroupedSt.HasRunning,
			HasNotification: ungroupedSt.HasNotification,
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

func (s *Sidebar) UpdateMatchCounts(matchesByPlan map[string]int, totalMatches int) {
	for i := range s.items {
		if s.items[i].IsSection {
			continue
		}
		if matchesByPlan == nil {
			s.items[i].MatchCount = -1
			continue
		}
		switch s.items[i].ID {
		case SidebarAll:
			s.items[i].MatchCount = totalMatches
		case SidebarUngrouped:
			s.items[i].MatchCount = matchesByPlan[""]
		default:
			if strings.HasPrefix(s.items[i].ID, SidebarPlanPrefix) {
				planFile := s.items[i].ID[len(SidebarPlanPrefix):]
				s.items[i].MatchCount = matchesByPlan[planFile]
			} else {
				s.items[i].MatchCount = 0
			}
		}
	}
}
```

4. **Run tests to verify they pass**

Run: `go test ./ui -run 'TestSidebarSetItems_PlanHeadersIncludeInstanceCounts|TestSidebarUpdateMatchCounts_UsesPlanFileKey|TestSidebarSetItems_IncludesPlansSectionBeforeTopics|TestGetSelectedPlanFile' -count=1`

Expected: PASS.

5. **Commit**

```bash
git add ui/sidebar.go ui/sidebar_test.go ui/sidebar_plan_counts_test.go
git commit -m "feat(ui): show per-plan instance counts and plan-keyed search matches"
```

### Task 3: App State Wiring (Grouping, Filtering, Search)

**Files:**
- Create: `app/app_state_plan_grouping_test.go`
- Modify: `app/app_state.go`
- Test: `app/app_state_plan_grouping_test.go`

1. **Write failing app-state tests for plan filter behavior and plan picker options**

```go
package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
)

func newHomeForPlanStateTests(t *testing.T) *home {
	t.Helper()
	sp := spinner.New()
	h := &home{
		ctx:     context.Background(),
		list:    ui.NewList(&sp, false),
		sidebar: ui.NewSidebar(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusInProgress},
			"2026-02-21-beta.md":  {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()
	return h
}

func addInstance(t *testing.T, h *home, title, planFile string) {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    title,
		Path:     ".",
		Program:  "claude",
		PlanFile: planFile,
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	_ = h.list.AddInstance(inst)
}

func TestFilterInstancesByPlan_SelectedPlanFiltersList(t *testing.T) {
	h := newHomeForPlanStateTests(t)
	addInstance(t, h, "alpha-worker", "2026-02-21-alpha.md")
	addInstance(t, h, "beta-worker", "2026-02-21-beta.md")
	h.updateSidebarItems()

	h.sidebar.ClickItem(2) // All, Plans(section), alpha(plan)
	h.filterInstancesByPlan()

	if h.list.NumInstances() != 1 {
		t.Fatalf("filtered instances = %d, want 1", h.list.NumInstances())
	}
	if got := h.list.GetSelectedInstance().Title; got != "alpha-worker" {
		t.Fatalf("selected title = %q, want %q", got, "alpha-worker")
	}
}

func TestFilterBySearch_IsGlobalAcrossPlans(t *testing.T) {
	h := newHomeForPlanStateTests(t)
	addInstance(t, h, "alpha-worker", "2026-02-21-alpha.md")
	addInstance(t, h, "beta-worker", "2026-02-21-beta.md")
	h.updateSidebarItems()

	h.sidebar.ClickItem(2) // select alpha plan first
	h.filterInstancesByPlan()
	h.sidebar.SetSearchQuery("beta-worker")

	h.filterBySearch()

	if h.list.NumInstances() != 1 {
		t.Fatalf("search result size = %d, want 1", h.list.NumInstances())
	}
	if got := h.list.GetSelectedInstance().Title; got != "beta-worker" {
		t.Fatalf("search selected = %q, want %q", got, "beta-worker")
	}
}

func TestGetAssignablePlanNames_IncludesUngroupedAndPlans(t *testing.T) {
	h := newHomeForPlanStateTests(t)

	items, mapping := h.getAssignablePlanNames()
	if len(items) != 3 {
		t.Fatalf("picker items len = %d, want 3", len(items))
	}
	if items[0] != "(Ungrouped)" {
		t.Fatalf("items[0] = %q, want (Ungrouped)", items[0])
	}
	if mapping["(Ungrouped)"] != "" {
		t.Fatalf("ungrouped mapping = %q, want empty", mapping["(Ungrouped)"])
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./app -run 'TestFilterInstancesByPlan_SelectedPlanFiltersList|TestFilterBySearch_IsGlobalAcrossPlans|TestGetAssignablePlanNames_IncludesUngroupedAndPlans' -count=1`

Expected: FAIL due to missing `filterInstancesByPlan` / `getAssignablePlanNames` and topic-based sidebar update logic.

3. **Implement app-state plan grouping and search wiring**

```go
// app/app_state.go

func (m *home) updateSidebarItems() {
	countByPlan := make(map[string]int)
	groupStatuses := make(map[string]ui.GroupStatus)
	ungroupedCount := 0

	for _, inst := range m.list.GetInstances() {
		planKey := inst.PlanFile // "" => ungrouped
		if planKey == "" {
			ungroupedCount++
		} else {
			countByPlan[planKey]++
		}

		st := groupStatuses[planKey]
		if inst.Started() && !inst.Paused() && !inst.PromptDetected {
			st.HasRunning = true
		}
		if inst.Notified {
			st.HasNotification = true
		}
		groupStatuses[planKey] = st
	}

	m.sidebar.SetItems(countByPlan, ungroupedCount, groupStatuses)
}

func (m *home) filterInstancesByPlan() {
	selectedID := m.sidebar.GetSelectedID()
	switch {
	case selectedID == ui.SidebarAll:
		m.list.SetFilter("")
	case selectedID == ui.SidebarUngrouped:
		m.list.SetFilter(ui.SidebarUngrouped)
	case strings.HasPrefix(selectedID, ui.SidebarPlanPrefix):
		m.list.SetFilter(strings.TrimPrefix(selectedID, ui.SidebarPlanPrefix))
	default:
		m.list.SetFilter("")
	}
}

func (m *home) filterBySearch() {
	query := strings.ToLower(m.sidebar.GetSearchQuery())
	if query == "" {
		m.sidebar.UpdateMatchCounts(nil, 0)
		m.filterInstancesByPlan()
		return
	}

	// Search is global across all instances regardless of selected plan.
	m.list.SetSearchFilter(query)

	matchesByPlan := make(map[string]int)
	totalMatches := 0
	for _, inst := range m.list.GetInstances() {
		if strings.Contains(strings.ToLower(inst.Title), query) ||
			strings.Contains(strings.ToLower(inst.PlanFile), query) {
			matchesByPlan[inst.PlanFile]++
			totalMatches++
		}
	}
	m.sidebar.UpdateMatchCounts(matchesByPlan, totalMatches)
}

func (m *home) getAssignablePlanNames() ([]string, map[string]string) {
	items := []string{"(Ungrouped)"}
	mapping := map[string]string{"(Ungrouped)": ""}
	if m.planState == nil {
		return items, mapping
	}

	planFiles := make([]string, 0, len(m.planState.Plans))
	for filename := range m.planState.Plans {
		planFiles = append(planFiles, filename)
	}
	sort.Strings(planFiles)

	for _, filename := range planFiles {
		label := planstate.DisplayName(filename)
		if _, exists := mapping[label]; exists {
			label = fmt.Sprintf("%s (%s)", label, filename)
		}
		items = append(items, label)
		mapping[label] = filename
	}

	return items, mapping
}
```

4. **Run test to verify it passes**

Run: `go test ./app -run 'TestFilterInstancesByPlan_SelectedPlanFiltersList|TestFilterBySearch_IsGlobalAcrossPlans|TestGetAssignablePlanNames_IncludesUngroupedAndPlans' -count=1`

Expected: PASS.

5. **Commit**

```bash
git add app/app_state.go app/app_state_plan_grouping_test.go
git commit -m "feat(app): wire sidebar selection and search to plan grouping"
```

### Task 4: `m` Keybind -> Assign to Plan Picker

**Files:**
- Create: `app/app_input_assign_plan_test.go`
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `keys/keys.go`
- Test: `app/app_input_assign_plan_test.go`

1. **Write failing input tests for plan assignment flow**

```go
package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
)

func newHomeForInputTests(t *testing.T) *home {
	t.Helper()
	sp := spinner.New()
	h := &home{
		ctx:            context.Background(),
		state:          stateDefault,
		program:        "claude",
		activeRepoPath: ".",
		list:           ui.NewList(&sp, false),
		menu:           ui.NewMenu(),
		sidebar:        ui.NewSidebar(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()
	h.updateSidebarItems()
	return h
}

func TestKeyMoveTo_OpensAssignPlanPicker(t *testing.T) {
	h := newHomeForInputTests(t)
	inst, _ := session.NewInstance(session.InstanceOptions{Title: "w", Path: ".", Program: "claude"})
	_ = h.list.AddInstance(inst)
	h.list.SetSelectedInstance(0)

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if h.state != stateMoveTo {
		t.Fatalf("state = %v, want stateMoveTo", h.state)
	}
	if h.pickerOverlay == nil {
		t.Fatalf("pickerOverlay should be initialized")
	}
	if got := h.pickerOverlay.Render(); got == "" {
		t.Fatalf("picker render should not be empty")
	}
}

func TestStateMoveTo_SubmitAssignsPlanFile(t *testing.T) {
	h := newHomeForInputTests(t)
	inst, _ := session.NewInstance(session.InstanceOptions{Title: "w", Path: ".", Program: "claude"})
	_ = h.list.AddInstance(inst)
	h.list.SetSelectedInstance(0)

	h.state = stateMoveTo
	h.planPickerMap = map[string]string{
		"(Ungrouped)": "",
		"alpha":       "2026-02-21-alpha.md",
	}
	h.pickerOverlay = overlay.NewPickerOverlay("Assign to plan", []string{"(Ungrouped)", "alpha"})

	// move selection to "alpha", then submit
	h.pickerOverlay.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	if inst.PlanFile != "2026-02-21-alpha.md" {
		t.Fatalf("PlanFile = %q, want 2026-02-21-alpha.md", inst.PlanFile)
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./app -run 'TestKeyMoveTo_OpensAssignPlanPicker|TestStateMoveTo_SubmitAssignsPlanFile' -count=1`

Expected: FAIL because `m` still opens topic picker and move-state submission writes `TopicName`.

3. **Implement keybind behavior and move-state assignment to PlanFile**

```go
// app/app.go (home struct)
// planPickerMap maps picker display text to plan filename.
planPickerMap map[string]string

// app/app_input.go (stateMoveTo handler)
if m.state == stateMoveTo {
	shouldClose := m.pickerOverlay.HandleKeyPress(msg)
	if shouldClose {
		selected := m.list.GetSelectedInstance()
		if selected != nil && m.pickerOverlay.IsSubmitted() {
			picked := m.pickerOverlay.Value()
			selected.PlanFile = m.planPickerMap[picked]
			m.updateSidebarItems()
			if err := m.saveAllInstances(); err != nil {
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				m.pickerOverlay = nil
				return m, m.handleError(err)
			}
		}
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.pickerOverlay = nil
		return m, tea.WindowSize()
	}
	return m, nil
}

// app/app_input.go (KeyMoveTo)
case keys.KeyMoveTo:
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return m, nil
	}
	m.state = stateMoveTo
	items, mapping := m.getAssignablePlanNames()
	m.planPickerMap = mapping
	m.pickerOverlay = overlay.NewPickerOverlay("Assign to plan", items)
	return m, nil

// app/app_input.go (new instance creation paths)
planFile := m.sidebar.GetSelectedPlanFile()
instance, err := session.NewInstance(session.InstanceOptions{
	Title:    "",
	Path:     m.activeRepoPath,
	Program:  m.program,
	PlanFile: planFile,
})

// app/app_input.go (search navigation)
case msg.String() == "up":
	m.sidebar.Up()
	m.filterBySearch()
	return m, m.instanceChanged()
case msg.String() == "down":
	m.sidebar.Down()
	m.filterBySearch()
	return m, m.instanceChanged()

// keys/keys.go
KeyMoveTo: key.NewBinding(
	key.WithKeys("m"),
	key.WithHelp("m", "assign to plan"),
),
```

4. **Run tests to verify they pass**

Run: `go test ./app -run 'TestKeyMoveTo_OpensAssignPlanPicker|TestStateMoveTo_SubmitAssignsPlanFile' -count=1`

Expected: PASS.

5. **Commit**

```bash
git add app/app.go app/app_input.go keys/keys.go app/app_input_assign_plan_test.go
git commit -m "feat(app): repurpose m key to assign instances to plans"
```

### Task 5: Plan Context Menu Action - Kill Running Instances

**Files:**
- Create: `app/app_actions_plan_context_test.go`
- Modify: `app/app_actions.go`
- Modify: `app/app_input.go`
- Test: `app/app_actions_plan_context_test.go`

1. **Write failing tests for plan context kill action**

```go
package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
)

func TestExecuteContextAction_KillRunningInstancesInPlan(t *testing.T) {
	sp := spinner.New()
	h := &home{
		ctx:     context.Background(),
		list:    ui.NewList(&sp, false),
		sidebar: ui.NewSidebar(),
		menu:    ui.NewMenu(),
		planState: &planstate.PlanState{Plans: map[string]planstate.PlanEntry{
			"2026-02-21-alpha.md": {Status: planstate.StatusInProgress},
			"2026-02-21-beta.md":  {Status: planstate.StatusReady},
		}},
	}
	h.updateSidebarPlans()

	mk := func(title, planFile string) *session.Instance {
		inst, _ := session.NewInstance(session.InstanceOptions{Title: title, Path: ".", Program: "claude", PlanFile: planFile})
		return inst
	}
	alpha1 := mk("alpha-1", "2026-02-21-alpha.md")
	alpha2 := mk("alpha-2", "2026-02-21-alpha.md")
	beta := mk("beta", "2026-02-21-beta.md")
	h.allInstances = []*session.Instance{alpha1, alpha2, beta}
	_ = h.list.AddInstance(alpha1)
	_ = h.list.AddInstance(alpha2)
	_ = h.list.AddInstance(beta)

	h.updateSidebarItems()
	h.sidebar.ClickItem(2) // alpha plan

	_, _ = h.executeContextAction("kill_running_instances_in_plan")
	if h.confirmationOverlay == nil {
		t.Fatalf("expected confirmation overlay")
	}

	h.confirmationOverlay.OnConfirm()

	if len(h.allInstances) != 1 || h.allInstances[0].Title != "beta" {
		t.Fatalf("remaining instances mismatch: %+v", h.allInstances)
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./app -run 'TestExecuteContextAction_KillRunningInstancesInPlan' -count=1`

Expected: FAIL because action `kill_running_instances_in_plan` does not exist and sidebar context menu still uses topic actions.

3. **Implement plan context-menu actions in keyboard and mouse paths**

```go
// app/app_actions.go
case "kill_running_instances_in_plan":
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" {
		return m, nil
	}
	killAction := func() tea.Msg {
		for i := len(m.allInstances) - 1; i >= 0; i-- {
			if m.allInstances[i].PlanFile == planFile {
				m.allInstances = append(m.allInstances[:i], m.allInstances[i+1:]...)
			}
		}
		m.list.KillInstancesByPlan(planFile)
		_ = m.saveAllInstances()
		m.updateSidebarItems()
		return instanceChangedMsg{}
	}
	message := fmt.Sprintf("[!] Kill running instances in plan '%s'?", planstate.DisplayName(planFile))
	return m, m.confirmAction(message, killAction)

// app/app_actions.go (openContextMenu sidebar branch)
if m.focusedPanel == 0 {
	if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
		items := []overlay.ContextMenuItem{
			{Label: "Kill running instances", Action: "kill_running_instances_in_plan"},
			// keep existing plan actions from plan 2 here
		}
		x := m.sidebarWidth
		y := 1 + 4 + m.sidebar.GetSelectedIdx()
		m.contextMenu = overlay.NewContextMenu(x, y, items)
		m.state = stateContextMenu
		return m, nil
	}
}

// app/app_input.go (handleRightClick sidebar branch)
if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
	items := []overlay.ContextMenuItem{
		{Label: "Kill running instances", Action: "kill_running_instances_in_plan"},
		// keep existing plan actions from plan 2 here
	}
	m.contextMenu = overlay.NewContextMenu(x, y, items)
	m.state = stateContextMenu
	return m, nil
}
```

4. **Run test to verify it passes**

Run: `go test ./app -run 'TestExecuteContextAction_KillRunningInstancesInPlan' -count=1`

Expected: PASS.

5. **Commit**

```bash
git add app/app_actions.go app/app_input.go app/app_actions_plan_context_test.go
git commit -m "feat(app): add plan context action to kill grouped instances"
```

### Task 6: Remove Legacy Topic References + Final Verification

**Files:**
- Create: `session/instance_grouping_fields_test.go`
- Modify: `session/instance.go`
- Modify: `session/storage.go`
- Modify: `app/app_state.go`
- Modify: `app/app_input.go`
- Modify: `app/app_actions.go`
- Modify: `ui/list.go`
- Test: `session/instance_grouping_fields_test.go`

1. **Write failing guard test that forbids legacy TopicName fields**

```go
package session

import (
	"reflect"
	"testing"
)

func TestPlanGroupingModel_HasNoLegacyTopicFields(t *testing.T) {
	if _, ok := reflect.TypeOf(Instance{}).FieldByName("TopicName"); ok {
		t.Fatalf("Instance still has legacy TopicName field")
	}
	if _, ok := reflect.TypeOf(InstanceOptions{}).FieldByName("TopicName"); ok {
		t.Fatalf("InstanceOptions still has legacy TopicName field")
	}
	if _, ok := reflect.TypeOf(InstanceData{}).FieldByName("TopicName"); ok {
		t.Fatalf("InstanceData still has legacy TopicName field")
	}

	if _, ok := reflect.TypeOf(Instance{}).FieldByName("PlanFile"); !ok {
		t.Fatalf("Instance must include PlanFile")
	}
}
```

2. **Run test to verify it fails**

Run: `go test ./session -run 'TestPlanGroupingModel_HasNoLegacyTopicFields' -count=1`

Expected: FAIL while legacy `TopicName` fields still exist.

3. **Delete remaining topic-era fields/references and normalize naming**

```go
// session/instance.go
type Instance struct {
	// ...
	PlanFile string
	// ...
}

type InstanceOptions struct {
	Title    string
	Path     string
	Program  string
	AutoYes  bool
	PlanFile string
}

func (i *Instance) ToInstanceData() InstanceData {
	return InstanceData{
		Title:    i.Title,
		Path:     i.Path,
		PlanFile: i.PlanFile,
		// ...
	}
}

// session/storage.go
type InstanceData struct {
	Title    string `json:"title"`
	Path     string `json:"path"`
	PlanFile string `json:"plan_file,omitempty"`
	// topic_name removed
	// ...
}

// app/ui references (all must use PlanFile only)
// - filterInstancesByTopic -> filterInstancesByPlan
// - KillInstancesByTopic -> KillInstancesByPlan
// - topic-based label strings -> plan-based label strings
```

4. **Run full test suite to verify plan-grouping refactor is stable**

Run: `go test ./... -count=1`

Expected: PASS for all packages.

5. **Commit**

```bash
git add session/instance.go session/storage.go session/instance_grouping_fields_test.go app/app_state.go app/app_input.go app/app_actions.go ui/list.go
git commit -m "refactor(session): remove legacy topic grouping and finalize plan-based model"
```
