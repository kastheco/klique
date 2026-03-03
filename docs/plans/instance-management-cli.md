# Instance Management CLI

**Goal:** Add `kas instance` subcommands (list, create, kill, pause, resume, restart, attach) so that agent sessions can be managed headlessly from the terminal without the TUI, enabling scripted workflows and remote management.

**Architecture:** A new `cmd/instance.go` file adds cobra subcommands under `kas instance`. Each subcommand reads instance state from the existing `session.Storage` (state.json), operates on `session.Instance` objects using their lifecycle methods, and writes state back. The `attach` command opens a tmux attach-session to the instance's tmux session. All commands are synchronous — no bubbletea dependency.

**Tech Stack:** Go 1.24, cobra CLI, `session` package (Instance, Storage), `session/tmux` (TmuxSession), `session/git` (GitWorktree)

**Size:** Medium (estimated ~3 hours, 2 tasks, 2 waves)

---

## Wave 1: Read and Mutate Commands

### Task 1: Add kas instance list, kill, pause, and resume commands

**Files:**
- Create: `cmd/instance.go` — `NewInstanceCmd` with `list`, `kill`, `pause`, `resume` subcommands
- Modify: `main.go` — wire `NewInstanceCmd` into root command
- Test: `cmd/instance_test.go`

**Step 1: write the failing test**

```go
// cmd/instance_test.go
package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceList_Empty(t *testing.T) {
	cmd := NewInstanceCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no instances")
}

func TestInstanceKill_NotFound(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"kill", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInstancePause_NotFound(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"pause", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInstanceResume_NotFound(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"resume", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run "TestInstanceList|TestInstanceKill|TestInstancePause|TestInstanceResume" -v
```

expected: FAIL — `NewInstanceCmd undefined`

**Step 3: write minimal implementation**

Create `cmd/instance.go` with the parent command and four subcommands:

```go
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/spf13/cobra"
)

func NewInstanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "manage agent instances",
	}
	cmd.AddCommand(newInstanceListCmd())
	cmd.AddCommand(newInstanceKillCmd())
	cmd.AddCommand(newInstancePauseCmd())
	cmd.AddCommand(newInstanceResumeCmd())
	return cmd
}
```

`list` displays a tabwriter table of all instances with title, status, program, branch, and task file. `kill` finds by title, calls `inst.Kill()`, removes from list, saves. `pause` and `resume` find by title and call the corresponding lifecycle method.

Factor out a shared helper for instance lookup:

```go
func loadAndFindInstance(title string) ([]*session.Instance, *session.Instance, error) {
	state, err := config.NewState()
	if err != nil {
		return nil, nil, fmt.Errorf("load state: %w", err)
	}
	storage, err := session.NewStorage(state)
	if err != nil {
		return nil, nil, fmt.Errorf("create storage: %w", err)
	}
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, nil, fmt.Errorf("load instances: %w", err)
	}
	for _, inst := range instances {
		if inst.Title == title {
			return instances, inst, nil
		}
	}
	return nil, nil, fmt.Errorf("instance %q not found", title)
}
```

Wire into `main.go`:

```go
rootCmd.AddCommand(cmd.NewInstanceCmd())
```

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./cmd/... -run "TestInstanceList|TestInstanceKill|TestInstancePause|TestInstanceResume" -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/instance.go cmd/instance_test.go main.go
git commit -m "feat: add kas instance list, kill, pause, and resume commands"
```

## Wave 2: Create, Attach, and Restart Commands

> **depends on wave 1:** The instance lookup helper and command wiring pattern from wave 1 are reused here.

### Task 2: Add kas instance create, attach, and restart commands

**Files:**
- Modify: `cmd/instance.go` — add `create`, `attach`, and `restart` subcommands
- Modify: `cmd/instance_test.go` — add tests for all three commands

**Step 1: write the failing test**

```go
func TestInstanceCreate_MissingTitle(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"create"})
	err := cmd.Execute()
	assert.Error(t, err) // requires at least a title
}

func TestInstanceAttach_NotFound(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"attach", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInstanceRestart_NotFound(t *testing.T) {
	cmd := NewInstanceCmd()
	cmd.SetArgs([]string{"restart", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./cmd/... -run "TestInstanceCreate|TestInstanceAttach|TestInstanceRestart" -v
```

expected: FAIL — no `create`/`attach`/`restart` subcommands

**Step 3: write minimal implementation**

**create** — creates a new instance with flags for program, branch, task file, and initial prompt:

```go
func newInstanceCreateCmd() *cobra.Command {
	var (
		program  string
		branch   string
		taskFile string
		prompt   string
	)
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "create and start a new agent instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeInstanceCreate(cmd, args[0], program, branch, taskFile, prompt)
		},
	}
	cmd.Flags().StringVar(&program, "program", "opencode", "agent program to run")
	cmd.Flags().StringVar(&branch, "branch", "", "git branch (creates worktree; empty = new branch)")
	cmd.Flags().StringVar(&taskFile, "task", "", "bind to a task file")
	cmd.Flags().StringVar(&prompt, "prompt", "", "initial prompt to send to the agent")
	return cmd
}

func executeInstanceCreate(cmd *cobra.Command, title, program, branch, taskFile, prompt string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    title,
		Path:     cwd,
		Program:  program,
		PlanFile: taskFile,
	})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	if prompt != "" {
		inst.QueuedPrompt = prompt
	}

	if branch != "" {
		if err := inst.StartOnBranch(branch); err != nil {
			return fmt.Errorf("start on branch: %w", err)
		}
	} else {
		if err := inst.Start(true); err != nil {
			return fmt.Errorf("start instance: %w", err)
		}
	}

	// Save to state
	state, err := config.NewState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	storage, err := session.NewStorage(state)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	instances, _ := storage.LoadInstances()
	instances = append(instances, inst)
	if err := storage.SaveInstances(instances); err != nil {
		return fmt.Errorf("save instances: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "created: %s (branch: %s)\n", inst.Title, inst.Branch)
	return nil
}
```

**attach** — finds the instance and execs into its tmux session:

```go
func executeInstanceAttach(cmd *cobra.Command, title string) error {
	_, inst, err := loadAndFindInstance(title)
	if err != nil {
		return err
	}
	if !inst.TmuxAlive() {
		return fmt.Errorf("instance %q tmux session is not running", title)
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxBin, []string{"tmux", "attach-session", "-t", inst.Title}, os.Environ())
}
```

**restart** — finds the instance and calls `inst.Restart()`, saves state.

Add all three to `NewInstanceCmd`.

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./cmd/... -run "TestInstanceCreate|TestInstanceAttach|TestInstanceRestart" -v
```

expected: PASS

**Step 5: commit**

```bash
git add cmd/instance.go cmd/instance_test.go
git commit -m "feat: add kas instance create, attach, and restart commands"
```
