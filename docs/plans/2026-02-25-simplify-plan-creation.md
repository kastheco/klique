# Simplify Plan Creation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the sequential name → description text input overlays with a single huh form popup.

**Architecture:** New `FormOverlay` component wraps an embedded `huh.Form` with two `huh.Input` fields. Custom key interception ensures Enter submits from either field, and arrow keys navigate between fields. A `ThemeRosePine()` function provides huh theme matching the app's palette. The app state machine collapses two states into one.

**Tech Stack:** Go, huh v0.8.0 (already a dependency), bubbletea, lipgloss

---

### Task 1: ThemeRosePine huh theme

**Files:**
- Modify: `ui/overlay/theme.go`
- Test: manual visual verification (theme is purely cosmetic)

**Step 1: Write ThemeRosePine function**

Add to `ui/overlay/theme.go` after the existing color vars:

```go
import "github.com/charmbracelet/huh"

// ThemeRosePine returns a huh theme matching the app's Rosé Pine Moon palette.
func ThemeRosePine() *huh.Theme {
	t := huh.ThemeBase()

	t.Focused.Base = t.Focused.Base.BorderForeground(colorIris)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(colorIris).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(colorIris).Bold(true).MarginBottom(1)
	t.Focused.Description = t.Focused.Description.Foreground(colorMuted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(colorLove)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(colorLove)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorIris)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(colorIris)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(colorIris)
	t.Focused.Option = t.Focused.Option.Foreground(colorText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(colorIris)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorFoam)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(colorFoam).SetString("✓ ")
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(colorMuted).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(colorText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(colorBase).Background(colorIris)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(colorSubtle).Background(colorOverlay)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorFoam)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(colorMuted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorIris)
	t.Focused.TextInput.Text = t.Focused.TextInput.Text.Foreground(colorText)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	return t
}
```

**Step 2: Verify it compiles**

Run: `go build ./ui/overlay/...`
Expected: clean build

**Step 3: Commit**

```bash
git add ui/overlay/theme.go
git commit -m "feat: add ThemeRosePine for huh forms"
```

---

### Task 2: FormOverlay component

**Files:**
- Create: `ui/overlay/formOverlay.go`
- Test: `ui/overlay/formOverlay_test.go`

**Step 1: Write the failing test**

Create `ui/overlay/formOverlay_test.go`:

```go
package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormOverlay_SubmitWithName(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	// Type a name
	for _, r := range "auth-refactor" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Enter submits
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "auth-refactor", f.Name())
	assert.Equal(t, "", f.Description())
}

func TestFormOverlay_SubmitWithNameAndDescription(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	// Type a name
	for _, r := range "auth" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Tab to description
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	// Type description
	for _, r := range "refactor jwt" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Enter submits from description field
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "auth", f.Name())
	assert.Equal(t, "refactor jwt", f.Description())
}

func TestFormOverlay_EmptyNameDoesNotSubmit(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	// Enter with empty name should not close
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, closed)
	assert.False(t, f.IsSubmitted())
}

func TestFormOverlay_EscCancels(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, f.IsSubmitted())
}

func TestFormOverlay_ArrowDownNavigates(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	// Type name
	for _, r := range "test" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Arrow down to description
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	// Type description
	for _, r := range "desc" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Submit
	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, closed)
	assert.Equal(t, "test", f.Name())
	assert.Equal(t, "desc", f.Description())
}

func TestFormOverlay_Render(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	output := f.Render()
	assert.NotEmpty(t, output)
	// Should contain the title
	assert.Contains(t, output, "new plan")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./ui/overlay/ -run TestFormOverlay -v`
Expected: FAIL — formOverlay.go doesn't exist yet

**Step 3: Write FormOverlay implementation**

Create `ui/overlay/formOverlay.go`:

```go
package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// FormOverlay is a multi-field form overlay backed by a huh.Form.
// Used for the plan creation popup (name + description).
type FormOverlay struct {
	form      *huh.Form
	nameVal   string
	descVal   string
	title     string
	submitted bool
	canceled  bool
	width     int
}

// NewFormOverlay creates a form overlay with name and description inputs.
func NewFormOverlay(title string, width int) *FormOverlay {
	f := &FormOverlay{
		title: title,
		width: width,
	}

	f.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("name").
				Value(&f.nameVal),
			huh.NewInput().
				Key("desc").
				Title("description (optional)").
				Value(&f.descVal),
		),
	).
		WithTheme(ThemeRosePine()).
		WithWidth(width - 6). // account for border + padding
		WithShowHelp(false).
		WithShowErrors(false)

	// Initialize the form so it's ready for Update calls.
	f.form.Init()

	return f
}

// HandleKeyPress processes a key and returns true if the overlay should close.
func (f *FormOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		f.canceled = true
		return true

	case tea.KeyEnter:
		if strings.TrimSpace(f.nameVal) == "" {
			return false // name required
		}
		f.submitted = true
		return true

	case tea.KeyDown:
		// Translate arrow down to tab for huh field navigation
		tabMsg := tea.KeyMsg{Type: tea.KeyTab}
		f.form.Update(tabMsg)
		return false

	case tea.KeyUp:
		// Translate arrow up to shift-tab for huh field navigation
		stMsg := tea.KeyMsg{Type: tea.KeyShiftTab}
		f.form.Update(stMsg)
		return false

	default:
		f.form.Update(msg)
		return false
	}
}

// Render returns the styled overlay string.
func (f *FormOverlay) Render() string {
	w := f.width
	if w < 40 {
		w = 40
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(colorIris).
		Bold(true).
		MarginBottom(1)

	hintStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		MarginTop(1)

	content := titleStyle.Render(f.title) + "\n"
	content += f.form.View() + "\n"
	content += hintStyle.Render("tab/↑↓ navigate · enter create")

	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorIris).
		Padding(1, 2).
		Width(w)

	return style.Render(content)
}

// Name returns the name field value.
func (f *FormOverlay) Name() string {
	return strings.TrimSpace(f.nameVal)
}

// Description returns the description field value.
func (f *FormOverlay) Description() string {
	return strings.TrimSpace(f.descVal)
}

// IsSubmitted returns true if the form was submitted (not canceled).
func (f *FormOverlay) IsSubmitted() bool {
	return f.submitted
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/overlay/ -run TestFormOverlay -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add ui/overlay/formOverlay.go ui/overlay/formOverlay_test.go
git commit -m "feat: add FormOverlay component backed by huh"
```

---

### Task 3: Wire FormOverlay into app state machine

**Files:**
- Modify: `app/app.go:59-62` (state enum)
- Modify: `app/app.go:138,151-154` (struct fields)
- Modify: `app/app.go:1086-1089` (View switch)
- Modify: `app/app_input.go:25` (menu highlighting guard)
- Modify: `app/app_input.go:665-716` (collapse two handlers)
- Modify: `app/app_input.go:718-753` (topic picker reads from formOverlay)
- Modify: `app/app_input.go:1290-1293` (trigger)

**Step 1: Update state enum in `app/app.go`**

Replace lines 59-64:

```go
	// stateNewPlan is the state when the user is creating a new plan (name + description form).
	stateNewPlan
	// stateNewPlanTopic is the state when the user is picking a topic for a new plan.
	stateNewPlanTopic
```

This removes `stateNewPlanName` and `stateNewPlanDescription`, replacing them with a single `stateNewPlan`.

**Step 2: Update struct fields in `app/app.go`**

In the `home` struct, replace `textInputOverlay` field (line 138) — keep it, it's used by other overlays. Add a new field after it:

```go
	// formOverlay handles multi-field form input (plan creation)
	formOverlay *overlay.FormOverlay
```

Remove `pendingPlanName` and `pendingPlanDesc` fields (lines 151-154). These values now live inside the FormOverlay.

**Step 3: Update View switch in `app/app.go`**

