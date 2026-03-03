# Add `--repo` Flag and Update Scaffold Templates for Worktree Awareness

**Goal:** Complete the remaining review items from plan-status-cli-bugs: add a `--repo` persistent flag to `kas plan` so users can explicitly specify the repo root, update scaffold agent/skill templates to document worktree behavior, mirror template changes to live files, and add a contract test ensuring templates stay in sync.

**Architecture:** The `--repo` flag is a persistent string flag on the `plan` parent command. When set, `resolvePlansDirFromFlag(repoFlag)` returns `<repo>/docs/plans/` directly, skipping CWD-based resolution entirely. The `resolvePlansDir()` fallback chain becomes: (1) `--repo` flag, (2) CWD + `docs/plans/`, (3) `resolveRepoRoot(cwd)` + `docs/plans/`. Scaffold template updates document that `kas plan` commands work from worktrees and the `--repo` flag exists. A contract test in `contracts/` verifies the scaffold templates mention worktree awareness. Live agent/skill files are symlinked from `.agents/skills/` so updating `.agents/skills/kasmos-coder/SKILL.md` automatically updates the live skill.

**Tech Stack:** Go, cobra (CLI flags), `cmd/plan.go`, `cmd/plan_test.go`, `contracts/`, `internal/initcmd/scaffold/templates/`

**Size:** Small (estimated ~1.5 hours, 3 tasks, 2 waves)

---

## Wave 1: Add `--repo` Flag

### Task 1: Add `--repo` persistent flag to `kas plan` and wire into resolution

**Files:**
- Modify: `cmd/plan.go`
- Modify: `cmd/plan_test.go`

**Step 1: write the failing test**

Add a test that verifies `resolvePlansDirWithRepo(repoPath)` returns `<repo>/docs/plans/` when `--repo` is provided, and falls back to the existing `resolvePlansDir()` when empty:

```go
func TestResolvePlansDirWithRepo(t *testing.T) {
    repo := t.TempDir()
    plansDir := filepath.Join(repo, "docs", "plans")
    require.NoError(t, os.MkdirAll(plansDir, 0o755))

    // Explicit --repo returns the correct plansDir.
    got, err := resolvePlansDirWithRepo(repo)
    require.NoError(t, err)
    assert.Equal(t, plansDir, got)

    // Missing docs/plans/ in the repo is an error.
    empty := t.TempDir()
    _, err = resolvePlansDirWithRepo(empty)
    assert.Error(t, err)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestResolvePlansDirWithRepo -v
```

expected: FAIL — `resolvePlansDirWithRepo` undefined

**Step 3: write minimal implementation**

1. Add `resolvePlansDirWithRepo(repo string) (string, error)` — joins `repo + docs/plans`, stats it, returns.

2. Add a `var repoFlag string` package-level var. On the `plan` parent command, add `planCmd.PersistentFlags().StringVar(&repoFlag, "repo", "", "explicit repo root (skips worktree detection)")`.

3. Update every subcommand's `RunE` that currently calls `resolvePlansDir()` to instead call a new helper:

```go
func resolveEffectivePlansDir() (string, error) {
    if repoFlag != "" {
        return resolvePlansDirWithRepo(repoFlag)
    }
    return resolvePlansDir()
}
```

Replace all `resolvePlansDir()` call sites with `resolveEffectivePlansDir()`.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/plan.go cmd/plan_test.go
git commit -m "feat: add --repo persistent flag to kas plan for explicit repo root"
```

---

## Wave 2: Template Updates, Live File Mirroring, and Contract Test

> **depends on wave 1:** the `--repo` flag must exist so templates can document it

### Task 2: Update scaffold templates to document worktree awareness

**Files:**
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/fixer.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/fixer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/reviewer.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md`

Add a short section to each template noting:
- `kas plan` commands work from git worktrees — the CLI auto-detects the main repo root
- Use `kas plan --repo <path>` to explicitly specify the repo root if auto-detection fails

This is documentation-only — no code changes in the templates.

**Step 1: write the failing test**

Add a contract test in `contracts/kas_plan_repo_root_contract_test.go`:

```go
func TestScaffoldTemplates_DocumentWorktreeAwareness(t *testing.T) {
    templates := []string{
        "internal/initcmd/scaffold/templates/opencode/agents/coder.md",
        "internal/initcmd/scaffold/templates/opencode/agents/fixer.md",
        "internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md",
        "internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md",
    }
    for _, tmpl := range templates {
        data, err := os.ReadFile(filepath.Join("..", tmpl))
        require.NoError(t, err, "read %s", tmpl)
        text := string(data)
        assert.Contains(t, text, "worktree", "%s should mention worktree awareness", tmpl)
    }
}
```

**Step 2: run test to verify it fails**

```bash
go test ./contracts/... -run TestScaffoldTemplates_DocumentWorktreeAwareness -v
```

expected: FAIL — templates don't contain "worktree"

**Step 3: write minimal implementation**

Add a `## Worktree Awareness` section to each listed template file (use `sd` or `comby` for batch insertion since 8 files need the same block). The section content:

```markdown
## Worktree Awareness

`kas plan` commands work from git worktrees — the CLI auto-detects the main repo root via `.git` file parsing. If auto-detection fails, use `kas plan --repo <path>` to specify the repo root explicitly.
```

**Step 4: run test to verify it passes**

```bash
go test ./contracts/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add internal/initcmd/scaffold/templates/ contracts/
git commit -m "docs: add worktree awareness section to scaffold agent/skill templates"
```

### Task 3: Mirror template updates to live agent/skill files

**Files:**
- Modify: `.opencode/agents/coder.md`
- Modify: `.opencode/agents/fixer.md`
- Modify: `.opencode/agents/reviewer.md`
- Modify: `.agents/skills/kasmos-coder/SKILL.md`
- Modify: `.agents/skills/kasmos-fixer/SKILL.md`

Copy the same `## Worktree Awareness` section from the scaffold templates into the live files. The live skills in `.opencode/skills/` are symlinks to `.agents/skills/` so only the `.agents/` copies need updating.

Note: the `.claude/agents/` copies mirror `.opencode/agents/` — if they exist and diverge, update them too.

**Step 1: verify which live files need updating**

```bash
rg -L 'worktree' .opencode/agents/coder.md .opencode/agents/fixer.md .opencode/agents/reviewer.md .agents/skills/kasmos-coder/SKILL.md .agents/skills/kasmos-fixer/SKILL.md
```

expected: no matches — none mention worktree yet

**Step 2: write minimal implementation**

Use `sd` or batch edit to append the same `## Worktree Awareness` block to each file (same content as the scaffold templates).

**Step 3: run tests**

```bash
go test ./contracts/... -v
go test ./... -count=1
```

expected: PASS

**Step 4: commit**

```bash
git add .opencode/agents/ .agents/skills/
git commit -m "docs: mirror worktree awareness to live agent/skill files"
```
