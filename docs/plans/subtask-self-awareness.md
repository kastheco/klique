# Subtask Self-Awareness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Give coder agents running in parallel on shared worktrees real awareness of their situation — machine-readable identity via env vars, behavioral guidance via enriched prompts, and explicit git prohibitions to prevent cross-agent interference.

**Architecture:** Three layers: (1) env vars (`KASMOS_TASK`, `KASMOS_WAVE`, `KASMOS_PEERS`) prepended to the tmux command string, (2) brief always-present section in `coder.md`, (3) detailed parallel context block in `buildTaskPrompt()` injected only for multi-task waves. Peer count flows from `spawnWaveTasks` → `InstanceOptions.PeerCount` → `TmuxSession` fields → command string.

**Tech Stack:** Go, tmux, testify

**Size:** Small (estimated ~1.5 hours, 3 tasks, no waves)

---

### Task 1: Wire environment variables through tmux session

**Files:**
- Modify: `session/tmux/tmux.go:30-46` (add fields to TmuxSession struct)
- Modify: `session/tmux/tmux.go:104-115` (add setters alongside SetAgentType/SetInitialPrompt)
- Modify: `session/tmux/tmux.go:199-201` (prepend env vars alongside KASMOS_MANAGED)
- Modify: `session/tmux/tmux_test.go:61-101` (update command string assertions)
- Modify: `session/instance.go:226-244` (add PeerCount to InstanceOptions)
- Modify: `session/instance_lifecycle.go:212-248` (set task/wave/peer on tmux session in StartInSharedWorktree)
- Modify: `session/instance_lifecycle.go:30-46` (set task/wave/peer on tmux session in Start)
- Create: `session/tmux/env_vars_test.go`

**Step 1: Write the failing test**

Create `session/tmux/env_vars_test.go` with a test that creates a TmuxSession, sets task/wave/peer, calls Start(), and asserts the command string contains the env vars:

```go
package tmux

import (
	"fmt"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartTmuxSession_WithTaskEnvVars(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-task", "claude", false, ptyFactory, cmdExec)
	session.SetTaskEnv(3, 2, 4) // task 3, wave 2, 4 peers

	err := session.Start(workdir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ptyFactory.cmds), 1)

	cmdStr := cmd2.ToString(ptyFactory.cmds[0])
	assert.Contains(t, cmdStr, "KASMOS_TASK=3")
	assert.Contains(t, cmdStr, "KASMOS_WAVE=2")
	assert.Contains(t, cmdStr, "KASMOS_PEERS=4")
	assert.Contains(t, cmdStr, "KASMOS_MANAGED=1")
}

func TestStartTmuxSession_WithoutTaskEnvVars(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-solo", "claude", false, ptyFactory, cmdExec)
	// Don't call SetTaskEnv — solo task

	err := session.Start(workdir)
	require.NoError(t, err)

	cmdStr := cmd2.ToString(ptyFactory.cmds[0])
	assert.NotContains(t, cmdStr, "KASMOS_TASK=")
	assert.NotContains(t, cmdStr, "KASMOS_WAVE=")
	assert.NotContains(t, cmdStr, "KASMOS_PEERS=")
	assert.Contains(t, cmdStr, "KASMOS_MANAGED=1") // always present
}
```

Add missing imports (`os/exec`, `strings`, `cmd2`) using the same import pattern as `tmux_test.go`.

**Step 2: Run test to verify it fails**

Run: `go test ./session/tmux/ -run TestStartTmuxSession_With -v`
Expected: FAIL — `SetTaskEnv` method not defined.

**Step 3: Implement the changes**

In `session/tmux/tmux.go`, add three fields to the `TmuxSession` struct (after `initialPrompt`):

```go
	// taskNumber, waveNumber, peerCount are set for wave task instances.
	// When non-zero, they are prepended as KASMOS_TASK, KASMOS_WAVE, KASMOS_PEERS
	// env vars to the program command string.
	taskNumber int
	waveNumber int
	peerCount  int
```

Add a setter method after `SetInitialPrompt`:

