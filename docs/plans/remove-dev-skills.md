# Remove Dev Skills from Scaffolding

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove kasmos-specific dev skills (tui-design, tmux-orchestration, golang-pro, project-auditor) from the scaffolding so `kas init` only distributes generic workflow skills.

**Architecture:** Delete 4 embedded skill directories from `templates/skills/`, strip all "Project Skills" references from 8 agent templates + codex AGENTS.md, update tests to reflect the smaller skill set, and clean up this repo's own live symlinks.

**Tech Stack:** Go embed, testify

**Size:** Small (estimated ~45 min, 3 tasks, no waves)

---

### Task 1: Delete embedded dev skill directories and clean live symlinks

**Files:**
- Delete: `internal/initcmd/scaffold/templates/skills/tui-design/` (6 files)
- Delete: `internal/initcmd/scaffold/templates/skills/tmux-orchestration/` (6 files)
- Delete: `internal/initcmd/scaffold/templates/skills/golang-pro/` (6 files)
- Delete: `internal/initcmd/scaffold/templates/skills/project-auditor/` (2 files)
- Delete: `.agents/skills/tui-design/`, `.agents/skills/tmux-orchestration/`, `.agents/skills/golang-pro/`, `.agents/skills/project-auditor/`
- Delete symlinks: `.opencode/skills/{tui-design,tmux-orchestration,golang-pro,project-auditor}`
- Delete symlinks: `.claude/skills/{tui-design,tmux-orchestration,golang-pro,project-auditor}`

**Step 1: Delete the 4 embedded skill trees from templates**

```bash
rm -rf internal/initcmd/scaffold/templates/skills/tui-design
rm -rf internal/initcmd/scaffold/templates/skills/tmux-orchestration
rm -rf internal/initcmd/scaffold/templates/skills/golang-pro
rm -rf internal/initcmd/scaffold/templates/skills/project-auditor
```

**Step 2: Delete the canonical copies in .agents/skills/**

```bash
rm -rf .agents/skills/tui-design
rm -rf .agents/skills/tmux-orchestration
rm -rf .agents/skills/golang-pro
rm -rf .agents/skills/project-auditor
```

**Step 3: Remove symlinks from .opencode/skills/ and .claude/skills/**

```bash
rm .opencode/skills/tui-design .opencode/skills/tmux-orchestration .opencode/skills/golang-pro .opencode/skills/project-auditor
rm .claude/skills/tui-design .claude/skills/tmux-orchestration .claude/skills/golang-pro .claude/skills/project-auditor
```

**Step 4: Verify only workflow + cli-tools skills remain**

```bash
ls internal/initcmd/scaffold/templates/skills/
```

Expected: `cli-tools/`, `executing-plans/`, `finishing-a-development-branch/`, `requesting-code-review/`, `subagent-driven-development/`, `writing-plans/`

**Step 5: Verify it compiles**

Run: `go build ./...`
Expected: success (embed walker is generic, no hardcoded skill names in Go code)

**Step 6: Commit**

```bash
git add -A && git commit -m "chore: remove dev skills from embedded scaffolding"
```

---

### Task 2: Strip dev skill references from agent templates

**Files:**
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/claude/agents/planner.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/claude/agents/reviewer.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/claude/agents/chat.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/planner.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/chat.md` — remove `## Project Skills` section
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md` — remove `Load project skills:` lines

**Step 1: Remove `## Project Skills` sections from all claude agent templates**

In each of `claude/agents/{coder,planner,reviewer,chat}.md`, delete the entire `## Project Skills` section (header + all bullet lines up to the next `##` or EOF). The `## CLI Tools` section that follows must remain.

For coder.md and chat.md, the section is 4 lines (header + 3 bullets).
For planner.md, it's 5 lines (header + conditional text + 2 bullets).
For reviewer.md, it's 5 lines (header + conditional text + 2 bullets).

**Step 2: Remove `## Project Skills` sections from all opencode agent templates**

Same edits in `opencode/agents/{coder,planner,reviewer,chat}.md` — identical content to the claude counterparts.

**Step 3: Strip project skill references from codex/AGENTS.md**

Remove the `Load project skills:` line from each of the 3 role sections (Coder, Reviewer, Planner).

Lines to remove:
- `Load project skills: \`tui-design\` (TUI components), \`tmux-orchestration\` (tmux/worker code), \`golang-pro\` (Go patterns).`
- `Load project skills: \`tui-design\` (always for TUI/UX reviews), \`tmux-orchestration\` (when reviewing tmux/worker code).`
- `Load project skills: \`tui-design\` (always for TUI work), \`tmux-orchestration\` (when task involves tmux/worker lifecycle).`

**Step 4: Verify no stale references remain**

```bash
rg 'tui-design|golang-pro|tmux-orchestration|project-auditor' internal/initcmd/scaffold/templates/
```

Expected: no matches

**Step 5: Commit**

```bash
git add -A && git commit -m "chore: strip dev skill references from agent templates"
```

---

### Task 3: Update scaffold tests

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold_test.go`

**Step 1: Update TestWriteProjectSkills**

In `TestWriteProjectSkills` (line 276), change the comment from "All four skills" to reflect the new count. Remove these assertions:

```go
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "SKILL.md"))
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"))
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "SKILL.md"))
```

And remove the reference file assertions:
```go
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "references", "bubbletea-patterns.md"))
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "references", "pane-orchestration.md"))
assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "references", "concurrency.md"))
```

Keep the `cli-tools` assertions and the generic `assert.Greater(t, len(results), 0)` check.

**Step 2: Rewrite TestSymlinkHarnessSkills**

Replace the hardcoded `[]string{"golang-pro", "tui-design", "tmux-orchestration"}` with workflow skills that still exist in templates. Use `[]string{"cli-tools", "writing-plans", "executing-plans"}` (or any 2-3 that have SKILL.md files in templates). Update all loop bodies and assertions accordingly.

**Step 3: Update TestSymlinkHarnessSkills_ReplacesExisting**

Replace the `tui-design` references with `cli-tools` (which still exists). Update the file path from `"tui-design"` to `"cli-tools"` throughout the test and adjust the SKILL.md path.

**Step 4: Update TestWriteProjectSkills_SkipsExisting and _ForceOverwrites**

Replace `tui-design` with `cli-tools` in these two tests (lines 309-337).

**Step 5: Update TestScaffoldAll_IncludesSkills**

Remove assertions for `tui-design`, `tmux-orchestration`, `golang-pro` at lines 411-413. Keep `cli-tools` assertion. Update the symlink check (lines 417-421) to use `cli-tools` instead of `tui-design`.

**Step 6: Run tests**

```bash
go test ./internal/initcmd/scaffold/ -v -run 'TestWriteProjectSkills|TestSymlinkHarnessSkills|TestScaffoldAll_IncludesSkills'
```

Expected: all pass

**Step 7: Run full test suite**

```bash
go test ./...
```

Expected: all pass

**Step 8: Commit**

```bash
git add -A && git commit -m "test: update scaffold tests after dev skill removal"
```
