# Task-Driven Orchestration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add superpowers plan awareness to klique — parse plan files, display task-grouped instances in the TUI, and auto-switch agents across the implement/verify/review lifecycle.

**Architecture:** New `plan/` package parses `docs/plans/*.md` into a structured task graph. Task state tracked in `~/.klique/state.json`. Instance list gains polymorphic items (phase headers, task rows, instance rows). Agent profiles in config map lifecycle phases to `{program, flags}` pairs; klique swaps agents in the same tmux session on phase transitions.

**Tech Stack:** Go 1.24+, bubbletea v1.3.10, lipgloss v1.1.0, testify, regex-based markdown parser

**Design doc:** `docs/plans/2026-02-20-task-orchestration-design.md`

---

## Phase 1: Plan Parser

### Task 1: Plan Data Model and Phase/Task Parsing

**Files:**
- Create: `plan/plan.go`
- Create: `plan/parser.go`
- Create: `plan/parser_test.go`

**Step 1: Write failing test for parsing phases from markdown**

In `plan/parser_test.go`, write a table-driven test that feeds a markdown string containing two phases with tasks and asserts the resulting `Plan` struct has the right phase count, names, and task counts.

Test input markdown:
```markdown
# Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans

**Goal:** Build the feature

---

## Phase 1: Foundation

### Task 1: Create types

**Files:**
- Create: `plan/plan.go`

**Step 1: Write the types**

Some description.

**Step 2: Commit**

```bash
git commit -m "feat: add types"
```

---

### Task 2: Add parser

**Files:**
- Create: `plan/parser.go`
- Modify: `plan/plan.go:10-20`
- Test: `plan/parser_test.go`

**Step 1: Write the parser**

Parser code here.

---

## Phase 2: Integration

### Task 3: Wire it up

**Files:**
- Modify: `app/app.go:50-60`

**Step 1: Add field**

Description.
```

Assert:
- `plan.Goal == "Build the feature"`
- `len(plan.Phases) == 2`
- `plan.Phases[0].Name == "Foundation"`, `plan.Phases[0].Number == 1`
- `len(plan.Phases[0].Tasks) == 2`
- `plan.Phases[0].Tasks[0].Name == "Create types"`, `.Number == 1`
- `plan.Phases[0].Tasks[1].Name == "Add parser"`, `.Number == 2`
- `plan.Phases[1].Name == "Integration"`, `.Number == 2`
- `len(plan.Phases[1].Tasks) == 1`

**Step 2: Run test to verify it fails**

Run: `go test ./plan/ -run TestParsePlan -v`
Expected: FAIL — package `plan` does not exist

**Step 3: Create plan data model in `plan/plan.go`**

```go
package plan

// FileAction describes what to do with a file.
type FileAction string

const (
	FileCreate    FileAction = "create"
	FileModify    FileAction = "modify"
	FileTest      FileAction = "test"
	FileReference FileAction = "reference"
)

// FileRef is a file referenced by a task.
type FileRef struct {
	Action FileAction
	Path   string
	Lines  string // e.g. "10-20", empty if not specified
}

// Step is a numbered step within a task.
type Step struct {
	Number      int
	Description string
	Command     string // from "Run: `...`" lines
	Expected    string // from "Expected: ..." lines
}

// PlanTask is a single task extracted from a plan.
type PlanTask struct {
	Number    int
	Name      string
	Files     []FileRef
	Steps     []Step
	CommitMsg string
	RawText   string // full markdown text of the task block
}

// Phase is a group of tasks within a plan.
type Phase struct {
	Number int
	Name   string
	Tasks  []PlanTask
}

// Plan is a parsed superpowers implementation plan.
type Plan struct {
	FilePath string
	Goal     string
	Phases   []Phase
}
```

**Step 4: Write the parser in `plan/parser.go`**

```go
package plan

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

var (
	phaseRe = regexp.MustCompile(`^## Phase (\d+):\s*(.+)`)
	taskRe  = regexp.MustCompile(`^### Task (\d+):\s*(.+)`)
	goalRe  = regexp.MustCompile(`^\*\*Goal:\*\*\s*(.+)`)
	stepRe  = regexp.MustCompile(`^\*\*Step (\d+):\s*(.+)\*\*`)
	fileRe  = regexp.MustCompile(`^- (Create|Modify|Test|Reference):\s*` + "`" + `([^` + "`" + `]+)` + "`" + `(.*)`)
	filesRe = regexp.MustCompile(`^\*\*Files:\*\*`)
)

