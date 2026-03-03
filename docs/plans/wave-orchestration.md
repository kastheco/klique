# Wave Orchestration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable parallel task execution within plans by parsing wave annotations from plan markdown and orchestrating per-task Claude Code instances on a shared worktree, with user-confirmed wave transitions.

**Architecture:** A `planparser` package extracts `## Wave` / `### Task` structure from plan markdown. A `WaveOrchestrator` struct in the app layer manages the state machine (idle → running → complete → confirm → next wave). Each task spawns as a separate `session.Instance` with new `TaskNumber`/`WaveNumber` fields. The existing metadata tick monitors `PromptDetected` for completion. Wave validation gates the "Implement" action — plans without wave headers are sent back to planning.

**Tech Stack:** Go, bubbletea, existing session/planstate packages

**Waves:** 3 (T1,T2 parallel → T3,T4,T5 parallel → T6,T7 sequential)

**Design doc:** `docs/plans/2026-02-22-wave-orchestration-design.md`

---

## Wave 1

### Task 1: Plan Parser Package

**Files:**
- Create: `config/planparser/planparser.go`
- Create: `config/planparser/planparser_test.go`

**Step 1: Write failing tests for wave/task parsing**

```go
package planparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlan_MultiWave(t *testing.T) {
	input := `# Feature Plan

> **For Claude:** ...

**Goal:** Build a thing
**Architecture:** Some approach
**Tech Stack:** Go

**Waves:** 2 (T1,T2 parallel → T3 sequential)

---

## Wave 1
### Task 1: First Thing

**Files:**
- Create: ` + "`path/to/file.go`" + `

**Step 1: Do something**

Some instructions here.

### Task 2: Second Thing

**Files:**
- Modify: ` + "`other/file.go`" + `

**Step 1: Do other thing**

More instructions.

## Wave 2
### Task 3: Final Thing

**Files:**
- Modify: ` + "`path/to/file.go`" + `

**Step 1: Wrap up**

Final instructions.
`
	plan, err := Parse(input)
	require.NoError(t, err)

	assert.Equal(t, "Build a thing", plan.Goal)
	assert.Equal(t, "Some approach", plan.Architecture)
	assert.Equal(t, "Go", plan.TechStack)

	require.Len(t, plan.Waves, 2)

	// Wave 1: two tasks
	require.Len(t, plan.Waves[0].Tasks, 2)
	assert.Equal(t, 1, plan.Waves[0].Number)
	assert.Equal(t, 1, plan.Waves[0].Tasks[0].Number)
	assert.Equal(t, "First Thing", plan.Waves[0].Tasks[0].Title)
	assert.Contains(t, plan.Waves[0].Tasks[0].Body, "Do something")
	assert.Equal(t, 2, plan.Waves[0].Tasks[1].Number)
	assert.Equal(t, "Second Thing", plan.Waves[0].Tasks[1].Title)

	// Wave 2: one task
	require.Len(t, plan.Waves[1].Tasks, 1)
	assert.Equal(t, 2, plan.Waves[1].Number)
	assert.Equal(t, 3, plan.Waves[1].Tasks[0].Number)
}

func TestParsePlan_NoWaveHeaders(t *testing.T) {
	input := `# Old Plan

**Goal:** Legacy thing

---

### Task 1: Something
Step 1: do it

### Task 2: Another
Step 1: do it too
`
	_, err := Parse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no wave headers found")
}

func TestParsePlan_EmptyPlan(t *testing.T) {
	_, err := Parse("")
	require.Error(t, err)
}

func TestParsePlan_HeaderExtraction(t *testing.T) {
	input := `# Plan

**Goal:** My goal here
**Architecture:** My arch here
**Tech Stack:** Go, bubbletea

## Wave 1
### Task 1: Only Task

Do the thing.
`
	plan, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "My goal here", plan.Goal)
	assert.Equal(t, "My arch here", plan.Architecture)
	assert.Equal(t, "Go, bubbletea", plan.TechStack)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./config/planparser/... -v`
Expected: FAIL — package doesn't exist yet.

**Step 3: Implement the parser**

