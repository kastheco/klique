# Details Tab Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the lazygit Git tab with a read-only Info tab showing instance and plan metadata.

**Architecture:** Delete `ui/git_pane.go` and all lazygit integration code. Create `ui/info_pane.go` with an `InfoPane` struct that renders styled metadata using lipgloss + bubbles viewport for scrolling. The `app` layer builds an `InfoData` struct on instance change and passes it down. Focus ring slot 3 stays but no longer supports focus/insert mode.

**Tech Stack:** Go, bubbletea, lipgloss, bubbles/viewport

---

## Wave 1: Remove Lazygit Integration

### Task 1: Delete git_pane.go and remove GitPane from TabbedWindow

**Files:**
- Delete: `ui/git_pane.go`
- Modify: `ui/tabbed_window.go`
- Modify: `ui/tabbed_window_test.go` (if exists)

**Step 1: Delete `ui/git_pane.go`**

```bash
rm ui/git_pane.go
```

**Step 2: Update `ui/tabbed_window.go`**

Rename `GitTab` constant to `InfoTab`:

```go
const (
	PreviewTab int = iota
	DiffTab
	InfoTab
)
```

Remove `git *GitPane` and `gitContent string` fields from `TabbedWindow`. Add a placeholder `info` field (will be replaced in wave 2):

```go
type TabbedWindow struct {
	tabs []string

	activeTab  int
	focusedTab int
	height     int
	width      int

	preview  *PreviewPane
	diff     *DiffPane
	info     *InfoPane
	instance *session.Instance
	focused  bool
	focusMode bool
}
```

Update `NewTabbedWindow` signature:

```go
func NewTabbedWindow(preview *PreviewPane, diff *DiffPane, info *InfoPane) *TabbedWindow {
	return &TabbedWindow{
		tabs: []string{
			"\uea85 Agent",
			"\ueae1 Diff",
			"\uea74 Info",
		},
		preview:    preview,
		diff:       diff,
		info:       info,
		focusedTab: -1,
	}
}
```

Update `String()` method — replace `case GitTab:` block:

```go
	case InfoTab:
		content = w.info.String()
```

Remove `SetGitContent()`, `GetGitPane()` methods. Replace `IsInGitTab()` with `IsInInfoTab()`:

```go
func (w *TabbedWindow) IsInInfoTab() bool {
	return w.activeTab == InfoTab
}
```

Update `SetSize` to call `w.info.SetSize(contentWidth, contentHeight)` instead of `w.git.SetSize(...)`.

Update `ScrollUp()`/`ScrollDown()`/`ContentScrollUp()`/`ContentScrollDown()` — replace git tab no-op comments with info pane scrolling:

```go
	case InfoTab:
		w.info.ScrollUp()
```

```go
	case InfoTab:
		w.info.ScrollDown()
```

**Step 3: Verify the project compiles (it won't yet — InfoPane doesn't exist). Move to task 2.**

---

### Task 2: Create InfoPane stub

**Files:**
- Create: `ui/info_pane.go`

**Step 1: Create the minimal InfoPane stub**