Replace the two plan overlay cases (lines 1086-1089):

```go
	case m.state == stateNewPlan && m.formOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.formOverlay.Render(), mainView, true, true)
```

**Step 4: Update menu highlighting guard in `app/app_input.go`**

On line 25, replace `stateNewPlanName || m.state == stateNewPlanDescription` with `stateNewPlan`.

**Step 5: Collapse plan creation handlers in `app/app_input.go`**

Replace the `stateNewPlanName` handler (lines 665-693) and `stateNewPlanDescription` handler (lines 695-716) with a single `stateNewPlan` handler:

```go
	// Handle new plan form state
	if m.state == stateNewPlan {
		if m.formOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.formOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.formOverlay.IsSubmitted() {
				name := m.formOverlay.Name()
				if name == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.formOverlay = nil
					return m, m.handleError(fmt.Errorf("plan name cannot be empty"))
				}
				// Stash values and show topic picker
				m.pendingPlanName = name
				m.pendingPlanDesc = m.formOverlay.Description()
				m.formOverlay = nil
				topicNames := m.getTopicNames()
				topicNames = append([]string{"(No topic)"}, topicNames...)
				m.pickerOverlay = overlay.NewPickerOverlay("assign to topic (optional)", topicNames)
				m.pickerOverlay.SetAllowCustom(true)
				m.state = stateNewPlanTopic
				return m, nil
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.formOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}
```

Note: We still need `pendingPlanName` and `pendingPlanDesc` to pass values from the form overlay to the topic picker step. Keep these fields in the struct but update their comments to reflect the new flow.

**Step 6: Update trigger in `app/app_input.go`**

Replace the plan creation trigger (lines 1290-1294):

```go
	case keys.KeyNewPlan:
		m.state = stateNewPlan
		m.formOverlay = overlay.NewFormOverlay("new plan", 60)
		return m, nil
```

**Step 7: Verify it compiles**

Run: `go build ./app/...`
Expected: clean build

**Step 8: Commit**

```bash
git add app/app.go app/app_input.go
git commit -m "feat: wire FormOverlay into plan creation flow"
```

---

### Task 4: Update tests

**Files:**
- Modify: `app/app_plan_creation_test.go`

**Step 1: Verify existing tests still pass**

Run: `go test ./app/ -run TestBuildPlanFilename -v && go test ./app/ -run TestRenderPlanStub -v && go test ./app/ -run TestCreatePlanRecord -v`
Expected: all PASS (these tests don't touch the overlay flow)

**Step 2: Run full test suite to catch breakage**

Run: `go test ./... 2>&1 | head -80`
Expected: identify any failures from the state enum renaming

**Step 3: Fix any compilation or test failures**

Search for any remaining references to the old state names:

```bash
rg 'stateNewPlanName|stateNewPlanDescription' --type go
```

Fix any found references. Common locations: test files, switch statements, string comparisons.

**Step 4: Run full test suite again**

Run: `go test ./...`
Expected: all PASS

**Step 5: Commit**

```bash
git add -A
git commit -m "test: fix tests for simplified plan creation flow"
```

---

### Task 5: Clean up dead code

**Files:**
- Modify: `app/app.go` (remove unused pendingPlan fields if truly unused)
- Modify: `app/app_input.go` (remove any leftover dead branches)

**Step 1: Search for dead references**

```bash
rg 'pendingPlanName|pendingPlanDesc' --type go
```

Verify these are only used in the `stateNewPlan` handler and `stateNewPlanTopic` handler. If `pendingPlanName`/`pendingPlanDesc` are still needed for the topic step handoff, keep them but update comments.

**Step 2: Update field comments**

Change the comments on `pendingPlanName`/`pendingPlanDesc` from "three-step" to "two-step":

```go
	// pendingPlanName stores the plan name during the two-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the two-step plan creation flow
	pendingPlanDesc string
```

**Step 3: Run tests**

Run: `go test ./...`
Expected: all PASS

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor: clean up dead code from plan creation simplification"
```
