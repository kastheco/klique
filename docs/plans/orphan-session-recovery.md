# Orphaned Tmux Session Recovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Detect orphaned tmux sessions from plan agent crashes and let users reattach or kill them from the sidebar.

**Architecture:** On sidebar rebuild, probe `tmux has-session` for each plan in planning/implementing state that has no managed instance. Surface orphans via a gold `◎` indicator and context menu actions (attach / kill).

**Tech Stack:** Go, bubbletea, tmux CLI, existing `session/tmux` package

---

## Wave 1: Detection + Indicator

### Task 1: Export tmux session name derivation

**Files:**
- Modify: `session/tmux/tmux.go:70-73`
- Test: `session/tmux/tmux_test.go`

**Step 1: Export the `toKasTmuxName` helper**

Rename `toKasTmuxName` to `ToKasTmuxName` so the app layer can derive expected session names for orphan detection.

```go
// ToKasTmuxName derives the tmux session name kasmos would use for a given title.
func ToKasTmuxName(str string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_") // tmux replaces all . with _
	return fmt.Sprintf("%s%s", TmuxPrefix, str)
}
```

Update all internal callers (line 88 in `newTmuxSession`) to use the new name.

**Step 2: Add `SessionExists` public helper**

Add a standalone function that checks if a tmux session with a given name exists, without needing a `TmuxSession` instance:

```go
// SessionExists returns true if a tmux session with the given full name exists.
func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", fmt.Sprintf("-t=%s", name))
	return cmd.Run() == nil
}
```

**Step 3: Run existing tests**

Run: `go test ./session/tmux/ -v -count=1`
Expected: PASS (rename is backwards-compatible since all callers are internal)

**Step 4: Update the test that references the sanitized name**

The test at `tmux_test.go:54` already uses the exported field. Verify it still passes. Add a test for `SessionExists` with a non-existent session:

```go
func TestSessionExists_NonExistent(t *testing.T) {
	assert.False(t, SessionExists("kas_definitely_not_real_session_12345"))
}
```

**Step 5: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go
git commit -m "refactor(tmux): export ToKasTmuxName and add SessionExists helper"
```

---

### Task 2: Add HasOrphanedSession to PlanDisplay and sidebarRow

**Files:**
- Modify: `ui/sidebar.go:67-73` (PlanDisplay struct)
- Modify: `ui/sidebar.go:94-110` (sidebarRow struct)

**Step 1: Add the field to PlanDisplay**

```go
type PlanDisplay struct {
	Filename           string
	Status             string
	Description        string
	Branch             string
	Topic              string
	HasOrphanedSession bool // true if an orphaned tmux session was detected
}
```

**Step 2: Add the field to sidebarRow**

```go
type sidebarRow struct {
	// ... existing fields ...
	HasOrphanedSession bool
	Indent             int
}
```

**Step 3: Propagate in rebuildRows**

In `rebuildRows()` at lines 580-588 (ungrouped plans) and 623-631 (topic plans), pass the new field through to sidebarRow:

```go
rows = append(rows, sidebarRow{
	Kind:               rowKindPlan,
	ID:                 SidebarPlanPrefix + p.Filename,
	Label:              planstate.DisplayName(p.Filename),
	PlanFile:           p.Filename,
	Collapsed:          !s.expandedPlans[p.Filename],
	HasRunning:         isPlanActive(effective.Status),
	HasNotification:    effective.Status == string(planstate.StatusReviewing),
	HasOrphanedSession: effective.HasOrphanedSession,
	Indent:             0,
})
```

Do the same for the topic-nested plan rows at line 623.

**Step 4: Verify build**

Run: `go build ./...`
Expected: compiles clean

**Step 5: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(sidebar): add HasOrphanedSession field to PlanDisplay and sidebarRow"
```

---

### Task 3: Render gold indicator for orphaned sessions

**Files:**
- Modify: `ui/sidebar.go` — `renderPlanRow` (~line 905-917), `renderTopicRow` (~line 854-860), and flat-mode plan rendering (~line 1070-1082)

**Step 1: Add orphan style**

Add next to the existing sidebar styles around line 124:

```go
var sidebarOrphanStyle = lipgloss.NewStyle().
	Foreground(ColorGold)
```

**Step 2: Update renderPlanRow (tree mode)**

In `renderPlanRow` around line 905-917, insert the orphan case between notification and running:

```go
var statusGlyph string
var statusStyle lipgloss.Style
switch {
case row.HasNotification:
	statusGlyph = "◉"
	statusStyle = sidebarNotifyStyle
case row.HasRunning:
	statusGlyph = "●"
	statusStyle = sidebarRunningStyle
case row.HasOrphanedSession:
	statusGlyph = "◎"
	statusStyle = sidebarOrphanStyle
default:
	statusGlyph = "○"
	statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
}
```

**Step 3: Update renderTopicRow (tree mode)**

In the topic row rendering around line 854-860, add the orphan case:

```go
if row.HasNotification {
	statusGlyph = "◉"
	statusStyle = sidebarNotifyStyle
} else if row.HasRunning {
	statusGlyph = "●"
	statusStyle = sidebarRunningStyle
} else if row.HasOrphanedSession {
	statusGlyph = "◎"
	statusStyle = sidebarOrphanStyle
}
```

**Step 4: Update flat-mode plan rendering**

In the flat-mode switch around line 1070-1082, add the orphan case:

```go
case item.HasOrphanedSession:
	statusGlyph = "◎"
	statusStyle = sidebarOrphanStyle
```

Insert after the `HasRunning` case, before `default`.

Note: `SidebarItem` also needs a `HasOrphanedSession bool` field for flat mode to work. Add it to the `SidebarItem` struct at line 148.

**Step 5: Run tests and verify build**

Run: `go test ./ui/ -v -count=1 && go build ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(sidebar): render gold orphan indicator for plans with stale tmux sessions"
```

---

### Task 4: Detect orphans in updateSidebarPlans

**Files:**
- Modify: `app/app_state.go:516-570` (`updateSidebarPlans`)

**Step 1: Add orphan detection helper**

Add a helper function near `updateSidebarPlans`:

```go
// planSessionName returns the tmux session name that kasmos would have created
// for a plan agent at the given lifecycle action (plan, implement, review).
func planSessionName(planFile, action string) string {
	title := planstate.DisplayName(planFile) + "-" + action
	return tmux.ToKasTmuxName(title)
}

// hasManagedInstance returns true if any running instance belongs to the given plan file.
func (m *home) hasManagedInstance(planFile string) bool {
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == planFile && inst.Started() {
			return true
		}
	}
	return false
}

// detectOrphanedSession checks whether a plan in an active state has an orphaned
// tmux session (no managed instance but tmux session still alive).
func (m *home) detectOrphanedSession(planFile string, status planstate.Status) bool {
	if status != planstate.StatusPlanning && status != planstate.StatusImplementing {
		return false
	}
	if m.hasManagedInstance(planFile) {
		return false
	}
	// Check for planner session
	if tmux.SessionExists(planSessionName(planFile, "plan")) {
		return true
	}
	// Check for coder session (implement stage may have task sessions)
	if tmux.SessionExists(planSessionName(planFile, "implement")) {
		return true
	}
	return false
}
```

**Step 2: Wire into updateSidebarPlans**

In `updateSidebarPlans`, when building `PlanDisplay` entries (both ungrouped at line 549 and topic-grouped at line 532), set the new field:

```go
planDisplays = append(planDisplays, ui.PlanDisplay{
	Filename:           p.Filename,
	Status:             string(p.Status),
	Description:        p.Description,
	Branch:             p.Branch,
	Topic:              p.Topic,
	HasOrphanedSession: m.detectOrphanedSession(p.Filename, p.Status),
})
```

Do the same for the ungrouped loop.

**Step 3: Add tmux import**

Add `"github.com/kastheco/kasmos/session/tmux"` to the imports in `app_state.go` if not already present.

**Step 4: Run tests and verify build**

Run: `go test ./app/ -v -count=1 && go build ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add app/app_state.go
git commit -m "feat(plans): detect orphaned tmux sessions on sidebar refresh"
```

---

## Wave 2: Context Menu Actions

### Task 5: Add context menu items for orphaned sessions

**Files:**
- Modify: `app/app_actions.go:447-485` (`openPlanContextMenu`)

**Step 1: Check for orphan and add menu items**

In `openPlanContextMenu`, after building the existing `items` slice and before appending the common items, check for an orphaned session:

```go
// Offer recovery actions for orphaned tmux sessions
if m.planState != nil {
	if entry, ok := m.planState.Plans[planFile]; ok {
		if m.detectOrphanedSession(planFile, entry.Status) {
			items = append(items,
				overlay.ContextMenuItem{Label: "Attach to session", Action: "attach_orphan"},
				overlay.ContextMenuItem{Label: "Kill stale session", Action: "kill_orphan"},
			)
		}
	}
}
```

Insert this block right after the existing status-based switch (after line 468) and before the common items append (line 470).

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles clean

**Step 3: Commit**

```bash
git add app/app_actions.go
git commit -m "feat(context-menu): add attach/kill options for orphaned plan sessions"
```

---

### Task 6: Implement attach_orphan action

**Files:**
- Modify: `app/app_actions.go` (`executeContextAction`)

**Step 1: Add the attach_orphan case**

Add to the `executeContextAction` switch:

```go
case "attach_orphan":
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" || m.planState == nil {
		return m, nil
	}
	entry, ok := m.planState.Entry(planFile)
	if !ok {
		return m, nil
	}

	// Determine which action's session is alive
	action := "plan"
	sessionName := planSessionName(planFile, "plan")
	if !tmux.SessionExists(sessionName) {
		action = "implement"
		sessionName = planSessionName(planFile, "implement")
		if !tmux.SessionExists(sessionName) {
			m.toastManager.Error("Orphaned session no longer exists")
			return m, m.toastTickCmd()
		}
	}

	agentType, _ := agentTypeForSubItem(action)
	title := planstate.DisplayName(planFile) + "-" + action

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      m.activeRepoPath,
		Program:   m.program,
		PlanFile:  planFile,
		AgentType: agentType,
	})
	if err != nil {
		return m, m.handleError(err)
	}

	// Wire up existing tmux session for restoration
	tmuxSess := tmux.NewTmuxSession(title, m.program, false)
	inst.SetTmuxSession(tmuxSess)

	m.newInstanceFinalizer = m.list.AddInstance(inst)
	m.list.SelectInstance(inst)
	inst.SetStatus(session.Loading)
	inst.LoadingMessage = "Reattaching to session..."

	startCmd := func() tea.Msg {
		var startErr error
		if action == "plan" {
			startErr = inst.StartOnMainBranch()
		} else {
			shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, entry.Branch)
			if err := shared.Setup(); err != nil {
				return instanceStartedMsg{instance: inst, err: err}
			}
			startErr = inst.StartInSharedWorktree(shared, entry.Branch)
		}
		return instanceStartedMsg{instance: inst, err: startErr}
	}

	return m, tea.Batch(tea.WindowSize(), startCmd)
```

**Step 2: Expose SetTmuxSession on Instance**

In `session/instance.go`, add a setter so the app layer can inject the pre-existing tmux session:

```go
// SetTmuxSession sets the tmux session for this instance. Used for reattaching
// to orphaned sessions where the tmux session already exists.
func (i *Instance) SetTmuxSession(ts *tmux.TmuxSession) {
	i.tmuxSession = ts
}
```

**Step 3: Verify the Start methods handle pre-set tmuxSession**

Both `StartOnMainBranch` (line 115-117) and `StartInSharedWorktree` (line 160-163) already check `if i.tmuxSession != nil` and reuse it. The `Start` method at line 153 in `tmux.go` reattaches via `Restore()` if the session already exists. This is the path we need — the existing tmux session is detected, `Restore()` attaches the PTY. Confirmed: no changes needed in lifecycle methods.

**Step 4: Add required imports to app_actions.go**

Ensure these imports are present:
- `tmux "github.com/kastheco/kasmos/session/tmux"`
- `gitpkg "github.com/kastheco/kasmos/session/git"` (already present from merge_plan action)

**Step 5: Run tests and verify build**

Run: `go test ./... -count=1 2>&1 | tail -25`
Expected: all PASS

**Step 6: Commit**

```bash
git add app/app_actions.go session/instance.go
git commit -m "feat(plans): implement attach_orphan to reconnect to surviving agent sessions"
```

---

### Task 7: Implement kill_orphan action

**Files:**
- Modify: `app/app_actions.go` (`executeContextAction`)

**Step 1: Add the kill_orphan case**

Add to the `executeContextAction` switch:

```go
case "kill_orphan":
	planFile := m.sidebar.GetSelectedPlanFile()
	if planFile == "" || m.planState == nil {
		return m, nil
	}

	// Kill any matching tmux sessions
	for _, action := range []string{"plan", "implement", "review"} {
		sessionName := planSessionName(planFile, action)
		if tmux.SessionExists(sessionName) {
			exec.Command("tmux", "kill-session", "-t", sessionName).Run()
		}
	}

	// Reset plan state: planning → ready
	entry, ok := m.planState.Entry(planFile)
	if ok && entry.Status == planstate.StatusPlanning {
		if err := m.fsm.Transition(planFile, planfsm.PlannerFinished); err != nil {
			return m, m.handleError(err)
		}
	}

	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateSidebarItems()
	m.toastManager.Info("Stale session killed")
	return m, tea.Batch(tea.WindowSize(), m.toastTickCmd())
```

**Step 2: Add `os/exec` import if not present**

**Step 3: Run tests and verify build**

Run: `go test ./app/ -v -count=1 && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add app/app_actions.go
git commit -m "feat(plans): implement kill_orphan to clean up stale tmux sessions"
```

---

## Wave 3: Testing

### Task 8: Add tests for orphan detection and rendering

**Files:**
- Modify: `ui/sidebar_test.go`

**Step 1: Test orphan indicator renders correctly**

Add a test that creates a `PlanDisplay` with `HasOrphanedSession: true` and verifies the gold `◎` glyph appears in the rendered output:

```go
func TestPlanRow_OrphanedSessionIndicator(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetPlans([]PlanDisplay{{
		Filename:           "orphan.md",
		Status:             string(planstate.StatusPlanning),
		HasOrphanedSession: true,
	}})
	s.SetUseTreeMode(true)
	s.rebuildRows()

	// Find the plan row
	planRow, ok := findRowByID(s.rows, SidebarPlanPrefix+"orphan.md")
	require.True(t, ok)
	assert.True(t, planRow.HasOrphanedSession)
	assert.False(t, planRow.HasRunning)
}
```

**Step 2: Run tests**

Run: `go test ./ui/ -run Orphan -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add ui/sidebar_test.go
git commit -m "test(sidebar): verify orphan indicator propagation"
```
