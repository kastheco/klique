# `kq check` — Skills & Wiring Health Audit

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `kq check` command that audits all three skill layers (personal/global, project, superpowers) and reports completeness, staleness, and broken symlinks per harness.

**Architecture:** New `internal/check/` package with pure-logic `Audit()` returning a typed result struct. Thin CLI wrapper in `check.go` at root. Each layer is checked independently; results are rendered with ✓/✗/⊘ glyphs and a summary line per harness.

**Tech Stack:** Go stdlib (os, path/filepath), existing `internal/initcmd/harness` registry for detection, `internal/initcmd/scaffold` for embedded skill manifest.

---

## Design

### Three audit layers

**1. Global skills** (`~/.agents/skills/` → harness global dirs)

For each installed harness, walk `~/.agents/skills/` and check whether a corresponding symlink exists in the harness global dir (`~/.claude/skills/`, `~/.config/opencode/skills/`). Track:
- **synced** — symlink exists, points to correct target, target is valid
- **skipped** — source entry is itself a symlink (superpowers-managed), intentionally not synced
- **missing** — source skill has no corresponding entry in harness dir
- **orphan** — entry in harness dir has no corresponding source skill (stale)
- **broken** — symlink exists but target doesn't resolve

**2. Project skills** (`<cwd>/.agents/skills/` → harness project dirs)

Compare the embedded skill manifest (golang-pro, tui-design, tmux-orchestration, cli-tools) against what actually exists in `<cwd>/.agents/skills/`. Then check harness project dirs for symlinks. Track same statuses as global, plus:
- **missing from canonical** — embedded skill never written to `.agents/skills/`

Only runs when cwd looks like a kq project (has `.agents/` dir).

**3. Superpowers**

Per harness:
- **claude:** check if `claude plugin list` contains "superpowers" (or skip if claude not installed)
- **opencode:** check if `~/.config/opencode/superpowers/` repo exists and `~/.config/opencode/plugins/superpowers.js` symlink is valid

### Output format

```
$ kq check

Global skills (~/.agents/skills):
  claude     15 synced  2 skipped  0 missing  1 orphan
  opencode   15 synced  2 skipped  0 missing  0 orphan

  Orphans:
    ~/.claude/skills/old-skill → (deleted)

Project skills (.agents/skills):
  ✓ golang-pro          claude ✓  opencode ✓
  ✓ tui-design          claude ✓  opencode ✓
  ✓ tmux-orchestration   claude ✓  opencode ✓
  ✗ cli-tools           MISSING from .agents/skills/

Superpowers:
  claude     ✓ plugin installed
  opencode   ✓ repo cloned, plugin symlinked

Health: 31/33 OK (93%)
```

The final summary line aggregates all checks into a single ratio. Exit code 0 if 100%, exit code 1 otherwise (useful for CI/scripts).

### Verbose flag

`kq check -v` expands each section to list every skill individually:

```
Global skills (~/.agents/skills):
  claude:
    ✓ brainstorming
    ✓ cli-tools
    ⊘ superpowers (managed externally)
    ...
```

---

## Implementation Plan

### Task 1: Audit result types in `internal/check/`

**Files:**
- Create: `internal/check/check.go`

**Step 1: Write the types**

