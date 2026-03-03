# Cleanup Prompts Implementation Plan

**Goal:** Strip the user-provided slug from the stored plan description when the first line is used as the plan name, preventing the slug from appearing twice in generated prompts (once in `Plan <name>` and again at the start of `Goal: <description>`).

**Architecture:** Add a `stripSlugPrefix` helper in `app/plan_title.go` that removes the first line and leading newlines from a multiline description when `firstLineIsViableSlug` is true. Call it in `app/app_input.go` at the point where `m.pendingPlanDesc` is assigned, so the stored description never contains the redundant slug. All downstream consumers (`buildPlanPrompt`, `buildSoloPrompt`) automatically benefit.

**Tech Stack:** Go, bubbletea (existing app code), testify (tests)

**Size:** Trivial (estimated ~20 min, 1 task, 1 wave)

---

## Wave 1: Fix slug duplication in plan descriptions

### Task 1: Strip slug prefix from description when first line is viable slug

**Files:**
- Modify: `app/plan_title.go`
- Modify: `app/plan_title_test.go`
- Modify: `app/app_input.go`
- Modify: `app/app_plan_actions_test.go`

**Step 1: write the failing test**

Add tests in `app/plan_title_test.go` for a new `stripSlugPrefix` function:

```go
func TestStripSlugPrefix_RemovesFirstLine(t *testing.T) {
	desc := "have-to-kill-twice\nThe planner agent needs to be killed twice"
	got := stripSlugPrefix(desc)
	assert.Equal(t, "The planner agent needs to be killed twice", got)
}

func TestStripSlugPrefix_TrimsLeadingNewlines(t *testing.T) {
	desc := "fix-auth-bug\n\n\nActual description starts here"
	got := stripSlugPrefix(desc)
	assert.Equal(t, "Actual description starts here", got)
}

func TestStripSlugPrefix_SingleLineReturnsAsIs(t *testing.T) {
	desc := "just a single line"
	got := stripSlugPrefix(desc)
	assert.Equal(t, "just a single line", got)
}

func TestStripSlugPrefix_EmptyReturnsEmpty(t *testing.T) {
	got := stripSlugPrefix("")
	assert.Equal(t, "", got)
}
```

Also add an integration-level test in `app/app_plan_actions_test.go`:

```go
func TestBuildPlanPrompt_NoSlugDuplication(t *testing.T) {
	// When the plan name IS the slug and the description still contains it,
	// the prompt should not repeat the slug in the Goal field.
	prompt := buildPlanPrompt("have-to-kill-twice", "The planner agent needs to be killed twice")
	assert.Contains(t, prompt, "Plan have-to-kill-twice")
	assert.Contains(t, prompt, "Goal: The planner agent needs to be killed twice")
	assert.NotContains(t, prompt, "Goal: have-to-kill-twice")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run 'TestStripSlugPrefix|TestBuildPlanPrompt_NoSlugDuplication' -v
```

expected: FAIL — `stripSlugPrefix` undefined

**Step 3: write minimal implementation**

In `app/plan_title.go`, add:

```go
// stripSlugPrefix removes the first line (and any trailing newlines after it)
// from a multiline description. Used when the first line was extracted as the
// plan name slug, so the stored description doesn't redundantly repeat it.
// Returns the input unchanged if it's single-line or empty.
func stripSlugPrefix(description string) string {
	text := strings.TrimSpace(description)
	idx := strings.IndexByte(text, '\n')
	if idx < 0 {
		return text
	}
	return strings.TrimSpace(text[idx+1:])
}
```

In `app/app_input.go`, at line 772 inside the `if firstLineIsViableSlug(description)` block, change:

```go
m.pendingPlanDesc = description
```

to:

```go
m.pendingPlanDesc = stripSlugPrefix(description)
```

Move the assignment inside the `if firstLineIsViableSlug` branch so only slug-prefixed descriptions get stripped. The else path (AI derivation) keeps the full description since the AI-derived title won't match the first line.

Specifically, restructure lines 771-784 to:

```go
m.pendingPlanName = heuristicPlanTitle(description)
m.textInputOverlay = nil

if firstLineIsViableSlug(description) {
    m.pendingPlanDesc = stripSlugPrefix(description)
    // ... rest of topic picker setup
} else {
    m.pendingPlanDesc = description
    // ... AI derivation path
}
```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run 'TestStripSlugPrefix|TestBuildPlanPrompt_NoSlugDuplication' -v
```

expected: PASS

Also run the full test suite to verify no regressions:

```bash
go test ./app/... ./config/... ./contracts/...
```

**Step 5: commit**

```bash
git add app/plan_title.go app/plan_title_test.go app/app_input.go app/app_plan_actions_test.go
git commit -m "fix: strip slug prefix from plan description to prevent duplication in prompts"
```
