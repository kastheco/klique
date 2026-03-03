# Simplify New Plan Creation

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the two-field form overlay (name + description) with a single multiline textarea ("describe what you want to work on..."), derive the plan title via AI (with heuristic fallback), and keep the topic picker as step 2.

**Architecture:** The `stateNewPlan` handler switches from `FormOverlay` to `TextInputOverlay` configured in multiline mode. On submit, a heuristic title is computed immediately and an async `tea.Cmd` shells out to `claude` for an AI-derived title. The topic picker opens instantly with the heuristic title. If the AI title arrives before the user submits the picker, `pendingPlanName` is silently upgraded. The plan is created with whichever title is best available at topic picker submission.

**Tech Stack:** Go, bubbletea, bubbles/textarea, lipgloss, exec.Command (claude CLI)

**Size:** Small (estimated ~1.5 hours, 3 tasks, no waves)

---

### Task 1: Add multiline mode and placeholder to TextInputOverlay

**Files:**
- Modify: `ui/overlay/textInput.go`
- Test: `ui/overlay/textInput_test.go` (create)

**Step 1: Write the failing tests**

Create `ui/overlay/textInput_test.go`:

```go
package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestTextInputOverlay_DefaultEnterSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEnterInsertsNewline(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Enter when textarea is focused should NOT submit in multiline mode
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, closed)
	assert.False(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEnterOnButtonSubmits(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	// Tab to button
	ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 1, ti.FocusIndex)
	// Enter on button submits
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, ti.IsSubmitted())
}

func TestTextInputOverlay_MultilineEscCancels(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetMultiline(true)
	closed := ti.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.True(t, ti.Canceled)
}

func TestTextInputOverlay_SetPlaceholder(t *testing.T) {
	ti := NewTextInputOverlay("title", "")
	ti.SetPlaceholder("describe what you want to work on...")
	assert.Contains(t, ti.Render(), "describe what you want to work on")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/overlay/ -run TestTextInputOverlay -v`
Expected: FAIL — `SetMultiline` and `SetPlaceholder` don't exist yet

**Step 3: Implement multiline mode and placeholder**

In `ui/overlay/textInput.go`, add the `multiline` field and methods:

```go
// Add field to TextInputOverlay struct:
	multiline bool

// SetMultiline enables multiline mode where Enter inserts newlines
// and the user must Tab to the submit button then press Enter to submit.
func (t *TextInputOverlay) SetMultiline(enabled bool) {
	t.multiline = enabled
}

// SetPlaceholder sets the textarea placeholder text.
func (t *TextInputOverlay) SetPlaceholder(text string) {
	t.textarea.Placeholder = text
}
```

Then modify `HandleKeyPress` — change the `tea.KeyEnter` case:

```go
	case tea.KeyEnter:
		if t.multiline && t.FocusIndex == 0 {
			// In multiline mode, Enter inserts a newline when textarea is focused
			t.textarea, _ = t.textarea.Update(msg)
			return false
		}
		// Submit (non-multiline, or button is focused in multiline)
		t.Submitted = true
		if t.OnSubmit != nil {
			t.OnSubmit()
		}
		return true
```

Also update the `Render` method to show different hints in multiline mode:

```go
	// After the existing button rendering, change the hint:
	if t.multiline {
		content += "  " + lipgloss.NewStyle().Foreground(colorMuted).Render("tab → enter submit · esc cancel")
	}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/overlay/ -run TestTextInputOverlay -v`
Expected: all PASS

**Step 5: Run full overlay test suite**

Run: `go test ./ui/overlay/ -v`
Expected: all PASS (backward compatible — default behavior unchanged)

**Step 6: Commit**

```bash
git add ui/overlay/textInput.go ui/overlay/textInput_test.go
git commit -m "feat: add multiline mode and placeholder to TextInputOverlay"
```

---

### Task 2: Title derivation — heuristic function, AI command, and message types

**Files:**
- Create: `app/plan_title.go`
- Test: `app/plan_title_test.go` (create)

**Step 1: Write the failing tests**

Create `app/plan_title_test.go`:

```go
package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeuristicPlanTitle_FirstLine(t *testing.T) {
	desc := "refactor auth to use JWT tokens\nThis needs to handle refresh tokens too"
	got := heuristicPlanTitle(desc)
	assert.Equal(t, "refactor auth to use JWT tokens", got)
}

func TestHeuristicPlanTitle_StripsFiller(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"i want to refactor the auth module", "refactor the auth module"},
		{"we need to add dark mode support", "add dark mode support"},
		{"please add search functionality", "add search functionality"},
		{"let's build a new dashboard", "build a new dashboard"},
		{"can you implement caching", "implement caching"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := heuristicPlanTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHeuristicPlanTitle_TruncatesLongInput(t *testing.T) {
	desc := "refactor the entire authentication subsystem to use JSON web tokens instead of session cookies across all microservices"
	got := heuristicPlanTitle(desc)
	// Should be at most 8 words
	words := len(splitWords(got))
	assert.LessOrEqual(t, words, 8)
}

func TestHeuristicPlanTitle_EmptyInput(t *testing.T) {
	got := heuristicPlanTitle("")
	assert.Equal(t, "new plan", got)
}

func TestHeuristicPlanTitle_WhitespaceOnly(t *testing.T) {
	got := heuristicPlanTitle("   \n\n  ")
	assert.Equal(t, "new plan", got)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/ -run TestHeuristicPlanTitle -v`
Expected: FAIL — `heuristicPlanTitle` doesn't exist

**Step 3: Implement title derivation**

Create `app/plan_title.go`:

```go
package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// planTitleMsg is sent when the async AI title derivation completes.
type planTitleMsg struct {
	title string
	err   error
}

// heuristicPlanTitle derives a short title from a plan description.
// Takes the first line, strips common filler prefixes, and truncates to 8 words.
func heuristicPlanTitle(description string) string {
	text := strings.TrimSpace(description)
	if text == "" {
		return "new plan"
	}

	// Take first line only
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	if text == "" {
		return "new plan"
	}

	// Strip common filler prefixes (case-insensitive)
	lower := strings.ToLower(text)
	fillers := []string{
		"i want to ", "i'd like to ", "we need to ", "we should ",
		"please ", "let's ", "let us ", "can you ", "could you ",
	}
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			text = text[len(f):]
			break
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "new plan"
	}

	// Truncate to 8 words
	words := splitWords(text)
	if len(words) > 8 {
		words = words[:8]
	}
	return strings.Join(words, " ")
}

// splitWords splits text on whitespace, returning non-empty tokens.
func splitWords(s string) []string {
	raw := strings.Fields(s)
	out := make([]string, 0, len(raw))
	for _, w := range raw {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// aiDerivePlanTitleCmd returns a tea.Cmd that shells out to claude to derive
// a concise plan title from the given description. Returns planTitleMsg.
func aiDerivePlanTitleCmd(description string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		prompt := fmt.Sprintf(
			"Generate a concise 3-5 word title for this software task. "+
				"Respond with ONLY the title in lowercase, nothing else. No quotes, no punctuation.\n\n%s",
			description,
		)

		cmd := exec.CommandContext(ctx, "claude",
			"-p", prompt,
			"--model", "claude-sonnet-4-20250514",
			"--output-format", "text",
		)
		out, err := cmd.Output()
		if err != nil {
			return planTitleMsg{err: err}
		}

		title := strings.TrimSpace(string(out))
		if title == "" {
			return planTitleMsg{err: fmt.Errorf("empty AI response")}
		}
		return planTitleMsg{title: title}
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./app/ -run TestHeuristicPlanTitle -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add app/plan_title.go app/plan_title_test.go
git commit -m "feat: add heuristic and AI plan title derivation"
```

---

### Task 3: Wire new plan flow — textarea overlay + AI title + topic picker

**Files:**
- Modify: `app/app.go` (msg handler, View switch, comment update)
- Modify: `app/app_input.go` (stateNewPlan handler, trigger)
- Modify: `ui/overlay/pickerOverlay.go` (add SetTitle method)
- Modify: `app/app_plan_creation_test.go` (update tests for new flow)

**Step 1: Add SetTitle to PickerOverlay**

In `ui/overlay/pickerOverlay.go`, add:

```go
// SetTitle updates the picker's title text (used when AI title arrives async).
func (p *PickerOverlay) SetTitle(title string) {
	p.title = title
}
```

**Step 2: Update the trigger in `app/app_input.go`**