```go
// SetTaskEnv sets the task identity env vars for parallel wave execution.
// When set, KASMOS_TASK, KASMOS_WAVE, and KASMOS_PEERS are prepended to the
// program command string at Start() time.
func (t *TmuxSession) SetTaskEnv(taskNumber, waveNumber, peerCount int) {
	t.taskNumber = taskNumber
	t.waveNumber = waveNumber
	t.peerCount = peerCount
}
```

In `Start()`, after the existing `KASMOS_MANAGED=1` prepend (line ~201), add:

```go
	// Prepend task identity env vars for parallel wave execution.
	if t.taskNumber > 0 {
		program = fmt.Sprintf("KASMOS_TASK=%d KASMOS_WAVE=%d KASMOS_PEERS=%d %s",
			t.taskNumber, t.waveNumber, t.peerCount, program)
	}
```

In `session/instance.go`, add `PeerCount` to `InstanceOptions`:

```go
	// PeerCount is the number of sibling tasks in the same wave (0 = not a wave task).
	PeerCount int
```

In `session/instance_lifecycle.go`, in every method that calls `tmuxSession.SetAgentType()`, add a call to wire the task env. Add this helper method to Instance:

```go
// setTmuxTaskEnv wires task/wave/peer identity to the tmux session for env var injection.
func (i *Instance) setTmuxTaskEnv() {
	if i.TaskNumber > 0 && i.tmuxSession != nil {
		i.tmuxSession.SetTaskEnv(i.TaskNumber, i.WaveNumber, i.PeerCount)
	}
}
```

Add a `PeerCount int` field to the `Instance` struct (after `WaveNumber`), set it from `opts.PeerCount` in `NewInstance`, and persist it in `InstanceData`/`toInstanceData`/`FromStorage`.

Call `i.setTmuxTaskEnv()` right after each `tmuxSession.SetAgentType(i.AgentType)` call in `Start()` (line ~37), `StartOnMainBranch` (line ~126), `StartOnBranch` (line ~172), and `StartInSharedWorktree` (line ~232).

Update `session/storage.go` to include `PeerCount` in `InstanceData`:

```go
	PeerCount              int       `json:"peer_count,omitempty"`
```

**Step 4: Update existing test assertions**

The existing `TestStartTmuxSession` (line 88) asserts the exact command string. It won't break (no task env set), but verify it still passes.

**Step 5: Run tests to verify everything passes**

Run: `go test ./session/tmux/ -v && go test ./session/ -v`
Expected: All PASS.

**Step 6: Commit**

```bash
git add session/tmux/tmux.go session/tmux/env_vars_test.go session/instance.go session/instance_lifecycle.go session/storage.go
git commit -m "feat: wire KASMOS_TASK/KASMOS_WAVE/KASMOS_PEERS env vars to tmux sessions"
```

---

### Task 2: Enrich dynamic prompt and thread peer count

**Files:**
- Modify: `app/wave_prompt.go` (rewrite parallel awareness block, add peerCount param)
- Modify: `app/wave_prompt_test.go` (update tests for new content and peer count)
- Modify: `app/app_state.go:1493-1508` (pass peer count to buildTaskPrompt and InstanceOptions)

**Step 1: Write the failing test**

Update `app/wave_prompt_test.go`:

```go
package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/planparser"
	"github.com/stretchr/testify/assert"
)

func TestBuildTaskPrompt(t *testing.T) {
	plan := &planparser.Plan{
		Goal:         "Build a feature",
		Architecture: "Modular approach",
		TechStack:    "Go, bubbletea",
	}
	task := planparser.Task{
		Number: 2,
		Title:  "Update Tests",
		Body:   "**Step 1:** Write the test\n\n**Step 2:** Run it",
	}

	prompt := buildTaskPrompt(plan, task, 1, 3, 4)

	// Plan context
	assert.Contains(t, prompt, "Build a feature")
	assert.Contains(t, prompt, "Modular approach")
	assert.Contains(t, prompt, "Go, bubbletea")
	assert.Contains(t, prompt, "cli-tools")

	// Task identity
	assert.Contains(t, prompt, "Task 2")
	assert.Contains(t, prompt, "Update Tests")
	assert.Contains(t, prompt, "Write the test")
	assert.Contains(t, prompt, "Wave 1 of 3")

	// Parallel awareness (multi-task)
	assert.Contains(t, prompt, "Task 2 of 4")
	assert.Contains(t, prompt, "3 other agents")
	assert.Contains(t, prompt, "NEVER run `git add .`")
	assert.Contains(t, prompt, "NEVER run `git stash`")
	assert.Contains(t, prompt, "NEVER run `git checkout --")
	assert.Contains(t, prompt, "formatters/linters")
	assert.Contains(t, prompt, "test failures in files outside your task")
	assert.Contains(t, prompt, "surgical changes")
}

