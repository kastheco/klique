# Project Skills Distribution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move project-specific skills (tui-design, tmux-orchestration, golang-pro) to a shared canonical location, embed them in the `kq` binary, and scaffold them + per-harness symlinks on `kq init`.

**Architecture:** Skills live canonically in `.agents/skills/` (committed to repo). The scaffold embeds all skill trees via `//go:embed` and writes them to the target project's `.agents/skills/` on init. Per-harness symlinks (`.claude/skills/<name>`, `.opencode/skills/<name>`) point back to canonical location. Agent templates reference tui-design and tmux-orchestration for planner/reviewer roles.

**Tech Stack:** Go embed FS, `os.Symlink`, `fs.WalkDir`, existing scaffold infrastructure.

---

### Task 1: Clean up tmux-orchestration duplicate files

**Files:**
- Delete: `.opencode/skills/tmux-orchestration/bubbletea-integration.md` (duplicate of `resources/`)
- Delete: `.opencode/skills/tmux-orchestration/pane-orchestration.md` (duplicate of `resources/`)
- Delete: `.opencode/skills/tmux-orchestration/tmux-cli-wrapper.md` (duplicate of `resources/`)
- Rename: `.opencode/skills/tmux-orchestration/resources/` → `.opencode/skills/tmux-orchestration/references/`
- Modify: `.opencode/skills/tmux-orchestration/SKILL.md` — already references `references/`, no change needed

**Step 1: Delete the three top-level duplicates**

```bash
rm .opencode/skills/tmux-orchestration/bubbletea-integration.md
rm .opencode/skills/tmux-orchestration/pane-orchestration.md
rm .opencode/skills/tmux-orchestration/tmux-cli-wrapper.md
```

**Step 2: Rename `resources/` → `references/`**

```bash
mv .opencode/skills/tmux-orchestration/resources .opencode/skills/tmux-orchestration/references
```

**Step 3: Verify SKILL.md references resolve**

SKILL.md lines 29, 31, 33 already reference `references/` — confirm they now point to existing files:

```bash
ls .opencode/skills/tmux-orchestration/references/
# Expected: bubbletea-integration.md  pane-orchestration.md  tmux-cli-wrapper.md
```

**Step 4: Verify final structure**

```bash
find .opencode/skills/tmux-orchestration -type f | sort
```

Expected:
```
.opencode/skills/tmux-orchestration/SKILL.md
.opencode/skills/tmux-orchestration/references/bubbletea-integration.md
.opencode/skills/tmux-orchestration/references/pane-orchestration.md
.opencode/skills/tmux-orchestration/references/tmux-cli-wrapper.md
.opencode/skills/tmux-orchestration/scripts/find-sessions.sh
.opencode/skills/tmux-orchestration/scripts/wait-for-text.sh
```

---

### Task 2: Move skills to `.agents/skills/` canonical location

**Files:**
- Move: `.opencode/skills/tui-design/` → `.agents/skills/tui-design/`
- Move: `.opencode/skills/tmux-orchestration/` → `.agents/skills/tmux-orchestration/`
- Delete: `.opencode/skills/` directory (will be recreated as symlinks)

**Step 1: Move tui-design**

```bash
mv .opencode/skills/tui-design .agents/skills/tui-design
```

**Step 2: Move tmux-orchestration**

```bash
mv .opencode/skills/tmux-orchestration .agents/skills/tmux-orchestration
```

**Step 3: Remove empty `.opencode/skills/` dir**

```bash
rmdir .opencode/skills
```

**Step 4: Create per-harness symlinks for this repo**

```bash
# OpenCode
mkdir -p .opencode/skills
ln -s ../../.agents/skills/tui-design .opencode/skills/tui-design
ln -s ../../.agents/skills/tmux-orchestration .opencode/skills/tmux-orchestration
ln -s ../../.agents/skills/golang-pro .opencode/skills/golang-pro

# Claude (golang-pro symlink already exists)
ln -s ../../.agents/skills/tui-design .claude/skills/tui-design
ln -s ../../.agents/skills/tmux-orchestration .claude/skills/tmux-orchestration
```

