# Fix Plan Status CLI Bugs: Worktree CWD and Stale Status

**Goal:** Fix `kas plan` CLI commands failing silently when run from worktrees (wrong CWD) and the planner-finished signal not transitioning plans from `planning` → `ready` → `implementing`, which blocks coder spawning.

**Architecture:** The root cause is `resolvePlansDir()` in `cmd/plan.go` using `os.Getwd()` to find `docs/plans/`. When agents run in worktrees, CWD is the worktree path (e.g., `/home/kas/dev/kasmos/.worktrees/plan-foo/`) which may not contain `docs/plans/`. Fix by adding a `resolveRepoRoot()` helper that shells out to `git rev-parse --show-toplevel` to find the true repo root, then derives `docs/plans/` from that. Add a `--repo` flag as an explicit override. Fix the stale-variable bug in `executePlanImplement()`. Update all agent/skill templates to document the auto-detection behavior.

**Tech Stack:** Go 1.24+, cobra CLI, git CLI (`rev-parse --show-toplevel`), planfsm, planstate, planstore (SQLite/HTTP)

**Size:** Medium (estimated ~3 hours, 3 tasks, 2 waves)

---

## Wave 1: Fix core CLI resolution and update templates

### Task 1: Add `resolveRepoRoot()`, fix `resolvePlansDir()`, fix stale variable, add `--repo` flag

**Files:**
- Modify: `cmd/plan.go`
- Modify: `cmd/plan_test.go`

**Step 1: write the failing test**

Add tests for the new `resolveRepoRoot()` function, the fixed `resolvePlansDir()`, the `--repo` flag helper, and the stale-variable fix in `executePlanImplement()`:

```go
func TestResolveRepoRoot_FindsGitRoot(t *testing.T) {
	// This test runs inside the actual kasmos repo, so rev-parse should work.
	root, err := resolveRepoRoot()
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(root, "docs", "plans"))
}

func TestResolvePlansDir_WithRepoFlag(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	resolved, err := resolvePlansDirWithRepo(root)
	require.NoError(t, err)
	assert.Equal(t, plansDir, resolved)
}

func TestResolvePlansDir_WithRepoFlag_MissingPlansDir(t *testing.T) {
	root := t.TempDir()
	_, err := resolvePlansDirWithRepo(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plans directory not found")
}

func TestPlanImplement_FromPlanning(t *testing.T) {
	store, dir := setupTestPlanState(t)
	repoRoot := filepath.Dir(filepath.Dir(dir))
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	project := projectFromPlansDir(dir)
	fsm := planfsm.New(store, project, dir)
	require.NoError(t, fsm.Transition("2026-02-20-test-plan.md", planfsm.PlanStart))

	ps, err := planstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	require.Equal(t, planstate.StatusPlanning, entry.Status)

	err = executePlanImplement(dir, "2026-02-20-test-plan.md", 1, store)
	require.NoError(t, err)

	ps, err = planstate.Load(store, project, dir)
	require.NoError(t, err)
	entry, _ = ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.StatusImplementing, entry.Status)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run "TestResolveRepoRoot_FindsGitRoot|TestResolvePlansDir_WithRepoFlag|TestPlanImplement_FromPlanning" -v
```

expected: FAIL — `resolveRepoRoot` and `resolvePlansDirWithRepo` undefined

**Step 3: write minimal implementation**

Three changes in `cmd/plan.go`:

1. Add `resolveRepoRoot()` using `git rev-parse --show-toplevel` and update `resolvePlansDir()` to use it instead of `os.Getwd()`:

```go
// resolveRepoRoot returns the git repository root by running
// `git rev-parse --show-toplevel`. This works from any subdirectory
// or worktree within the repo.
func resolveRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository (git rev-parse --show-toplevel failed): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func resolvePlansDir() (string, error) {
	root, err := resolveRepoRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "docs", "plans")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("plans directory not found: %s", dir)
	}
	return dir, nil
}
```

2. Add `resolvePlansDirWithRepo()` and `resolvePlansDirFromFlag()`, plus a persistent `--repo` flag on the `plan` parent command:

```go
func resolvePlansDirWithRepo(repoRoot string) (string, error) {
	dir := filepath.Join(repoRoot, "docs", "plans")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("plans directory not found: %s", dir)
	}
	return dir, nil
}

func resolvePlansDirFromFlag(repoFlag string) (string, error) {
	if repoFlag != "" {
		return resolvePlansDirWithRepo(repoFlag)
	}
	return resolvePlansDir()
}
```

In `NewPlanCmd()`:
```go
var repoFlag string
planCmd.PersistentFlags().StringVar(&repoFlag, "repo", "",
    "repository root path (default: auto-detect via git rev-parse)")
```

Update all subcommand `RunE` closures to call `resolvePlansDirFromFlag(repoFlag)` instead of `resolvePlansDir()`.

3. Fix the stale variable in `executePlanImplement()` — after the `PlannerFinished` transition, update `current`:

```go
if current == planfsm.StatusPlanning {
    if err := fsm.Transition(planFile, planfsm.PlannerFinished); err != nil {
        return err
    }
    current = planfsm.StatusReady // reflect the transition
}
```