```go
package planparser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Task represents a single task extracted from a plan.
type Task struct {
	Number int    // Task number (1-indexed, from ### Task N: Title)
	Title  string // Task title (text after "Task N: ")
	Body   string // Full task body (everything between this ### Task and the next heading)
}

// Wave represents a group of tasks that can run in parallel.
type Wave struct {
	Number int    // Wave number (1-indexed)
	Tasks  []Task // Tasks in this wave
}

// Plan represents a parsed plan with header metadata and wave-grouped tasks.
type Plan struct {
	Goal         string
	Architecture string
	TechStack    string
	Waves        []Wave
}

// HeaderContext returns the plan header as a string suitable for task prompts.
func (p *Plan) HeaderContext() string {
	var sb strings.Builder
	if p.Goal != "" {
		sb.WriteString("**Goal:** " + p.Goal + "\n")
	}
	if p.Architecture != "" {
		sb.WriteString("**Architecture:** " + p.Architecture + "\n")
	}
	if p.TechStack != "" {
		sb.WriteString("**Tech Stack:** " + p.TechStack + "\n")
	}
	return sb.String()
}

var (
	waveHeaderRe = regexp.MustCompile(`(?m)^## Wave (\d+)\s*$`)
	taskHeaderRe = regexp.MustCompile(`(?m)^### Task (\d+):\s*(.+)$`)
	goalRe       = regexp.MustCompile(`(?m)^\*\*Goal:\*\*\s*(.+)$`)
	archRe       = regexp.MustCompile(`(?m)^\*\*Architecture:\*\*\s*(.+)$`)
	techRe       = regexp.MustCompile(`(?m)^\*\*Tech Stack:\*\*\s*(.+)$`)
)

// Parse extracts waves and tasks from plan markdown content.
// Returns an error if no ## Wave headers are found.
func Parse(content string) (*Plan, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("empty plan content")
	}

	plan := &Plan{}

	// Extract header fields
	if m := goalRe.FindStringSubmatch(content); len(m) > 1 {
		plan.Goal = strings.TrimSpace(m[1])
	}
	if m := archRe.FindStringSubmatch(content); len(m) > 1 {
		plan.Architecture = strings.TrimSpace(m[1])
	}
	if m := techRe.FindStringSubmatch(content); len(m) > 1 {
		plan.TechStack = strings.TrimSpace(m[1])
	}

	// Find all wave header positions
	waveMatches := waveHeaderRe.FindAllStringSubmatchIndex(content, -1)
	if len(waveMatches) == 0 {
		return nil, fmt.Errorf("no wave headers found in plan; add ## Wave N sections before implementing")
	}

	// Split content into wave sections
	for i, wm := range waveMatches {
		waveNumStr := content[wm[2]:wm[3]]
		waveNum, _ := strconv.Atoi(waveNumStr)

		// Determine the section boundaries for this wave
		sectionStart := wm[1] // end of "## Wave N" line
		var sectionEnd int
		if i+1 < len(waveMatches) {
			sectionEnd = waveMatches[i+1][0] // start of next wave header
		} else {
			sectionEnd = len(content)
		}
		section := content[sectionStart:sectionEnd]

		// Extract tasks from this wave section
		tasks, err := parseTasks(section)
		if err != nil {
			return nil, fmt.Errorf("wave %d: %w", waveNum, err)
		}

		plan.Waves = append(plan.Waves, Wave{
			Number: waveNum,
			Tasks:  tasks,
		})
	}

	return plan, nil
}