**Step 5: Verify symlinks resolve**

```bash
ls -la .opencode/skills/
ls -la .claude/skills/
# All should show -> ../../.agents/skills/<name>
cat .opencode/skills/tui-design/SKILL.md | head -3
cat .claude/skills/tmux-orchestration/SKILL.md | head -3
```

---

### Task 3: Copy skills into scaffold embed tree

**Files:**
- Create: `internal/initcmd/scaffold/templates/skills/golang-pro/` (mirror of `.agents/skills/golang-pro/`)
- Create: `internal/initcmd/scaffold/templates/skills/tui-design/` (mirror of `.agents/skills/tui-design/`)
- Create: `internal/initcmd/scaffold/templates/skills/tmux-orchestration/` (mirror of `.agents/skills/tmux-orchestration/`)

**Step 1: Create the skills template directory**

```bash
mkdir -p internal/initcmd/scaffold/templates/skills
```

**Step 2: Copy all three skills**

```bash
cp -r .agents/skills/golang-pro internal/initcmd/scaffold/templates/skills/
cp -r .agents/skills/tui-design internal/initcmd/scaffold/templates/skills/
cp -r .agents/skills/tmux-orchestration internal/initcmd/scaffold/templates/skills/
```

**Step 3: Verify embed tree**

```bash
find internal/initcmd/scaffold/templates/skills -type f | sort | wc -l
# Expected: 17 files (golang-pro: 6, tui-design: 6, tmux-orchestration: 6 after dedup)
```

Note: The existing `//go:embed templates` directive in `scaffold.go:13` already captures
everything under `templates/` — no directive change needed.

---

### Task 4: Add `WriteProjectSkills` to scaffold.go

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go`

**Step 1: Write the failing test**

Add to `internal/initcmd/scaffold/scaffold_test.go`:

```go
func TestWriteProjectSkills(t *testing.T) {
	dir := t.TempDir()

	results, err := WriteProjectSkills(dir, false)
	require.NoError(t, err)

	// All three skills written
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "SKILL.md"))

	// Reference files included
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "references", "bubbletea-patterns.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "references", "pane-orchestration.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "references", "concurrency.md"))

	// Results track what was written
	assert.Greater(t, len(results), 0)
	for _, r := range results {
		assert.True(t, r.Created)
	}
}

func TestWriteProjectSkills_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "tui-design")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("custom"), 0o644))

	_, err := WriteProjectSkills(dir, false) // force=false
	require.NoError(t, err)

	content, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.Equal(t, "custom", string(content))
}

func TestWriteProjectSkills_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "tui-design")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("old"), 0o644))

	_, err := WriteProjectSkills(dir, true) // force=true
	require.NoError(t, err)

	content, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.NotEqual(t, "old", string(content))
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/initcmd/scaffold/ -run TestWriteProjectSkills -v
```

Expected: FAIL — `WriteProjectSkills` undefined.

**Step 3: Implement `WriteProjectSkills`**

Add to `scaffold.go`, after the existing imports add `"io/fs"`, then add:

```go
// WriteProjectSkills writes embedded skill trees to <dir>/.agents/skills/.
// Each skill is a directory containing SKILL.md and reference/script files.
func WriteProjectSkills(dir string, force bool) ([]WriteResult, error) {
	const prefix = "templates/skills"
	var results []WriteResult

	err := fs.WalkDir(templates, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// path is e.g. "templates/skills/tui-design/references/foo.md"
		// strip prefix to get "tui-design/references/foo.md"
		rel := path[len(prefix)+1:]
		dest := filepath.Join(dir, ".agents", "skills", rel)

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create skill dir: %w", err)
		}

		content, err := templates.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded skill %s: %w", path, err)
		}

		written, err := writeFile(dest, content, force)
		if err != nil {
			return fmt.Errorf("write skill %s: %w", rel, err)
		}

		relResult, relErr := filepath.Rel(dir, dest)
		if relErr != nil {
			relResult = dest
		}
		results = append(results, WriteResult{Path: relResult, Created: written})
		return nil
	})

	return results, err
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/initcmd/scaffold/ -run TestWriteProjectSkills -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/scaffold.go internal/initcmd/scaffold/scaffold_test.go
git commit -m "feat(scaffold): add WriteProjectSkills to write embedded skill trees"
```

---

### Task 5: Add `SymlinkHarnessSkills` to scaffold.go

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go`
- Modify: `internal/initcmd/scaffold/scaffold_test.go`