// Parse reads a superpowers plan markdown string and returns a Plan.
func Parse(content string) (*Plan, error) {
	p := &Plan{}
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentPhase *Phase
	var currentTask *PlanTask
	var taskTextBuilder strings.Builder
	inFiles := false

	flushTask := func() {
		if currentTask != nil {
			currentTask.RawText = strings.TrimSpace(taskTextBuilder.String())
			if currentPhase != nil {
				currentPhase.Tasks = append(currentPhase.Tasks, *currentTask)
			}
			currentTask = nil
			taskTextBuilder.Reset()
		}
	}

	flushPhase := func() {
		flushTask()
		if currentPhase != nil {
			p.Phases = append(p.Phases, *currentPhase)
			currentPhase = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Goal line
		if m := goalRe.FindStringSubmatch(line); m != nil {
			p.Goal = m[1]
			continue
		}

		// Phase header
		if m := phaseRe.FindStringSubmatch(line); m != nil {
			flushPhase()
			num, _ := strconv.Atoi(m[1])
			currentPhase = &Phase{Number: num, Name: strings.TrimSpace(m[2])}
			continue
		}

		// Task header
		if m := taskRe.FindStringSubmatch(line); m != nil {
			flushTask()
			num, _ := strconv.Atoi(m[1])
			currentTask = &PlanTask{Number: num, Name: strings.TrimSpace(m[2])}
			inFiles = false
			taskTextBuilder.WriteString(line + "\n")
			continue
		}

		// Accumulate raw text for current task
		if currentTask != nil {
			taskTextBuilder.WriteString(line + "\n")
		}

		// Files section start
		if filesRe.MatchString(line) {
			inFiles = true
			continue
		}

		// File reference lines
		if inFiles && currentTask != nil {
			if m := fileRe.FindStringSubmatch(line); m != nil {
				action := FileAction(strings.ToLower(m[1]))
				path := m[2]
				lines := ""
				if idx := strings.Index(m[2], ":"); idx != -1 {
					path = m[2][:idx]
					lines = m[2][idx+1:]
				}
				currentTask.Files = append(currentTask.Files, FileRef{
					Action: action,
					Path:   path,
					Lines:  lines,
				})
				continue
			}
			// Non-file line ends the files section
			if strings.TrimSpace(line) != "" {
				inFiles = false
			}
		}

		// Step header
		if currentTask != nil {
			if m := stepRe.FindStringSubmatch(line); m != nil {
				num, _ := strconv.Atoi(m[1])
				currentTask.Steps = append(currentTask.Steps, Step{
					Number:      num,
					Description: strings.TrimSpace(m[2]),
				})
				inFiles = false
			}
		}
	}

	flushPhase()
	return p, scanner.Err()
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./plan/ -run TestParsePlan -v`
Expected: PASS

**Step 6: Commit**

```bash
git add plan/plan.go plan/parser.go plan/parser_test.go
git commit -m "feat(plan): add plan data model and markdown parser"
```

---

### Task 2: Plan Discovery and Reload Detection

**Files:**
- Create: `plan/discovery.go`
- Create: `plan/discovery_test.go`
- Modify: `plan/plan.go`

**Step 1: Write failing test for plan discovery**

In `plan/discovery_test.go`, create a temp directory with two `.md` files (different mtimes), call `DiscoverPlans(dir)`, and assert:
- Returns both file paths sorted by mtime (newest first)
- `SelectActivePlan(paths)` returns the newest
- Empty directory returns nil

Also test `NeedsReload(plan, currentMtime)` — returns true when file mtime is newer than stored mtime, false otherwise.

**Step 2: Run test to verify it fails**

Run: `go test ./plan/ -run TestDiscoverPlans -v`
Expected: FAIL — `DiscoverPlans` undefined

**Step 3: Implement discovery in `plan/discovery.go`**

```go
package plan

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PlanFile represents a discovered plan file with its modification time.
type PlanFile struct {
	Path    string
	ModTime time.Time
}

// DiscoverPlans finds all .md files in the given directory, sorted by mtime (newest first).
func DiscoverPlans(dir string) ([]PlanFile, error) {
	pattern := filepath.Join(dir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var plans []PlanFile
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		plans = append(plans, PlanFile{Path: path, ModTime: info.ModTime()})
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].ModTime.After(plans[j].ModTime)
	})
	return plans, nil
}

// SelectActivePlan returns the most recently modified plan, or empty string if none.
func SelectActivePlan(plans []PlanFile) string {
	if len(plans) == 0 {
		return ""
	}
	return plans[0].Path
}

// NeedsReload returns true if the file at path has been modified since lastModTime.
func NeedsReload(path string, lastModTime time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.ModTime().After(lastModTime)
}
```

**Step 4: Add `ModTime` field to `Plan` struct in `plan/plan.go`**

Add `ModTime time.Time` to the `Plan` struct so we can track when it was last parsed.

**Step 5: Run tests to verify they pass**

Run: `go test ./plan/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add plan/discovery.go plan/discovery_test.go plan/plan.go
git commit -m "feat(plan): add plan file discovery and reload detection"
```

---

## Phase 2: Config and State Extensions

### Task 3: Agent Profiles in Config

**Files:**
- Modify: `config/config.go`
- Create: `config/profile.go`
- Create: `config/profile_test.go`

**Step 1: Write failing test for profile resolution**

In `config/profile_test.go`, test `ResolveProfile`:
- Config with profiles and phase_roles: `ResolveProfile("implementing")` returns the mapped profile
- Config with profiles but no phase_roles: returns fallback (default_program, no flags)
- Config with phase_roles pointing to unknown profile: returns fallback
- Empty config: returns fallback
- Test all phases: `implementing`, `spec_review`, `quality_review`

**Step 2: Run test to verify it fails**

Run: `go test ./config/ -run TestResolveProfile -v`
Expected: FAIL — `ResolveProfile` undefined

**Step 3: Create `config/profile.go`**

```go
package config

// AgentProfile defines a named agent configuration.
type AgentProfile struct {
	Program string   `json:"program"`
	Flags   []string `json:"flags,omitempty"`
}

// ResolveProfile returns the AgentProfile for a given task phase.
// Falls back to defaultProgram if no matching profile is configured.
func (c *Config) ResolveProfile(phase string, defaultProgram string) AgentProfile {
	if c.PhaseRoles == nil || c.Profiles == nil {
		return AgentProfile{Program: defaultProgram}
	}
	roleName, ok := c.PhaseRoles[phase]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	profile, ok := c.Profiles[roleName]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	if profile.Program == "" {
		return AgentProfile{Program: defaultProgram}
	}
	return profile
}

// BuildCommand returns the full command string for the profile.
func (p AgentProfile) BuildCommand() string {
	if len(p.Flags) == 0 {
		return p.Program
	}
	return p.Program + " " + joinFlags(p.Flags)
}