// parseTasks extracts ### Task entries from a wave section.
func parseTasks(section string) ([]Task, error) {
	taskMatches := taskHeaderRe.FindAllStringSubmatchIndex(section, -1)
	if len(taskMatches) == 0 {
		return nil, nil
	}

	var tasks []Task
	for i, tm := range taskMatches {
		numStr := section[tm[2]:tm[3]]
		num, _ := strconv.Atoi(numStr)
		title := strings.TrimSpace(section[tm[4]:tm[5]])

		// Task body: from end of header line to start of next task (or end of section)
		bodyStart := tm[1]
		var bodyEnd int
		if i+1 < len(taskMatches) {
			bodyEnd = taskMatches[i+1][0]
		} else {
			bodyEnd = len(section)
		}
		body := strings.TrimSpace(section[bodyStart:bodyEnd])

		tasks = append(tasks, Task{
			Number: num,
			Title:  title,
			Body:   body,
		})
	}

	return tasks, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/planparser/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/planparser/planparser.go config/planparser/planparser_test.go
git commit -m "feat: add planparser package for wave/task extraction from plan markdown"
```

---

### Task 2: Add TaskNumber and WaveNumber to Instance

**Files:**
- Modify: `session/instance.go:56-67`
- Modify: `session/storage.go:14-34`
- Create: `session/instance_wave_test.go`

**Step 1: Write failing tests**

```go
package session

import (
	"testing"
)

func TestNewInstance_SetsWaveAndTaskNumber(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:      "test-T1",
		Path:       "/tmp",
		Program:    "echo",
		PlanFile:   "plan.md",
		TaskNumber: 1,
		WaveNumber: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.TaskNumber != 1 {
		t.Fatalf("TaskNumber = %d, want 1", inst.TaskNumber)
	}
	if inst.WaveNumber != 1 {
		t.Fatalf("WaveNumber = %d, want 1", inst.WaveNumber)
	}
}

func TestInstanceData_RoundTripWaveFields(t *testing.T) {
	inst, _ := NewInstance(InstanceOptions{
		Title:      "test-T2",
		Path:       "/tmp",
		Program:    "echo",
		PlanFile:   "plan.md",
		TaskNumber: 3,
		WaveNumber: 2,
	})

	data := inst.ToInstanceData()
	if data.TaskNumber != 3 {
		t.Fatalf("InstanceData TaskNumber = %d, want 3", data.TaskNumber)
	}
	if data.WaveNumber != 2 {
		t.Fatalf("InstanceData WaveNumber = %d, want 2", data.WaveNumber)
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.TaskNumber != 3 {
		t.Fatalf("restored TaskNumber = %d, want 3", restored.TaskNumber)
	}
	if restored.WaveNumber != 2 {
		t.Fatalf("restored WaveNumber = %d, want 2", restored.WaveNumber)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/... -run TestNewInstance_SetsWaveAndTaskNumber -v`
Expected: FAIL — fields don't exist.

**Step 3: Add fields to Instance and InstanceOptions**

In `session/instance.go`, add after the `AgentType` field (line ~61):
```go
// TaskNumber is the task number within a plan (1-indexed). 0 = not a wave task.
TaskNumber int
// WaveNumber is the wave number this task belongs to (1-indexed). 0 = not a wave task.
WaveNumber int
```

In `InstanceOptions` (line ~222), add:
```go
// TaskNumber is the task number within a plan (1-indexed, 0 = not a wave task).
TaskNumber int
// WaveNumber is the wave this task belongs to (1-indexed, 0 = not a wave task).
WaveNumber int
```

In `NewInstance` (line ~234), add to the struct literal:
```go
TaskNumber: opts.TaskNumber,
WaveNumber: opts.WaveNumber,
```

In `ToInstanceData` and `FromInstanceData`, add round-trip for both fields.

In `session/storage.go`, add to `InstanceData`:
```go
TaskNumber int `json:"task_number,omitempty"`
WaveNumber int `json:"wave_number,omitempty"`
```

**Step 4: Run tests to verify they pass**

Run: `go test ./session/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/instance.go session/storage.go session/instance_wave_test.go
git commit -m "feat: add TaskNumber and WaveNumber fields to Instance"
```

---

## Wave 2

### Task 3: WaveOrchestrator Core

**Files:**
- Create: `app/wave_orchestrator.go`
- Create: `app/wave_orchestrator_test.go`

**Step 1: Write failing tests**

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWaveOrchestrator(t *testing.T) {
	plan := &planparser.Plan{
		Goal: "test",
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
			{Number: 2, Tasks: []planparser.Task{
				{Number: 3, Title: "Third", Body: "do third"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	assert.Equal(t, WaveStateIdle, orch.State())
	assert.Equal(t, 2, orch.TotalWaves())
	assert.Equal(t, 3, orch.TotalTasks())
}

func TestWaveOrchestrator_StartWave(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	tasks := orch.StartNextWave()

	assert.Equal(t, WaveStateRunning, orch.State())
	assert.Equal(t, 1, orch.CurrentWaveNumber())
	require.Len(t, tasks, 1)
	assert.Equal(t, "First", tasks[0].Title)
}

func TestWaveOrchestrator_TaskCompleted(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	orch.StartNextWave()

	assert.False(t, orch.IsCurrentWaveComplete())

	orch.MarkTaskComplete(1)
	assert.False(t, orch.IsCurrentWaveComplete())

	orch.MarkTaskComplete(2)
	assert.True(t, orch.IsCurrentWaveComplete())
	assert.Equal(t, WaveStateWaveComplete, orch.State())
}

func TestWaveOrchestrator_TaskFailed(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	orch.StartNextWave()

	orch.MarkTaskFailed(1)
	orch.MarkTaskComplete(2)

	assert.Equal(t, WaveStateWaveComplete, orch.State())
	assert.Equal(t, 1, orch.FailedTaskCount())
	assert.Equal(t, 1, orch.CompletedTaskCount())
}

func TestWaveOrchestrator_MultiWaveProgression(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
			{Number: 2, Tasks: []planparser.Task{
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)

	// Wave 1
	orch.StartNextWave()
	orch.MarkTaskComplete(1)
	assert.Equal(t, WaveStateWaveComplete, orch.State())

	// Advance to wave 2
	tasks := orch.StartNextWave()
	assert.Equal(t, WaveStateRunning, orch.State())
	assert.Equal(t, 2, orch.CurrentWaveNumber())
	require.Len(t, tasks, 1)

	// Complete wave 2
	orch.MarkTaskComplete(2)
	assert.Equal(t, WaveStateAllComplete, orch.State())
}

func TestWaveOrchestrator_AllComplete(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "Only", Body: "do it"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1)

	// No more waves — should be AllComplete
	assert.Equal(t, WaveStateAllComplete, orch.State())
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/... -run TestWaveOrchestrator -v`
Expected: FAIL — types don't exist.

**Step 3: Implement WaveOrchestrator**

```go
package app

import (
	"github.com/kastheco/kasmos/config/planparser"
)

// WaveState represents the current state of wave orchestration for a plan.
type WaveState int

const (
	WaveStateIdle         WaveState = iota // Not started
	WaveStateRunning                       // Current wave's tasks are running
	WaveStateWaveComplete                  // Current wave finished, awaiting user confirmation
	WaveStateAllComplete                   // All waves finished
)

// taskStatus tracks the completion state of a single task.
type taskStatus int

const (
	taskPending   taskStatus = iota
	taskRunning
	taskComplete
	taskFailed
)

// WaveOrchestrator manages wave-based parallel task execution for a single plan.
type WaveOrchestrator struct {
	planFile    string
	plan        *planparser.Plan
	state       WaveState
	currentWave int            // 0-indexed into plan.Waves
	taskStates  map[int]taskStatus // task number → status
}

// NewWaveOrchestrator creates an orchestrator for the given plan.
func NewWaveOrchestrator(planFile string, plan *planparser.Plan) *WaveOrchestrator {
	return &WaveOrchestrator{
		planFile:   planFile,
		plan:       plan,
		state:      WaveStateIdle,
		taskStates: make(map[int]taskStatus),
	}
}

// State returns the current orchestration state.
func (o *WaveOrchestrator) State() WaveState {
	return o.state
}

// PlanFile returns the plan filename this orchestrator manages.
func (o *WaveOrchestrator) PlanFile() string {
	return o.planFile
}

// TotalWaves returns the number of waves in the plan.
func (o *WaveOrchestrator) TotalWaves() int {
	return len(o.plan.Waves)
}

// TotalTasks returns the total number of tasks across all waves.
func (o *WaveOrchestrator) TotalTasks() int {
	total := 0
	for _, w := range o.plan.Waves {
		total += len(w.Tasks)
	}
	return total
}

// CurrentWaveNumber returns the 1-indexed wave number currently active.
func (o *WaveOrchestrator) CurrentWaveNumber() int {
	if o.currentWave >= len(o.plan.Waves) {
		return 0
	}
	return o.plan.Waves[o.currentWave].Number
}

// CurrentWaveTasks returns the tasks in the current wave.
func (o *WaveOrchestrator) CurrentWaveTasks() []planparser.Task {
	if o.currentWave >= len(o.plan.Waves) {
		return nil
	}
	return o.plan.Waves[o.currentWave].Tasks
}

// StartNextWave advances to the next wave and returns its tasks.
// Returns nil if all waves are complete.
func (o *WaveOrchestrator) StartNextWave() []planparser.Task {
	if o.state == WaveStateAllComplete {
		return nil
	}
	if o.state == WaveStateWaveComplete {
		o.currentWave++
	}
	if o.currentWave >= len(o.plan.Waves) {
		o.state = WaveStateAllComplete
		return nil
	}

	o.state = WaveStateRunning
	tasks := o.plan.Waves[o.currentWave].Tasks
	for _, t := range tasks {
		o.taskStates[t.Number] = taskRunning
	}
	return tasks
}

// MarkTaskComplete marks a task as successfully completed.
// If all tasks in the current wave are done, transitions state.
func (o *WaveOrchestrator) MarkTaskComplete(taskNumber int) {
	o.taskStates[taskNumber] = taskComplete
	o.checkWaveComplete()
}

// MarkTaskFailed marks a task as failed.
// Other tasks in the wave continue. Wave completes when all tasks resolve.
func (o *WaveOrchestrator) MarkTaskFailed(taskNumber int) {
	o.taskStates[taskNumber] = taskFailed
	o.checkWaveComplete()
}

// IsCurrentWaveComplete returns true if all tasks in the current wave have resolved.
func (o *WaveOrchestrator) IsCurrentWaveComplete() bool {
	return o.state == WaveStateWaveComplete || o.state == WaveStateAllComplete
}

// CompletedTaskCount returns the number of completed tasks in the current wave.
func (o *WaveOrchestrator) CompletedTaskCount() int {
	return o.countCurrentWaveByStatus(taskComplete)
}

// FailedTaskCount returns the number of failed tasks in the current wave.
func (o *WaveOrchestrator) FailedTaskCount() int {
	return o.countCurrentWaveByStatus(taskFailed)
}

// HeaderContext returns the plan header for inclusion in task prompts.
func (o *WaveOrchestrator) HeaderContext() string {
	return o.plan.HeaderContext()
}

func (o *WaveOrchestrator) checkWaveComplete() {
	if o.currentWave >= len(o.plan.Waves) {
		return
	}
	tasks := o.plan.Waves[o.currentWave].Tasks
	for _, t := range tasks {
		s := o.taskStates[t.Number]
		if s == taskRunning || s == taskPending {
			return // still in progress
		}
	}
	// All tasks resolved — check if more waves remain
	if o.currentWave+1 >= len(o.plan.Waves) {
		o.state = WaveStateAllComplete
	} else {
		o.state = WaveStateWaveComplete
	}
}

func (o *WaveOrchestrator) countCurrentWaveByStatus(s taskStatus) int {
	if o.currentWave >= len(o.plan.Waves) {
		return 0
	}
	count := 0
	for _, t := range o.plan.Waves[o.currentWave].Tasks {
		if o.taskStates[t.Number] == s {
			count++
		}
	}
	return count
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./app/... -run TestWaveOrchestrator -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/wave_orchestrator.go app/wave_orchestrator_test.go
git commit -m "feat: add WaveOrchestrator state machine for parallel task execution"
```

---

### Task 4: Wave Validation Gate

**Files:**
- Modify: `app/app_actions.go:430-447`
- Create: `app/app_wave_validation_test.go`

**Step 1: Write failing test**

```go
package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePlanHasWaves_WithWaves(t *testing.T) {
	dir := t.TempDir()
	planFile := "test-plan.md"
	content := `# Plan

**Goal:** Test

## Wave 1
### Task 1: Something

Do it.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, planFile), []byte(content), 0o644))

	err := validatePlanHasWaves(dir, planFile)
	assert.NoError(t, err)
}