```go
package ui

import "github.com/charmbracelet/bubbles/viewport"

// InfoData holds the data to render in the info pane.
// Built by the app layer from instance + plan + wave state.
type InfoData struct {
	// Instance fields
	Title   string
	Program string
	Branch  string
	Path    string
	Created string
	Status  string

	// Plan fields (empty for ad-hoc)
	PlanName        string
	PlanDescription string
	PlanStatus      string
	PlanTopic       string
	PlanBranch      string
	PlanCreated     string

	// Wave fields (zero values = no wave)
	AgentType  string
	WaveNumber int
	TotalWaves int
	TaskNumber int
	TotalTasks int
	WaveTasks  []WaveTaskInfo

	// HasPlan is true when the instance is bound to a plan.
	HasPlan bool
	// HasInstance is true when an instance is selected.
	HasInstance bool
}

// WaveTaskInfo describes a single task in the current wave.
type WaveTaskInfo struct {
	Number int
	State  string // "complete", "running", "failed", "pending"
}

// InfoPane renders instance and plan metadata in the info tab.
type InfoPane struct {
	width, height int
	data          InfoData
	viewport      viewport.Model
}

// NewInfoPane creates a new InfoPane.
func NewInfoPane() *InfoPane {
	vp := viewport.New(0, 0)
	return &InfoPane{viewport: vp}
}

// SetSize updates the pane dimensions.
func (p *InfoPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width
	p.viewport.Height = height
}

// SetData updates the data to render.
func (p *InfoPane) SetData(data InfoData) {
	p.data = data
	p.viewport.SetContent(p.render())
	p.viewport.GotoTop()
}

// ScrollUp scrolls the viewport up.
func (p *InfoPane) ScrollUp() {
	p.viewport.LineUp(1)
}

// ScrollDown scrolls the viewport down.
func (p *InfoPane) ScrollDown() {
	p.viewport.LineDown(1)
}

// String renders the info pane content.
func (p *InfoPane) String() string {
	if !p.data.HasInstance {
		return "no instance selected"
	}
	return p.viewport.View()
}

// render builds the styled content string. Called internally when data changes.
func (p *InfoPane) render() string {
	return "info pane placeholder"
}
```

**Step 2: Verify the project compiles**

Run: `go build ./...`

**Step 3: Commit**

```bash
git add -A && git commit -m "feat: replace git tab with info tab stub, delete lazygit integration"
```

---

### Task 3: Remove lazygit references from app layer

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Modify: `app/app_input.go`

**Step 1: Update `app/app.go`**

In `newHome()`, change `NewTabbedWindow` call:

```go
tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
```

Delete `gitTabTickMsg` struct and its `case gitTabTickMsg:` handler in `Update()` (lines ~467-483 and ~1185-1186).

**Step 2: Update `app/app_state.go`**

Delete `enterGitFocusMode()` function (lines ~239-263).

Delete `spawnGitTab()` function (lines ~572-600).

Delete `killGitTab()` function (lines ~602-605).

Remove lazygit respawn block from `instanceChanged()` (lines ~551-561):

```go
	// Respawn lazygit if the selected instance changed while on the git tab
	if m.tabbedWindow.IsInGitTab() {
		...
	}
```

**Step 3: Update `app/app_input.go`**

In the focus mode handler (~line 476-537): remove the entire git tab focus block:

```go
		// Git tab focus: forward to lazygit
		if m.tabbedWindow.IsInGitTab() {
			...
			return m, nil
		}
```

In the `keys.KeyTab` handler (~line 1042-1054): remove lazygit spawn/kill logic. Replace with:

```go
	case keys.KeyTab:
		m.nextFocusSlot()
		return m, m.instanceChanged()
```

In the `keys.KeyGitTab` handler (~line 1073-1084): simplify to just set focus slot without spawning lazygit:

```go
	case keys.KeyGitTab:
		m.setFocusSlot(slotGit)
		return m, m.instanceChanged()
```

In the focus mode `!/@/#` jump handler (~line 484-507): remove `wasGitTab`/`killGitTab`/`spawnGitTab` logic. Simplify to:

```go
		if doJump {
			m.exitFocusMode()
			m.setFocusSlot(jumpSlot)
			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		}
```

Update comment at line ~476 from "agent's or lazygit's PTY" to "agent's PTY".

**Step 4: Verify the project compiles**

Run: `go build ./...`

**Step 5: Commit**

```bash
git add -A && git commit -m "refactor: remove all lazygit spawn/kill/tick code from app layer"
```

---

### Task 4: Remove lazygit keybinds and update help text

**Files:**
- Modify: `keys/keys.go`
- Modify: `app/help.go`

**Step 1: Update `keys/keys.go`**

Rename `KeyGitTab` to `KeyInfoTab`. Update the string mapping from `"g": KeyGitTab` to `"g": KeyInfoTab`. Update `KeyTabGit` to `KeyTabInfo`. Update all binding descriptions referencing "git" to "info".

**Step 2: Update `app/help.go`**