func TestBuildTaskPrompt_SingleTask(t *testing.T) {
	plan := &planparser.Plan{Goal: "Simple"}
	task := planparser.Task{Number: 1, Title: "Only Task", Body: "Do it"}

	prompt := buildTaskPrompt(plan, task, 1, 1, 1)

	// Single task shouldn't mention parallel coordination
	assert.NotContains(t, prompt, "parallel")
	assert.NotContains(t, prompt, "NEVER run")
	assert.NotContains(t, prompt, "other agents")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBuildTaskPrompt -v`
Expected: FAIL — `buildTaskPrompt` has wrong number of arguments (5 instead of 4).

**Step 3: Rewrite `buildTaskPrompt()`**

Replace the entire function in `app/wave_prompt.go`:

```go
package app

import (
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/config/planparser"
)

// buildTaskPrompt constructs the prompt for a single task instance.
func buildTaskPrompt(plan *planparser.Plan, task planparser.Task, waveNumber, totalWaves, peerCount int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement Task %d: %s\n\n", task.Number, task.Title))
	sb.WriteString("Load the `cli-tools` skill before starting.\n\n")

	// Plan context
	header := plan.HeaderContext()
	if header != "" {
		sb.WriteString("## Plan Context\n\n")
		sb.WriteString(header)
		sb.WriteString("\n")
	}

	// Wave context
	sb.WriteString(fmt.Sprintf("## Wave %d of %d\n\n", waveNumber, totalWaves))

	// Parallel awareness — only for multi-task waves
	if peerCount > 1 {
		sb.WriteString(fmt.Sprintf("## Parallel Execution\n\n"))
		sb.WriteString(fmt.Sprintf("You are Task %d of %d in Wave %d. %d other agents are working in parallel on this same worktree.\n\n",
			task.Number, peerCount, waveNumber, peerCount-1))

		sb.WriteString("Your assigned files are listed in the Task Instructions below. Prioritize those files. ")
		sb.WriteString("If you must touch a shared file (go.mod, go.sum, imports), make minimal surgical changes — ")
		sb.WriteString("do not reorganize, reformat, or refactor anything outside your task scope.\n\n")

		sb.WriteString("CRITICAL — shared worktree rules:\n")
		sb.WriteString("- NEVER run `git add .` or `git add -A` — you will commit other agents' in-progress work\n")
		sb.WriteString("- NEVER run `git stash` or `git reset` — you will destroy sibling agents' changes\n")
		sb.WriteString("- NEVER run `git checkout -- <file>` on files you didn't modify — you will revert a sibling's edits\n")
		sb.WriteString("- NEVER run formatters/linters across the whole project (e.g. `go fmt ./...`) — scope them to your files only\n")
		sb.WriteString("- NEVER try to fix test failures in files outside your task — they may be caused by incomplete parallel work\n")
		sb.WriteString("- DO `git add` only the specific files you changed\n")
		sb.WriteString("- DO commit frequently with your task number in the message\n")
		sb.WriteString("- DO expect untracked files and uncommitted changes that are not yours — ignore them\n\n")
	}

	// Task body
	sb.WriteString("## Task Instructions\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n")

	return sb.String()
}
```

**Step 4: Update the caller in `app/app_state.go`**

In `spawnWaveTasks` (line ~1494), change the `buildTaskPrompt` call:

```go
		prompt := buildTaskPrompt(orch.plan, task, orch.CurrentWaveNumber(), orch.TotalWaves(), len(tasks))
```

And add `PeerCount` to the `InstanceOptions` (line ~1496):

```go
		inst, err := session.NewInstance(session.InstanceOptions{
			Title:      fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number),
			Path:       m.activeRepoPath,
			Program:    m.program,
			PlanFile:   planFile,
			AgentType:  session.AgentTypeCoder,
			TaskNumber: task.Number,
			WaveNumber: orch.CurrentWaveNumber(),
			PeerCount:  len(tasks),
		})