func TestValidatePlanHasWaves_NoWaves(t *testing.T) {
	dir := t.TempDir()
	planFile := "test-plan.md"
	content := `# Plan

**Goal:** Test

### Task 1: Something

Do it.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, planFile), []byte(content), 0o644))

	err := validatePlanHasWaves(dir, planFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no wave headers")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/... -run TestValidatePlanHasWaves -v`
Expected: FAIL — function doesn't exist.

**Step 3: Implement validation function**

Add to `app/app_actions.go`:

```go
// validatePlanHasWaves reads a plan file and checks it has ## Wave headers.
// Returns an error if the plan lacks wave annotations.
func validatePlanHasWaves(plansDir, planFile string) error {
	content, err := os.ReadFile(filepath.Join(plansDir, planFile))
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	_, err = planparser.Parse(string(content))
	return err
}
```

Add the import for `planparser` and `os` at the top of the file.

**Step 4: Wire validation into the implement action**

In `app/app_actions.go`, in the `case "implement":` block (around line 440), add validation before spawning:

```go
case "implement":
	// Validate plan has wave annotations
	plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
	if err := validatePlanHasWaves(plansDir, planFile); err != nil {
		// Revert to planning status
		if setErr := m.planState.SetStatus(planFile, planstate.StatusPlanning); setErr != nil {
			return m, m.handleError(setErr)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		m.toastManager.Error("Plan needs wave annotations before implementation. Returning to planning.")
		return m, tea.Batch(m.toastTickCmd(), func() tea.Msg { return planRefreshMsg{} })
	}
	// ... existing implement logic continues
```

**Step 5: Run tests**

Run: `go test ./app/... -run TestValidatePlanHasWaves -v`
Expected: PASS

Run: `go build ./...`
Expected: Clean build.

**Step 6: Commit**

```bash
git add app/app_actions.go app/app_wave_validation_test.go
git commit -m "feat: validate plan has wave headers before implementation"
```

---

### Task 5: Task Prompt Builder

**Files:**
- Create: `app/wave_prompt.go`
- Create: `app/wave_prompt_test.go`

**Step 1: Write failing test**

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planparser"
	"github.com/stretchr/testify/assert"
)

