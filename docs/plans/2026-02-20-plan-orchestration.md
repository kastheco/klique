# Plan-Aware Orchestration + Plan Browser Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make plans first-class in klique: browse unfinished plans in the sidebar, spawn coder sessions from them, and auto-spawn a reviewer when all tasks complete.

**Architecture:** New `config/planstate` package reads/writes `plan-state.json`. Sidebar gains a "Plans" section above topics. Instance gains a `PlanFile` field binding it to a plan. The existing tick loop polls plan state for bound instances; on all-done, spawns a reviewer session from a template. Session exit is a fallback trigger.

**Tech Stack:** Go, bubbletea, lipgloss, existing sidebar/instance/storage infrastructure.

---

### Task 1: Create `config/planstate` package — read/write plan-state.json

**Files:**
- Create: `config/planstate/planstate.go`
- Create: `config/planstate/planstate_test.go`

**Step 1: Write the failing test**

Create `config/planstate/planstate_test.go`:

```go
package planstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs", "plans", "plan-state.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`{
		"my-plan.md": {"status": "ready"},
		"done-plan.md": {"status": "done", "implemented": "2026-02-20"}
	}`), 0o644))

	ps, err := Load(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, ps.Plans, 2)
	assert.Equal(t, "ready", ps.Plans["my-plan.md"].Status)
	assert.Equal(t, "done", ps.Plans["done-plan.md"].Status)
}

func TestLoad_Missing(t *testing.T) {
	dir := t.TempDir()
	ps, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, ps.Plans)
}

func TestUnfinished(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: "ready"},
			"b.md": {Status: "in_progress"},
			"c.md": {Status: "reviewing"},
			"d.md": {Status: "done"},
		},
	}
	unfinished := ps.Unfinished()
	assert.Len(t, unfinished, 3)
	// done should not appear
	for _, p := range unfinished {
		assert.NotEqual(t, "d.md", p.Filename)
	}
}

func TestAllTasksDone(t *testing.T) {
	ps := &PlanState{
		Dir: "/tmp",
		Plans: map[string]PlanEntry{
			"a.md": {Status: "done"},
			"b.md": {Status: "done"},
		},
	}
	assert.True(t, ps.AllTasksDone("a.md"))

	ps.Plans["c.md"] = PlanEntry{Status: "in_progress"}
	assert.True(t, ps.AllTasksDone("a.md"))  // checks specific plan
}

func TestSetStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a.md": {"status": "in_progress"}}`), 0o644))

	ps, err := Load(dir)
	require.NoError(t, err)

	require.NoError(t, ps.SetStatus("a.md", "reviewing"))
	assert.Equal(t, "reviewing", ps.Plans["a.md"].Status)

	// Verify it persisted
	ps2, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "reviewing", ps2.Plans["a.md"].Status)
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./config/planstate/ -v
```

Expected: FAIL — package doesn't exist.

**Step 3: Implement the package**

Create `config/planstate/planstate.go`:

```go
package planstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// PlanEntry is one plan's state in plan-state.json.
type PlanEntry struct {
	Status      string `json:"status"`
	Implemented string `json:"implemented,omitempty"`
}

// PlanState holds all plan entries and the directory they were loaded from.
type PlanState struct {
	Dir   string
	Plans map[string]PlanEntry
}

// PlanInfo is a plan entry with its filename attached, for display.
type PlanInfo struct {
	Filename string
	Status   string
}

const stateFile = "plan-state.json"

// Load reads plan-state.json from dir. Returns empty state if file missing.
func Load(dir string) (*PlanState, error) {
	path := filepath.Join(dir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PlanState{Dir: dir, Plans: make(map[string]PlanEntry)}, nil
		}
		return nil, fmt.Errorf("read plan state: %w", err)
	}

	var plans map[string]PlanEntry
	if err := json.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("parse plan state: %w", err)
	}
	return &PlanState{Dir: dir, Plans: plans}, nil
}

