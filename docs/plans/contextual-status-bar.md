# Contextual Status Bar Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a full-width top status bar showing repo name, branch, plan name, and wave progress glyphs, adapting content based on the focused/selected sidebar or list item.

**Architecture:** New `ui/statusbar.go` component receives a `StatusBarData` struct computed by the app model. The bar renders as a single full-width line with `ColorSurface` background. Layout integration subtracts 1 from `contentHeight` and prepends the bar in `View()`.

**Tech Stack:** Go, bubbletea, lipgloss (Rosé Pine Moon palette from `ui/theme.go`)

---

## Wave 1: StatusBar component and data model

### Task 1: StatusBar data types and renderer

**Files:**
- Create: `ui/statusbar.go`
- Create: `ui/statusbar_test.go`

**Step 1: Write the failing test**

Create `ui/statusbar_test.go` with tests for the StatusBar renderer:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusBar_Baseline(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(80)
	sb.SetData(StatusBarData{
		RepoName: "kasmos",
		Branch:   "main",
	})

	result := sb.String()
	assert.Contains(t, result, "kasmos")
	assert.Contains(t, result, "main")
	// Should be exactly 1 line (no newlines in output)
	assert.Equal(t, 0, strings.Count(result, "\n"))
}

func TestStatusBar_PlanContext(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
	})

	result := sb.String()
	assert.Contains(t, result, "kasmos")
	assert.Contains(t, result, "plan/auth-refactor")
	assert.Contains(t, result, "auth-refactor")
	assert.Contains(t, result, "implementing")
}

func TestStatusBar_WaveGlyphs(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:   "kasmos",
		Branch:     "plan/auth-refactor",
		PlanName:   "auth-refactor",
		PlanStatus: "implementing",
		WaveLabel:  "wave 2/4",
		TaskGlyphs: []TaskGlyph{
			TaskGlyphComplete,
			TaskGlyphComplete,
			TaskGlyphRunning,
			TaskGlyphFailed,
			TaskGlyphPending,
		},
	})

	result := sb.String()
	assert.Contains(t, result, "wave 2/4")
	// Glyphs should be present (check the raw glyph chars)
	assert.Contains(t, result, "✓")
	assert.Contains(t, result, "●")
	assert.Contains(t, result, "✕")
	assert.Contains(t, result, "○")
}

func TestStatusBar_Truncation(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(40) // narrow terminal
	sb.SetData(StatusBarData{
		RepoName: "very-long-repository-name-that-wont-fit",
		Branch:   "feature/extremely-long-branch-name-here",
	})

	result := sb.String()
	// Should not exceed width (lipgloss handles this, but verify no panic)
	require.NotEmpty(t, result)
}