func joinFlags(flags []string) string {
	result := ""
	for i, f := range flags {
		if i > 0 {
			result += " "
		}
		result += f
	}
	return result
}
```

**Step 4: Add profile fields to Config struct in `config/config.go`**

Add to the `Config` struct:
```go
Profiles  map[string]AgentProfile `json:"profiles,omitempty"`
PhaseRoles map[string]string      `json:"phase_roles,omitempty"`
AutoVerify bool                   `json:"auto_verify,omitempty"`
```

These use `omitempty` so existing config files without these fields load cleanly.

**Step 5: Run tests**

Run: `go test ./config/ -v`
Expected: All PASS (existing + new)

**Step 6: Commit**

```bash
git add config/config.go config/profile.go config/profile_test.go
git commit -m "feat(config): add agent profiles and phase role mapping"
```

---

### Task 4: Plan State in state.json

**Files:**
- Modify: `config/state.go`
- Create: `config/plan_state.go`
- Create: `config/plan_state_test.go`

**Step 1: Write failing test for plan state round-trip**

In `config/plan_state_test.go`:
- Create a `PlanStateMap` with two plans, each with tasks in various phases
- Serialize to JSON, deserialize back, assert equality
- Test that a state file without a `plans` key deserializes with empty `PlanStateMap` (backward compat)

**Step 2: Run test to verify it fails**

Run: `go test ./config/ -run TestPlanState -v`
Expected: FAIL — `PlanStateMap` undefined

**Step 3: Create `config/plan_state.go`**

```go
package config

import "time"

// TaskPhase represents a task's current lifecycle phase.
type TaskPhase string

const (
	PhasePlanned      TaskPhase = "planned"
	PhaseImplementing TaskPhase = "implementing"
	PhaseVerifying    TaskPhase = "verifying"
	PhaseSpecReview   TaskPhase = "spec_review"
	PhaseQualReview   TaskPhase = "quality_review"
	PhaseDone         TaskPhase = "done"
)

// ValidTransitions defines allowed phase transitions.
var ValidTransitions = map[TaskPhase][]TaskPhase{
	PhasePlanned:      {PhaseImplementing},
	PhaseImplementing: {PhaseVerifying},
	PhaseVerifying:    {PhaseImplementing, PhaseSpecReview},
	PhaseSpecReview:   {PhaseQualReview, PhaseImplementing},
	PhaseQualReview:   {PhaseDone, PhaseImplementing},
}