// Unfinished returns plans that are not "done", sorted by filename.
func (ps *PlanState) Unfinished() []PlanInfo {
	var result []PlanInfo
	for name, entry := range ps.Plans {
		if entry.Status != "done" {
			result = append(result, PlanInfo{Filename: name, Status: entry.Status})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// AllTasksDone returns true if the given plan has status "done".
func (ps *PlanState) AllTasksDone(filename string) bool {
	entry, ok := ps.Plans[filename]
	if !ok {
		return false
	}
	return entry.Status == "done"
}

// SetStatus updates a plan's status and persists to disk.
func (ps *PlanState) SetStatus(filename, status string) error {
	entry := ps.Plans[filename]
	entry.Status = status
	ps.Plans[filename] = entry
	return ps.save()
}

func (ps *PlanState) save() error {
	data, err := json.MarshalIndent(ps.Plans, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan state: %w", err)
	}
	path := filepath.Join(ps.Dir, stateFile)
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
```

**Step 4: Run tests**

```bash
go test ./config/planstate/ -v
```

Expected: ALL PASS

**Step 5: Commit**

```bash
git add config/planstate/
git commit -m "feat: add config/planstate package for plan-state.json read/write"
```

---

### Task 2: Add `PlanFile` field to Instance + serialization

**Files:**
- Modify: `session/instance.go` — add `PlanFile` field to `Instance` and `InstanceOptions`
- Modify: `session/storage.go` — add `PlanFile` to `InstanceData`

**Step 1: Add `PlanFile` to `InstanceData`**

In `session/storage.go`, add to the `InstanceData` struct:

```go
PlanFile string `json:"plan_file,omitempty"`
```

**Step 2: Add `PlanFile` to `Instance` and `InstanceOptions`**

In `session/instance.go`:

Add to `Instance` struct (after `TopicName`):
```go
// PlanFile is the plan filename this instance is implementing (empty = no plan).
PlanFile string
```

Add to `InstanceOptions` struct:
```go
// PlanFile binds this instance to a plan from plan-state.json.
PlanFile string
```

**Step 3: Update `ToInstanceData`**

Add `PlanFile: i.PlanFile` to the `InstanceData` literal in `ToInstanceData()`.

**Step 4: Update `FromInstanceData`**

Add `PlanFile: data.PlanFile` to the `Instance` literal in `FromInstanceData()`.

**Step 5: Update `NewInstance`**

Add `PlanFile: opts.PlanFile` to the `Instance` literal in `NewInstance()`.

**Step 6: Verify build**

```bash
go build ./...
```

Expected: clean build.

**Step 7: Run existing tests**

```bash
go test ./session/... -v
```

Expected: PASS (additive field, no breakage).

**Step 8: Commit**

```bash
git add session/instance.go session/storage.go
git commit -m "feat: add PlanFile field to Instance for plan↔session binding"
```

---

### Task 3: Add Plans section to Sidebar

**Files:**
- Modify: `ui/sidebar.go`

**Step 1: Add plan-related types and constants**

Add a new `SidebarPlan` constant prefix and plan item tracking. Add to the constants:

```go
SidebarPlanPrefix = "__plan__"
```

Add a new struct for plan display info (or reuse `SidebarItem` with a plan-specific ID).

**Step 2: Add `SetPlans` method to Sidebar**

```go
// PlanDisplay holds plan info for sidebar rendering.
type PlanDisplay struct {
	Filename string
	Status   string // "ready", "in_progress", "reviewing"
}

// plans stores unfinished plans for sidebar display.
// Set by the app on each tick when plan state changes.
func (s *Sidebar) SetPlans(plans []PlanDisplay) {
	s.plans = plans
}
```

Add `plans []PlanDisplay` field to the `Sidebar` struct.

**Step 3: Update `SetItems` to include plans section**

After building the "All" item but before topics, insert the Plans section if there are
unfinished plans:

```go
if len(s.plans) > 0 {
	items = append(items, SidebarItem{Name: "Plans", IsSection: true})
	for _, p := range s.plans {
		items = append(items, SidebarItem{
			Name: planDisplayName(p.Filename),
			ID:   SidebarPlanPrefix + p.Filename,
			// Encode status via HasRunning/HasNotification for icon reuse:
			// in_progress → HasRunning, reviewing → HasNotification
			HasRunning:      p.Status == "in_progress",
			HasNotification: p.Status == "reviewing",
		})
	}
}
```

Add a helper to derive a short display name from the plan filename:

```go
func planDisplayName(filename string) string {
	// "2026-02-20-my-feature.md" → "my-feature"
	name := strings.TrimSuffix(filename, ".md")
	// Strip date prefix (YYYY-MM-DD-)
	if len(name) > 11 && name[4] == '-' && name[7] == '-' && name[10] == '-' {
		name = name[11:]
	}
	return name
}
```

**Step 4: Add `GetSelectedPlanFile` helper**

```go
// GetSelectedPlanFile returns the plan filename if a plan item is selected, or "".
func (s *Sidebar) GetSelectedPlanFile() string {
	id := s.GetSelectedID()
	if strings.HasPrefix(id, SidebarPlanPrefix) {
		return id[len(SidebarPlanPrefix):]
	}
	return ""
}
```

**Step 5: Add status glyphs for plans in the render**

In the `String()` method, when rendering items, add a prefix glyph based on plan status.
Plans use the same item rendering but with a status indicator:

- `○` for `ready` (using existing muted style)
- `●` for `in_progress` (using existing running style)
- `◉` for `reviewing` (using existing notification style)

This can be done by checking `strings.HasPrefix(item.ID, SidebarPlanPrefix)` during
rendering to use the appropriate prefix glyph instead of the default `" "` / `"▸"`.

**Step 6: Verify build**

```bash
go build ./...
```

Expected: clean.

**Step 7: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): add Plans section to sidebar showing unfinished plans"
```

---

### Task 4: Wire plan state polling into app tick loop

**Files:**
- Modify: `app/app.go` — add `planState` field to `home`, load on init
- Modify: `app/app_state.go` — add `updatePlanState()` called from tick, update sidebar

**Step 1: Add `planState` to `home` model**

In `app/app.go`, add to the `home` struct:

```go
planState    *planstate.PlanState
planStateDir string // path to docs/plans/ for the active repo
```

Import `"github.com/kastheco/kasmos/config/planstate"`.

**Step 2: Load plan state on init and repo switch**

In the initialization code (wherever `m.activeRepoPath` is set), load plan state:

```go
func (m *home) loadPlanState() {
	dir := filepath.Join(m.activeRepoPath, "docs", "plans")
	ps, err := planstate.Load(dir)
	if err != nil {
		log.WarningLog.Printf("could not load plan state: %v", err)
		return
	}
	m.planState = ps
	m.planStateDir = dir
}
```

Call this from init and whenever the active repo changes.

**Step 3: Add plan state refresh to tick**

In the `tickUpdateMetadataMessage` handler in `app.go` (around line 358), after the
instance loop, add:

```go
// Refresh plan state from disk
m.loadPlanState()
if m.planState != nil {
	var plans []ui.PlanDisplay
	for _, p := range m.planState.Unfinished() {
		plans = append(plans, ui.PlanDisplay{
			Filename: p.Filename,
			Status:   p.Status,
		})
	}
	m.sidebar.SetPlans(plans)
}
```

**Step 4: Update `updateSidebarItems` to pass plan data**

The existing `updateSidebarItems()` calls `s.sidebar.SetItems(...)`. Plans are set
separately via `SetPlans`, which is called before `SetItems` during the tick.
`SetItems` already reads `s.plans` to build the items list (from Task 3).

**Step 5: Verify build**

```bash
go build ./...
```

Expected: clean.

**Step 6: Commit**

```bash
git add app/app.go app/app_state.go
git commit -m "feat: poll plan-state.json on tick, feed plans to sidebar"
```

---

### Task 5: Handle plan selection → spawn coder session

**Files:**
- Modify: `app/app_input.go` — handle Enter on a plan sidebar item

**Step 1: Detect plan selection in Enter handler**

In `app_input.go`, where sidebar Enter/selection is handled, add a check:

```go
if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
	return m.spawnPlanSession(planFile)
}
```

**Step 2: Implement `spawnPlanSession`**

Add to `app/app_state.go` or `app/app_actions.go`:

```go
func (m *home) spawnPlanSession(planFile string) (tea.Model, tea.Cmd) {
	// Prevent duplicate sessions for the same plan
	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == planFile {
			// Select the existing instance instead
			// (find its index and select it)
			return m, m.handleError(fmt.Errorf("plan already has an active session"))
		}
	}

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:    planDisplayTitle(planFile),
		Path:     m.activeRepoPath,
		Program:  m.program,
		PlanFile: planFile,
	})
	if err != nil {
		return m, m.handleError(err)
	}

	m.newInstanceFinalizer = m.list.AddInstance(instance)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)

	// Send the plan prompt after instance starts
	prompt := fmt.Sprintf("Implement docs/plans/%s using the executing-plans skill, task by task.", planFile)
	instance.QueuedPrompt = prompt

	m.state = stateDefault
	return m, m.startInstance(instance)
}
```

Note: `QueuedPrompt` is a new field on Instance that gets sent after the session is ready.
Alternatively, use the existing `promptAfterName` flow — skip the naming step (title
derived from plan filename) and go straight to starting with a prompt.

**Step 3: Handle queued prompt delivery**

In the tick loop, after detecting an instance is Ready for the first time, check if it
has a queued prompt and send it:

```go
if instance.PlanFile != "" && instance.QueuedPrompt != "" && instance.Status == session.Ready {
	if err := instance.SendPrompt(instance.QueuedPrompt); err != nil {
		log.WarningLog.Printf("could not send plan prompt: %v", err)
	}
	instance.QueuedPrompt = ""
}
```

**Step 4: Verify build**

```bash
go build ./...
```

Expected: clean.

**Step 5: Commit**

```bash
git add app/app_input.go app/app_state.go session/instance.go
git commit -m "feat: spawn coder session from plan sidebar selection"
```

---

### Task 6: Add review prompt template to scaffold

**Files:**
- Create: `internal/initcmd/scaffold/templates/shared/review-prompt.md`

**Step 1: Create the review prompt template**

```markdown
Review the implementation of plan: {{PLAN_NAME}}

Plan file: {{PLAN_FILE}}

Read the plan file to understand the goals, architecture, and tasks that were implemented.
Then review all changes made during implementation. Load the `requesting-code-review`
superpowers skill for structured review methodology.

Focus areas:
- Does the implementation match the plan's stated goals and architecture?
- Were any tasks implemented incorrectly or incompletely?
- Code quality, error handling, test coverage
- Regressions or unintended side effects
```

**Step 2: Verify it's picked up by the existing embed**

The `//go:embed templates` directive already captures everything under `templates/`.
No code change needed for embed — just verify:

```bash
go build ./...
```

Expected: clean.

**Step 3: Add a `LoadReviewPrompt` function to scaffold**

In `scaffold.go`:

```go
// LoadReviewPrompt reads the embedded review prompt template and fills placeholders.
func LoadReviewPrompt(planFile, planName string) string {
	content, err := templates.ReadFile("templates/shared/review-prompt.md")
	if err != nil {
		return fmt.Sprintf("Review the implementation of plan: %s\nPlan file: %s", planName, planFile)
	}
	result := strings.ReplaceAll(string(content), "{{PLAN_FILE}}", planFile)
	result = strings.ReplaceAll(result, "{{PLAN_NAME}}", planName)
	return result
}
```

**Step 4: Commit**

```bash
git add internal/initcmd/scaffold/templates/shared/review-prompt.md internal/initcmd/scaffold/scaffold.go
git commit -m "feat(scaffold): add embedded review prompt template"
```

---

### Task 7: Plan completion detection + auto-spawn reviewer

**Files:**
- Modify: `app/app_state.go` — add completion detection in tick loop

**Step 1: Add completion detection**

In the tick handler, after refreshing plan state (added in Task 4), add:

```go
// Check for plan completion → spawn reviewer
m.checkPlanCompletion()
```

**Step 2: Implement `checkPlanCompletion`**

```go
func (m *home) checkPlanCompletion() tea.Cmd {
	if m.planState == nil {
		return nil
	}

	for _, inst := range m.list.GetInstances() {
		if inst.PlanFile == "" {
			continue
		}
		// Only trigger for coder sessions (not reviewer sessions)
		if inst.IsReviewer {
			continue
		}
		// Check if this plan's status is now "done" in plan-state.json
		if m.planState.AllTasksDone(inst.PlanFile) {
			return m.transitionToReview(inst)
		}
	}
	return nil
}
```

**Step 3: Implement `transitionToReview`**

```go
func (m *home) transitionToReview(coderInst *session.Instance) tea.Cmd {
	planFile := coderInst.PlanFile

	// Update plan state to "reviewing"
	if err := m.planState.SetStatus(planFile, "reviewing"); err != nil {
		log.WarningLog.Printf("could not set plan to reviewing: %v", err)
	}

	// Derive plan name from filename
	planName := planDisplayTitle(planFile)
	planPath := fmt.Sprintf("docs/plans/%s", planFile)

	// Build review prompt from template
	prompt := scaffold.LoadReviewPrompt(planPath, planName)

	// Spawn reviewer instance
	reviewerInst, err := session.NewInstance(session.InstanceOptions{
		Title:    planName + "-review",
		Path:     m.activeRepoPath,
		Program:  m.reviewerProgram, // may need config for reviewer harness
		PlanFile: planFile,
	})
	if err != nil {
		log.WarningLog.Printf("could not create reviewer instance: %v", err)
		return nil
	}
	reviewerInst.IsReviewer = true
	reviewerInst.QueuedPrompt = prompt

	m.newInstanceFinalizer = m.list.AddInstance(reviewerInst)
	return m.startInstance(reviewerInst)
}
```

Note: `IsReviewer` is a new bool field on Instance to distinguish coder vs reviewer
sessions for the same plan. The reviewer program/harness may come from config — for now,
use the same program as the coder. A future enhancement could allow configuring a
separate reviewer program.

**Step 4: Add fallback on session exit**

In the code that handles tmux session death (where `DoesSessionExist()` returns false
for a running instance), add:

```go
if inst.PlanFile != "" && !inst.IsReviewer && m.planState != nil {
	if m.planState.AllTasksDone(inst.PlanFile) {
		cmds = append(cmds, m.transitionToReview(inst))
	}
}
```

**Step 5: Handle reviewer completion → mark plan done**

When a reviewer instance exits (session dies or user closes it), if it has `IsReviewer`
and `PlanFile` set, transition the plan to "done":

```go
if inst.PlanFile != "" && inst.IsReviewer && m.planState != nil {
	if err := m.planState.SetStatus(inst.PlanFile, "done"); err != nil {
		log.WarningLog.Printf("could not set plan to done: %v", err)
	}
}
```

**Step 6: Verify build**

```bash
go build ./...
```

Expected: clean.

**Step 7: Commit**

```bash
git add app/app_state.go session/instance.go
git commit -m "feat: auto-spawn reviewer on plan completion, mark done on reviewer exit"
```

---

### Task 8: Add `IsReviewer` and `QueuedPrompt` to Instance serialization

**Files:**
- Modify: `session/instance.go`
- Modify: `session/storage.go`

**Step 1: Add fields to `Instance`**

```go
// IsReviewer is true when this instance is a reviewer session for a plan.
IsReviewer bool
// QueuedPrompt is sent to the session once it becomes ready. Cleared after send.
QueuedPrompt string
```

**Step 2: Add to `InstanceData`**

```go
IsReviewer   bool   `json:"is_reviewer,omitempty"`
QueuedPrompt string `json:"queued_prompt,omitempty"`
```

**Step 3: Update `ToInstanceData` and `FromInstanceData`**

Add the two fields to both conversion functions.

**Step 4: Run tests**

```bash
go test ./session/... -v
go build ./...
```

Expected: PASS

**Step 5: Commit**

```bash
git add session/instance.go session/storage.go
git commit -m "feat: persist IsReviewer and QueuedPrompt in instance storage"
```

---

### Task 9: Integration test — full plan lifecycle

**Files:**
- Create: `config/planstate/planstate_test.go` (extend with lifecycle test)

**Step 1: Write lifecycle test**

```go
func TestPlanLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"test-plan.md": {"status": "ready"}
	}`), 0o644))

	// Load
	ps, err := Load(dir)
	require.NoError(t, err)

	// Start implementing
	require.NoError(t, ps.SetStatus("test-plan.md", "in_progress"))
	unfinished := ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, "in_progress", unfinished[0].Status)

	// Complete implementation → reviewing
	require.NoError(t, ps.SetStatus("test-plan.md", "done"))
	assert.True(t, ps.AllTasksDone("test-plan.md"))

	// Transition to reviewing (klique does this)
	require.NoError(t, ps.SetStatus("test-plan.md", "reviewing"))
	unfinished = ps.Unfinished()
	require.Len(t, unfinished, 1)
	assert.Equal(t, "reviewing", unfinished[0].Status)

	// Review complete → done
	require.NoError(t, ps.SetStatus("test-plan.md", "done"))
	unfinished = ps.Unfinished()
	assert.Empty(t, unfinished)
}
```

**Step 2: Run tests**

```bash
go test ./config/planstate/ -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add config/planstate/planstate_test.go
git commit -m "test: add plan lifecycle integration test"
```

---

### Task 10: Build verification and final cleanup

**Step 1: Run full test suite**

```bash
go test ./... -count=1
```

Expected: ALL PASS

**Step 2: Build the binary**

```bash
go build ./...
```

Expected: clean.

**Step 3: Manual smoke test**

1. Run `kq`
2. Verify sidebar shows "Plans" section with unfinished plans
3. Select a plan, press Enter — coder session spawns with plan prompt
4. Verify plan state transitions work

**Step 4: Run typos check**

```bash
typos config/planstate/ app/ session/instance.go session/storage.go ui/sidebar.go
```

Expected: no typos.

**Step 5: Final commit**

```bash
git add -A
git commit -m "feat: plan-aware orchestration with sidebar browser and auto-review"
```