func TestStatusBar_EmptyData(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(80)
	sb.SetData(StatusBarData{})

	result := sb.String()
	// Should still render the app name
	assert.Contains(t, result, "kasmos")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestStatusBar -v`
Expected: FAIL — `NewStatusBar`, `StatusBarData`, `TaskGlyph` types don't exist yet.

**Step 3: Write minimal implementation**

Create `ui/statusbar.go`:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// TaskGlyph represents the status of a single task in wave progress.
type TaskGlyph int

const (
	TaskGlyphComplete TaskGlyph = iota
	TaskGlyphRunning
	TaskGlyphFailed
	TaskGlyphPending
)

// StatusBarData holds the contextual information displayed in the status bar.
type StatusBarData struct {
	RepoName   string
	Branch     string
	PlanName   string      // empty = no plan context
	PlanStatus string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel  string      // "wave 2/4" or empty
	TaskGlyphs []TaskGlyph // per-task status for wave progress
}

// StatusBar is the top status bar component.
type StatusBar struct {
	width int
	data  StatusBarData
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetSize sets the terminal width for the status bar.
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetData updates the status bar content.
func (s *StatusBar) SetData(data StatusBarData) {
	s.data = data
}

var statusBarStyle = lipgloss.NewStyle().
	Background(ColorSurface).
	Foreground(ColorText).
	Padding(0, 1)

var statusBarAppNameStyle = lipgloss.NewStyle().
	Foreground(ColorIris).
	Background(ColorSurface).
	Bold(true)

var statusBarSepStyle = lipgloss.NewStyle().
	Foreground(ColorOverlay).
	Background(ColorSurface)

var statusBarBranchStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Background(ColorSurface)

var statusBarPlanNameStyle = lipgloss.NewStyle().
	Foreground(ColorText).
	Background(ColorSurface)

var statusBarWaveLabelStyle = lipgloss.NewStyle().
	Foreground(ColorSubtle).
	Background(ColorSurface)

// planStatusStyle returns a styled plan status string.
func planStatusStyle(status string) string {
	var fg lipgloss.TerminalColor
	switch status {
	case "implementing":
		fg = ColorFoam
	case "reviewing":
		fg = ColorRose
	case "done":
		fg = ColorFoam
	default: // ready, planning
		fg = ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg).Background(ColorSurface).Render(status)
}

// taskGlyphStr returns the styled glyph for a task status.
func taskGlyphStr(g TaskGlyph) string {
	switch g {
	case TaskGlyphComplete:
		return lipgloss.NewStyle().Foreground(ColorFoam).Background(ColorSurface).Render("✓")
	case TaskGlyphRunning:
		return lipgloss.NewStyle().Foreground(ColorIris).Background(ColorSurface).Render("●")
	case TaskGlyphFailed:
		return lipgloss.NewStyle().Foreground(ColorLove).Background(ColorSurface).Render("✕")
	case TaskGlyphPending:
		return lipgloss.NewStyle().Foreground(ColorMuted).Background(ColorSurface).Render("○")
	default:
		return ""
	}
}

const statusBarSep = " │ "

func (s *StatusBar) String() string {
	if s.width < 10 {
		return ""
	}

	var parts []string

	// App name (always)
	parts = append(parts, statusBarAppNameStyle.Render("kasmos"))

	// Repo name
	if s.data.RepoName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.RepoName))
	}

	// Branch
	if s.data.Branch != "" {
		parts = append(parts, statusBarBranchStyle.Render("\ue725 "+s.data.Branch))
	}

	// Plan name
	if s.data.PlanName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.PlanName))
	}

	// Plan status (without wave) or wave glyphs
	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		var glyphs strings.Builder
		for _, g := range s.data.TaskGlyphs {
			glyphs.WriteString(taskGlyphStr(g))
		}
		parts = append(parts, statusBarWaveLabelStyle.Render(s.data.WaveLabel)+" "+glyphs.String())
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	sep := statusBarSepStyle.Render(statusBarSep)
	content := strings.Join(parts, sep)

	// Pad to full width with surface background
	contentWidth := runewidth.StringWidth(lipgloss.PlainText(content))
	padWidth := s.width - contentWidth - 2 // account for Padding(0,1)
	if padWidth < 0 {
		padWidth = 0
	}
	padding := lipgloss.NewStyle().Background(ColorSurface).Render(
		fmt.Sprintf("%*s", padWidth, ""),
	)

	return statusBarStyle.Width(s.width).Render(lipgloss.PlainText(content + padding)[:0] +
		content + padding)
}
```

Wait — the above String() has some complexity around padding. Let me simplify: just use lipgloss `.Width(s.width)` on the whole bar. That handles truncation and padding automatically.

Simplified `String()`:

```go
func (s *StatusBar) String() string {
	if s.width < 10 {
		return ""
	}

	var parts []string

	// App name (always)
	parts = append(parts, statusBarAppNameStyle.Render("kasmos"))

	// Repo name
	if s.data.RepoName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.RepoName))
	}

	// Branch
	if s.data.Branch != "" {
		parts = append(parts, statusBarBranchStyle.Render("\ue725 "+s.data.Branch))
	}

	// Plan name
	if s.data.PlanName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.PlanName))
	}

	// Plan status or wave glyphs
	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		var glyphs strings.Builder
		for _, g := range s.data.TaskGlyphs {
			glyphs.WriteString(taskGlyphStr(g))
		}
		parts = append(parts, statusBarWaveLabelStyle.Render(s.data.WaveLabel)+" "+glyphs.String())
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	sep := statusBarSepStyle.Render(statusBarSep)
	content := strings.Join(parts, sep)

	return statusBarStyle.Width(s.width).Render(content)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./ui/ -run TestStatusBar -v`
Expected: PASS

**Step 5: Commit**

```bash
git add ui/statusbar.go ui/statusbar_test.go
git commit -m "feat(ui): add StatusBar component with data model and renderer"
```

---

## Wave 2: Layout integration and data wiring

### Task 2: Layout integration — subtract height and render bar in View()

**Files:**
- Modify: `app/app.go` — `updateHandleWindowSizeEvent`, `View()`, `home` struct

**Step 1: Write the failing test**

Add to `app/app_test.go` (or a new test file if appropriate):

```go
func TestStatusBarIncludedInView(t *testing.T) {
	// Create a minimal home model and verify View() output contains "kasmos"
	// at the top (status bar baseline).
	h := &home{
		// ... minimal init with mocked components ...
	}
	// This test verifies the status bar is rendered.
	// Since full model init is complex, we test at the component level instead
	// and verify integration manually.
}
```

Given the complexity of the `home` model constructor, the integration test approach should be:
- Unit test the StatusBar component (done in Task 1)
- Integration test via the existing `app_test.go` patterns

**Step 2: Add StatusBar to home struct**

In `app/app.go`, add to the `home` struct:

```go
// statusBar displays the contextual top status bar
statusBar *ui.StatusBar
```

In `newHome()`, after existing component initialization:

```go
h.statusBar = ui.NewStatusBar()
```

**Step 3: Update updateHandleWindowSizeEvent**

In `app/app.go`, modify `updateHandleWindowSizeEvent`:

```go
// Status bar takes 1 line at the top
statusBarHeight := 1
contentHeight := msg.Height - menuHeight - statusBarHeight
```

Also set the status bar width:

```go
m.statusBar.SetSize(msg.Width)
```

**Step 4: Remove PaddingTop from colStyle in View()**

In `View()`, change:

```go
colStyle := lipgloss.NewStyle().PaddingTop(1).Height(m.contentHeight + 1)
```

to:

```go
colStyle := lipgloss.NewStyle().Height(m.contentHeight)
```

**Step 5: Render status bar in View()**

In `View()`, before `listAndPreview`, add status bar rendering:

```go
statusBarView := m.statusBar.String()
```

Then change the vertical join:

```go
mainView := lipgloss.JoinVertical(
	lipgloss.Left,
	statusBarView,
	listAndPreview,
	m.menu.String(),
)
```

**Step 6: Run tests and verify**

Run: `go test ./app/ -v -count=1`
Run: `go build ./...`
Expected: PASS, builds clean

**Step 7: Commit**

```bash
git add app/app.go
git commit -m "feat(ui): integrate StatusBar into layout — subtract height, render at top"
```

### Task 3: Compute StatusBarData from app state

**Files:**
- Modify: `app/app_state.go` — new `computeStatusBarData()` method

**Step 1: Write the failing test**

Add to `app/app_state_sidebar_status_test.go` or a new test file:

```go
func TestComputeStatusBarData_Baseline(t *testing.T) {
	h := &home{
		activeRepoPath: "/home/user/repos/kasmos",
	}
	h.sidebar = ui.NewSidebar()
	h.sidebar.SetRepoName("kasmos")
	h.list = ui.NewList(&h.spinner, false)

	data := h.computeStatusBarData()
	assert.Equal(t, "kasmos", data.RepoName)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestComputeStatusBarData -v`
Expected: FAIL — `computeStatusBarData` doesn't exist.

**Step 3: Implement computeStatusBarData**

Add to `app/app_state.go`:

```go
// computeStatusBarData builds the StatusBarData from the current app state.
func (m *home) computeStatusBarData() ui.StatusBarData {
	data := ui.StatusBarData{
		RepoName: filepath.Base(m.activeRepoPath),
	}

	// Determine branch and plan context from sidebar selection.
	planFile := m.sidebar.GetSelectedPlanFile()
	selected := m.list.GetSelectedInstance()

	switch {
	case planFile != "" && m.planState != nil:
		// Plan is selected in sidebar — show plan branch and status.
		entry, ok := m.planState.Entry(planFile)
		if ok {
			data.Branch = entry.Branch
			data.PlanName = planstate.DisplayName(planFile)
			data.PlanStatus = string(entry.Status)

			// Wave orchestration glyphs.
			if orch, orchOk := m.waveOrchestrators[planFile]; orchOk {
				waveNum := orch.CurrentWaveNumber()
				totalWaves := orch.TotalWaves()
				if waveNum > 0 {
					data.WaveLabel = fmt.Sprintf("wave %d/%d", waveNum, totalWaves)
					tasks := orch.CurrentWaveTasks()
					data.TaskGlyphs = make([]ui.TaskGlyph, len(tasks))
					for i, task := range tasks {
						switch {
						case orch.IsTaskComplete(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphComplete
						case orch.IsTaskFailed(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphFailed
						case orch.IsTaskRunning(task.Number):
							data.TaskGlyphs[i] = ui.TaskGlyphRunning
						default:
							data.TaskGlyphs[i] = ui.TaskGlyphPending
						}
					}
				}
			}
		}
	case selected != nil && selected.Branch != "":
		// Instance selected — show its branch.
		data.Branch = selected.Branch
		// If the instance has a plan, show plan context too.
		if selected.PlanFile != "" && m.planState != nil {
			entry, ok := m.planState.Entry(selected.PlanFile)
			if ok {
				data.PlanName = planstate.DisplayName(selected.PlanFile)
				data.PlanStatus = string(entry.Status)
			}
		}
	default:
		// No specific selection — show active repo's default branch.
		data.Branch = "main"
	}

	return data
}
```

Note: The orchestrator needs `IsTaskComplete` and `IsTaskFailed` methods. These don't exist yet — they need to be added in Task 4.

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestComputeStatusBarData -v`
Expected: PASS (baseline test only; plan/wave tests need Task 4).

**Step 5: Wire computeStatusBarData into View()**

In `app/app.go` `View()`, before rendering the status bar:

```go
m.statusBar.SetData(m.computeStatusBarData())
statusBarView := m.statusBar.String()
```

**Step 6: Commit**

```bash
git add app/app_state.go app/app.go
git commit -m "feat(app): compute StatusBarData from sidebar selection, plan state, and wave orchestrators"
```

### Task 4: Add task status query methods to WaveOrchestrator

**Files:**
- Modify: `app/wave_orchestrator.go`
- Modify: `app/wave_orchestrator_test.go`

**Step 1: Write the failing test**

Add to `app/wave_orchestrator_test.go`:

```go
func TestWaveOrchestrator_TaskStatusQueries(t *testing.T) {
	plan := &planparser.Plan{
		Waves: []planparser.Wave{
			{Number: 1, Tasks: []planparser.Task{
				{Number: 1, Title: "Task 1"},
				{Number: 2, Title: "Task 2"},
				{Number: 3, Title: "Task 3"},
			}},
		},
	}
	orch := NewWaveOrchestrator("test.md", plan)
	orch.StartNextWave()

	// All should be running initially.
	assert.True(t, orch.IsTaskRunning(1))
	assert.False(t, orch.IsTaskComplete(1))
	assert.False(t, orch.IsTaskFailed(1))

	orch.MarkTaskComplete(1)
	assert.True(t, orch.IsTaskComplete(1))
	assert.False(t, orch.IsTaskRunning(1))

	orch.MarkTaskFailed(2)
	assert.True(t, orch.IsTaskFailed(2))
	assert.False(t, orch.IsTaskRunning(2))

	// Task 3 still running.
	assert.True(t, orch.IsTaskRunning(3))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestWaveOrchestrator_TaskStatusQueries -v`
Expected: FAIL — `IsTaskComplete`, `IsTaskFailed` don't exist.

**Step 3: Implement the methods**

Add to `app/wave_orchestrator.go`:

```go
// IsTaskComplete returns true if the given task number has completed successfully.
func (o *WaveOrchestrator) IsTaskComplete(taskNumber int) bool {
	return o.taskStates[taskNumber] == taskComplete
}

// IsTaskFailed returns true if the given task number has failed.
func (o *WaveOrchestrator) IsTaskFailed(taskNumber int) bool {
	return o.taskStates[taskNumber] == taskFailed
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestWaveOrchestrator_TaskStatusQueries -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/wave_orchestrator.go app/wave_orchestrator_test.go
git commit -m "feat(app): add IsTaskComplete/IsTaskFailed query methods to WaveOrchestrator"
```

### Task 5: Update status bar on state changes

**Files:**
- Modify: `app/app.go` — ensure `computeStatusBarData()` is called on relevant state changes

The status bar data is already recomputed every `View()` call (from Task 3 wiring). Since `View()` is called on every bubbletea render cycle, the status bar automatically reflects the latest state. No additional wiring is needed — bubbletea's render-on-message architecture handles this.

Verify by running the full test suite:

Run: `go test ./... -count=1`
Expected: PASS

**Commit:**

```bash
git add -A
git commit -m "feat: contextual status bar — complete implementation"
```