**Step 1: Write the failing test**

```go
func TestSymlinkHarnessSkills(t *testing.T) {
	dir := t.TempDir()

	// Create canonical skill dirs (simulating WriteProjectSkills already ran)
	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", name), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, ".agents", "skills", name, "SKILL.md"),
			[]byte("test"), 0o644))
	}

	// Symlink for claude
	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		link := filepath.Join(dir, ".claude", "skills", name)
		target, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked", name)
		assert.Equal(t, filepath.Join("..", "..", ".agents", "skills", name), target)

		// Symlink should resolve to actual content
		content, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
		require.NoError(t, err)
		assert.Equal(t, "test", string(content))
	}

	// Symlink for opencode
	err = SymlinkHarnessSkills(dir, "opencode")
	require.NoError(t, err)

	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		link := filepath.Join(dir, ".opencode", "skills", name)
		_, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked for opencode", name)
	}
}

func TestSymlinkHarnessSkills_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()

	// Create canonical
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "tui-design"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"),
		[]byte("new"), 0o644))

	// Create stale symlink
	skillsDir := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(skillsDir, "tui-design")))

	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	// Should have replaced the stale symlink
	content, err := os.ReadFile(filepath.Join(skillsDir, "tui-design", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/initcmd/scaffold/ -run TestSymlinkHarnessSkills -v
```

Expected: FAIL — `SymlinkHarnessSkills` undefined.

**Step 3: Implement `SymlinkHarnessSkills`**

Add to `scaffold.go`:

```go
// SymlinkHarnessSkills creates symlinks from .<harnessName>/skills/<skill>
// to ../../.agents/skills/<skill> for each skill in .agents/skills/.
// Replaces existing symlinks. Skips non-symlink entries (user-managed dirs).
func SymlinkHarnessSkills(dir, harnessName string) error {
	srcDir := filepath.Join(dir, ".agents", "skills")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no skills to link
		}
		return fmt.Errorf("read skills dir: %w", err)
	}

	destDir := filepath.Join(dir, "."+harnessName, "skills")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s skills dir: %w", harnessName, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		link := filepath.Join(destDir, name)
		target := filepath.Join("..", "..", ".agents", "skills", name)

		// Check if link already exists
		if fi, err := os.Lstat(link); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				// Replace existing symlink
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove existing symlink %s: %w", name, err)
				}
			} else {
				// Non-symlink entry (user-managed) — skip
				continue
			}
		}

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s skill %s: %w", harnessName, name, err)
		}
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/initcmd/scaffold/ -run TestSymlinkHarnessSkills -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/scaffold.go internal/initcmd/scaffold/scaffold_test.go
git commit -m "feat(scaffold): add SymlinkHarnessSkills for per-harness skill symlinks"
```

---