// CanTransition returns true if moving from current to next is valid.
func CanTransition(from, to TaskPhase) bool {
	for _, allowed := range ValidTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// TaskState tracks a single task's lifecycle state.
type TaskState struct {
	Phase            TaskPhase  `json:"phase"`
	InstanceTitle    string     `json:"instance_title,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	VerifyAttempts   int        `json:"verify_attempts,omitempty"`
	LastVerifyResult string     `json:"last_verify_result,omitempty"`
}

// PlanState tracks all task states for a single plan file.
type PlanState struct {
	Active bool                  `json:"active"`
	Tasks  map[string]*TaskState `json:"tasks"`
}

// PlanStateMap tracks all plans, keyed by file path.
type PlanStateMap map[string]*PlanState

// GetOrCreate returns the PlanState for a path, creating it if absent.
func (m PlanStateMap) GetOrCreate(path string) *PlanState {
	if ps, ok := m[path]; ok {
		return ps
	}
	ps := &PlanState{Tasks: make(map[string]*TaskState)}
	m[path] = ps
	return ps
}

// GetTaskState returns a task's state, initializing to Planned if absent.
func (ps *PlanState) GetTaskState(taskNumber string) *TaskState {
	if ts, ok := ps.Tasks[taskNumber]; ok {
		return ts
	}
	ts := &TaskState{Phase: PhasePlanned}
	ps.Tasks[taskNumber] = ts
	return ts
}
```

**Step 4: Add `PlansData` to `State` struct in `config/state.go`**

Add to `State`:
```go
PlansData json.RawMessage `json:"plans,omitempty"`
```

Add `PlanStorage` interface:
```go
type PlanStorage interface {
	SavePlans(data json.RawMessage) error
	GetPlans() json.RawMessage
}
```

Add to `StateManager` composition: `PlanStorage`.

Implement on `*State`:
```go
func (s *State) SavePlans(data json.RawMessage) error {
	s.PlansData = data
	return SaveState(s)
}

func (s *State) GetPlans() json.RawMessage {
	return s.PlansData
}
```

**Step 5: Run tests**

Run: `go test ./config/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add config/state.go config/plan_state.go config/plan_state_test.go
git commit -m "feat(config): add plan state persistence and phase transition machine"
```

---

## Phase 3: TUI — Task-Grouped Instance List

### Task 5: ListItem Interface and Concrete Types

**Files:**
- Create: `ui/list_item.go`
- Create: `ui/list_item_test.go`

**Step 1: Write failing test for ListItem types**

Test that `PhaseHeaderItem`, `TaskRowItem`, and `InstanceRowItem` all implement `ListItem`. Test `IndentLevel()`, `ItemType()`, `IsExpandable()`, `IsSelectable()` for each type.

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestListItem -v`
Expected: FAIL — `ListItem` undefined

**Step 3: Create `ui/list_item.go`**

```go
package ui

import (
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/session"
)

// ListItemType identifies the kind of list item.
type ListItemType int

const (
	ItemPhaseHeader ListItemType = iota
	ItemTaskRow
	ItemInstanceRow
)

// ListItem is the polymorphic interface for items in the instance list.
type ListItem interface {
	ItemType() ListItemType
	GetTitle() string
	IndentLevel() int
	IsExpandable() bool
	IsSelectable() bool
}

// PhaseHeaderItem represents a collapsible phase group header.
type PhaseHeaderItem struct {
	PhaseNumber int
	PhaseName   string
	TaskCount   int
	DoneCount   int
	Blocked     bool
	Expanded    bool
}

func (p *PhaseHeaderItem) ItemType() ListItemType { return ItemPhaseHeader }
func (p *PhaseHeaderItem) GetTitle() string        { return p.PhaseName }
func (p *PhaseHeaderItem) IndentLevel() int        { return 0 }
func (p *PhaseHeaderItem) IsExpandable() bool      { return true }
func (p *PhaseHeaderItem) IsSelectable() bool      { return true }

// TaskRowItem represents a task within a phase.
type TaskRowItem struct {
	TaskNumber int
	TaskName   string
	Phase      config.TaskPhase
	Blocked    bool
	Expanded   bool
	PlanPath   string // identifies which plan this task belongs to

	VerifyAttempts   int
	LastVerifyResult string
	InstanceTitle    string // associated instance, empty if none
}

func (t *TaskRowItem) ItemType() ListItemType { return ItemTaskRow }
func (t *TaskRowItem) GetTitle() string        { return t.TaskName }
func (t *TaskRowItem) IndentLevel() int        { return 1 }
func (t *TaskRowItem) IsExpandable() bool      { return t.InstanceTitle != "" }
func (t *TaskRowItem) IsSelectable() bool      { return true }

// InstanceRowItem wraps an existing Instance for the list.
type InstanceRowItem struct {
	Instance *session.Instance
	TaskAssociated bool // true if this instance is a child of a TaskRowItem
}

func (i *InstanceRowItem) ItemType() ListItemType { return ItemInstanceRow }
func (i *InstanceRowItem) GetTitle() string        { return i.Instance.Title }
func (i *InstanceRowItem) IndentLevel() int {
	if i.TaskAssociated {
		return 2
	}
	return 0
}
func (i *InstanceRowItem) IsExpandable() bool { return false }
func (i *InstanceRowItem) IsSelectable() bool { return true }
```

**Step 4: Run tests**

Run: `go test ./ui/ -run TestListItem -v`
Expected: PASS

**Step 5: Commit**

```bash
git add ui/list_item.go ui/list_item_test.go
git commit -m "feat(ui): add ListItem interface with phase, task, and instance types"
```

---

### Task 6: Refactor List to Support Mixed Item Types

**Files:**
- Modify: `ui/list.go`
- Modify: `ui/list_renderer.go`
- Create: `ui/task_renderer.go`

This is the largest single task. The List currently holds `[]*session.Instance`. We need it to hold `[]ListItem` while maintaining full backward compatibility for the existing instance-only rendering path.

**Step 1: Add `planItems` field to List**

In `ui/list.go`, add a new field to the `List` struct:
```go
planItems []ListItem // task-grouped items rendered above instances
```

The existing `items []*session.Instance` stays as-is. The `String()` method renders `planItems` first, then a separator, then `items`. When `planItems` is empty, the rendering is identical to the current behavior.

**Step 2: Create `ui/task_renderer.go` for rendering phase headers and task rows**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/klique/config"
	"github.com/mattn/go-runewidth"
)

var (
	phaseHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#aaaaaa"}).
		Bold(true).
		Padding(0, 1)

	phaseBlockedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#555555"}).
		Padding(0, 1)

	taskActiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7EC8D8")).
		Padding(0, 1)

	taskDoneStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"}).
		Padding(0, 1)

	taskBlockedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#555555"}).
		Padding(0, 1)

	taskPlannedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"}).
		Padding(0, 1)

	taskSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#dde4f0")).
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"}).
		Padding(0, 1)
)

// phaseIcon returns the expand/collapse icon for a phase header.
func phaseIcon(expanded bool) string {
	if expanded {
		return "\u25bc" // ▼
	}
	return "\u25b8" // ▸
}

// taskStatusIcon returns a status icon for a task's phase.
func taskStatusIcon(phase config.TaskPhase, blocked bool) string {
	if blocked {
		return "\u25cb" // ○
	}
	switch phase {
	case config.PhaseDone:
		return "\u2713" // ✓
	case config.PhasePlanned:
		return " "
	default:
		return "\u25cf" // ●
	}
}

// taskPhaseLabel returns a short label for the task's current phase.
func taskPhaseLabel(phase config.TaskPhase, verifyAttempts int) string {
	switch phase {
	case config.PhasePlanned:
		return "planned"
	case config.PhaseImplementing:
		if verifyAttempts > 0 {
			return fmt.Sprintf("implement (v:%d)", verifyAttempts)
		}
		return "implement"
	case config.PhaseVerifying:
		return "verifying"
	case config.PhaseSpecReview:
		return "spec-review"
	case config.PhaseQualReview:
		return "qual-review"
	case config.PhaseDone:
		return "done"
	default:
		return string(phase)
	}
}

// RenderPhaseHeader renders a phase header line.
func RenderPhaseHeader(item *PhaseHeaderItem, selected bool, width int) string {
	icon := phaseIcon(item.Expanded)
	progress := fmt.Sprintf("%d/%d", item.DoneCount, item.TaskCount)

	style := phaseHeaderStyle
	if item.Blocked {
		style = phaseBlockedStyle
	}
	if selected {
		style = taskSelectedStyle.Bold(true)
	}

	label := fmt.Sprintf("%s Phase %d: %s", icon, item.PhaseNumber, item.PhaseName)
	labelWidth := runewidth.StringWidth(label)
	progressWidth := runewidth.StringWidth(progress)
	pad := width - labelWidth - progressWidth - 4
	if pad < 1 {
		pad = 1
	}
	line := label + strings.Repeat(" ", pad) + progress
	return style.Width(width).Render(line)
}

// RenderTaskRow renders a task row.
func RenderTaskRow(item *TaskRowItem, selected bool, width int) string {
	icon := taskStatusIcon(item.Phase, item.Blocked)
	label := fmt.Sprintf("  %s T%d: %s", icon, item.TaskNumber, item.TaskName)
	phaseLabel := taskPhaseLabel(item.Phase, item.VerifyAttempts)

	style := taskPlannedStyle
	switch {
	case selected:
		style = taskSelectedStyle
	case item.Blocked:
		style = taskBlockedStyle
	case item.Phase == config.PhaseDone:
		style = taskDoneStyle
	case item.Phase != config.PhasePlanned:
		style = taskActiveStyle
	}

	labelWidth := runewidth.StringWidth(label)
	phaseWidth := runewidth.StringWidth(phaseLabel)
	pad := width - labelWidth - phaseWidth - 4
	if pad < 1 {
		pad = 1
	}
	line := label + strings.Repeat(" ", pad) + phaseLabel
	return style.Width(width).Render(line)
}
```

**Step 3: Update `List.String()` in `ui/list_renderer.go` to render planItems**

Before the instance loop in `String()`, add rendering of `planItems`:
```go
// Render plan items (phases + tasks) if present
if len(l.planItems) > 0 {
    for i, item := range l.planItems {
        planSelected := l.planSelectedIdx >= 0 && i == l.planSelectedIdx
        switch v := item.(type) {
        case *PhaseHeaderItem:
            b.WriteString(RenderPhaseHeader(v, planSelected, AdjustPreviewWidth(l.width)))
        case *TaskRowItem:
            b.WriteString(RenderTaskRow(v, planSelected, AdjustPreviewWidth(l.width)))
        case *InstanceRowItem:
            b.WriteString(l.renderer.Render(v.Instance, planSelected, l.focused, len(l.repos) > 1, i))
        }
        b.WriteString("\n")
    }
    // Separator between plan items and ad-hoc instances
    sep := strings.Repeat("─", AdjustPreviewWidth(l.width))
    b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render(sep))
    b.WriteString("\n\n")
}
```

**Step 4: Add plan item management methods to List**

```go
func (l *List) SetPlanItems(items []ListItem) {
    l.planItems = items
}

func (l *List) GetPlanItems() []ListItem {
    return l.planItems
}
```

Add `planSelectedIdx int` field (default -1 means no plan item selected) and `inPlanSection bool` to track whether focus is in the plan section or the instance section.

**Step 5: Run all existing tests to verify no regressions**

Run: `go test ./ui/ -v`
Expected: All PASS — existing tests should still pass because `planItems` is nil by default, so rendering is unchanged.

**Step 6: Commit**

```bash
git add ui/list.go ui/list_renderer.go ui/task_renderer.go
git commit -m "feat(ui): add task-grouped rendering to instance list"
```

---

### Task 7: Expand/Collapse and Task Navigation

**Files:**
- Modify: `ui/list.go`
- Create: `ui/list_plan_nav.go`
- Create: `ui/list_plan_nav_test.go`

**Step 1: Write failing test for plan item navigation**

Test `BuildPlanItems(plan, planState)`:
- Given a plan with 2 phases (phase 1 expanded, phase 2 collapsed), assert the returned `[]ListItem` contains: phase header, tasks for phase 1, task instances, phase header for phase 2 (no children since collapsed).
- Test `ToggleExpand(idx)` on a phase header: toggles `Expanded`, rebuilds visible items.
- Test `NavigateUp()`/`NavigateDown()` skips non-selectable items.

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestPlanNav -v`
Expected: FAIL

**Step 3: Implement in `ui/list_plan_nav.go`**

```go
package ui

import (
	"fmt"
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/plan"
)

// BuildPlanItems constructs the visible list items from a parsed plan and its state.
// Expanded phases show their tasks; collapsed phases show only the header.
// Tasks with associated instances show the instance as a child.
func BuildPlanItems(
	p *plan.Plan,
	planState *config.PlanState,
	instances map[string]*InstanceRowItem, // keyed by instance title
	expandedPhases map[int]bool,
	expandedTasks map[int]bool,
) []ListItem {
	if p == nil {
		return nil
	}

	var items []ListItem
	allPreviousPhaseDone := true

	for _, phase := range p.Phases {
		// Compute phase blocked state
		phaseBlocked := !allPreviousPhaseDone

		// Count done tasks in this phase
		doneCount := 0
		for _, task := range phase.Tasks {
			ts := planState.GetTaskState(fmt.Sprintf("%d", task.Number))
			if ts.Phase == config.PhaseDone {
				doneCount++
			}
		}

		expanded := expandedPhases[phase.Number]
		header := &PhaseHeaderItem{
			PhaseNumber: phase.Number,
			PhaseName:   phase.Name,
			TaskCount:   len(phase.Tasks),
			DoneCount:   doneCount,
			Blocked:     phaseBlocked,
			Expanded:    expanded,
		}
		items = append(items, header)

		if expanded {
			prevTaskDone := true
			for _, task := range phase.Tasks {
				taskKey := fmt.Sprintf("%d", task.Number)
				ts := planState.GetTaskState(taskKey)

				// Task is blocked if phase is blocked or previous task in phase isn't done
				taskBlocked := phaseBlocked || !prevTaskDone

				taskItem := &TaskRowItem{
					TaskNumber:       task.Number,
					TaskName:         task.Name,
					Phase:            ts.Phase,
					Blocked:          taskBlocked,
					Expanded:         expandedTasks[task.Number],
					PlanPath:         p.FilePath,
					VerifyAttempts:   ts.VerifyAttempts,
					LastVerifyResult: ts.LastVerifyResult,
					InstanceTitle:    ts.InstanceTitle,
				}
				items = append(items, taskItem)

				// Show associated instance if task is expanded and has one
				if taskItem.Expanded && ts.InstanceTitle != "" {
					if inst, ok := instances[ts.InstanceTitle]; ok {
						items = append(items, inst)
					}
				}

				prevTaskDone = ts.Phase == config.PhaseDone
			}
		}

		// Update all-previous-phases-done for next phase
		if doneCount < len(phase.Tasks) {
			allPreviousPhaseDone = false
		}
	}

	return items
}
```

**Step 4: Add navigation methods**

Add to `List`:
- `TogglePlanExpand()` — if selected item is a `PhaseHeaderItem`, toggle `Expanded` and rebuild. If `TaskRowItem` with instance, toggle task expansion.
- `PlanNavigateUp()`/`PlanNavigateDown()` — move `planSelectedIdx`, skip non-selectable items. When reaching the boundary, transfer focus to the instance section (or back).

**Step 5: Run tests**

Run: `go test ./ui/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add ui/list.go ui/list_plan_nav.go ui/list_plan_nav_test.go
git commit -m "feat(ui): add plan item navigation with expand/collapse"
```

---

### Task 8: Wire Plan Loading into App

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `keys/keys.go`

**Step 1: Add plan-related fields to `home` struct in `app/app.go`**

```go
// Plan orchestration
activePlan      *plan.Plan
planState       *config.PlanState
planStateMap    config.PlanStateMap
expandedPhases  map[int]bool
expandedTasks   map[int]bool
```

**Step 2: Load plan on startup in `newHome()`**

After loading instances and before returning, add plan discovery:
```go
// Discover and load active plan
h.expandedPhases = make(map[int]bool)
h.expandedTasks = make(map[int]bool)
h.planStateMap = h.loadPlanStateMap()
planDir := filepath.Join(h.activeRepoPath, "docs", "plans")
if plans, err := plan.DiscoverPlans(planDir); err == nil {
    if activePath := plan.SelectActivePlan(plans); activePath != "" {
        if content, err := os.ReadFile(activePath); err == nil {
            if p, err := plan.Parse(string(content)); err == nil {
                p.FilePath = activePath
                h.activePlan = p
                h.planState = h.planStateMap.GetOrCreate(activePath)
                h.planState.Active = true
                // Expand first non-done phase by default
                for _, phase := range p.Phases {
                    h.expandedPhases[phase.Number] = true
                    break
                }
                h.rebuildPlanItems()
            }
        }
    }
}
```

**Step 3: Add `rebuildPlanItems()` method**

```go
func (m *home) rebuildPlanItems() {
    instanceMap := make(map[string]*ui.InstanceRowItem)
    for _, inst := range m.allInstances {
        instanceMap[inst.Title] = &ui.InstanceRowItem{
            Instance:       inst,
            TaskAssociated: true,
        }
    }
    items := ui.BuildPlanItems(m.activePlan, m.planState, instanceMap, m.expandedPhases, m.expandedTasks)
    m.list.SetPlanItems(items)
}
```

**Step 4: Add plan reload on metadata tick**

In the `tickUpdateMetadataMessage` handler in `app.go`, add:
```go
// Reload plan if file changed
if m.activePlan != nil && plan.NeedsReload(m.activePlan.FilePath, m.activePlan.ModTime) {
    // re-parse and rebuild
}
```

**Step 5: Add keybinds in `keys/keys.go`**

Add new key constants:
```go
KeyTaskLaunch    KeyName = iota // after last existing key
KeyTaskTransition
KeyTaskSendBack
KeyPlanSwitch
```

Add to `GlobalKeyStringsMap`:
```go
"enter": KeyTaskLaunch,  // context-dependent: launch task when task selected
"t":     KeyTaskTransition,
"b":     KeyTaskSendBack,
"s":     KeyPlanSwitch, // note: check for collision with existing 's' (PR shortcut)
```

Note: `s` is already used for the PR/push shortcut. Use `S` (shift-s) for plan switch, or use a different key. Check `keys.go` for the exact current mapping and choose a non-conflicting key. If `s` is taken, use `ctrl+s` or `f4`.

**Step 6: Add keybind handlers in `app/app_input.go`**

In the default state switch in `handleKeyPress`, add cases for the new keys. These will check if the list's focus is in the plan section and dispatch accordingly. For now, stub the handlers — actual orchestration logic comes in Phase 4.

**Step 7: Commit**

```bash
git add app/app.go app/app_input.go keys/keys.go
git commit -m "feat(app): wire plan loading and task keybinds into TUI"
```

---

## Phase 4: Orchestration

### Task 9: Launch Instance from Task

**Files:**
- Modify: `app/app.go`
- Create: `app/app_plan.go`

**Step 1: Create `app/app_plan.go` with task launch logic**

```go
package app

import (
	"fmt"
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

// launchTask creates a new instance for a task and transitions it to implementing.
func (m *home) launchTask(taskItem *ui.TaskRowItem) tea.Cmd {
	if taskItem.Blocked || taskItem.Phase != config.PhasePlanned {
		return nil
	}

	// Resolve the agent profile for the implementing phase
	profile := m.appConfig.ResolveProfile(string(config.PhaseImplementing), m.program)

	title := fmt.Sprintf("T%d: %s", taskItem.TaskNumber, taskItem.TaskName)
	branch := fmt.Sprintf("task-%d-%s", taskItem.TaskNumber,
		sanitizeForBranch(taskItem.TaskName))

	return func() tea.Msg {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   title,
			Path:    m.activeRepoPath,
			Program: profile.BuildCommand(),
		})
		if err != nil {
			return errMsg{err}
		}

		// Update task state
		taskKey := fmt.Sprintf("%d", taskItem.TaskNumber)
		ts := m.planState.GetTaskState(taskKey)
		now := time.Now()
		ts.Phase = config.PhaseImplementing
		ts.InstanceTitle = title
		ts.StartedAt = &now
		m.savePlanState()

		return instanceStartedMsg{instance: instance}
	}
}

func sanitizeForBranch(name string) string {
	// Simple sanitization: lowercase, replace spaces with dashes, remove special chars
	result := ""
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			result += string(r)
		case r >= 'A' && r <= 'Z':
			result += string(r + 32) // lowercase
		case r == ' ':
			result += "-"
		}
	}
	return result
}
```

**Step 2: Wire launch into the keybind handler**

In the `KeyTaskLaunch` case in `app_input.go`:
```go
case keys.KeyTaskLaunch:
    if m.list.InPlanSection() {
        if item := m.list.GetSelectedPlanItem(); item != nil {
            if taskItem, ok := item.(*ui.TaskRowItem); ok {
                return m, m.launchTask(taskItem)
            }
        }
    }
    // Fall through to existing Enter behavior for instances