```go
package check

// SkillStatus represents the state of a single skill entry.
type SkillStatus int

const (
	StatusSynced  SkillStatus = iota // symlink exists, valid target
	StatusSkipped                    // source is symlink, intentionally not synced
	StatusMissing                    // source exists, no link in harness
	StatusOrphan                     // link in harness, no source
	StatusBroken                     // symlink exists, target doesn't resolve
)

func (s SkillStatus) String() string {
	switch s {
	case StatusSynced:
		return "synced"
	case StatusSkipped:
		return "skipped"
	case StatusMissing:
		return "missing"
	case StatusOrphan:
		return "orphan"
	case StatusBroken:
		return "broken"
	default:
		return "unknown"
	}
}

// SkillEntry is one skill's audit result for one harness.
type SkillEntry struct {
	Name    string
	Status  SkillStatus
	Detail  string // e.g. symlink target, error message
}

// HarnessResult holds audit results for one harness.
type HarnessResult struct {
	Name      string
	Installed bool
	Skills    []SkillEntry
}

// ProjectSkillEntry is one embedded skill's status in the project.
type ProjectSkillEntry struct {
	Name           string
	InCanonical    bool              // exists in .agents/skills/
	HarnessStatus  map[string]SkillStatus // harness name → status
}

// SuperpowersResult holds superpowers check for one harness.
type SuperpowersResult struct {
	Name      string
	Installed bool
	Detail    string
}

// AuditResult is the complete output of kq check.
type AuditResult struct {
	Global       []HarnessResult
	Project      []ProjectSkillEntry
	Superpowers  []SuperpowersResult
	InProject    bool // whether cwd is a kq project
}

// Summary returns (ok, total) counts across all checks.
func (r *AuditResult) Summary() (int, int) {
	ok, total := 0, 0
	for _, h := range r.Global {
		for _, s := range h.Skills {
			if s.Status == StatusSkipped {
				continue // don't count intentional skips
			}
			total++
			if s.Status == StatusSynced {
				ok++
			}
		}
	}
	for _, p := range r.Project {
		if !p.InCanonical {
			total++
			continue
		}
		for _, st := range p.HarnessStatus {
			total++
			if st == StatusSynced {
				ok++
			}
		}
	}
	for _, sp := range r.Superpowers {
		total++
		if sp.Installed {
			ok++
		}
	}
	return ok, total
}
```

**Step 2: Commit**

```
git add internal/check/check.go
git commit -m "feat(check): add audit result types"
```

---

### Task 2: Global skills audit logic

**Files:**
- Create: `internal/check/global.go`
- Test: `internal/check/global_test.go`

**Step 1: Write the failing test**

Test sets up a temp dir with `~/.agents/skills/{foo,bar,superpowers}` where `superpowers` is a symlink, creates a harness dir with `foo` symlinked correctly and `stale` as an orphan, then asserts `AuditGlobal` returns the right statuses.

**Step 2: Implement `AuditGlobal`**

```go
// AuditGlobal checks ~/.agents/skills/ against one harness's global skill dir.
func AuditGlobal(home, harnessName string) HarnessResult
```

Logic:
1. Read `~/.agents/skills/` entries
2. For each dir entry, check if it's a symlink (→ StatusSkipped) or real dir
3. For real dirs, check if harness global dir has a symlink pointing to it
4. Walk harness global dir for orphans (entries with no corresponding source)
5. Check symlink targets resolve (broken detection)

Use `globalSkillsDir()` from existing `harness/sync.go` — will need to export it or duplicate the logic.

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```
git commit -m "feat(check): global skills audit with orphan/broken detection"
```

---

### Task 3: Project skills audit logic

**Files:**
- Create: `internal/check/project.go`
- Test: `internal/check/project_test.go`

**Step 1: Write the failing test**

Test creates a temp project dir with `.agents/skills/{tui-design,golang-pro}` and `.claude/skills/tui-design` symlinked, `.opencode/skills/` empty. Asserts tui-design shows synced for claude/missing for opencode, golang-pro shows missing for both.

**Step 2: Implement `AuditProject`**

```go
// EmbeddedSkillNames returns the list of skill names that kq init writes.
// Kept as a constant list to avoid pulling in embed FS.
var EmbeddedSkillNames = []string{"cli-tools", "golang-pro", "tmux-orchestration", "tui-design"}

// AuditProject checks <dir>/.agents/skills/ against expected embedded skills
// and verifies harness project skill dirs have valid symlinks.
func AuditProject(dir string, harnessNames []string) []ProjectSkillEntry
```

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```
git commit -m "feat(check): project skills audit against embedded manifest"
```