Also add `"os/exec"` to the imports.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -v
```

expected: PASS (all existing + new tests)

**Step 5: commit**

```bash
git add cmd/plan.go cmd/plan_test.go
git commit -m "fix: resolve plans dir from git root instead of CWD, add --repo flag, fix stale status variable"
```

### Task 2: Update agent/skill templates to document auto-detection behavior

**Files:**
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/fixer.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/fixer.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-coder/SKILL.md`
- Modify: `internal/initcmd/scaffold/templates/skills/kasmos-fixer/SKILL.md`
- Test: `contracts/kas_plan_repo_root_contract_test.go`

**Step 1: write the failing test**

Add a contract test that verifies all agent templates mentioning `kas plan` also contain the repo root auto-detection note:

```go
// contracts/kas_plan_repo_root_contract_test.go
package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTemplates_KasPlanHasRepoRootNote(t *testing.T) {
	templates := []string{
		"internal/initcmd/scaffold/templates/opencode/agents/coder.md",
		"internal/initcmd/scaffold/templates/opencode/agents/fixer.md",
		"internal/initcmd/scaffold/templates/claude/agents/coder.md",
		"internal/initcmd/scaffold/templates/claude/agents/fixer.md",
	}
	for _, tmpl := range templates {
		t.Run(filepath.Base(filepath.Dir(tmpl))+"/"+filepath.Base(tmpl), func(t *testing.T) {
			data, err := os.ReadFile(tmpl)
			require.NoError(t, err)
			content := string(data)
			if strings.Contains(content, "kas plan") {
				assert.Contains(t, content, "repo root",
					"templates mentioning 'kas plan' must document repo root auto-detection")
			}
		})
	}
}
```

**Step 2: run test to verify it fails**

```bash
go test ./contracts/... -run TestAgentTemplates_KasPlanHasRepoRootNote -v
```

expected: FAIL — templates mention `kas plan` but don't contain "repo root"

**Step 3: write minimal implementation**

Update each agent template and skill that references `kas plan` to include the auto-detection note. Use `sd` for batch updates where the same text block is added.

For **coder agent templates** (`opencode/agents/coder.md` and `claude/agents/coder.md`), add after the `kas plan` CLI section:

```
`kas plan` auto-detects the repo root via `git rev-parse --show-toplevel`, so it works from
worktrees and subdirectories. If auto-detection fails, use `kas plan --repo /path/to/repo`.
```

For **fixer agent templates** (`opencode/agents/fixer.md` and `claude/agents/fixer.md`), add after the operations list:

```
`kas plan` auto-detects the repo root via `git rev-parse --show-toplevel`, so it works from
worktrees and subdirectories. If auto-detection fails, use `kas plan --repo /path/to/repo`.
```

For **planner agent templates** (`opencode/agents/planner.md` and `claude/agents/planner.md`), add after the plan registration section:

```
`kas plan` auto-detects the repo root via `git rev-parse --show-toplevel`, so it works from
worktrees and subdirectories. If auto-detection fails, use `kas plan --repo /path/to/repo`.
```

For **skill templates** (`kasmos-coder/SKILL.md`, `kasmos-fixer/SKILL.md`), add a note in the CLI commands section explaining the auto-detection.

**Step 4: run test to verify it passes**

```bash
go test ./contracts/... -run TestAgentTemplates_KasPlanHasRepoRootNote -v
```

expected: PASS

**Step 5: commit**

```bash
git add internal/initcmd/scaffold/templates/ contracts/
git commit -m "docs: add repo root auto-detection note to all agent templates referencing kas plan"
```

## Wave 2: Mirror template changes to live project files

> **depends on wave 1:** The template content from Wave 1 Task 2 must be finalized before mirroring to live files, since the live files should match the scaffold templates exactly.

### Task 3: Mirror template updates to live project agent/skill files

**Files:**
- Modify: `.opencode/agents/coder.md` (if exists)
- Modify: `.opencode/agents/fixer.md` (if exists)
- Modify: `.opencode/agents/planner.md` (if exists)
- Modify: `.agents/skills/superpowers/kasmos-coder/SKILL.md` (if exists, or `.opencode/skills/kasmos-coder/SKILL.md`)
- Modify: `.agents/skills/superpowers/kasmos-fixer/SKILL.md` (if exists, or `.opencode/skills/kasmos-fixer/SKILL.md`)

**Step 1: write the failing test**

No new test needed — this task mirrors scaffold template content to live project files. The contract test from Task 2 already validates the template content. The coder should verify the live files match the templates by visual diff.

Trivial task — TDD steps 1-2 omitted because there is no testable logic (pure content mirroring).

**Step 3: write minimal implementation**

For each live project file that has a corresponding scaffold template, copy the updated `kas plan` section from the template to the live file. Use `sd` or manual edits to add the same "repo root auto-detection" note.

Check which live files exist:
```bash
fd -t f 'coder.md|fixer.md|planner.md' .opencode/ .agents/ 2>/dev/null
```

For each file found, add the same note that was added to the scaffold template in Task 2.

**Step 4: verify the changes**

```bash
# Verify live files contain the note
rg 'repo root' .opencode/agents/ .agents/skills/ .opencode/skills/ 2>/dev/null
```

**Step 5: commit**

```bash
git add .opencode/ .agents/ 2>/dev/null
git commit -m "docs: mirror repo root auto-detection note to live project agent files"
```