```

**Step 3: Add `savePlanState()` helper**

```go
func (m *home) savePlanState() {
    data, err := json.Marshal(m.planStateMap)
    if err != nil {
        return
    }
    m.storage.SavePlans(data)
}

func (m *home) loadPlanStateMap() config.PlanStateMap {
    raw := m.storage.GetPlans()
    if raw == nil {
        return make(config.PlanStateMap)
    }
    var psm config.PlanStateMap
    if err := json.Unmarshal(raw, &psm); err != nil {
        return make(config.PlanStateMap)
    }
    return psm
}
```

**Step 4: Run build to verify compilation**

Run: `go build -o kq .`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add app/app_plan.go app/app.go app/app_input.go
git commit -m "feat(app): launch instances from plan tasks with profile resolution"
```

---

### Task 10: Agent Switching Mechanism

**Files:**
- Create: `session/switch.go`
- Create: `session/switch_test.go`
- Modify: `session/tmux/tmux.go`

**Step 1: Write failing test for agent switching**

In `session/switch_test.go`, test `SwitchAgent`:
- Mock TmuxSession: verify SIGTERM is sent, new program is spawned via send-keys
- Verify worktree is NOT touched
- Verify the instance's `Program` field is updated

**Step 2: Run test to verify it fails**

Run: `go test ./session/ -run TestSwitchAgent -v`
Expected: FAIL

