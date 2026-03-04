# Instance Management CLI Implementation Plan

**Goal:** Add a `kas instance` CLI command tree that enables headless instance monitoring and lifecycle management — listing, killing, pausing, resuming, and prompting agent sessions without launching the TUI.

**Architecture:** A new `cmd/instance.go` file follows the established pattern from `cmd/task.go`: testable `executeXxx()` functions with thin cobra wrappers. The `list` command reads raw `InstanceData` from `config.State` without reconstructing live tmux sessions. Lifecycle commands (`kill`, `pause`, `resume`, `send`) reconstruct the target `Instance` via `session.FromInstanceData` and call the existing lifecycle methods, then persist state back. All commands are wired into both `cmd/cmd.go` `NewRootCmd()` and `main.go` `init()`.

**Tech Stack:** Go, cobra, `session.Storage`/`session.InstanceData`, `config.State`, `encoding/json`

**Size:** Small (estimated ~2 hours, 2 tasks, 2 waves)

---

## Wave 1: List Command and Infrastructure

### Task 1: Instance List Command with JSON Support

**Files:**
- Create: `cmd/instance.go`
- Create: `cmd/instance_test.go`
- Modify: `cmd/cmd.go`
- Modify: `main.go`

**Step 1: write the failing test**

```go
// cmd/instance_test.go
package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestState returns a State pre-populated with the given instances.
func newTestState(t *testing.T, instances []session.InstanceData) *config.State {
	t.Helper()
	raw, err := json.Marshal(instances)
	require.NoError(t, err)
	s := config.DefaultState()
	s.InstancesData = raw
	return s
}

func TestInstanceList_Text(t *testing.T) {
	state := newTestState(t, []session.InstanceData{
		{Title: "planner-foo", Status: session.Running, Branch: "plan/foo", Program: "claude", TaskFile: "foo.md"},
		{Title: "coder-bar", Status: session.Ready, Branch: "plan/bar", Program: "opencode", TaskFile: "bar.md"},
		{Title: "solo-baz", Status: session.Paused, Branch: "plan/baz", Program: "claude"},
	})

	output := executeInstanceList(state, "text")
	assert.Contains(t, output, "planner-foo")
	assert.Contains(t, output, "coder-bar")
	assert.Contains(t, output, "solo-baz")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "paused")
}

func TestInstanceList_JSON(t *testing.T) {
	instances := []session.InstanceData{
		{Title: "agent-1", Status: session.Running, Branch: "plan/agent", Program: "claude"},
	}
	state := newTestState(t, instances)

	output := executeInstanceList(state, "json")
	var parsed []map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 1)
	assert.Equal(t, "agent-1", parsed[0]["title"])
}

func TestInstanceList_Empty(t *testing.T) {
	state := newTestState(t, []session.InstanceData{})
	output := executeInstanceList(state, "text")
	assert.Equal(t, "no instances\n", output)
}

func TestInstanceList_StatusFilter(t *testing.T) {
	state := newTestState(t, []session.InstanceData{
		{Title: "running-1", Status: session.Running, Program: "claude"},
		{Title: "paused-1", Status: session.Paused, Program: "claude"},
	})

	output := executeInstanceList(state, "text", "paused")
	assert.Contains(t, output, "paused-1")
	assert.NotContains(t, output, "running-1")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run TestInstanceList -v
```

expected: FAIL — `executeInstanceList` undefined

**Step 3: write minimal implementation**

Create `cmd/instance.go` with:
- `statusString(s session.Status) string` helper to convert numeric status to text
- `executeInstanceList(state config.StateManager, format string, statusFilters ...string) string` — reads raw `InstancesData` from state, unmarshals to `[]session.InstanceData`, formats as text table or JSON
- `NewInstanceCmd() *cobra.Command` — parent `kas instance` command with `list` subcommand
- `list` subcommand with `--format` (text/json) and `--status` filter flags

Wire into `cmd/cmd.go` `NewRootCmd()`:
```go
root.AddCommand(NewInstanceCmd())
```

Wire into `main.go` `init()`:
```go
rootCmd.AddCommand(cmd2.NewInstanceCmd())
```

Text output format (tab-aligned):
```
TITLE              STATUS    BRANCH              PROGRAM    TASK
planner-foo        running   plan/foo            claude     foo.md
coder-bar          ready     plan/bar            opencode   bar.md
```