func TestBuildTaskPrompt(t *testing.T) {
	plan := &planparser.Plan{
		Goal:         "Build a feature",
		Architecture: "Modular approach",
		TechStack:    "Go, bubbletea",
	}
	task := planparser.Task{
		Number: 2,
		Title:  "Update Tests",
		Body:   "**Step 1:** Write the test\n\n**Step 2:** Run it",
	}

	prompt := buildTaskPrompt(plan, task, 1, 3)

	assert.Contains(t, prompt, "Build a feature")
	assert.Contains(t, prompt, "Modular approach")
	assert.Contains(t, prompt, "Go, bubbletea")
	assert.Contains(t, prompt, "Task 2")
	assert.Contains(t, prompt, "Update Tests")
	assert.Contains(t, prompt, "Write the test")
	assert.Contains(t, prompt, "cli-tools")
	assert.Contains(t, prompt, "Only modify the files listed in your task")
	assert.Contains(t, prompt, "Wave 1 of 3")
}

func TestBuildTaskPrompt_SingleWave(t *testing.T) {
	plan := &planparser.Plan{Goal: "Simple"}
	task := planparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := buildTaskPrompt(plan, task, 1, 1)

	// Single wave shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/... -run TestBuildTaskPrompt -v`
Expected: FAIL — function doesn't exist.

**Step 3: Implement prompt builder**

```go
package app

import (
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/config/planparser"
)

// buildTaskPrompt constructs the prompt for a single task instance.
func buildTaskPrompt(plan *planparser.Plan, task planparser.Task, waveNumber, totalWaves int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement Task %d: %s\n\n", task.Number, task.Title))
	sb.WriteString("Load the `cli-tools` skill before starting.\n\n")

	// Plan context
	header := plan.HeaderContext()
	if header != "" {
		sb.WriteString("## Plan Context\n\n")
		sb.WriteString(header)
		sb.WriteString("\n")
	}

	// Wave context
	sb.WriteString(fmt.Sprintf("## Wave %d of %d\n\n", waveNumber, totalWaves))
	if totalWaves > 1 {
		sb.WriteString("You are implementing one task of a multi-task plan. Other tasks in this wave may be running in parallel on the same worktree. Only modify the files listed in your task.\n\n")
	}

	// Task body
	sb.WriteString("## Task Instructions\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n")

	return sb.String()
}
```

**Step 4: Run tests**

Run: `go test ./app/... -run TestBuildTaskPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/wave_prompt.go app/wave_prompt_test.go
git commit -m "feat: task prompt builder with plan context and wave coordination"
```

---

## Wave 3

### Task 6: Wire Wave Orchestration into Implement Action

**Files:**
- Modify: `app/app.go` (add orchestrators map field)
- Modify: `app/app_actions.go` (replace single-instance implement with wave spawning)
- Modify: `app/app_state.go` (wave monitoring in metadata tick)

This task wires everything together. It replaces the existing single-instance "implement" flow with wave-based spawning.

**Step 1: Add orchestrators map to home struct**

In `app/app.go`, add to the `home` struct:
```go
// waveOrchestrators tracks active wave orchestrations by plan filename.
waveOrchestrators map[string]*WaveOrchestrator
```

Initialize it in the constructor (wherever `home` is created):
```go
waveOrchestrators: make(map[string]*WaveOrchestrator),
```

**Step 2: Rewrite the implement case in executePlanStageAction**

In `app/app_actions.go`, replace the `case "implement":` block with wave-based spawning:

```go
case "implement":
	// Validate plan has wave annotations
	plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
	if err := validatePlanHasWaves(plansDir, planFile); err != nil {
		if setErr := m.planState.SetStatus(planFile, planstate.StatusPlanning); setErr != nil {
			return m, m.handleError(setErr)
		}
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		m.toastManager.Error("Plan needs wave annotations before implementation. Returning to planning.")
		return m, tea.Batch(m.toastTickCmd(), func() tea.Msg { return planRefreshMsg{} })
	}

	// Parse plan and create orchestrator
	content, err := os.ReadFile(filepath.Join(plansDir, planFile))
	if err != nil {
		return m, m.handleError(err)
	}
	plan, err := planparser.Parse(string(content))
	if err != nil {
		return m, m.handleError(err)
	}

	orch := NewWaveOrchestrator(planFile, plan)
	m.waveOrchestrators[planFile] = orch

	if err := m.planState.SetStatus(planFile, planstate.StatusImplementing); err != nil {
		return m, m.handleError(err)
	}
	m.loadPlanState()
	m.updateSidebarPlans()
	m.updateSidebarItems()

	return m.startNextWave(orch, entry)
```

**Step 3: Implement startNextWave helper**

Add to `app/app_state.go`:

```go
// startNextWave spawns all task instances for the orchestrator's next wave.
func (m *home) startNextWave(orch *WaveOrchestrator, entry planstate.PlanEntry) (tea.Model, tea.Cmd) {
	tasks := orch.StartNextWave()
	if tasks == nil {
		return m, nil
	}

	planFile := orch.PlanFile()
	planName := planstate.DisplayName(planFile)

	// Set up shared worktree
	shared := gitpkg.NewSharedPlanWorktree(m.activeRepoPath, entry.Branch)
	if err := shared.Setup(); err != nil {
		return m, m.handleError(err)
	}

	var cmds []tea.Cmd
	for _, task := range tasks {
		prompt := buildTaskPrompt(orch.plan, task, orch.CurrentWaveNumber(), orch.TotalWaves())

		inst, err := session.NewInstance(session.InstanceOptions{
			Title:      fmt.Sprintf("%s-T%d", planName, task.Number),
			Path:       m.activeRepoPath,
			Program:    m.program,
			PlanFile:   planFile,
			AgentType:  session.AgentTypeCoder,
			TaskNumber: task.Number,
			WaveNumber: orch.CurrentWaveNumber(),
		})
		if err != nil {
			return m, m.handleError(err)
		}
		inst.QueuedPrompt = prompt

		m.newInstanceFinalizer = m.list.AddInstance(inst)

		startCmd := func() tea.Msg {
			err := inst.StartInSharedWorktree(shared, entry.Branch)
			return instanceStartedMsg{instance: inst, err: err}
		}
		cmds = append(cmds, startCmd)
	}

	waveNum := orch.CurrentWaveNumber()
	taskCount := len(tasks)
	m.toastManager.Info(fmt.Sprintf("Wave %d started: %d task(s) running", waveNum, taskCount))
	cmds = append(cmds, tea.WindowSize(), m.toastTickCmd())

	return m, tea.Batch(cmds...)
}
```

**Step 4: Add wave monitoring to metadata tick**

In `app/app.go`, in the `tickUpdateMetadataMessage` handler (around the coder-exit detection section), add wave completion monitoring:

```go
// Wave completion monitoring: check if all tasks in the current wave
// have PromptDetected, and trigger wave transition.
for planFile, orch := range m.waveOrchestrators {
	if orch.State() != WaveStateRunning {
		continue
	}
	// Check each task in the current wave
	for _, task := range orch.CurrentWaveTasks() {
		taskTitle := fmt.Sprintf("%s-T%d", planstate.DisplayName(planFile), task.Number)
		for _, inst := range m.list.GetInstances() {
			if inst.Title != taskTitle {
				continue
			}
			// Check if task completed (PromptDetected) or failed (tmux dead)
			alive, collected := tmuxAliveMap[inst.Title]
			if !collected {
				continue
			}
			if inst.PromptDetected {
				orch.MarkTaskComplete(task.Number)
			} else if !alive {
				orch.MarkTaskFailed(task.Number)
			}
		}
	}

	// If wave just completed, show confirmation
	if orch.IsCurrentWaveComplete() && orch.State() == WaveStateWaveComplete {
		waveNum := orch.CurrentWaveNumber()
		completed := orch.CompletedTaskCount()
		failed := orch.FailedTaskCount()
		total := completed + failed

		var message string
		if failed > 0 {
			message = fmt.Sprintf("Wave %d: %d/%d complete, %d failed.\nRetry failed / Skip to Wave %d / Abort",
				waveNum, completed, total, failed, waveNum+1)
		} else {
			message = fmt.Sprintf("Wave %d complete (%d/%d). Start Wave %d?",
				waveNum, completed, total, waveNum+1)
		}

		entry, _ := m.planState.Entry(planFile)
		advanceAction := func() tea.Msg {
			return waveAdvanceMsg{planFile: planFile, entry: entry}
		}
		asyncCmds = append(asyncCmds, m.confirmAction(message, advanceAction))
	}

	// If all waves complete, transition plan
	if orch.State() == WaveStateAllComplete {
		delete(m.waveOrchestrators, planFile)
		// Plan implementation is done — trigger the existing push/review flow
	}
}
```

**Step 5: Add waveAdvanceMsg handler**

Add a new message type and handler:

```go
// waveAdvanceMsg is sent when the user confirms advancing to the next wave.
type waveAdvanceMsg struct {
	planFile string
	entry    planstate.PlanEntry
}
```

In the `Update` method's message switch, add:

```go
case waveAdvanceMsg:
	orch, ok := m.waveOrchestrators[msg.planFile]
	if !ok {
		return m, nil
	}
	// Auto-pause completed wave's instances
	planName := planstate.DisplayName(msg.planFile)
	for _, task := range orch.CurrentWaveTasks() {
		taskTitle := fmt.Sprintf("%s-T%d", planName, task.Number)
		for _, inst := range m.list.GetInstances() {
			if inst.Title == taskTitle && inst.PromptDetected {
				if err := inst.Pause(); err != nil {
					log.WarningLog.Printf("could not pause task %s: %v", taskTitle, err)
				}
			}
		}
	}
	return m.startNextWave(orch, msg.entry)
```

**Step 6: Run tests and verify build**

Run: `go build ./...`
Expected: Clean build.

Run: `go test ./app/... -v`
Expected: All tests pass.

**Step 7: Commit**

```bash
git add app/app.go app/app_actions.go app/app_state.go
git commit -m "feat: wire wave orchestration into implement action with monitoring and transitions"
```

---

### Task 7: Instance List Wave Badge Rendering

**Files:**
- Modify: `ui/list_renderer.go`
- Modify: `ui/list_styles.go`

**Step 1: Add wave badge to instance rendering**

In `ui/list_renderer.go`, in the `Render` method of `InstanceRenderer`, add a wave badge after the status indicator when the instance has a non-zero `WaveNumber`:

Find where the title line is rendered and add:

```go
// Wave badge for task instances
if i.WaveNumber > 0 {
	waveBadge := waveBadgeStyle.Render(fmt.Sprintf("W%d", i.WaveNumber))
	// Append badge after the title
	titleLine = titleLine + " " + waveBadge
}
```

In `ui/list_styles.go`, add the badge style:

```go
var waveBadgeStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Bold(false)
```

**Step 2: Run and verify**

Run: `go build ./...`
Expected: Clean build.

**Step 3: Commit**

```bash
git add ui/list_renderer.go ui/list_styles.go
git commit -m "feat: render wave badge on task instances in list view"
```