**Step 3: Implement `session/switch.go`**

```go
package session

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// SwitchAgent terminates the current agent process in the tmux session
// and spawns a new one with the given program command.
// The worktree and branch are preserved.
func (i *Instance) SwitchAgent(newProgram string) error {
	if i.tmuxSession == nil {
		return fmt.Errorf("no tmux session")
	}

	// Get the child process PID of the tmux session
	pid, err := i.tmuxSession.GetChildPID()
	if err == nil && pid > 0 {
		// Send SIGTERM
		proc, err := os.FindProcess(pid)
		if err == nil {
			_ = proc.Signal(syscall.SIGTERM)

			// Wait up to 3 seconds for exit
			done := make(chan struct{})
			go func() {
				proc.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Clean exit
			case <-time.After(3 * time.Second):
				// Force kill
				_ = proc.Signal(syscall.SIGKILL)
				<-done
			}
		}
	}

	// Spawn new agent in the same tmux session
	i.Program = newProgram
	err = i.tmuxSession.SendKeys(newProgram + "\n")
	if err != nil {
		return fmt.Errorf("failed to spawn new agent: %w", err)
	}

	i.Status = Running
	return nil
}

// InjectPrompt sends text to the agent after it starts.
func (i *Instance) InjectPrompt(text string) error {
	if i.tmuxSession == nil {
		return fmt.Errorf("no tmux session")
	}
	// Wait a moment for the agent to be ready
	time.Sleep(2 * time.Second)
	return i.tmuxSession.SendKeys(text + "\n")
}
```