Change tab cycle description:

```go
keyStyle.Render("tab/shift+tab")+descStyle.Render(" - cycle tabs (agent → diff → info)"),
```

Replace the `g` lazygit line:

```go
keyStyle.Render("!/@ /#")+descStyle.Render("        - jump to agent/diff/info tab"),
```

Remove:

```go
keyStyle.Render("g")+descStyle.Render("             - open lazygit"),
```

Replace with:

```go
keyStyle.Render("g")+descStyle.Render("             - info tab"),
```

Also update the `helpTypeInstanceStart` help text — change `tab` description to `(!/@ /# to jump to agent/diff/info)`.

**Step 3: Update `app/app_input.go`** references from `keys.KeyGitTab` to `keys.KeyInfoTab` and from `keys.KeyTabGit` to `keys.KeyTabInfo`.

**Step 4: Verify the project compiles**

Run: `go build ./...`

**Step 5: Commit**

```bash
git add -A && git commit -m "refactor: rename git keybinds to info, update help text"
```

---

### Task 5: Update tests referencing lazygit

**Files:**
- Modify: `session/tmux/tmux_test.go`

**Step 1: Update the tmux cleanup test**

The test at line ~149 includes `kas_lazygit_session1` in mock output. Remove it from the mock data and update the assertion count from 4 to 3 (or keep it as legacy cleanup test data — the `kas_` prefix matcher will still kill it). The `kas_lazygit_` prefix is matched by the `kas_` prefix anyway, so the test data is still valid. Just update the test name:

Change: `"kills kas, legacy klique/hivemind, and lazygit sessions"`
To: `"kills kas and legacy klique/hivemind sessions"`

The mock data can keep the lazygit line since `kas_lazygit_` still starts with `kas_` — the cleanup function correctly kills it.

**Step 2: Run all tests**

Run: `go test ./...`

**Step 3: Commit**

```bash
git add -A && git commit -m "test: update tmux cleanup test name after lazygit removal"
```

---

## Wave 2: Build Info Pane Renderer

### Task 6: Write InfoPane render tests

**Files:**
- Create: `ui/info_pane_test.go`

**Step 1: Write tests for the three rendering modes**

```go
package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfoPane_NoInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{HasInstance: false})
	assert.Contains(t, p.String(), "no instance selected")
}

func TestInfoPane_AdHocInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance: true,
		HasPlan:     false,
		Title:       "fix-login-bug",
		Program:     "opencode",
		Branch:      "kas/fix-login-bug",
		Path:        "/home/kas/dev/myapp",
		Created:     "2026-02-25 14:30",
		Status:      "running",
	})
	output := p.String()
	assert.Contains(t, output, "fix-login-bug")
	assert.Contains(t, output, "opencode")
	assert.Contains(t, output, "kas/fix-login-bug")
	assert.Contains(t, output, "running")
	// Should NOT contain plan section
	assert.NotContains(t, output, "plan")
}

func TestInfoPane_PlanBoundInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance:     true,
		HasPlan:         true,
		Title:           "my-feature-coder",
		Program:         "claude",
		Branch:          "plan/my-feature",
		Status:          "running",
		PlanName:        "my-feature",
		PlanDescription: "add dark mode toggle",
		PlanStatus:      "implementing",
		PlanTopic:       "ui",
		PlanBranch:      "plan/my-feature",
		PlanCreated:     "2026-02-25",
		AgentType:       "coder",
		WaveNumber:      2,
		TotalWaves:      3,
		TaskNumber:      4,
		TotalTasks:      6,
	})
	output := p.String()
	assert.Contains(t, output, "my-feature")
	assert.Contains(t, output, "add dark mode toggle")
	assert.Contains(t, output, "implementing")
	assert.Contains(t, output, "coder")
}

func TestInfoPane_WaveProgress(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance: true,
		HasPlan:     true,
		Title:       "test-coder",
		Program:     "claude",
		Status:      "running",
		PlanName:    "test-plan",
		PlanStatus:  "implementing",
		WaveTasks: []WaveTaskInfo{
			{Number: 1, State: "complete"},
			{Number: 2, State: "running"},
			{Number: 3, State: "pending"},
		},
	})
	output := p.String()
	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "●")
	assert.Contains(t, output, "○")
}

func TestInfoPane_Scrolling(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 5) // very short to force overflow
	p.SetData(InfoData{
		HasInstance:     true,
		HasPlan:         true,
		Title:           "test",
		Program:         "claude",
		PlanName:        "test",
		PlanDescription: "desc",
		PlanStatus:      "ready",
		WaveTasks: []WaveTaskInfo{
			{Number: 1, State: "complete"},
			{Number: 2, State: "complete"},
			{Number: 3, State: "running"},
			{Number: 4, State: "pending"},
			{Number: 5, State: "pending"},
			{Number: 6, State: "pending"},
			{Number: 7, State: "pending"},
			{Number: 8, State: "pending"},
		},
	})
	before := p.String()
	require.NotEmpty(t, before)
	p.ScrollDown()
	after := p.String()
	// Content should shift after scrolling
	assert.NotEqual(t, before, after)
}
```