```

**Step 5: Run tests to verify everything passes**

Run: `go test ./app/ -run TestBuildTaskPrompt -v && go test ./app/ -v`
Expected: All PASS.

**Step 6: Commit**

```bash
git add app/wave_prompt.go app/wave_prompt_test.go app/app_state.go
git commit -m "feat: enrich task prompt with parallel awareness and git prohibitions"
```

---

### Task 3: Update agent definitions and CLAUDE.md

**Files:**
- Modify: `.opencode/agents/coder.md`
- Modify: `.claude/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/coder.md`
- Modify: `CLAUDE.md:38`
- Create: `contracts/coder_prompt_contract_test.go`

**Step 1: Write the contract test**

Create `contracts/coder_prompt_contract_test.go`:

```go
package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoderPromptParallelSection(t *testing.T) {
	coderFiles := []string{
		filepath.Join("..", ".opencode", "agents", "coder.md"),
		filepath.Join("..", ".claude", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "opencode", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "claude", "agents", "coder.md"),
	}

	required := []string{
		"## Parallel Execution",
		"KASMOS_TASK",
		"shared worktree",
		"dirty git state",
	}

	for _, f := range coderFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read coder prompt %s: %v", f, err)
		}
		text := string(data)

		for _, needle := range required {
			if !strings.Contains(text, needle) {
				t.Errorf("%s missing required text: %q", f, needle)
			}
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./contracts/ -run TestCoderPromptParallelSection -v`
Expected: FAIL — coder.md files don't contain the required text yet.

**Step 3: Update all four `coder.md` files**

Add the following section to each file, after the "## CLI Tools" section:

```markdown

## Parallel Execution

You may be running alongside other agents on a shared worktree. When `KASMOS_TASK` is set,
you are one of several concurrent agents — each assigned a specific task. Expect dirty git
state from sibling agents (untracked files, uncommitted changes in files you don't own).
Focus exclusively on your assigned task. The dynamic prompt you receive has specific rules.
```

All four files get identical content:
- `.opencode/agents/coder.md`
- `.claude/agents/coder.md`
- `internal/initcmd/scaffold/templates/opencode/agents/coder.md`
- `internal/initcmd/scaffold/templates/claude/agents/coder.md`

**Step 4: Update CLAUDE.md**

Replace the last sentence of line 38 in `CLAUDE.md`. Change:

```
Development follows a wave-based plan execution lifecycle. Each agent works only on the specific task it has been assigned — do not expand scope beyond your assigned work package. When the `KLIQUE_TASK` environment variable is set, it identifies your assigned task; implement only that task.
```

To:

```
Development follows a wave-based plan execution lifecycle. Each agent works only on the specific task it has been assigned — do not expand scope beyond your assigned work package. When `KASMOS_TASK` is set, you are one of several concurrent agents on a shared worktree. `KASMOS_WAVE` identifies your wave, `KASMOS_PEERS` the number of sibling agents. Implement only your assigned task — see your dynamic prompt for specific rules.
```

**Step 5: Run contract tests to verify they pass**

Run: `go test ./contracts/ -v`
Expected: All PASS (both the existing planner contract and the new coder contract).

**Step 6: Commit**

```bash
git add .opencode/agents/coder.md .claude/agents/coder.md \
  internal/initcmd/scaffold/templates/opencode/agents/coder.md \
  internal/initcmd/scaffold/templates/claude/agents/coder.md \
  CLAUDE.md contracts/coder_prompt_contract_test.go
git commit -m "feat: add parallel execution awareness to coder agent definitions"
```