---

### Task 4: Superpowers audit logic

**Files:**
- Create: `internal/check/superpowers.go`
- Test: `internal/check/superpowers_test.go`

**Step 1: Write the failing test**

Test checks that `AuditSuperpowers` for opencode correctly detects repo dir + plugin symlink presence. Claude detection uses `exec.LookPath` mock or just checks plugin list output.

**Step 2: Implement `AuditSuperpowers`**

```go
// AuditSuperpowers checks superpowers installation for each harness.
func AuditSuperpowers(home string, harnessNames []string) []SuperpowersResult
```

- **claude:** shell out to `claude plugin list`, check for "superpowers" in output. If claude not found, mark as not installed with detail.
- **opencode:** check `~/.config/opencode/superpowers/.git` exists + `~/.config/opencode/plugins/superpowers.js` is a valid symlink.
- **codex:** skip (no superpowers concept).

**Step 3: Run tests, verify pass**

**Step 4: Commit**

```
git commit -m "feat(check): superpowers audit for claude and opencode"
```

---

### Task 5: Top-level `Audit()` orchestrator

**Files:**
- Modify: `internal/check/check.go`
- Test: `internal/check/check_test.go`

**Step 1: Add `Audit` function**

```go
// Audit runs all three audit layers and returns a complete result.
func Audit(home, projectDir string, registry *harness.Registry) *AuditResult
```

Calls `AuditGlobal`, `AuditProject`, `AuditSuperpowers` for each detected harness. Sets `InProject` based on whether `<projectDir>/.agents/` exists.

**Step 2: Test the orchestrator**

**Step 3: Commit**

```
git commit -m "feat(check): top-level Audit orchestrator"
```

---

### Task 6: CLI command `kq check`

**Files:**
- Create: `check.go` (root package, next to `skills.go`)

**Step 1: Write the command**

```go
func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Audit skills sync health across all harnesses",
		RunE:  runCheck,
	}
	cmd.Flags().BoolP("verbose", "v", false, "show per-skill detail")
	return cmd
}
```

**Step 2: Implement `runCheck`**

Calls `check.Audit()`, then renders each section with the output format from the design. Uses `--verbose` to expand per-skill listings. Prints summary line with percentage. Returns exit code 1 if not 100%.

**Step 3: Register in `init()`**

```go
func init() {
	rootCmd.AddCommand(newCheckCmd())
}
```

**Step 4: Manual test**

```
go run . check
go run . check -v
```

**Step 5: Commit**

```
git commit -m "feat: add kq check command for skills health audit"
```

---

### Task 7: Export `globalSkillsDir` from harness package

**Files:**
- Modify: `internal/initcmd/harness/sync.go` — rename `globalSkillsDir` → `GlobalSkillsDir` (export)
- Modify: any callers of the old name

**Step 1: Rename and verify**

Use ast-grep or sd to rename. Run `go build ./...` to verify no breakage.

**Step 2: Commit**

```
git commit -m "refactor(harness): export GlobalSkillsDir for use by check package"
```

*Note: This task should be done first if Task 2 needs it, or the check package can duplicate the switch statement. Exporting is cleaner.*

---

### Task 8: Integration test

**Files:**
- Create: `check_test.go`

**Step 1: Write integration test**

Sets up a temp home + project dir with known skill layout. Runs `newCheckCmd().Execute()` capturing stdout. Asserts output contains expected glyphs and summary percentage.

**Step 2: Run full test suite**

```
go test ./...
```

**Step 3: Commit**

```
git commit -m "test(check): integration test for kq check command"
```

---

## Recommended task order

Task 7 (export) → Task 1 (types) → Task 2 (global) → Task 3 (project) → Task 4 (superpowers) → Task 5 (orchestrator) → Task 6 (CLI) → Task 8 (integration test)