**Step 4: Add `GetChildPID()` to TmuxSession**

In `session/tmux/tmux.go`, add:
```go
// GetChildPID returns the PID of the main process running inside the tmux pane.
func (t *TmuxSession) GetChildPID() (int, error) {
    output, err := t.cmdExec.Run("tmux", "display-message", "-t", t.sanitizedName, "-p", "#{pane_pid}")
    if err != nil {
        return 0, err
    }
    pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
    return pid, err
}
```

**Step 5: Run tests**

Run: `go test ./session/ -run TestSwitchAgent -v`
Expected: PASS

**Step 6: Commit**

```bash
git add session/switch.go session/switch_test.go session/tmux/tmux.go
git commit -m "feat(session): add agent switching within tmux session"
```

---

### Task 11: Phase Transition and Verify Loop

**Files:**
- Modify: `app/app_plan.go`
- Modify: `app/app.go`

**Step 1: Add transition handler to `app_plan.go`**

```go
// transitionTask moves a task to the next phase, switching the agent.
func (m *home) transitionTask(taskItem *ui.TaskRowItem) tea.Cmd {
    taskKey := fmt.Sprintf("%d", taskItem.TaskNumber)
    ts := m.planState.GetTaskState(taskKey)

    // Determine next phase
    var nextPhase config.TaskPhase
    switch ts.Phase {
    case config.PhaseImplementing:
        nextPhase = config.PhaseVerifying
    case config.PhaseVerifying:
        // This is called when verify passes
        nextPhase = config.PhaseSpecReview
    case config.PhaseSpecReview:
        nextPhase = config.PhaseQualReview
    case config.PhaseQualReview:
        nextPhase = config.PhaseDone
    default:
        return nil
    }

    if !config.CanTransition(ts.Phase, nextPhase) {
        return nil
    }

    return func() tea.Msg {
        // Find associated instance
        inst := m.findInstanceByTitle(ts.InstanceTitle)
        if inst == nil && nextPhase != config.PhaseDone {
            return errMsg{fmt.Errorf("no instance for task %d", taskItem.TaskNumber)}
        }

        if nextPhase == config.PhaseDone {
            now := time.Now()
            ts.Phase = config.PhaseDone
            ts.CompletedAt = &now
            m.savePlanState()
            m.rebuildPlanItems()
            return instanceChangedMsg{}
        }

        // Resolve profile for next phase
        profile := m.appConfig.ResolveProfile(string(nextPhase), m.program)

        // Switch agent
        if err := inst.SwitchAgent(profile.BuildCommand()); err != nil {
            return errMsg{err}
        }

        // Update state
        ts.Phase = nextPhase
        m.savePlanState()

        // Inject context for review phases
        if nextPhase == config.PhaseSpecReview || nextPhase == config.PhaseQualReview {
            taskText := m.getTaskRawText(taskItem.TaskNumber)
            var prompt string
            if nextPhase == config.PhaseSpecReview {
                prompt = fmt.Sprintf("Review this implementation against the spec. Task:\n\n%s", taskText)
            } else {
                prompt = fmt.Sprintf("Review this code for quality, patterns, and maintainability. Task:\n\n%s", taskText)
            }
            inst.InjectPrompt(prompt)
        }

        m.rebuildPlanItems()
        return instanceChangedMsg{}
    }
}

// sendBackTask reverts a task from review back to implementing with feedback.
func (m *home) sendBackTask(taskItem *ui.TaskRowItem) tea.Cmd {
    taskKey := fmt.Sprintf("%d", taskItem.TaskNumber)
    ts := m.planState.GetTaskState(taskKey)

    if ts.Phase != config.PhaseSpecReview && ts.Phase != config.PhaseQualReview {
        return nil
    }

    return func() tea.Msg {
        inst := m.findInstanceByTitle(ts.InstanceTitle)
        if inst == nil {
            return errMsg{fmt.Errorf("no instance for task %d", taskItem.TaskNumber)}
        }

        profile := m.appConfig.ResolveProfile(string(config.PhaseImplementing), m.program)
        if err := inst.SwitchAgent(profile.BuildCommand()); err != nil {
            return errMsg{err}
        }

        ts.Phase = config.PhaseImplementing
        m.savePlanState()
        m.rebuildPlanItems()
        return instanceChangedMsg{}
    }
}

func (m *home) findInstanceByTitle(title string) *session.Instance {
    for _, inst := range m.allInstances {
        if inst.Title == title {
            return inst
        }
    }
    return nil
}

func (m *home) getTaskRawText(taskNumber int) string {
    if m.activePlan == nil {
        return ""
    }
    for _, phase := range m.activePlan.Phases {
        for _, task := range phase.Tasks {
            if task.Number == taskNumber {
                return task.RawText
            }
        }
    }
    return ""
}
```

