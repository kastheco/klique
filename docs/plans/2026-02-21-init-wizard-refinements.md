# Init Wizard Refinements

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Three improvements to `kq init` wizard: bold highlight for customized agents, single-keypress y/n, remove phase stage and add non-interactive `chat` role.

**Architecture:** All changes are in `internal/initcmd/wizard/` (stages + state) and `internal/initcmd/scaffold/templates/` (chat prompts). Phase stage is deleted entirely. Chat role is added to defaults but excluded from the customize loop.

**Tech Stack:** Go, `golang.org/x/term` (already in deps), ANSI escape codes for bold.

---

### Task 1: Bold highlight for customized agent summaries

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go` — add `IsCustomized` helper
- Modify: `internal/initcmd/wizard/stage_agents.go` — use bold ANSI in `PromptCustomize`
- Modify: `internal/initcmd/wizard/wizard_test.go` — add `TestIsCustomized`

**Step 1: Write failing test for IsCustomized**

Add to `wizard_test.go`:

```go
func TestIsCustomized(t *testing.T) {
	t.Run("matches defaults returns false", func(t *testing.T) {
		defaults := RoleDefaults()
		assert.False(t, IsCustomized(defaults["coder"], "opencode"))
	})

	t.Run("different model returns true", func(t *testing.T) {
		a := RoleDefaults()["coder"]
		a.Model = "anthropic/claude-opus-4-6"
		assert.True(t, IsCustomized(a, "opencode"))
	})

	t.Run("different harness returns true", func(t *testing.T) {
		a := RoleDefaults()["coder"]
		a.Harness = "claude"
		assert.True(t, IsCustomized(a, "opencode"))
	})

	t.Run("different effort returns true", func(t *testing.T) {
		a := RoleDefaults()["coder"]
		a.Effort = "high"
		assert.True(t, IsCustomized(a, "opencode"))
	})

	t.Run("different temperature returns true", func(t *testing.T) {
		a := RoleDefaults()["coder"]
		a.Temperature = "0.5"
		assert.True(t, IsCustomized(a, "opencode"))
	})

	t.Run("disabled returns true", func(t *testing.T) {
		a := RoleDefaults()["coder"]
		a.Enabled = false
		assert.True(t, IsCustomized(a, "opencode"))
	})

	t.Run("unknown role returns false", func(t *testing.T) {
		a := AgentState{Role: "unknown", Enabled: true}
		assert.False(t, IsCustomized(a, "opencode"))
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestIsCustomized -v`
Expected: FAIL — `IsCustomized` undefined

**Step 3: Implement IsCustomized in wizard.go**

Add to `wizard.go`:

```go
// IsCustomized returns true if the agent's settings differ from factory RoleDefaults.
// defaultHarness is the harness that would be assigned if the user didn't customize.
func IsCustomized(a AgentState, defaultHarness string) bool {
	defaults, ok := RoleDefaults()[a.Role]
	if !ok {
		return false // unknown role, can't compare
	}
	defaults.Harness = defaultHarness
	return a.Harness != defaults.Harness ||
		a.Model != defaults.Model ||
		a.Effort != defaults.Effort ||
		a.Temperature != defaults.Temperature ||
		a.Enabled != defaults.Enabled
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestIsCustomized -v`
Expected: PASS

**Step 5: Update PromptCustomize to bold customized summaries**

In `stage_agents.go`, update `PromptCustomize`:

```go
// PromptCustomize prints an agent summary and asks "customize? y[n]".
// If customized is true, the summary is rendered in bold to indicate
// it differs from factory defaults.
func PromptCustomize(r io.Reader, w io.Writer, role string, summary string, customized bool) bool {
	if customized {
		fmt.Fprintf(w, "  %-10s \033[1m%s\033[0m\n", role, summary)
	} else {
		fmt.Fprintf(w, "  %-10s %s\n", role, summary)
	}
	fmt.Fprintf(w, "  customize? y[n]: ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(answer, "y")
}
```

Update the call site in `runAgentStage`:

```go
	for i := range state.Agents {
		agent := &state.Agents[i]
		summary := FormatAgentSummary(*agent)
		customized := IsCustomized(*agent, defaultHarness)

		if PromptCustomize(os.Stdin, os.Stdout, agent.Role, summary, customized) {
```

**Step 6: Update PromptCustomize tests**

Update all `TestPromptCustomize` subtests to pass the new `customized bool` parameter (use `false` for existing tests). Add one test verifying bold output:

```go
	t.Run("customized agent renders bold", func(t *testing.T) {
		var buf strings.Builder
		r := strings.NewReader("n\n")
		PromptCustomize(r, &buf, "coder", "opencode / custom-model", true)
		assert.Contains(t, buf.String(), "\033[1m")
		assert.Contains(t, buf.String(), "\033[0m")
	})

	t.Run("default agent renders plain", func(t *testing.T) {
		var buf strings.Builder
		r := strings.NewReader("n\n")
		PromptCustomize(r, &buf, "coder", "opencode / default-model", false)
		assert.NotContains(t, buf.String(), "\033[1m")
	})
```

**Step 7: Run all wizard tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: PASS

**Step 8: Commit**

```
feat(init): bold-highlight agent summaries that differ from defaults
```

---

### Task 2: Single-keypress y/n auto-submit

**Files:**
- Modify: `internal/initcmd/wizard/stage_agents.go` — raw terminal read
- Modify: `internal/initcmd/wizard/wizard_test.go` — update PromptCustomize tests

**Step 1: Refactor PromptCustomize for single-keypress**

Replace the `bufio.Scanner` approach with raw terminal input via `golang.org/x/term`.
Use a `keyReader` function parameter for testability:

```go
// KeyReader reads a single keypress and returns it.
// Production uses raw terminal mode; tests inject a simple reader.
type KeyReader func() (byte, error)

// TerminalKeyReader returns a KeyReader that uses raw terminal mode on stdin.
func TerminalKeyReader() KeyReader {
	return func() (byte, error) {
		fd := int(os.Stdin.Fd())
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return 0, fmt.Errorf("make raw: %w", err)
		}
		defer term.Restore(fd, oldState)

		buf := make([]byte, 1)
		_, err = os.Stdin.Read(buf)
		if err != nil {
			return 0, err
		}
		// Handle Ctrl-C
		if buf[0] == 3 {
			return 0, fmt.Errorf("interrupted")
		}
		return buf[0], nil
	}
}

// ReaderKeyReader returns a KeyReader that reads from an io.Reader (for tests).
func ReaderKeyReader(r io.Reader) KeyReader {
	return func() (byte, error) {
		buf := make([]byte, 1)
		_, err := r.Read(buf)
		return buf[0], err
	}
}
```

Update `PromptCustomize`:

```go
func PromptCustomize(readKey KeyReader, w io.Writer, role string, summary string, customized bool) (bool, error) {
	if customized {
		fmt.Fprintf(w, "  %-10s \033[1m%s\033[0m\n", role, summary)
	} else {
		fmt.Fprintf(w, "  %-10s %s\n", role, summary)
	}
	fmt.Fprintf(w, "  customize? y[n]: ")

	key, err := readKey()
	if err != nil {
		return false, err
	}

	// Echo the key and newline
	fmt.Fprintf(w, "%c\n", key)

	switch key {
	case 'y', 'Y':
		return true, nil
	default:
		// n, N, Enter, anything else = no
		return false, nil
	}
}
```

**Step 2: Update call site in runAgentStage**

```go
	readKey := TerminalKeyReader()
	for i := range state.Agents {
		agent := &state.Agents[i]
		summary := FormatAgentSummary(*agent)
		customized := IsCustomized(*agent, defaultHarness)

		customize, err := PromptCustomize(readKey, os.Stdout, agent.Role, summary, customized)
		if err != nil {
			return err
		}
		if customize {
			if err := runSingleAgentForm(state, i, modelCache); err != nil {
				return err
			}
		}
	}
```

**Step 3: Update all PromptCustomize tests**

Change from `io.Reader` to `ReaderKeyReader(r)` and handle the `(bool, error)` return:

```go
func TestPromptCustomize(t *testing.T) {
	t.Run("Enter returns false", func(t *testing.T) {
		readKey := ReaderKeyReader(strings.NewReader("\n"))
		result, err := PromptCustomize(readKey, io.Discard, "coder", "summary", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("n returns false", func(t *testing.T) {
		readKey := ReaderKeyReader(strings.NewReader("n"))
		result, err := PromptCustomize(readKey, io.Discard, "coder", "summary", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("y returns true", func(t *testing.T) {
		readKey := ReaderKeyReader(strings.NewReader("y"))
		result, err := PromptCustomize(readKey, io.Discard, "coder", "summary", false)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("Y returns true", func(t *testing.T) {
		readKey := ReaderKeyReader(strings.NewReader("Y"))
		result, err := PromptCustomize(readKey, io.Discard, "coder", "summary", false)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("junk defaults to false", func(t *testing.T) {
		readKey := ReaderKeyReader(strings.NewReader("x"))
		result, err := PromptCustomize(readKey, io.Discard, "coder", "summary", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("customized agent renders bold", func(t *testing.T) {
		var buf strings.Builder
		readKey := ReaderKeyReader(strings.NewReader("n"))
		_, _ = PromptCustomize(readKey, &buf, "coder", "summary", true)
		assert.Contains(t, buf.String(), "\033[1m")
	})

	t.Run("default agent renders plain", func(t *testing.T) {
		var buf strings.Builder
		readKey := ReaderKeyReader(strings.NewReader("n"))
		_, _ = PromptCustomize(readKey, &buf, "coder", "summary", false)
		assert.NotContains(t, buf.String(), "\033[1m")
	})
}
```

**Step 4: Run all wizard tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: PASS

**Step 5: Commit**

```
feat(init): auto-submit on single y/n keypress without Enter
```

---

### Task 3: Remove phase stage, hardcode mapping

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go` — remove `runPhaseStage` call, hardcode mapping
- Delete: `internal/initcmd/wizard/stage_phases.go`
- Modify: `internal/initcmd/wizard/wizard_test.go` — remove phase-related test adjustments if any

**Step 1: Replace runPhaseStage call with hardcoded mapping**

In `wizard.go`, replace the Stage 3 block in `Run()`:

```go
	// Stage 3: Hardcoded phase mapping
	state.PhaseMapping = map[string]string{
		"implementing":   "coder",
		"spec_review":    "reviewer",
		"quality_review": "reviewer",
		"planning":       "planner",
	}
```

**Step 2: Delete stage_phases.go**

Run: `rm internal/initcmd/wizard/stage_phases.go`

**Step 3: Run all wizard tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: PASS

**Step 4: Update main.go help text**

Remove the "3. Map lifecycle phases to agent roles" line from the init command description.

**Step 5: Commit**

```
refactor(init): remove phase mapping stage, hardcode defaults
```

---

### Task 4: Add `chat` agent role

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go` — add chat to roles and defaults
- Modify: `internal/initcmd/wizard/stage_agents.go` — skip chat in customize loop
- Modify: `internal/initcmd/wizard/wizard_test.go` — update tests
- Create: `internal/initcmd/scaffold/templates/opencode/agents/chat.md`
- Create: `internal/initcmd/scaffold/templates/claude/agents/chat.md`

**Step 1: Update DefaultAgentRoles and RoleDefaults**

In `wizard.go`:

```go
func DefaultAgentRoles() []string {
	return []string{"coder", "reviewer", "planner", "chat"}
}
```

Add to `RoleDefaults()`:

```go
		"chat": {
			Role:        "chat",
			Model:       "anthropic/claude-sonnet-4-6",
			Effort:      "high",
			Temperature: "0.3",
			Enabled:     true,
		},
```

**Step 2: Skip chat in the customize loop**

In `stage_agents.go`, update the loop in `runAgentStage`:

```go
	for i := range state.Agents {
		agent := &state.Agents[i]

		// chat role is not configurable in the wizard
		if agent.Role == "chat" {
			continue
		}

		summary := FormatAgentSummary(*agent)
		customized := IsCustomized(*agent, defaultHarness)

		customize, err := PromptCustomize(readKey, os.Stdout, agent.Role, summary, customized)
		// ...
	}
```

**Step 3: Update tests**

In `wizard_test.go`, update `TestDefaultAgentRoles`:

```go
func TestDefaultAgentRoles(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Equal(t, []string{"coder", "reviewer", "planner", "chat"}, roles)
}
```

Update `TestRoleDefaults` — add:

```go
	t.Run("chat defaults", func(t *testing.T) {
		ch := defaults["chat"]
		assert.Equal(t, "anthropic/claude-sonnet-4-6", ch.Model)
		assert.Equal(t, "high", ch.Effort)
		assert.Equal(t, "0.3", ch.Temperature)
		assert.True(t, ch.Enabled)
	})
```

Update `TestStateToTOMLConfig` and `TestStateToAgentConfigs` to include a chat agent in the test state.

**Step 4: Run tests**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: PASS

**Step 5: Commit**

```
feat(init): add chat agent role with fixed defaults
```

---

### Task 5: Chat agent prompt templates

**Files:**
- Create: `internal/initcmd/scaffold/templates/opencode/agents/chat.md`
- Create: `internal/initcmd/scaffold/templates/claude/agents/chat.md`

**Step 1: Create opencode chat template**

Write `internal/initcmd/scaffold/templates/opencode/agents/chat.md`:

```markdown
---
description: General-purpose assistant - answers questions, explores code, quick tasks
mode: primary
---

You are the chat agent. Help the user understand their codebase, answer questions, and handle quick one-off tasks.

## Role

You are a general-purpose assistant for interactive use. Unlike the coder agent (which follows TDD workflows and formal processes), you optimize for fast, accurate responses in conversation.

- Answer questions about the codebase — architecture, patterns, dependencies, how things work
- Do quick one-off tasks — rename a variable, add a comment, check a type signature
- Explore and explain — trace call chains, find usages, summarize modules
- For substantial implementation work, delegate to the coder agent

## Guidelines

- Be concise. The user is asking interactively, not requesting a report.
- Read code before answering questions about it. Don't guess from filenames.
- When a task grows beyond a quick fix, say so and suggest using the coder agent instead.
- Use project skills when they're relevant, but don't load heavy workflows (TDD, debugging) for simple questions.

## Project Skills

Load only when directly relevant to the question:
- `tui-design` — when asked about TUI components, views, or styles
- `tmux-orchestration` — when asked about tmux pane management or process lifecycle
- `golang-pro` — when asked about Go patterns, concurrency, interfaces

{{TOOLS_REFERENCE}}
```

**Step 2: Create claude chat template**

Write `internal/initcmd/scaffold/templates/claude/agents/chat.md`:

```markdown
---
name: chat
description: General-purpose assistant for questions and quick tasks
model: {{MODEL}}
---

You are the chat agent. Help the user understand their codebase, answer questions, and handle quick one-off tasks.

## Role

You are a general-purpose assistant for interactive use. Unlike the coder agent (which follows TDD workflows and formal processes), you optimize for fast, accurate responses in conversation.

- Answer questions about the codebase — architecture, patterns, dependencies, how things work
- Do quick one-off tasks — rename a variable, add a comment, check a type signature
- Explore and explain — trace call chains, find usages, summarize modules
- For substantial implementation work, delegate to the coder agent

## Guidelines

- Be concise. The user is asking interactively, not requesting a report.
- Read code before answering questions about it. Don't guess from filenames.
- When a task grows beyond a quick fix, say so and suggest using the coder agent instead.
- Use project skills when they're relevant, but don't load heavy workflows (TDD, debugging) for simple questions.

## Project Skills

Load only when directly relevant to the question:
- `tui-design` — when asked about TUI components, views, or styles
- `tmux-orchestration` — when asked about tmux pane management or process lifecycle
- `golang-pro` — when asked about Go patterns, concurrency, interfaces

{{TOOLS_REFERENCE}}
```

**Step 3: Verify scaffold picks up the new templates**

Run: `go test ./internal/initcmd/scaffold/ -v`
Expected: PASS (existing tests still pass; new templates are embedded automatically)

**Step 4: Commit**

```
feat(init): add chat agent prompt templates for opencode and claude
```

---

### Task 6: Final integration test

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

**Step 2: Build and smoke test**

Run: `go build -o /tmp/kq . && echo "build OK"`

**Step 3: Commit any fixups if needed**

---
