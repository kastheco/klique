# prompt-via-cli Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bake the initial prompt into the agent CLI command at launch time (`--prompt` for opencode, positional arg for claude) instead of delivering it via send-keys after the TUI boots.

**Architecture:** Add `initialPrompt` field to `TmuxSession` with a setter. In `Start()`, append the shell-escaped prompt to the program command string using per-program syntax. In `Instance.Start*()` methods, transfer `QueuedPrompt` to `initialPrompt` when the program supports CLI prompts, clearing `QueuedPrompt` so the send-keys fallback doesn't fire.

**Tech Stack:** Go, tmux, shell escaping (POSIX single-quote)

---

## Wave 1

### Task 1: Add shell escape helper and tests

**Files:**
- Create: `session/tmux/shell.go`
- Create: `session/tmux/shell_test.go`

**Step 1: Write the failing test**

Create `session/tmux/shell_test.go`:

```go
package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellEscapeSingleQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello world", "'hello world'"},
		{"empty", "", "''"},
		{"single quote", "it's here", "'it'\\''s here'"},
		{"newlines", "line1\nline2\nline3", "'line1\nline2\nline3'"},
		{"backticks", "run `cmd` now", "'run `cmd` now'"},
		{"dollar sign", "cost is $5", "'cost is $5'"},
		{"double quotes", `say "hi"`, `'say "hi"'`},
		{"markdown prompt", "## Task 1: Auth\n\nImplement JWT auth.\n\n```go\nfunc main() {}\n```", "'## Task 1: Auth\n\nImplement JWT auth.\n\n```go\nfunc main() {}\n```'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellEscapeSingleQuote(tt.input))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./session/tmux/ -run TestShellEscapeSingleQuote -v`
Expected: FAIL — `shellEscapeSingleQuote` not defined

**Step 3: Write minimal implementation**

Create `session/tmux/shell.go`:

```go
package tmux

import "strings"

// shellEscapeSingleQuote wraps s in POSIX single quotes, escaping any
// embedded single quotes with the '\'' idiom. This is safe for all content
// (newlines, $, backticks, double quotes) except NUL bytes.
func shellEscapeSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./session/tmux/ -run TestShellEscapeSingleQuote -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/tmux/shell.go session/tmux/shell_test.go
git commit -m "feat(tmux): add POSIX single-quote shell escaping helper"
```

---

### Task 2: Add `initialPrompt` field and setter to TmuxSession

**Files:**
- Modify: `session/tmux/tmux.go`

**Step 1: Write the failing test**

Add to `session/tmux/tmux_test.go`:

```go
func TestSetInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	s := newTmuxSession("prompt-test", "opencode", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("hello world")

	// Verify the field is set (accessed via the Start command construction).
	assert.Equal(t, "hello world", s.initialPrompt)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./session/tmux/ -run TestSetInitialPrompt -v`
Expected: FAIL — `SetInitialPrompt` not defined, `initialPrompt` not a field

**Step 3: Write minimal implementation**

In `session/tmux/tmux.go`, add the field to `TmuxSession` struct after `agentType`:

```go
	// initialPrompt, when non-empty, is baked into the CLI command at Start()
	// using per-program syntax (--prompt for opencode, positional for claude).
	initialPrompt string
```

Add the setter after `SetAgentType`:

```go
// SetInitialPrompt sets the initial prompt to bake into the CLI command at launch.
// Supported programs: opencode (--prompt), claude (positional arg).
// For unsupported programs the prompt is ignored; callers should keep
// QueuedPrompt set so the send-keys fallback fires.
func (t *TmuxSession) SetInitialPrompt(prompt string) {
	t.initialPrompt = prompt
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./session/tmux/ -run TestSetInitialPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go
git commit -m "feat(tmux): add initialPrompt field and SetInitialPrompt setter"
```

---

### Task 3: Inject prompt into program command in Start()

**Files:**
- Modify: `session/tmux/tmux.go` (the `Start()` method)
- Modify: `session/tmux/tmux_test.go`

