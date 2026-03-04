# Fix Agents Reading From Disk Implementation Plan

**Goal:** Stop agents from reading plan content via disk files at `docs/plans/`. Add a `kas task show` CLI command so agents can retrieve plan content from the database, and update all prompt builders, review templates, and skill documentation to reference the CLI instead of disk paths.

**Architecture:** Three parallel changes: (1) Add a `kas task show <plan-file>` subcommand to `cmd/task.go` that reads content from the task store and prints raw markdown to stdout — follows the existing `resolvePlansDir()` → `resolveStore()` pattern. (2) Update all prompt-building functions in `app/app_state.go`, `orchestration/prompt.go`, and `app/app_actions.go` to reference `kas task show` instead of `docs/plans/` — also update `review-prompt.md` template and corresponding test assertions. (3) Update skill documentation (scaffold templates + live copies) to document `kas task show` as the standard way agents retrieve plan content.

**Tech Stack:** Go, cobra (CLI), config/taskstore, config/taskstate, testify, `sd` for batch markdown edits

**Size:** Small (estimated ~2 hours, 3 tasks, 1 wave)

---

## Wave 1: Add CLI command and update all agent-facing references

### Task 1: Add `kas task show` CLI subcommand

**Files:**
- Modify: `cmd/task.go`
- Test: `cmd/task_test.go`

**Step 1: write the failing test**

Add a test in `cmd/task_test.go` that calls `executeTaskShow` and verifies it returns the stored plan content:

```go
func TestExecuteTaskShow(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store := taskstore.NewTestSQLiteStore(t)
	project := projectFromPlansDir(plansDir)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReady,
		Content:  "# My Plan\n\n## Wave 1\n\n### Task 1: Do it\n\nDo the thing.\n",
	}))

	content, err := executeTaskShow(plansDir, "my-plan.md", store)
	require.NoError(t, err)
	assert.Equal(t, "# My Plan\n\n## Wave 1\n\n### Task 1: Do it\n\nDo the thing.\n", content)
}

func TestExecuteTaskShow_NotFound(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store := taskstore.NewTestSQLiteStore(t)
	_, err := executeTaskShow(plansDir, "nonexistent.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecuteTaskShow_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store := taskstore.NewTestSQLiteStore(t)
	project := projectFromPlansDir(plansDir)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "empty.md",
		Status:   taskstore.StatusReady,
	}))

	_, err := executeTaskShow(plansDir, "empty.md", store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no content")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run "TestExecuteTaskShow" -v
```

expected: FAIL — `executeTaskShow` is undefined

**Step 3: write minimal implementation**

Add `executeTaskShow` function and the `show` cobra subcommand in `cmd/task.go`:

```go
// executeTaskShow retrieves plan content from the task store and returns it
// as raw markdown. Returns an error if the plan doesn't exist or has no content.
func executeTaskShow(plansDir, planFile string, store taskstore.Store) (string, error) {
	ps, err := loadTaskState(plansDir, store)
	if err != nil {
		return "", err
	}
	if _, ok := ps.Entry(planFile); !ok {
		return "", fmt.Errorf("task not found: %s", planFile)
	}
	content, err := ps.GetContent(planFile)
	if err != nil {
		return "", fmt.Errorf("get content for %s: %w", planFile, err)
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("no content stored for %s", planFile)
	}
	return content, nil
}
```

Add the cobra subcommand inside `NewTaskCmd()`, alongside the existing subcommands:

```go
// kq task show
showCmd := &cobra.Command{
	Use:   "show <plan-file>",
	Short: "print plan content from the task store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plansDir, err := resolvePlansDir()
		if err != nil {
			return err
		}
		content, err := executeTaskShow(plansDir, args[0], resolveStore(plansDir))
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	},
}
planCmd.AddCommand(showCmd)
```

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run "TestExecuteTaskShow" -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/task.go cmd/task_test.go
git commit -m "feat(task-1): add kas task show subcommand for CLI-based plan content retrieval"
```

### Task 2: Update prompt builders and review template to reference `kas task show`

**Files:**
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Modify: `orchestration/prompt.go`
- Modify: `internal/initcmd/scaffold/templates/shared/review-prompt.md`
- Test: `app/app_task_actions_test.go`
- Test: `orchestration/prompt_test.go`

**Step 1: write the failing test**

Update existing tests in `app/app_task_actions_test.go` to assert prompts reference `kas task show` instead of `docs/plans/`:

```go
// TestBuildImplementPrompt — assert prompt says "kas task show" not "docs/plans/"
func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("auth-refactor.md")
	assert.Contains(t, prompt, "kas task show auth-refactor.md")
	assert.NotContains(t, prompt, "docs/plans/")
}