**Step 2: Run the tests — they should fail (render is a placeholder)**

Run: `go test ./ui/ -run TestInfoPane -v`

**Step 3: Commit**

```bash
git add -A && git commit -m "test: add info pane rendering tests"
```

---

### Task 7: Implement InfoPane renderer

**Files:**
- Modify: `ui/info_pane.go`

**Step 1: Implement the `render()` method**

Replace the placeholder `render()` with a full implementation using lipgloss styles:

```go
func (p *InfoPane) render() string {
	var sections []string

	if p.data.HasPlan {
		sections = append(sections, p.renderPlanSection())
	}
	sections = append(sections, p.renderInstanceSection())
	if len(p.data.WaveTasks) > 0 {
		sections = append(sections, p.renderWaveSection())
	}

	return strings.Join(sections, "\n\n")
}
```

Implement `renderPlanSection()`:

```go
var (
	infoSectionStyle = lipgloss.NewStyle().Foreground(ColorFoam).Bold(true)
	infoDividerStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	infoLabelStyle   = lipgloss.NewStyle().Foreground(ColorMuted).Width(20)
	infoValueStyle   = lipgloss.NewStyle().Foreground(ColorText)
)

func statusColor(status string) lipgloss.TerminalColor {
	switch status {
	case "implementing":
		return ColorIris
	case "done":
		return ColorFoam
	case "reviewing":
		return ColorGold
	case "running":
		return ColorFoam
	case "ready":
		return ColorMuted
	case "planning":
		return ColorIris
	case "cancelled":
		return ColorMuted
	case "paused":
		return ColorMuted
	default:
		return ColorText
	}
}

func (p *InfoPane) renderRow(label, value string) string {
	return infoLabelStyle.Render(label) + infoValueStyle.Render(value)
}

func (p *InfoPane) renderStatusRow(label, value string) string {
	return infoLabelStyle.Render(label) + lipgloss.NewStyle().Foreground(statusColor(value)).Render(value)
}

func (p *InfoPane) renderDivider() string {
	w := p.width - 4
	if w < 10 {
		w = 10
	}
	return infoDividerStyle.Render(strings.Repeat("─", w))
}

func (p *InfoPane) renderPlanSection() string {
	lines := []string{
		infoSectionStyle.Render("plan"),
		p.renderDivider(),
	}
	if p.data.PlanName != "" {
		lines = append(lines, p.renderRow("name", p.data.PlanName))
	}
	if p.data.PlanDescription != "" {
		lines = append(lines, p.renderRow("description", p.data.PlanDescription))
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
	return strings.Join(lines, "\n")
}
```

Implement `renderInstanceSection()`:

```go
func (p *InfoPane) renderInstanceSection() string {
	lines := []string{
		infoSectionStyle.Render("instance"),
		p.renderDivider(),
	}
	if p.data.Title != "" {
		lines = append(lines, p.renderRow("title", p.data.Title))
	}
	if p.data.AgentType != "" {
		lines = append(lines, p.renderRow("role", p.data.AgentType))
	}
	if p.data.Program != "" {
		lines = append(lines, p.renderRow("program", p.data.Program))
	}
	if p.data.Status != "" {
		lines = append(lines, p.renderStatusRow("status", p.data.Status))
	}
	if p.data.Branch != "" {
		lines = append(lines, p.renderRow("branch", p.data.Branch))
	}
	if p.data.Path != "" {
		lines = append(lines, p.renderRow("path", p.data.Path))
	}
	if p.data.Created != "" {
		lines = append(lines, p.renderRow("created", p.data.Created))
	}
	if p.data.WaveNumber > 0 {
		lines = append(lines, p.renderRow("wave", fmt.Sprintf("%d/%d", p.data.WaveNumber, p.data.TotalWaves)))
	}
	if p.data.TaskNumber > 0 {
		lines = append(lines, p.renderRow("task", fmt.Sprintf("%d of %d", p.data.TaskNumber, p.data.TotalTasks)))
	}
	return strings.Join(lines, "\n")
}
```

Implement `renderWaveSection()`:

```go
func (p *InfoPane) renderWaveSection() string {
	lines := []string{
		infoSectionStyle.Render("wave progress"),
		p.renderDivider(),
	}
	for _, task := range p.data.WaveTasks {
		var glyph string
		var glyphColor lipgloss.TerminalColor
		switch task.State {
		case "complete":
			glyph = "✓"
			glyphColor = ColorFoam
		case "running":
			glyph = "●"
			glyphColor = ColorIris
		case "failed":
			glyph = "✗"
			glyphColor = ColorLove
		default:
			glyph = "○"
			glyphColor = ColorMuted
		}
		label := fmt.Sprintf("task %d", task.Number)
		value := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph) + " " + task.State
		lines = append(lines, infoLabelStyle.Render(label)+value)
	}
	return strings.Join(lines, "\n")
}
```

Add required imports: `"fmt"`, `"strings"`, `"github.com/charmbracelet/lipgloss"`.

**Step 2: Run the tests**

Run: `go test ./ui/ -run TestInfoPane -v`
Expected: all pass

**Step 3: Commit**

```bash
git add -A && git commit -m "feat: implement info pane renderer with plan/instance/wave sections"
```

---

### Task 8: Wire InfoPane data from app layer

**Files:**
- Modify: `app/app_state.go`
- Modify: `app/app.go`

**Step 1: Add `updateInfoPane` method to `app/app_state.go`**

This method builds `InfoData` from the selected instance, plan state, and wave orchestrator, then calls `SetData` on the info pane:

```go
// updateInfoPane refreshes the info tab data from the selected instance.
func (m *home) updateInfoPane() {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		m.tabbedWindow.SetInfoData(ui.InfoData{HasInstance: false})
		return
	}

	data := ui.InfoData{
		HasInstance: true,
		Title:      selected.Title,
		Program:    selected.Program,
		Branch:     selected.Branch,
		Path:       selected.Path,
		Status:     statusString(selected.Status),
		AgentType:  selected.AgentType,
		TaskNumber: selected.TaskNumber,
		WaveNumber: selected.WaveNumber,
	}

	if !selected.CreatedAt.IsZero() {
		data.Created = selected.CreatedAt.Format("2006-01-02 15:04")
	}

	if selected.PlanFile != "" && m.planState != nil {
		entry, ok := m.planState.Entry(selected.PlanFile)
		if ok {
			data.HasPlan = true
			data.PlanName = planstate.DisplayName(selected.PlanFile)
			data.PlanDescription = entry.Description
			data.PlanStatus = string(entry.Status)
			data.PlanTopic = entry.Topic
			data.PlanBranch = entry.Branch
			if !entry.CreatedAt.IsZero() {
				data.PlanCreated = entry.CreatedAt.Format("2006-01-02")
			}
		}

		if orch, ok := m.waveOrchestrators[selected.PlanFile]; ok {
			data.TotalWaves = orch.TotalWaves()
			data.TotalTasks = orch.TotalTasks()
			tasks := orch.CurrentWaveTasks()
			data.WaveTasks = make([]ui.WaveTaskInfo, len(tasks))
			for i, task := range tasks {
				state := "pending"
				if orch.IsTaskComplete(task.Number) {
					state = "complete"
				} else if orch.IsTaskFailed(task.Number) {
					state = "failed"
				} else if orch.IsTaskRunning(task.Number) {
					state = "running"
				}
				data.WaveTasks[i] = ui.WaveTaskInfo{
					Number: task.Number,
					State:  state,
				}
			}
		}
	}

	m.tabbedWindow.SetInfoData(data)
}
```