**Step 1: Write the failing tests**

Add to `session/tmux/tmux_test.go`:

```go
func TestStartOpenCodeWithInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := newTmuxSession("oc-prompt", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")
	s.SetInitialPrompt("Plan auth. Goal: JWT tokens.")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_oc-prompt -c %s KASMOS_MANAGED=1 opencode --agent planner --prompt 'Plan auth. Goal: JWT tokens.'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}

func TestStartClaudeWithInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
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
	s := newTmuxSession("claude-prompt", "claude", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("Implement the auth module.")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_claude-prompt -c %s KASMOS_MANAGED=1 claude 'Implement the auth module.'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}

func TestStartOpenCodeWithPromptContainingSingleQuotes(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := newTmuxSession("oc-quote", "opencode", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("it's a test")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_oc-quote -c %s KASMOS_MANAGED=1 opencode --prompt 'it'\\''s a test'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/tmux/ -run "TestStart.*WithInitialPrompt|TestStart.*WithPromptContaining" -v`
Expected: FAIL — prompt not appearing in command string

**Step 3: Write implementation**

In `session/tmux/tmux.go`, in the `Start()` method, after the `agentType` append block (line 175) and before the `KASMOS_MANAGED=1` prepend (line 179), add:

```go
	if t.initialPrompt != "" {
		escaped := shellEscapeSingleQuote(t.initialPrompt)
		switch {
		case isOpenCodeProgram(t.program):
			program = program + " --prompt " + escaped
		case isClaudeProgram(t.program):
			program = program + " " + escaped
		}
		// aider/gemini: no CLI prompt support — callers keep QueuedPrompt
		// set so the send-keys fallback fires from the app tick handler.
	}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./session/tmux/ -run "TestStart.*WithInitialPrompt|TestStart.*WithPromptContaining" -v`
Expected: PASS

**Step 5: Run full tmux test suite**

Run: `go test ./session/tmux/ -v`
Expected: all existing tests still pass