### Task 6: Wire skills into `ScaffoldAll` and `initcmd.Run`

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go` — update `ScaffoldAll`
- Modify: `internal/initcmd/initcmd.go` — no change needed (ScaffoldAll already called)

**Step 1: Write the failing test**

```go
func TestScaffoldAll_IncludesSkills(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
	}

	results, err := ScaffoldAll(dir, agents, false)
	require.NoError(t, err)

	// Skills written to canonical location
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "SKILL.md"))

	// Symlinks created for each active harness
	for _, h := range []string{"claude", "opencode"} {
		link := filepath.Join(dir, "."+h, "skills", "tui-design")
		_, err := os.Readlink(link)
		assert.NoError(t, err, "%s should have tui-design symlink", h)
	}

	// Codex not scaffolded (no codex agent), so no codex symlinks
	assert.NoFileExists(t, filepath.Join(dir, ".codex", "skills"))

	// Results include skill files
	var skillResults int
	for _, r := range results {
		if filepath.HasPrefix(r.Path, ".agents/skills/") {
			skillResults++
		}
	}
	assert.Greater(t, skillResults, 0)
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/initcmd/scaffold/ -run TestScaffoldAll_IncludesSkills -v
```

Expected: FAIL — skills not written, symlinks not created.

**Step 3: Update `ScaffoldAll`**

Modify the `ScaffoldAll` function in `scaffold.go` to write skills first, then create
symlinks for each harness that has agents:

```go
func ScaffoldAll(dir string, agents []harness.AgentConfig, force bool) ([]WriteResult, error) {
	var results []WriteResult

	// Write project skills to .agents/skills/
	skillResults, err := WriteProjectSkills(dir, force)
	if err != nil {
		return results, fmt.Errorf("scaffold skills: %w", err)
	}
	results = append(results, skillResults...)

	// Group agents by harness
	byHarness := make(map[string][]harness.AgentConfig)
	for _, a := range agents {
		byHarness[a.Harness] = append(byHarness[a.Harness], a)
	}

	type scaffoldFn func(string, []harness.AgentConfig, bool) ([]WriteResult, error)
	scaffolders := map[string]scaffoldFn{
		"claude":   WriteClaudeProject,
		"opencode": WriteOpenCodeProject,
		"codex":    WriteCodexProject,
	}

	// Iterate in stable order so results are deterministic across runs.
	for _, harnessName := range []string{"claude", "opencode", "codex"} {
		harnessAgents, ok := byHarness[harnessName]
		if !ok {
			continue
		}
		harnessResults, err := scaffolders[harnessName](dir, harnessAgents, force)
		if err != nil {
			return results, fmt.Errorf("scaffold %s: %w", harnessName, err)
		}
		results = append(results, harnessResults...)

		// Create skill symlinks for this harness
		if err := SymlinkHarnessSkills(dir, harnessName); err != nil {
			return results, fmt.Errorf("symlink %s skills: %w", harnessName, err)
		}
	}

	return results, nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/initcmd/scaffold/ -v
```

Expected: ALL PASS (including existing tests + new test)

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/scaffold.go internal/initcmd/scaffold/scaffold_test.go
git commit -m "feat(scaffold): wire skills + symlinks into ScaffoldAll"
```

---

### Task 7: Update agent templates — planner

**Files:**
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md` (planner section)
- Modify: `.opencode/agents/planner.md` (live, this repo)
- Modify: `.claude/agents/planner.md` (live, this repo — already has custom header)

**Step 1: Update opencode planner template**

Add project skills section after the Plan State section, before `{{TOOLS_REFERENCE}}`:

```markdown
## Project Skills

Always load when working on this project's TUI:
- `tui-design` — design-first workflow for bubbletea/lipgloss interfaces

Load when task involves tmux panes, worker lifecycle, or process management:
- `tmux-orchestration` — tmux pane management from Go, parking pattern, crash resilience
```

The full `internal/initcmd/scaffold/templates/opencode/agents/planner.md` becomes:

```markdown
---
description: Planning agent - specs, plans, task decomposition
mode: primary
---

You are the planner agent. Write specs, implementation plans, and decompose work into packages.

## Workflow

Before planning, load the relevant superpowers skill:
- **New features**: `brainstorming` — explore requirements before committing to a design
- **Writing plans**: `writing-plans` — structured plan format with phases and tasks
- **Large scope**: use `scc` for codebase metrics when estimating effort

## Plan State

Plans live in `docs/plans/`. State is tracked separately in `docs/plans/plan-state.json`
(never modify plan file content for state tracking). When creating a new plan, add an entry
with `"status": "ready"`. Transition to `"in_progress"` when implementation begins, `"done"`
when complete. Valid statuses: `ready`, `in_progress`, `done`.

## Project Skills

Always load when working on this project's TUI:
- `tui-design` — design-first workflow for bubbletea/lipgloss interfaces

Load when task involves tmux panes, worker lifecycle, or process management:
- `tmux-orchestration` — tmux pane management from Go, parking pattern, crash resilience

{{TOOLS_REFERENCE}}
```

**Step 2: Update claude planner template**

Same project skills section. The full `internal/initcmd/scaffold/templates/claude/agents/planner.md`:

```markdown
---
name: planner
description: Planning agent for specifications and architecture
model: {{MODEL}}
---

You are the planner agent. Write specs, implementation plans, and decompose work into packages.

## Workflow

Before planning, load the relevant superpowers skill:
- **New features**: `brainstorming` — explore requirements before committing to a design
- **Writing plans**: `writing-plans` — structured plan format with phases and tasks
- **Large scope**: use `scc` for codebase metrics when estimating effort

## Plan State

Plans live in `docs/plans/`. State is tracked separately in `docs/plans/plan-state.json`
(never modify plan file content for state tracking). When creating a new plan, add an entry
with `"status": "ready"`. Transition to `"in_progress"` when implementation begins, `"done"`
when complete. Valid statuses: `ready`, `in_progress`, `done`.

## Project Skills

Always load when working on this project's TUI:
- `tui-design` — design-first workflow for bubbletea/lipgloss interfaces

Load when task involves tmux panes, worker lifecycle, or process management:
- `tmux-orchestration` — tmux pane management from Go, parking pattern, crash resilience

{{TOOLS_REFERENCE}}
```

**Step 3: Update codex AGENTS.md planner section**

Add skill references to the Planner section in `internal/initcmd/scaffold/templates/codex/AGENTS.md`:

```markdown
## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Load superpowers skills: `brainstorming`, `writing-plans`.
Load project skills: `tui-design` (always for TUI work), `tmux-orchestration` (when task involves tmux/worker lifecycle).
```

**Step 4: Update live planner files for this repo**

Apply the same project skills section to `.opencode/agents/planner.md` and
`.claude/agents/planner.md` (the klique-specific live files).

Note: `.claude/agents/planner.md` has extra klique-specific context in its header — preserve
that, just add the project skills section.

**Step 5: Run scaffold tests**

```bash
go test ./internal/initcmd/scaffold/ -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/initcmd/scaffold/templates/ .opencode/agents/planner.md .claude/agents/planner.md
git commit -m "feat(agents): add tui-design and tmux-orchestration skill refs to planner"
```

---

### Task 8: Update agent templates — reviewer

**Files:**
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md` (reviewer section)
- Modify: `.opencode/agents/reviewer.md` (live, this repo)

**Step 1: Update opencode reviewer template**

Add project skills section after the workflow section, before `{{TOOLS_REFERENCE}}`:

```markdown
## Project Skills

Always load when reviewing TUI/UX changes:
- `tui-design` — terminal aesthetic principles, anti-patterns to flag

Load when reviewing tmux integration, worker backends, or pane management:
- `tmux-orchestration` — architecture principles, error handling philosophy
```

Full `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`:

```markdown
---
description: Review agent - checks quality, security, spec compliance
mode: primary
---

You are the reviewer agent. Review code for quality, security, and spec compliance.

## Workflow

Before reviewing, load the relevant superpowers skill:
- **Code reviews**: `requesting-code-review` — structured review against requirements
- **Receiving feedback**: `receiving-code-review` — verify suggestions before applying

Use `difft` for structural diffs (not line-based `git diff`) when reviewing changes.
Use `sg` (ast-grep) to verify patterns across the codebase rather than spot-checking.
Be specific about issues — cite file paths and line numbers.

## Project Skills

Always load when reviewing TUI/UX changes:
- `tui-design` — terminal aesthetic principles, anti-patterns to flag

Load when reviewing tmux integration, worker backends, or pane management:
- `tmux-orchestration` — architecture principles, error handling philosophy

{{TOOLS_REFERENCE}}
```

**Step 2: Update claude reviewer template**

Same project skills section. Full `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`:

```markdown
---
name: reviewer
description: Code review agent for quality and spec compliance
model: {{MODEL}}
---

You are the reviewer agent. Review code for quality, security, and spec compliance.

## Workflow

Before reviewing, load the relevant superpowers skill:
- **Code reviews**: `requesting-code-review` — structured review against requirements
- **Receiving feedback**: `receiving-code-review` — verify suggestions before applying

Use `difft` for structural diffs (not line-based `git diff`) when reviewing changes.
Use `sg` (ast-grep) to verify patterns across the codebase rather than spot-checking.
Be specific about issues — cite file paths and line numbers.

## Project Skills

Always load when reviewing TUI/UX changes:
- `tui-design` — terminal aesthetic principles, anti-patterns to flag

Load when reviewing tmux integration, worker backends, or pane management:
- `tmux-orchestration` — architecture principles, error handling philosophy

{{TOOLS_REFERENCE}}
```

**Step 3: Update codex AGENTS.md reviewer section**

```markdown
## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs (not line-based `git diff`).
Use `sg` (ast-grep) to verify patterns across the codebase.
Load superpowers skills: `requesting-code-review`, `receiving-code-review`.
Load project skills: `tui-design` (always for TUI/UX reviews), `tmux-orchestration` (when reviewing tmux/worker code).
```

**Step 4: Update live reviewer for this repo**

Apply same project skills section to `.opencode/agents/reviewer.md`.

**Step 5: Run scaffold tests**

```bash
go test ./internal/initcmd/scaffold/ -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/initcmd/scaffold/templates/ .opencode/agents/reviewer.md
git commit -m "feat(agents): add tui-design and tmux-orchestration skill refs to reviewer"
```

---

### Task 9: Update coder templates with project skills

**Files:**
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md` (coder section)

**Step 1: Add project skills to coder templates**

Coders should also know about these skills. Add after Plan State, before `{{TOOLS_REFERENCE}}`:

```markdown
## Project Skills

Load based on what you're implementing:
- `tui-design` — when building or modifying TUI components, views, or styles
- `tmux-orchestration` — when working on tmux pane management, worker backends, or process lifecycle
- `golang-pro` — for concurrency patterns, interface design, generics, testing best practices
```

**Step 2: Update all three coder templates and codex AGENTS.md**

Apply the section to opencode, claude, and codex coder templates. Update codex AGENTS.md
coder section to include:

```markdown
Load project skills: `tui-design` (TUI components), `tmux-orchestration` (tmux/worker code), `golang-pro` (Go patterns).
```

**Step 3: Run scaffold tests**

```bash
go test ./internal/initcmd/scaffold/ -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/initcmd/scaffold/templates/ 
git commit -m "feat(agents): add project skill refs to coder templates"
```

---

### Task 10: Build verification and final cleanup

**Files:**
- Verify: all modified files

**Step 1: Run full test suite**

```bash
go test ./... -count=1
```

Expected: ALL PASS

**Step 2: Build the binary**

```bash
go build ./...
```

Expected: clean build, no errors.

**Step 3: Verify embedded skills in binary**

Run a quick smoke test that the init would work by checking binary size grew:

```bash
ls -la kq 2>/dev/null || go build -o kq .
# Binary should be noticeably larger (~300K) due to embedded skills
```

**Step 4: Verify file structure**

```bash
# Canonical skills
find .agents/skills -type f | sort

# Symlinks
ls -la .claude/skills/
ls -la .opencode/skills/

# Embedded templates
find internal/initcmd/scaffold/templates/skills -type f | sort
```

**Step 5: Final commit**

```bash
git add -A
git commit -m "chore: project skills distribution — canonical location, embed, symlinks"
```
