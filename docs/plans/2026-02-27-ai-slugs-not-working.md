# Fix AI Slug Derivation Race Condition

**Goal:** Ensure AI-derived plan titles are used for slugs instead of the truncated-first-line heuristic, by waiting for the AI response before opening the topic picker.

**Architecture:** Add a `stateNewPlanDeriving` intermediate state between description submit and topic picker. On description submit, show a toast ("deriving title...") and fire the existing `aiDerivePlanTitleCmd`. When the `planTitleMsg` arrives (success or error), set `pendingPlanName` to the AI title (or heuristic fallback on error) and transition to `stateNewPlanTopic` with the picker. The existing `planTitleMsg` handler for `stateNewPlanTopic` becomes a dead-code safety net. Also improve the heuristic fallback to use punctuation-aware truncation for single-line inputs.

**Tech Stack:** Go, bubbletea, existing `overlay` package, `plan_title.go`

**Size:** Small (estimated ~1 hour, 1 task, 1 wave)

---

## Wave 1

### Task 1: Add `stateNewPlanDeriving` intermediate state and improve heuristic

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/plan_title.go`
- Modify: `app/app_plan_creation_test.go`
- Modify: `app/plan_title_test.go`

**Step 1: write the failing tests**

Add tests for the new intermediate state, improved heuristic, and update existing tests that assert the old flow:

```go
// In app/plan_title_test.go — add tests for single-line heuristic improvement

func TestHeuristicPlanTitle_SingleLineLong_UsesNaturalBreak(t *testing.T) {
	desc := "implement a custom verification process, including static analysis and reality checks for all plans"
	got := heuristicPlanTitle(desc)
	assert.Equal(t, "implement a custom verification process", got)
}

func TestHeuristicPlanTitle_SingleLineNoPunctuation_Truncates(t *testing.T) {
	desc := "refactor the entire authentication subsystem to use JSON web tokens instead of session cookies"
	got := heuristicPlanTitle(desc)
	words := len(splitWords(got))
	assert.LessOrEqual(t, words, 6)
}
```

```go
// In app/app_plan_creation_test.go — new tests for deriving state

func TestNewPlanSubmitEntersDerivingState(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "refactor auth module"),
	}
	h.textInputOverlay.SetMultiline(true)

	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)
	require.Equal(t, "refactor auth module", updated.pendingPlanDesc)
	require.NotEmpty(t, updated.pendingPlanName)
	require.NotNil(t, cmd)
}

func TestDerivingStateTransitionsToTopicOnAITitle(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "heuristic-fallback",
		pendingPlanDesc: "some description",
	}

	model, _ := h.Update(planTitleMsg{title: "ai derived title"})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "ai derived title", updated.pendingPlanName)
	require.NotNil(t, updated.pickerOverlay)
}

func TestDerivingStateFallsBackOnAIError(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "heuristic fallback title",
		pendingPlanDesc: "some description",
	}

	model, _ := h.Update(planTitleMsg{err: fmt.Errorf("timeout")})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "heuristic fallback title", updated.pendingPlanName)
	require.NotNil(t, updated.pickerOverlay)
}

func TestDerivingStateBlocksKeyInput(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "test",
		pendingPlanDesc: "test desc",
	}

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateNewPlanDeriving, updated.state)
	require.Nil(t, cmd)
}