**Step 6: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go
git commit -m "feat(tmux): inject initial prompt into CLI command at launch"
```

---

### Task 4: Add `programSupportsCliPrompt` helper

**Files:**
- Create: `session/cli_prompt.go`
- Create: `session/cli_prompt_test.go`

**Step 1: Write the failing test**

Create `session/cli_prompt_test.go`:

```go
package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgramSupportsCliPrompt(t *testing.T) {
	tests := []struct {
		program  string
		expected bool
	}{
		{"opencode", true},
		{"claude", true},
		{"/usr/local/bin/claude", true},
		{"/home/user/.local/bin/opencode", true},
		{"aider --model ollama_chat/gemma3:1b", false},
		{"gemini", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.program, func(t *testing.T) {
			assert.Equal(t, tt.expected, programSupportsCliPrompt(tt.program))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./session/ -run TestProgramSupportsCliPrompt -v`
Expected: FAIL — `programSupportsCliPrompt` not defined

**Step 3: Write minimal implementation**

Create `session/cli_prompt.go`:

```go
package session

import "strings"

// programSupportsCliPrompt returns true if the program supports an initial
// prompt via CLI flag (opencode --prompt) or positional arg (claude).
func programSupportsCliPrompt(program string) bool {
	return strings.HasSuffix(program, "opencode") || strings.HasSuffix(program, "claude")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./session/ -run TestProgramSupportsCliPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/cli_prompt.go session/cli_prompt_test.go
git commit -m "feat(session): add programSupportsCliPrompt helper"
```

---

### Task 5: Transfer QueuedPrompt to initialPrompt in instance lifecycle

**Files:**
- Modify: `session/instance_lifecycle.go`
- Modify: `session/instance_lifecycle_test.go` (create if needed)

**Step 1: Write the failing test**

Check if `session/instance_lifecycle_test.go` exists. If not, create it. Add:

```go
package session

import (
	"os/exec"
	"testing"

	cmd_test "github.com/kastheco/kasmos/cmd/testing"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartTransfersQueuedPromptForOpenCode(t *testing.T) {
	ptyFactory := tmux.NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Ask anything"), nil
		},
	}

	inst := &Instance{
		Title:        "test-transfer",
		Path:         t.TempDir(),
		Program:      "opencode",
		QueuedPrompt: "Plan auth.",
		tmuxSession:  tmux.NewTmuxSessionWithDeps("test-transfer", "opencode", false, ptyFactory, cmdExec),
	}

	// Simulate StartOnMainBranch which is the simplest path.
	err := inst.StartOnMainBranch()
	require.NoError(t, err)

	// QueuedPrompt should be cleared (transferred to initialPrompt).
	assert.Empty(t, inst.QueuedPrompt)
}

func TestStartKeepsQueuedPromptForAider(t *testing.T) {
	ptyFactory := tmux.NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Open documentation url for more info"), nil
		},
	}

	inst := &Instance{
		Title:        "test-aider",
		Path:         t.TempDir(),
		Program:      "aider --model ollama_chat/gemma3:1b",
		QueuedPrompt: "Fix the bug.",
		tmuxSession:  tmux.NewTmuxSessionWithDeps("test-aider", "aider --model ollama_chat/gemma3:1b", false, ptyFactory, cmdExec),
	}

	err := inst.StartOnMainBranch()
	require.NoError(t, err)

	// QueuedPrompt should remain — aider doesn't support CLI prompts.
	assert.Equal(t, "Fix the bug.", inst.QueuedPrompt)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/ -run "TestStartTransfersQueuedPrompt|TestStartKeepsQueuedPrompt" -v`
Expected: FAIL — QueuedPrompt not being transferred

**Step 3: Write implementation**

In `session/instance_lifecycle.go`, add a helper method:

```go
// transferPromptToCli moves QueuedPrompt into the tmux session's initialPrompt
// for programs that support CLI prompt injection. For unsupported programs,
// QueuedPrompt stays set so the send-keys fallback fires.
func (i *Instance) transferPromptToCli() {
	if i.QueuedPrompt != "" && programSupportsCliPrompt(i.Program) {
		i.tmuxSession.SetInitialPrompt(i.QueuedPrompt)
		i.QueuedPrompt = ""
	}
}
```

Then call `i.transferPromptToCli()` in each Start method, right after `tmuxSession.SetAgentType(i.AgentType)`:

1. In `Start()` — after line 37 (`tmuxSession.SetAgentType(i.AgentType)`), add `i.transferPromptToCli()`
2. In `StartOnMainBranch()` — after line 120 (`tmuxSession.SetAgentType(i.AgentType)`), add `i.transferPromptToCli()`
3. In `StartInSharedWorktree()` — after line 166 (`tmuxSession.SetAgentType(i.AgentType)`), add `i.transferPromptToCli()`

**Step 4: Run tests to verify they pass**

Run: `go test ./session/ -run "TestStartTransfersQueuedPrompt|TestStartKeepsQueuedPrompt" -v`
Expected: PASS

**Step 5: Run full session test suite**

Run: `go test ./session/... -v`
Expected: all existing tests still pass

**Step 6: Commit**

```bash
git add session/instance_lifecycle.go session/cli_prompt.go session/instance_lifecycle_test.go
git commit -m "feat(session): transfer QueuedPrompt to CLI flag at launch for opencode/claude"
```

---

### Task 6: Verify full build and test suite

**Files:** none (verification only)

**Step 1: Build the project**

Run: `go build ./...`
Expected: clean compile

**Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass

**Step 3: Run existing tmux tests specifically**

Run: `go test ./session/tmux/ -v -count=1`
Expected: all pass, including the new prompt injection tests

**Step 4: Run existing app tests**

Run: `go test ./app/... -v -count=1`
Expected: all pass — the QueuedPrompt delivery in the tick handler still works for instances where QueuedPrompt wasn't transferred (aider/gemini, or instances created without a prompt)

**Step 5: Final commit if any fixups needed**

If any test failures were found and fixed, commit the fixes.
