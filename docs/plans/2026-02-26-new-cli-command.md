# `kas new` CLI Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `kas new <title> [description] [--topic <name>]` command (alias `kas n`) that creates a fully scaffolded plan from the terminal without launching the TUI.

**Architecture:** Extract the plan scaffolding helpers (`slugifyPlanName`, `buildPlanFilename`, `renderPlanStub`, `dedupePlanFilename`) from `app/app_state.go` into `config/planstate/` so both the TUI and the new CLI command can use them. The new cobra command wires those helpers together with the existing `planstate.Create`, `git.CommitPlanScaffoldOnMain`, and `git.EnsurePlanBranch` functions. No TUI, no bubbletea — pure headless execution.

**Tech Stack:** Go, cobra, `config/planstate`, `session/git`

**Size:** Small (estimated ~1.5 hours, 3 tasks, no waves)

---

### Task 1: Extract plan scaffolding helpers into `config/planstate`

The functions `slugifyPlanName`, `buildPlanFilename`, `renderPlanStub`, and `dedupePlanFilename` currently live in `app/app_state.go` as unexported package-private functions. They need to move to `config/planstate/scaffold.go` as exported functions so the CLI command can use them without importing the TUI app package.

**Files:**
- Create: `config/planstate/scaffold.go`
- Create: `config/planstate/scaffold_test.go`
- Modify: `app/app_state.go` (replace local functions with calls to planstate)
- Modify: `app/app_plan_creation_test.go` (update test calls)
- Modify: `app/clickup_import_test.go` (update test calls)

**Step 1: Write the failing tests**

Create `config/planstate/scaffold_test.go`:

```go
package planstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugifyPlanName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Auth Refactor", "auth-refactor"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "special-chars"},
		{"already-slug", "already-slug"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, SlugifyPlanName(tt.input))
		})
	}
}

func TestBuildPlanFilename(t *testing.T) {
	got := BuildPlanFilename("Auth Refactor", time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-02-21-auth-refactor.md", got)
}

func TestBuildPlanFilenameEmpty(t *testing.T) {
	got := BuildPlanFilename("", time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-02-21-plan.md", got)
}

func TestRenderPlanStub(t *testing.T) {
	stub := RenderPlanStub("Auth Refactor", "Refactor JWT auth", "2026-02-21-auth-refactor.md")
	assert.Contains(t, stub, "# Auth Refactor")
	assert.Contains(t, stub, "Refactor JWT auth")
	assert.Contains(t, stub, "2026-02-21-auth-refactor.md")
}

func TestDedupePlanFilename(t *testing.T) {
	dir := t.TempDir()
	base := "2026-02-24-test-task.md"

	// No collision
	assert.Equal(t, base, DedupePlanFilename(dir, base))

	// Create the file to force collision
	require.NoError(t, os.WriteFile(filepath.Join(dir, base), []byte("x"), 0o644))
	name2 := DedupePlanFilename(dir, base)
	assert.Equal(t, "2026-02-24-test-task-2.md", name2)

	// Create the -2 file too
	require.NoError(t, os.WriteFile(filepath.Join(dir, name2), []byte("x"), 0o644))
	assert.Equal(t, "2026-02-24-test-task-3.md", DedupePlanFilename(dir, base))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./config/planstate/ -run "TestSlugify|TestBuildPlan|TestRenderPlan|TestDedupe" -v`
Expected: FAIL — exported functions don't exist yet

**Step 3: Write the implementation**

Create `config/planstate/scaffold.go`:

```go
package planstate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SlugifyPlanName converts a human plan name to a URL-safe slug.
// "Auth Refactor" → "auth-refactor"
func SlugifyPlanName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// BuildPlanFilename derives the plan filename from a human name and creation time.
// "Auth Refactor" → "2026-02-21-auth-refactor.md"
func BuildPlanFilename(name string, now time.Time) string {
	slug := SlugifyPlanName(name)
	if slug == "" {
		slug = "plan"
	}
	return now.UTC().Format("2006-01-02") + "-" + slug + ".md"
}

// RenderPlanStub returns the initial markdown content for a new plan file.
func RenderPlanStub(name, description, filename string) string {
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by kas lifecycle flow\n- Plan file: %s\n", name, description, filename)
}

// DedupePlanFilename appends a numeric suffix if filename already exists in plansDir.
func DedupePlanFilename(plansDir, filename string) string {
	planPath := filepath.Join(plansDir, filename)
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return filename
	}
	base := strings.TrimSuffix(filename, ".md")
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d.md", base, i)
		if _, err := os.Stat(filepath.Join(plansDir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	return filename
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/planstate/ -run "TestSlugify|TestBuildPlan|TestRenderPlan|TestDedupe" -v`
Expected: all PASS

**Step 5: Update `app/app_state.go` to delegate to the new exports**

Replace the local `slugifyPlanName`, `buildPlanFilename`, `renderPlanStub`, and `dedupePlanFilename` functions with thin wrappers that call the planstate package:

```go
// In app/app_state.go — replace the four function bodies:

func slugifyPlanName(name string) string {
	return planstate.SlugifyPlanName(name)
}

func buildPlanFilename(name string, now time.Time) string {
	return planstate.BuildPlanFilename(name, now)
}

func renderPlanStub(name, description, filename string) string {
	return planstate.RenderPlanStub(name, description, filename)
}

func dedupePlanFilename(plansDir, filename string) string {
	return planstate.DedupePlanFilename(plansDir, filename)
}
```

The `planstate` import already exists in `app/app_state.go`. The existing tests in `app/app_plan_creation_test.go` and `app/clickup_import_test.go` call the local wrappers, so they continue to pass without modification.

**Step 6: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS — existing behavior unchanged

**Step 7: Commit**

```bash
git add config/planstate/scaffold.go config/planstate/scaffold_test.go app/app_state.go
git commit -m "refactor: extract plan scaffolding helpers to config/planstate"
```

---

### Task 2: Add `kas new` cobra command

Wire the extracted helpers and existing git/planstate functions into a new cobra command registered in `main.go`.

**Files:**
- Create: `newcmd.go` (in package `main`, next to `check.go`)
- Modify: `main.go` (register the command in `init()`)

**Step 1: Write the command implementation**

Create `newcmd.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session/git"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var topicFlag string

	cmd := &cobra.Command{
		Use:     "new <title> [description]",
		Aliases: []string{"n"},
		Short:   "Create a new plan without launching the TUI",
		Long: `Create a new plan entry with an optional description and topic.
Writes the plan stub, registers it in plan-state.json, commits on main,
and creates the plan branch.

Examples:
  kas new "auth refactor"
  kas new "auth refactor" "migrate from session cookies to JWT"
  kas new "auth refactor" --topic workflow
  kas n "quick fix"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			description := ""
			if len(args) > 1 {
				description = args[1]
			}
			return runNewPlan(title, description, topicFlag)
		},
	}

	cmd.Flags().StringVarP(&topicFlag, "topic", "t", "", "assign plan to a topic")
	return cmd
}

func runNewPlan(title, description, topic string) error {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	if !git.IsGitRepo(cwd) {
		return fmt.Errorf("error: kas must be run from within a git repository")
	}

	plansDir := filepath.Join(cwd, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	now := time.Now().UTC()
	filename := planstate.BuildPlanFilename(title, now)
	filename = planstate.DedupePlanFilename(plansDir, filename)
	branch := git.PlanBranchFromFile(filename)

	// Write stub markdown file
	stubContent := planstate.RenderPlanStub(title, description, filename)
	planPath := filepath.Join(plansDir, filename)
	if err := os.WriteFile(planPath, []byte(stubContent), 0o644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}

	// Register in plan-state.json
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return fmt.Errorf("failed to load plan state: %w", err)
	}
	if err := ps.Create(filename, description, branch, topic, now); err != nil {
		return fmt.Errorf("failed to register plan: %w", err)
	}

	// Commit on main and create plan branch
	if err := git.CommitPlanScaffoldOnMain(cwd, filename); err != nil {
		return fmt.Errorf("failed to commit plan scaffold: %w", err)
	}
	if err := git.EnsurePlanBranch(cwd, branch); err != nil {
		return fmt.Errorf("failed to create plan branch: %w", err)
	}

	// Print summary
	fmt.Println("plan created")
	fmt.Printf("  file:   docs/plans/%s\n", filename)
	fmt.Printf("  branch: %s\n", branch)
	if topic != "" {
		fmt.Printf("  topic:  %s\n", topic)
	}

	return nil
}
```

**Step 2: Register in `main.go`**

In the `init()` function, after `rootCmd.AddCommand(kasSetupCmd)`, add:

```go
	rootCmd.AddCommand(newNewCmd())
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: clean build