Add a helper `statusString`:

```go
func statusString(s session.Status) string {
	switch s {
	case session.Running:
		return "running"
	case session.Ready:
		return "ready"
	case session.Loading:
		return "loading"
	case session.Paused:
		return "paused"
	default:
		return "unknown"
	}
}
```

**Step 2: Add `SetInfoData` to TabbedWindow**

In `ui/tabbed_window.go`:

```go
// SetInfoData updates the info pane with new data.
func (w *TabbedWindow) SetInfoData(data InfoData) {
	w.info.SetData(data)
}
```

**Step 3: Call `updateInfoPane` from `instanceChanged()`**

In `app/app_state.go`, in the `instanceChanged()` function, add a call to `m.updateInfoPane()` after the diff and preview updates.

Also call it from the metadata tick handler in `app/app.go` so the info pane stays fresh when wave progress changes.

**Step 4: Verify the project compiles and tests pass**

Run: `go build ./... && go test ./...`

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: wire info pane data from app layer on instance change"
```

---

## Wave 3: Documentation and Cleanup

### Task 9: Update documentation and comments

**Files:**
- Modify: `app/help.go`
- Modify: `app/app_input.go` (comments)
- Modify: `app/app_state.go` (comments)
- Modify: `CLAUDE.md` (if lazygit referenced)

**Step 1: Search for all remaining lazygit/git-tab references in code comments**

Run: `rg -i 'lazygit|git.tab|git tab' --glob '*.go'`

Fix every remaining reference:
- Comments saying "lazygit" → remove or update to "info tab"
- Comments saying "git tab" → "info tab"
- `slotGit` comment → "info tab" (or rename to `slotInfo`)

**Step 2: Rename focus slot constant for clarity**

In `app/app_state.go`, rename `slotGit` to `slotInfo`:

```go
const (
	slotSidebar = 0
	slotAgent   = 1
	slotDiff    = 2
	slotInfo    = 3
	slotList    = 4
	slotCount   = 5
)
```

Update all references across `app/app_input.go`, `app/app_state.go`, and any other files.

**Step 3: Search docs for lingering references**

Run: `rg -i 'lazygit|git.tab' --glob '*.md' --glob '!docs/plans/*'`

Update any hits in README, CLAUDE.md, or other user-facing docs.

**Step 4: Run full test suite**

Run: `go test ./...`

**Step 5: Commit**

```bash
git add -A && git commit -m "docs: remove all lazygit references, rename slotGit to slotInfo"
```

---

### Task 10: Remove git focus mode guard

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`

**Step 1: Ensure info tab doesn't support focus/insert mode**

In `app/app_input.go`, in the handler for `keys.KeyFocusMode` (the `i` key), add a guard:

```go
// Info tab is read-only — don't enter focus mode
if m.focusSlot == slotInfo {
    return m, nil
}
```

Or, if the existing focus mode entry already gates on the agent tab, verify that `slotInfo` doesn't accidentally trigger focus mode. Trace the `i` key handler and confirm.

**Step 2: Remove `enterGitFocusMode` references**

Search for any remaining calls to `enterGitFocusMode` and delete them.

**Step 3: Run tests**

Run: `go test ./...`

**Step 4: Commit**

```bash
git add -A && git commit -m "fix: prevent focus mode entry on info tab"
```