Replace the `keys.KeyNewPlan` case (around line 1296-1299):

```go
	case keys.KeyNewPlan:
		m.state = stateNewPlan
		m.textInputOverlay = overlay.NewTextInputOverlay("new plan", "")
		m.textInputOverlay.SetMultiline(true)
		m.textInputOverlay.SetPlaceholder("describe what you want to work on...")
		m.textInputOverlay.SetSize(70, 8)
		return m, nil
```

**Step 3: Replace the stateNewPlan handler in `app/app_input.go`**

Replace the existing `stateNewPlan` block (around lines 664-697) with:

```go
	// Handle new plan description state
	if m.state == stateNewPlan {
		if m.textInputOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				description := strings.TrimSpace(m.textInputOverlay.GetValue())
				if description == "" {
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.textInputOverlay = nil
					return m, m.handleError(fmt.Errorf("description cannot be empty"))
				}
				// Derive heuristic title immediately
				m.pendingPlanName = heuristicPlanTitle(description)
				m.pendingPlanDesc = description
				m.textInputOverlay = nil

				// Show topic picker with heuristic title
				topicNames := m.getTopicNames()
				topicNames = append([]string{"(No topic)"}, topicNames...)
				pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
				m.pickerOverlay = overlay.NewPickerOverlay(pickerTitle, topicNames)
				m.pickerOverlay.SetAllowCustom(true)
				m.state = stateNewPlanTopic

				// Fire async AI title derivation
				return m, aiDerivePlanTitleCmd(description)
			}
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.textInputOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	}
```

**Step 4: Update the View switch in `app/app.go`**

Replace the `stateNewPlan` rendering case (around line 1317-1318):

```go
	case m.state == stateNewPlan && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
```

**Step 5: Add planTitleMsg handler in `app/app.go` Update function**

Add a new case in the msg switch (alongside other msg handlers, e.g. after `plannerCompleteMsg`):

```go
	case planTitleMsg:
		if msg.err == nil && msg.title != "" {
			// Only update if we're still in the topic picker flow
			if m.state == stateNewPlanTopic && m.pendingPlanDesc != "" {
				m.pendingPlanName = msg.title
				if m.pickerOverlay != nil {
					m.pickerOverlay.SetTitle(
						fmt.Sprintf("assign to topic for '%s'", msg.title),
					)
				}
			}
		}
		return m, nil
```

**Step 6: Update tests in `app/app_plan_creation_test.go`**

Replace `TestHandleDefaultStateStartsCombinedPlanForm`:

```go
func TestHandleDefaultStateStartsDescriptionOverlay(t *testing.T) {
	h := &home{
		state:        stateDefault,
		keySent:      true,
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
	}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlan, updated.state)
	require.NotNil(t, updated.textInputOverlay)
}
```

Replace `TestHandleKeyPressNewPlanWithoutOverlayReturnsDefault`:

```go
func TestHandleKeyPressNewPlanWithoutOverlayReturnsDefault(t *testing.T) {
	h := &home{state: stateNewPlan}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Nil(t, cmd)

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
}
```

Replace `TestNewPlanTopicPickerShowsPendingPlanName` — this test typed into a FormOverlay; now it needs to type into a TextInputOverlay. The multiline textarea in bubbles handles character input differently from huh, so the test should verify the end-to-end flow by setting `pendingPlanName` directly and testing the topic picker transition:

```go
func TestNewPlanSubmitShowsTopicPicker(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "refactor auth module"),
	}
	h.textInputOverlay.SetMultiline(true)

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.NotNil(t, updated.pickerOverlay)
	require.NotEmpty(t, updated.pendingPlanName)
	require.Equal(t, "refactor auth module", updated.pendingPlanDesc)
	// cmd should be the AI title derivation command (non-nil)
	require.NotNil(t, cmd)
}
```

**Step 7: Verify all tests pass**

Run: `go test ./app/ -v -count=1 2>&1 | tail -30`
Run: `go test ./ui/overlay/ -v -count=1`
Run: `go test ./... 2>&1 | tail -20`

Fix any failures.

**Step 8: Commit**

```bash
git add app/app.go app/app_input.go app/app_plan_creation_test.go ui/overlay/pickerOverlay.go
git commit -m "feat: simplify new plan to single description textarea with AI title"
```