**Step 4: Commit**

```bash
git add newcmd.go main.go
git commit -m "feat: add 'kas new' CLI command for headless plan creation"
```

---

### Task 3: Add tests for `kas new` and verify full suite

**Files:**
- Create: `newcmd_test.go` (in package `main`)

**Step 1: Write the tests**

Create `newcmd_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initBareGitRepo creates a minimal git repo for testing.
func initBareGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}
	return dir
}

func TestRunNewPlan_Basic(t *testing.T) {
	dir := initBareGitRepo(t)
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	err := runNewPlan("auth refactor", "migrate to JWT", "")
	require.NoError(t, err)

	// Verify plan file exists
	matches, _ := filepath.Glob(filepath.Join(dir, "docs", "plans", "*-auth-refactor.md"))
	require.Len(t, matches, 1)

	// Verify plan-state.json has the entry
	ps, err := planstate.Load(filepath.Join(dir, "docs", "plans"))
	require.NoError(t, err)
	plans := ps.Unfinished()
	require.Len(t, plans, 1)
	assert.Equal(t, "migrate to JWT", plans[0].Description)
	assert.Contains(t, plans[0].Branch, "plan/auth-refactor")
	assert.Equal(t, planstate.StatusReady, plans[0].Status)
}

func TestRunNewPlan_WithTopic(t *testing.T) {
	dir := initBareGitRepo(t)
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	err := runNewPlan("auth refactor", "", "workflow")
	require.NoError(t, err)

	ps, err := planstate.Load(filepath.Join(dir, "docs", "plans"))
	require.NoError(t, err)
	plans := ps.PlansByTopic("workflow")
	require.Len(t, plans, 1)
	assert.Contains(t, plans[0].Filename, "auth-refactor")
}

func TestRunNewPlan_NotGitRepo(t *testing.T) {
	dir := t.TempDir() // not a git repo
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	err := runNewPlan("test", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git repository")
}

func TestRunNewPlan_DuplicateDedupes(t *testing.T) {
	dir := initBareGitRepo(t)
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	require.NoError(t, runNewPlan("dup test", "", ""))
	require.NoError(t, runNewPlan("dup test", "", ""))

	ps, err := planstate.Load(filepath.Join(dir, "docs", "plans"))
	require.NoError(t, err)
	plans := ps.Unfinished()
	require.Len(t, plans, 2)
	// Filenames should differ
	assert.NotEqual(t, plans[0].Filename, plans[1].Filename)
}

func TestRunNewPlan_PlanStateJsonStructure(t *testing.T) {
	dir := initBareGitRepo(t)
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	require.NoError(t, runNewPlan("structure test", "desc", "mytopic"))

	data, err := os.ReadFile(filepath.Join(dir, "docs", "plans", "plan-state.json"))
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, string(raw["plans"]), "structure-test")
	assert.Contains(t, string(raw["topics"]), "mytopic")
}
```

**Step 2: Run the new tests**

Run: `go test . -run "TestRunNewPlan" -v -count=1`
Expected: all PASS

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 4: Commit**

```bash
git add newcmd_test.go
git commit -m "test: add tests for kas new CLI command"
```