**Step 2: Add verify trigger to metadata tick**

In the `tickUpdateMetadataMessage` handler, after the existing instance status polling, add:
```go
// Auto-verify: when an implementing task's agent goes idle, trigger verify
if m.appConfig.AutoVerify && m.activePlan != nil {
    for _, phase := range m.activePlan.Phases {
        for _, task := range phase.Tasks {
            taskKey := fmt.Sprintf("%d", task.Number)
            ts := m.planState.GetTaskState(taskKey)
            if ts.Phase == config.PhaseImplementing && ts.InstanceTitle != "" {
                inst := m.findInstanceByTitle(ts.InstanceTitle)
                if inst != nil && inst.Status == session.Ready && inst.PromptDetected {
                    // Agent is idle — trigger verify
                    ts.Phase = config.PhaseVerifying
                    ts.VerifyAttempts++
                    m.savePlanState()
                    inst.InjectPrompt("/kas:verify")
                    m.rebuildPlanItems()
                }
            }
        }
    }
}
```

**Step 3: Wire transition and send-back keybinds**

In `app_input.go`:
```go
case keys.KeyTaskTransition:
    if m.list.InPlanSection() {
        if item := m.list.GetSelectedPlanItem(); item != nil {
            if taskItem, ok := item.(*ui.TaskRowItem); ok {
                return m, m.transitionTask(taskItem)
            }
        }
    }

case keys.KeyTaskSendBack:
    if m.list.InPlanSection() {
        if item := m.list.GetSelectedPlanItem(); item != nil {
            if taskItem, ok := item.(*ui.TaskRowItem); ok {
                return m, m.sendBackTask(taskItem)
            }
        }
    }
```

**Step 4: Run build**

Run: `go build -o kq .`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add app/app_plan.go app/app.go app/app_input.go
git commit -m "feat(app): add phase transitions, verify loop, and agent switching"
```

---

### Task 12: Integration Testing and Edge Cases

**Files:**
- Create: `app/app_plan_test.go`
- Modify: `plan/parser_test.go`

**Step 1: Add parser edge case tests**

In `plan/parser_test.go`, add table-driven tests for:
- Empty markdown → empty Plan, no error
- Plan with no phases (only tasks at top level) → tasks in a default phase
- Plan with single task, no phase header → single task, single phase
- Malformed phase header (no number) → skipped
- Task with no steps → empty Steps slice
- Unicode in task names → preserved correctly

**Step 2: Run parser tests**

Run: `go test ./plan/ -v`
Expected: All PASS

**Step 3: Add phase transition state machine tests**

In `config/plan_state_test.go`, add:
- Table-driven tests for all valid transitions (from design doc)
- Tests for invalid transitions returning false
- Test transition from Done → anything returns false

**Step 4: Add dependency resolution tests**

In `ui/list_plan_nav_test.go`, add:
- Plan with 3 phases, all tasks done in phase 1: phase 2 tasks should be unblocked
- Plan with phase 1 task 2 still implementing: phase 1 task 3 should be blocked, phase 2 should be blocked
- Single-phase plan: first task available, rest blocked until previous done

**Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS, zero regressions

**Step 6: Commit**

```bash
git add app/app_plan_test.go plan/parser_test.go config/plan_state_test.go ui/list_plan_nav_test.go
git commit -m "test: add edge case and integration tests for plan orchestration"
```

---