// TestBuildSoloPrompt_WithDescription — assert prompt says "kas task show" not "docs/plans/"
func TestBuildSoloPrompt_WithDescription(t *testing.T) {
	prompt := buildSoloPrompt("auth-refactor", "Refactor JWT auth", "auth-refactor.md")
	assert.Contains(t, prompt, "kas task show auth-refactor.md")
	assert.NotContains(t, prompt, "docs/plans/")
}

// TestBuildSoloPrompt_StubOnly — no plan file, no kas task show reference
func TestBuildSoloPrompt_StubOnly(t *testing.T) {
	prompt := buildSoloPrompt("quick-fix", "Fix the login bug", "")
	assert.NotContains(t, prompt, "kas task show")
	assert.NotContains(t, prompt, "docs/plans/")
}
```

Update `orchestration/prompt_test.go` for `BuildWaveAnnotationPrompt`:

```go
func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := BuildWaveAnnotationPrompt("my-feature.md")
	assert.Contains(t, prompt, "kas task show my-feature.md")
	assert.Contains(t, prompt, "## Wave")
	assert.Contains(t, prompt, "planner-finished-my-feature.md")
	assert.NotContains(t, prompt, "The plan at docs/plans/")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run "TestBuildImplementPrompt|TestBuildSoloPrompt" -v
