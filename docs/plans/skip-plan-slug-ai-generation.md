# Skip Plan Slug AI Generation

**Goal:** When a multiline description's first line is already a viable slug (≤6 words, no truncation needed), use it directly as the plan name instead of calling the AI title derivation.

**Architecture:** Add a `isViableSlug(description)` predicate to `plan_title.go` that checks whether the description is multiline and the first line survives `heuristicPlanTitle` without truncation. In `app_input.go`, call this predicate before entering `stateNewPlanDeriving` — if viable, skip straight to `stateNewPlanTopic` with the heuristic title.

**Tech Stack:** Go, bubbletea (tea.Cmd flow), existing `heuristicPlanTitle` + `splitWords` helpers

**Size:** Trivial (estimated ~20 min, 1 task, 1 wave)

---

## Wave 1: Skip AI When First Line Is Viable Slug

### Task 1: Add viable-slug predicate and short-circuit AI call

**Files:**
- Modify: `app/plan_title.go`
- Modify: `app/plan_title_test.go`
- Modify: `app/app_input.go`
- Modify: `app/app_plan_creation_test.go`

**Step 1: write the failing tests**

Add tests to `app/plan_title_test.go` for the new `firstLineIsViableSlug` predicate:

```go
func TestFirstLineIsViableSlug_MultilineShortFirstLine(t *testing.T) {
	desc := "fix auth token refresh\ndetails about the bug and how to reproduce it"
	assert.True(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_SingleLine(t *testing.T) {
	// Single-line descriptions should NOT be considered viable — they need AI
	desc := "fix auth token refresh"
	assert.False(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_MultilineLongFirstLine(t *testing.T) {
	// First line is too long (>6 words after filler strip) — needs AI
	desc := "refactor the entire authentication subsystem to use JWT tokens\nmore details here"
	assert.False(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_MultilineWithFiller(t *testing.T) {
	// First line has filler prefix but after stripping is ≤6 words
	desc := "i want to fix auth refresh\ndetails about the bug"
	assert.True(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_EmptyFirstLine(t *testing.T) {
	desc := "\nactual description here"
	assert.False(t, firstLineIsViableSlug(desc))
}
```

Add a test to `app/app_plan_creation_test.go` verifying the submit path skips deriving:

```go
func TestNewPlanSubmitSkipsAIWhenFirstLineIsViableSlug(t *testing.T) {
	h := &home{
		state:            stateNewPlan,
		textInputOverlay: overlay.NewTextInputOverlay("new plan", "fix auth refresh\ndetails about the bug"),
	}
	h.textInputOverlay.SetMultiline(true)

	// Tab to submit button, then Enter
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	updated, ok := model.(*home)
	require.True(t, ok)
	// Should skip stateNewPlanDeriving and go straight to topic picker
	require.Equal(t, stateNewPlanTopic, updated.state)
	require.Equal(t, "fix auth refresh", updated.pendingPlanName)
	// No AI command should be returned
	require.Nil(t, cmd)
}
```

**Step 2: run tests to verify they fail**

```bash
go test ./app/... -run 'TestFirstLineIsViableSlug|TestNewPlanSubmitSkipsAI' -v
```

expected: FAIL — `firstLineIsViableSlug` undefined, skip logic not yet implemented

**Step 3: write minimal implementation**

In `app/plan_title.go`, add the predicate:

```go
// firstLineIsViableSlug returns true when the description is multiline and the
// first line (after filler-stripping) is short enough to use as a slug without
// truncation — meaning heuristicPlanTitle would return it verbatim.
func firstLineIsViableSlug(description string) bool {
	text := strings.TrimSpace(description)
	if text == "" {
		return false
	}

	// Must be multiline — single-line descriptions benefit from AI summarization
	idx := strings.IndexByte(text, '\n')
	if idx < 0 {
		return false
	}

	firstLine := strings.TrimSpace(text[:idx])
	if firstLine == "" {
		return false
	}

	// Strip filler prefixes (same logic as heuristicPlanTitle)
	lower := strings.ToLower(firstLine)
	fillers := []string{
		"i want to ", "i'd like to ", "we need to ", "we should ",
		"please ", "let's ", "let us ", "can you ", "could you ",
	}
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			firstLine = firstLine[len(f):]
			break
		}
	}
	firstLine = strings.TrimSpace(firstLine)
	if firstLine == "" {
		return false
	}

	// Viable if ≤6 words (no truncation needed)
	return len(splitWords(firstLine)) <= 6
}
```

In `app/app_input.go`, modify the `stateNewPlan` submit handler (around line 755-764) to short-circuit when the first line is a viable slug:

```go
// Set heuristic title as fallback; AI title will replace it when it arrives
m.pendingPlanName = heuristicPlanTitle(description)
m.pendingPlanDesc = description
m.textInputOverlay = nil

// If the first line is already a viable slug, skip AI derivation
if firstLineIsViableSlug(description) {
	topicNames := m.getTopicNames()
	topicNames = append([]string{"(No topic)"}, topicNames...)
	pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
	m.pickerOverlay = overlay.NewPickerOverlay(pickerTitle, topicNames)
	m.pickerOverlay.SetAllowCustom(true)
	m.state = stateNewPlanTopic
	return m, nil
}

m.state = stateNewPlanDeriving
if m.toastManager != nil {
	m.toastManager.Info("deriving title...")
	return m, tea.Batch(aiDerivePlanTitleCmd(description), m.toastTickCmd())
}
return m, aiDerivePlanTitleCmd(description)
```

**Step 4: run tests to verify they pass**

```bash
go test ./app/... -run 'TestFirstLineIsViableSlug|TestNewPlanSubmitSkipsAI|TestNewPlanSubmit|TestDerivingState|TestHeuristicPlanTitle' -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/plan_title.go app/plan_title_test.go app/app_input.go app/app_plan_creation_test.go
git commit -m "fix: skip AI title derivation when first line is already a viable slug"
```