func TestDerivingStateEscapeCancels(t *testing.T) {
	h := &home{
		state:           stateNewPlanDeriving,
		pendingPlanName: "test",
		pendingPlanDesc: "test desc",
	}

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEscape})

	updated, ok := model.(*home)
	require.True(t, ok)
	require.Equal(t, stateDefault, updated.state)
	require.Empty(t, updated.pendingPlanName)
	require.Empty(t, updated.pendingPlanDesc)
}
```

Also update existing tests:

- `TestNewPlanSubmitShowsTopicPicker` — assert `stateNewPlanDeriving` instead of `stateNewPlanTopic`
- `TestNewPlanTopicPickerShowsPendingPlanName` — set up from `stateNewPlanDeriving` + send `planTitleMsg` to reach topic picker

**Step 2: run tests to verify they fail**

```bash
go test ./app/... -run "TestNewPlanSubmitEntersDerivingState|TestDerivingState|TestHeuristicPlanTitle_SingleLine|TestNewPlanSubmitShowsTopicPicker|TestNewPlanTopicPickerShowsPendingPlanName" -v
```

Expected: FAIL — `stateNewPlanDeriving` undefined, assertions don't match current flow.

**Step 3: write minimal implementation**

**3a. Add `stateNewPlanDeriving` to the state enum** in `app/app.go` (between `stateNewPlan` and `stateNewPlanTopic`):

```go
// stateNewPlanDeriving is the state when the AI is deriving a plan title.
stateNewPlanDeriving
```

Add it to the overlay-blocking guard on line 28 of `app/app_input.go`.

**3b. Change `stateNewPlan` submit handler** in `app/app_input.go` (~line 748-770):

Instead of opening the topic picker immediately, enter the deriving state:

```go
if m.textInputOverlay.IsSubmitted() {
    description := strings.TrimSpace(m.textInputOverlay.GetValue())
    if description == "" {
        m.state = stateDefault
        m.menu.SetState(ui.StateDefault)
        m.textInputOverlay = nil
        return m, m.handleError(fmt.Errorf("description cannot be empty"))
    }
    m.pendingPlanName = heuristicPlanTitle(description)
    m.pendingPlanDesc = description
    m.textInputOverlay = nil
    m.state = stateNewPlanDeriving
    m.toastManager.Info("deriving title...")
    return m, tea.Batch(aiDerivePlanTitleCmd(description), m.toastTickCmd())
}
```

**3c. Add deriving state key handler** in `app/app_input.go` (before the `stateNewPlanTopic` block):

```go
if m.state == stateNewPlanDeriving {
    if msg.Type == tea.KeyEscape {
        m.state = stateDefault
        m.pendingPlanName = ""
        m.pendingPlanDesc = ""
        return m, nil
    }
    return m, nil
}
```

**3d. Update `planTitleMsg` handler** in `app/app.go` (~line 1274):

Add the `stateNewPlanDeriving` case that opens the topic picker:

```go
case planTitleMsg:
    if m.state == stateNewPlanDeriving {
        if msg.err == nil && msg.title != "" {
            m.pendingPlanName = msg.title
        }
        topicNames := m.getTopicNames()
        topicNames = append([]string{"(No topic)"}, topicNames...)
        pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
        m.pickerOverlay = overlay.NewPickerOverlay(pickerTitle, topicNames)
        m.pickerOverlay.SetAllowCustom(true)
        m.state = stateNewPlanTopic
        return m, nil
    }
    // Safety net: if title arrives while already in topic picker, update silently
    if msg.err == nil && msg.title != "" {
        if m.state == stateNewPlanTopic && m.pendingPlanDesc != "" {
            m.pendingPlanName = msg.title
            if m.pickerOverlay != nil {
                m.pickerOverlay.SetTitle(
                    fmt.Sprintf("assign to topic for '%s'", msg.title),
                )
                return m, tea.WindowSize()
            }
        }
    }
    return m, nil
```

**3e. Improve `heuristicPlanTitle`** in `app/plan_title.go`:

Replace the fixed 8-word truncation with punctuation-aware breaking:

```go
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

    words := splitWords(text)
    if len(words) <= 6 {
        return strings.Join(words, " ")
    }

    // Look for a natural break within first 8 words
    limit := min(8, len(words))
    first8 := strings.Join(words[:limit], " ")
    for _, sep := range []string{", ", "; ", ": ", ". ", " - "} {
        if idx := strings.Index(first8, sep); idx > 0 {
            candidate := strings.TrimSpace(first8[:idx])
            if len(splitWords(candidate)) >= 3 {
                return candidate
            }
        }
    }

    // No natural break — truncate to 6 words
    return strings.Join(words[:6], " ")
}
```

**Step 4: run tests to verify they pass**

```bash
go test ./app/... -v -count=1
```

Expected: PASS — all new and updated tests green.

**Step 5: commit**

```bash
git add app/app.go app/app_input.go app/plan_title.go app/app_plan_creation_test.go app/plan_title_test.go
git commit -m "fix: wait for AI title before topic picker to prevent truncated slugs"
```