JSON output: array of objects with `title`, `status`, `branch`, `program`, `task_file`, `agent_type`, `created_at` fields.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run TestInstanceList -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/instance.go cmd/instance_test.go cmd/cmd.go main.go
git commit -m "feat: add kas instance list command with text/json output"
```

---

## Wave 2: Lifecycle Commands

> **depends on wave 1:** uses `NewInstanceCmd()` command tree, `statusString()` helper, and state loading patterns established in Task 1

### Task 2: Kill, Pause, Resume, and Send Commands

**Files:**
- Modify: `cmd/instance.go`
- Modify: `cmd/instance_test.go`

**Step 1: write the failing test**

```go
func TestFindInstanceData_Found(t *testing.T) {
	records := []session.InstanceData{
		{Title: "alpha", Status: session.Running},
		{Title: "beta", Status: session.Paused},
	}
	found, err := findInstanceData(records, "beta")
	require.NoError(t, err)
	assert.Equal(t, "beta", found.Title)
	assert.Equal(t, session.Paused, found.Status)
}

func TestFindInstanceData_NotFound(t *testing.T) {
	records := []session.InstanceData{
		{Title: "alpha", Status: session.Running},
	}
	_, err := findInstanceData(records, "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindInstanceData_FuzzyMatch(t *testing.T) {
	records := []session.InstanceData{
		{Title: "planner-my-feature", Status: session.Running},
		{Title: "coder-my-feature-task-1", Status: session.Running},
	}
	// Exact match should work
	found, err := findInstanceData(records, "planner-my-feature")
	require.NoError(t, err)
	assert.Equal(t, "planner-my-feature", found.Title)

	// Substring match when no exact match
	found, err = findInstanceData(records, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "coder-my-feature-task-1", found.Title)
}

func TestFindInstanceData_AmbiguousSubstring(t *testing.T) {
	records := []session.InstanceData{
		{Title: "planner-foo", Status: session.Running},
		{Title: "coder-foo", Status: session.Running},
	}
	_, err := findInstanceData(records, "foo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestValidateInstanceStatus_Kill(t *testing.T) {
	// Kill should work on any non-paused, non-exited instance
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Running}, "kill"))
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Ready}, "kill"))
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Paused}, "kill"))
}

func TestValidateInstanceStatus_Pause(t *testing.T) {
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Running}, "pause"))
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Ready}, "pause"))
	assert.Error(t, validateStatusForAction(session.InstanceData{Status: session.Paused}, "pause"))
}

func TestValidateInstanceStatus_Resume(t *testing.T) {
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Paused}, "resume"))
	assert.Error(t, validateStatusForAction(session.InstanceData{Status: session.Running}, "resume"))
}

func TestValidateInstanceStatus_Send(t *testing.T) {
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Running}, "send"))
	assert.NoError(t, validateStatusForAction(session.InstanceData{Status: session.Ready}, "send"))
	assert.Error(t, validateStatusForAction(session.InstanceData{Status: session.Paused}, "send"))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run "TestFindInstanceData|TestValidateInstanceStatus" -v
```

expected: FAIL — `findInstanceData`, `validateStatusForAction` undefined

**Step 3: write minimal implementation**

Add to `cmd/instance.go`:

Helper functions:
- `loadInstanceRecords(state config.StateManager) ([]session.InstanceData, error)` — unmarshals raw state to `[]InstanceData`
- `findInstanceData(records []session.InstanceData, title string) (session.InstanceData, error)` — exact match first, then substring match; returns error on not-found or ambiguous
- `validateStatusForAction(data session.InstanceData, action string) error` — validates the instance is in a compatible status for the requested action
- `removeInstanceFromState(state config.StateManager, title string) error` — removes the named instance from persisted state
- `updateInstanceInState(state config.StateManager, title string, updater func(*session.Instance) error) error` — reconstructs via `FromInstanceData`, applies the updater, saves state back

Cobra subcommands:
- `kill <title>` — validates status, reconstructs Instance, calls `Kill()`, removes from state
- `pause <title>` — validates not-paused, reconstructs Instance, calls `Pause()`, updates state
- `resume <title>` — validates paused, reconstructs Instance, calls `Resume()`, updates state
- `send <title> <prompt>` — validates running/ready, reconstructs Instance, calls `SendPrompt()`, no state change needed

All commands print a confirmation message on success (e.g., `killed: planner-foo`) and return errors via cobra's error handling.

**Step 4: run test to verify it passes**

```bash
go test ./cmd/... -run "TestFindInstanceData|TestValidateInstanceStatus" -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/instance.go cmd/instance_test.go
git commit -m "feat: add kas instance kill/pause/resume/send commands"
```