go test ./orchestration/... -run "TestBuildWaveAnnotationPrompt" -v
```

expected: FAIL — prompts still reference `docs/plans/`

**Step 3: write minimal implementation**

**3a. Update `app/app_state.go`** — modify these functions:

`buildImplementPrompt`:
```go
func buildImplementPrompt(planFile string) string {
	return fmt.Sprintf(
		"Implement %s using the `kasmos-coder` skill. "+
			"Retrieve the full plan with `kas task show %s` and execute all tasks sequentially.",
		planFile, planFile,
	)
}
```

`buildSoloPrompt`:
```go
func buildSoloPrompt(planName, description, planFile string) string {
	if planFile != "" {
		return fmt.Sprintf(
			"Implement %s. Goal: %s. Retrieve the full plan with `kas task show %s`.",
			planName, description, planFile,
		)
	}
	return fmt.Sprintf("Implement %s. Goal: %s.", planName, description)
}
```

`buildModifyTaskPrompt`:
```go
func buildModifyTaskPrompt(planFile string) string {
	return fmt.Sprintf(
		"Modify existing plan %s. Retrieve current content with `kas task show %s`. "+
			"Keep the same filename and update only what changed.",
		planFile, planFile,
	)
}
```

`buildWaveAnnotationPrompt` in `app/app_state.go`:
```go
func buildWaveAnnotationPrompt(planFile string) string {
	return fmt.Sprintf(
		"The plan %[1]s is missing ## Wave N headers required for kasmos wave orchestration. "+
			"Retrieve the plan content with `kas task show %[1]s`, then annotate it by wrapping "+
			"all tasks under ## Wave N sections. "+
			"Every plan needs at least ## Wave 1 — even single-task trivial plans. "+
			"Keep all existing task content intact; only add the ## Wave headers.\n\n"+
			"After annotating:\n"+
			"1. Write the updated plan to docs/plans/%[1]s\n"+
			"2. Commit: git add docs/plans/%[1]s && git commit -m \"plan: add wave headers to %[1]s\"\n"+
			"3. Signal completion: touch .kasmos/signals/planner-finished-%[1]s\n"+
			"Do not edit plan-state.json directly.",
		planFile,
	)
}
```

Note: The annotation prompt still references `docs/plans/` for the *write* path (commit step). The plan is read via CLI, written to disk for git commit, and the TUI ingests the committed content. This is correct — the goal is to stop agents from *reading* from disk, not from *writing* for commits.

`buildChatAboutTaskPrompt`:
```go
func buildChatAboutTaskPrompt(planFile string, entry taskstate.TaskEntry, question string) string {
	name := taskstate.DisplayName(planFile)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are answering a question about the plan '%s'.\n\n", name))
	sb.WriteString("## Plan Context\n\n")
	sb.WriteString(fmt.Sprintf("- **Plan:** %s\n", planFile))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", entry.Status))
	if entry.Description != "" {
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", entry.Description))
	}
	if entry.Branch != "" {
		sb.WriteString(fmt.Sprintf("- **Branch:** %s\n", entry.Branch))
	}
	if entry.Topic != "" {
		sb.WriteString(fmt.Sprintf("- **Topic:** %s\n", entry.Topic))
	}
	sb.WriteString(fmt.Sprintf("\nRetrieve the full plan with `kas task show %s` for details.\n\n", planFile))
	sb.WriteString("## User Question\n\n")
	sb.WriteString(question)
	return sb.String()
}
```

Update the ClickUp import prompt (around line 1202-1206):
```go
prompt := fmt.Sprintf(`Analyze this imported ClickUp task. The task details and subtasks are included as reference in the plan.

Determine if the task is well-specified enough for implementation or needs further analysis. Write a proper implementation plan with ## Wave sections, task breakdowns, architecture notes, and tech stack. Use the ClickUp subtasks as a starting point but reorganize into waves based on dependencies.

Retrieve the current plan content with: kas task show %s`, filename)
```

**3b. Update `orchestration/prompt.go`** — modify `BuildWaveAnnotationPrompt`:
```go
func BuildWaveAnnotationPrompt(planFile string) string {
	return fmt.Sprintf(
		"The plan %[1]s is missing ## Wave N headers required for kasmos wave orchestration. "+
			"Retrieve the plan content with `kas task show %[1]s`, then annotate it by wrapping "+
			"all tasks under ## Wave N sections. "+
			"Every plan needs at least ## Wave 1 — even single-task trivial plans. "+
			"Keep all existing task content intact; only add the ## Wave headers.\n\n"+
			"After annotating:\n"+
			"1. Write the updated plan to docs/plans/%[1]s\n"+
			"2. Commit: git add docs/plans/%[1]s && git commit -m \"plan: add wave headers to %[1]s\"\n"+
			"3. Signal completion: touch .kasmos/signals/planner-finished-%[1]s\n"+
			"Do not edit plan-state.json directly.",
		planFile,
	)
}
```

**3c. Update `app/app_actions.go`** — change `LoadReviewPrompt` call (around line 882) from:
```go
reviewPrompt := scaffold.LoadReviewPrompt("docs/plans/"+planFile, planName)
```
to:
```go
reviewPrompt := scaffold.LoadReviewPrompt(planFile, planName)
```

Also update the call in `app/app_state.go` (around line 728) from:
```go
planPath := "docs/plans/" + planFile
prompt := scaffold.LoadReviewPrompt(planPath, planName)
```
to:
```go
prompt := scaffold.LoadReviewPrompt(planFile, planName)
```

(Remove the now-unused `planPath` variable.)

**3d. Update `internal/initcmd/scaffold/templates/shared/review-prompt.md`**:

Replace the opening lines:
```
Review the implementation of plan: {{PLAN_NAME}}

Plan file: {{PLAN_FILE}}

Read the plan content to understand the goals, architecture, and tasks that were implemented.
```

With:
```
Review the implementation of plan: {{PLAN_NAME}}

Retrieve the plan content with `kas task show {{PLAN_FILE}}` to understand the goals, architecture, and tasks that were implemented.
```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run "TestBuild" -v
go test ./orchestration/... -run "TestBuild" -v
```

expected: PASS

Also update the test assertion in `TestSoloActionChecksStoreNotDisk` (around line 633) — change:
```go
assert.Contains(t, soloInst.QueuedPrompt, "docs/plans/"+planFile, ...)
```
to:
```go
assert.Contains(t, soloInst.QueuedPrompt, "kas task show "+planFile, ...)
```

**Step 5: commit**

```bash
git add app/app_state.go app/app_actions.go app/app_task_actions_test.go orchestration/prompt.go orchestration/prompt_test.go internal/initcmd/scaffold/templates/shared/review-prompt.md
git commit -m "feat(task-2): update all prompt builders to reference kas task show instead of docs/plans/"
```

### Task 3: Document `kas task show` in skill files

**Files:**
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md`
- Modify: `.opencode/skills/kasmos-coder/SKILL.md`
- Modify: `.opencode/skills/kasmos-fixer/SKILL.md`
- Modify: `.opencode/skills/kasmos-lifecycle/SKILL.md`

**Step 1: write the failing test**

No unit tests — these are documentation files. Verification is by checking that the new `kas task show` reference exists and stale "read from disk" instructions are removed.

Baseline verification:

```bash
rg 'kas task show' internal/initcmd/scaffold/templates/skills/ .opencode/skills/ -c
# expected: 0 matches (not added yet)
```

**Step 2: run verification to confirm stale references exist (baseline)**

```bash
rg 'kas task show' internal/initcmd/scaffold/templates/skills/ .opencode/skills/
# expected: no matches
```

**Step 3: write changes**

**3a. kasmos-coder skill** — In both `internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md` and `.opencode/skills/kasmos-coder/SKILL.md`:

Update the managed-mode instructions (around line 67) from:
```
1. Read the task content from the task store (via `kas task` CLI or the task content API). Find your wave (`KASMOS_WAVE`) and task (`KASMOS_TASK`).
```
to:
```
1. Retrieve the plan content with `kas task show <plan-file>`. Find your wave (`KASMOS_WAVE`) and task (`KASMOS_TASK`) in the plan.
```

Update the manual-mode instructions (around line 81) from:
```
1. Read the plan. Check its **Size** field to determine wave structure.
```
to:
```
1. Retrieve the plan with `kas task show <plan-file>`. Check its **Size** field to determine wave structure.
```

**3b. kasmos-fixer skill** — In both `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md` and `.opencode/skills/kasmos-fixer/SKILL.md`:

Add `kas task show` to the "Available CLI Commands" section, after the `### kas task list` entry (around line 286):

```markdown
### `kas task show <plan-file>`

Print the plan's full markdown content from the task store. Use this to retrieve plan details
without reading from disk.

\`\`\`bash
kas task show my-plan.md
\`\`\`
```

**3c. kasmos-lifecycle skill** — In both `internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md` and `.opencode/skills/kasmos-lifecycle/SKILL.md`:

This skill is minimal and doesn't have a CLI commands section, but it should mention that plan content is retrieved via `kas task show`, not from disk. Add a sentence after "Agents only write sentinel files (managed mode) or use `kas task` CLI commands (manual mode)." (around line 24):

```
To retrieve plan content, agents use `kas task show <plan-file>` — never read directly from `docs/plans/` on disk.
```

Use `sd` for batch replacements across the 6 files where the same change applies to both scaffold and live copies.

**Step 4: run verification**

```bash
rg 'kas task show' internal/initcmd/scaffold/templates/skills/ .opencode/skills/ -c
# expected: matches in kasmos-coder, kasmos-fixer, kasmos-lifecycle (both scaffold + live)
```

Verify scaffold and live copies are identical:

```bash
difft .opencode/skills/kasmos-coder/SKILL.md internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md
difft .opencode/skills/kasmos-fixer/SKILL.md internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md
difft .opencode/skills/kasmos-lifecycle/SKILL.md internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md
```

expected: "No changes." for all three pairs

**Step 5: commit**

```bash
git add internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md \
       internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md \
       internal/initcmd/scaffold/templates/skills/kasmos-lifecycle/SKILL.md \
       .opencode/skills/kasmos-coder/SKILL.md \
       .opencode/skills/kasmos-fixer/SKILL.md \
       .opencode/skills/kasmos-lifecycle/SKILL.md
git commit -m "feat(task-3): document kas task show in coder, fixer, and lifecycle skills"
```
