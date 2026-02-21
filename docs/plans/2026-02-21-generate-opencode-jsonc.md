# Generate opencode.jsonc from kq init wizard

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `kq init` generate `.opencode/opencode.jsonc` with model/temperature/effort from the wizard, role-based permissions, and dynamic path substitution for `$HOME` and project directory.

**Architecture:** Embedded JSONC template with `{{PLACEHOLDER}}` tokens, rendered by a new `renderOpenCodeConfig()` function in the scaffold package. Static `chat` agent block auto-injected. Conditional stripping of agent blocks whose harness isn't opencode.

**Tech Stack:** Go, embed, strings, os, testify

---

## Task 1: Create the opencode.jsonc template

**Files:**
- Create: `internal/initcmd/scaffold/templates/opencode/opencode.jsonc`

**Step 1: Write the template file**

Create `internal/initcmd/scaffold/templates/opencode/opencode.jsonc` with:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "agent": {
    "build": {
      "disable": true
    },
    "plan": {
      "disable": true
    },
    "chat": {
      "model": "anthropic/claude-sonnet-4-6",
      "permission": {
        "bash": "allow",
        "edit": {
          "*": "deny",
          "/tmp/*": "allow",
          "/tmp/**": "allow"
        },
        "external_directory": {
          "*": "ask",
          "{{PROJECT_DIR}}/*": "allow",
          "{{PROJECT_DIR}}/**": "allow",
          "/tmp/*": "allow",
          "/tmp/**": "allow",
          "{{HOME_DIR}}/.config/opencode/*": "allow",
          "{{HOME_DIR}}/.config/opencode/**": "allow"
        },
        "glob": "allow",
        "grep": "allow",
        "read": "allow",
        "write": {
          "*": "deny",
          "/tmp/*": "allow",
          "/tmp/**": "allow"
        }
      },
      "temperature": 0.3,
      "textVerbosity": "medium"
    },
    "coder": {
      "model": "{{CODER_MODEL}}",
      "permission": {
        "bash": "allow",
        "edit": "allow",
        "external_directory": {
          "*": "ask",
          "{{PROJECT_DIR}}/*": "allow",
          "{{PROJECT_DIR}}/**": "allow",
          "/tmp/*": "allow",
          "/tmp/**": "allow",
          "{{HOME_DIR}}/.config/opencode/*": "allow",
          "{{HOME_DIR}}/.config/opencode/**": "allow"
        },
        "glob": "allow",
        "grep": "allow",
        "read": "allow",
        "write": "allow"
      },
      {{CODER_EFFORT_LINE}}
      "temperature": {{CODER_TEMP}},
      "textVerbosity": "low"
    },
    "planner": {
      "model": "{{PLANNER_MODEL}}",
      "permission": {
        "bash": "allow",
        "edit": "allow",
        "external_directory": {
          "*": "ask",
          "{{PROJECT_DIR}}/*": "allow",
          "{{PROJECT_DIR}}/**": "allow",
          "/tmp/*": "allow",
          "/tmp/**": "allow",
          "{{HOME_DIR}}/.config/opencode/*": "allow",
          "{{HOME_DIR}}/.config/opencode/**": "allow"
        },
        "glob": "allow",
        "grep": "allow",
        "read": "allow",
        "write": "allow"
      },
      {{PLANNER_EFFORT_LINE}}
      "temperature": {{PLANNER_TEMP}},
      "textVerbosity": "medium"
    },
    "reviewer": {
      "model": "{{REVIEWER_MODEL}}",
      "permission": {
        "bash": "allow",
        "edit": {
          "*": "deny",
          "/tmp/*": "allow",
          "/tmp/**": "allow"
        },
        "external_directory": {
          "*": "ask",
          "{{PROJECT_DIR}}/*": "allow",
          "{{PROJECT_DIR}}/**": "allow",
          "/tmp/*": "allow",
          "/tmp/**": "allow"
        },
        "glob": "allow",
        "grep": "allow",
        "read": "allow",
        "write": {
          "*": "deny",
          "/tmp/*": "allow",
          "/tmp/**": "allow"
        }
      },
      {{REVIEWER_EFFORT_LINE}}
      "temperature": {{REVIEWER_TEMP}},
      "textVerbosity": "medium"
    }
  }
}
```

Note: `{{ROLE_EFFORT_LINE}}` is a full-line placeholder. When effort is set, it renders as
`"reasoningEffort": "medium",`. When empty, the entire line is removed.

**Step 2: Verify the template is embedded**

```bash
go build ./...
```

Expected: compiles (the `//go:embed templates` in scaffold.go already picks up all files under `templates/`).

---

## Task 2: Create the chat agent prompt template

**Files:**
- Create: `internal/initcmd/scaffold/templates/opencode/agents/chat.md`

**Step 1: Write the chat agent prompt**

Create `internal/initcmd/scaffold/templates/opencode/agents/chat.md` with:

```markdown
---
description: Research agent - codebase exploration, questions, analysis
mode: primary
---

You are the chat agent. Answer questions about the codebase, research topics, and help with analysis.

## Workflow

You are a read-only research agent. You explore, search, and analyze — but you do not modify files.

- Use `rg` (ripgrep) and `sg` (ast-grep) for structural code search
- Use `scc` for codebase metrics and line counts
- Use `difft` for reviewing changes structurally
- Read files freely, grep broadly, but do not write or edit project files

When asked a question:
1. Search the codebase for relevant code
2. Read surrounding context to understand the architecture
3. Provide a clear, concise answer with file paths and line references

## Project Skills

Load based on what you're researching:
- `tui-design` — when exploring TUI components, views, or styles
- `tmux-orchestration` — when exploring tmux pane management, worker backends, or process lifecycle
- `golang-pro` — for understanding concurrency patterns, interface design, generics

{{TOOLS_REFERENCE}}
```

**Step 2: Verify build**

```bash
go build ./...
```

Expected: compiles.

---

## Task 3: Write `renderOpenCodeConfig` and integrate into `WriteOpenCodeProject`

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go:103-106`

**Step 1: Write the failing test**

Add to `internal/initcmd/scaffold/scaffold_test.go`:

```go
func TestWriteOpenCodeProject_GeneratesConfig(t *testing.T) {
	dir := t.TempDir()
	temp := 0.1
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: &temp, Effort: "medium", Enabled: true},
		{Role: "planner", Harness: "opencode", Model: "anthropic/claude-opus-4-6", Temperature: ptrFloat(0.5), Effort: "max", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "openai/gpt-5.3-codex", Temperature: ptrFloat(0.2), Effort: "xhigh", Enabled: true},
	}

	results, err := WriteOpenCodeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	// Config file created
	configPath := filepath.Join(dir, ".opencode", "opencode.jsonc")
	assert.FileExists(t, configPath)

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	s := string(content)

	// Schema present
	assert.Contains(t, s, `"$schema": "https://opencode.ai/config.json"`)

	// Disabled built-in agents
	assert.Contains(t, s, `"build"`)
	assert.Contains(t, s, `"plan"`)
	assert.Contains(t, s, `"disable": true`)

	// Chat agent with fixed defaults
	assert.Contains(t, s, `"chat"`)
	assert.Contains(t, s, `"anthropic/claude-sonnet-4-6"`)

	// Wizard-configured agents have correct models
	assert.Contains(t, s, `"anthropic/claude-opus-4-6"`)
	assert.Contains(t, s, `"openai/gpt-5.3-codex"`)

	// Temperature rendered as bare numbers (no quotes)
	assert.Contains(t, s, "0.1")
	assert.Contains(t, s, "0.5")
	assert.Contains(t, s, "0.2")

	// Effort values present
	assert.Contains(t, s, `"reasoningEffort": "medium"`)
	assert.Contains(t, s, `"reasoningEffort": "max"`)
	assert.Contains(t, s, `"reasoningEffort": "xhigh"`)

	// No raw placeholders left
	assert.NotContains(t, s, "{{")
	assert.NotContains(t, s, "}}")

	// Dynamic paths resolved (home dir and project dir)
	homeDir, _ := os.UserHomeDir()
	assert.Contains(t, s, homeDir)
	assert.Contains(t, s, dir)

	// Config is in the results list
	var found bool
	for _, r := range results {
		if r.Path == ".opencode/opencode.jsonc" {
			found = true
			assert.True(t, r.Created)
		}
	}
	assert.True(t, found, "opencode.jsonc should be in results")
}

func TestWriteOpenCodeProject_NoEffort(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block should NOT have reasoningEffort line
	// Find the coder section and check it doesn't contain reasoningEffort
	coderIdx := strings.Index(s, `"coder"`)
	require.Greater(t, coderIdx, 0)
	// Look at the next ~500 chars after "coder" for the effort line
	coderSection := s[coderIdx:min(coderIdx+500, len(s))]
	assert.NotContains(t, coderSection, "reasoningEffort")
}

func TestWriteOpenCodeProject_NoTemp(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: nil, Effort: "medium", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block should NOT have temperature line
	coderIdx := strings.Index(s, `"coder"`)
	require.Greater(t, coderIdx, 0)
	coderSection := s[coderIdx:min(coderIdx+500, len(s))]
	assert.NotContains(t, coderSection, "temperature")
}

func TestWriteOpenCodeProject_SkipsNonOpencodeAgents(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block present (opencode harness)
	assert.Contains(t, s, `"coder"`)
	assert.Contains(t, s, `"anthropic/claude-sonnet-4-6"`)

	// Reviewer block removed (claude harness, not opencode)
	assert.NotContains(t, s, `"reviewer"`)
	assert.NotContains(t, s, `"claude-opus-4-6"`)
}

func ptrFloat(f float64) *float64 { return &f }
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/initcmd/scaffold/ -run TestWriteOpenCodeProject_Generates -v
```

Expected: FAIL — config file not created (current `WriteOpenCodeProject` only scaffolds `.md` files).

**Step 3: Implement `renderOpenCodeConfig`**

Add to `internal/initcmd/scaffold/scaffold.go`:

```go
// renderOpenCodeConfig reads the embedded opencode.jsonc template and substitutes
// wizard-collected values (model, temperature, effort) and dynamic paths (home dir,
// project dir). Agent blocks for roles not using the opencode harness are removed.
func renderOpenCodeConfig(dir string, agents []harness.AgentConfig) (string, error) {
	content, err := templates.ReadFile("templates/opencode/opencode.jsonc")
	if err != nil {
		return "", fmt.Errorf("read opencode.jsonc template: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	rendered := string(content)
	rendered = strings.ReplaceAll(rendered, "{{HOME_DIR}}", homeDir)
	rendered = strings.ReplaceAll(rendered, "{{PROJECT_DIR}}", dir)

	// Build lookup of opencode agents by role
	agentByRole := make(map[string]harness.AgentConfig)
	for _, a := range agents {
		if a.Harness == "opencode" {
			agentByRole[a.Role] = a
		}
	}

	// Substitute per-role placeholders for wizard-configurable agents
	for _, role := range []string{"coder", "planner", "reviewer"} {
		upper := strings.ToUpper(role)
		agent, ok := agentByRole[role]
		if !ok {
			// Remove entire agent block for this role
			rendered = removeJSONBlock(rendered, role)
			continue
		}

		rendered = strings.ReplaceAll(rendered, "{{"+upper+"_MODEL}}", agent.Model)

		// Temperature: bare number or remove line
		if agent.Temperature != nil {
			rendered = strings.ReplaceAll(rendered, "{{"+upper+"_TEMP}}", fmt.Sprintf("%g", *agent.Temperature))
		} else {
			rendered = removeLine(rendered, "{{"+upper+"_TEMP}}")
		}

		// Effort: full line or remove
		if agent.Effort != "" {
			rendered = strings.ReplaceAll(rendered, "{{"+upper+"_EFFORT_LINE}}", fmt.Sprintf(`"reasoningEffort": "%s",`, agent.Effort))
		} else {
			rendered = removeLine(rendered, "{{"+upper+"_EFFORT_LINE}}")
		}
	}

	return rendered, nil
}

// removeLine removes any line containing the given substring.
func removeLine(s, substr string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.Contains(line, substr) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// removeJSONBlock removes a top-level agent block like `"role": { ... }` from the
// JSONC content. Uses brace counting to find the matching closing brace.
// Handles the trailing comma on the previous line or this block's closing line.
func removeJSONBlock(s, role string) string {
	lines := strings.Split(s, "\n")
	marker := fmt.Sprintf(`"%s":`, role)

	startIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), marker) {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return s
	}

	// Find matching closing brace via depth counting
	depth := 0
	endIdx := startIdx
	for i := startIdx; i < len(lines); i++ {
		for _, c := range lines[i] {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
			}
		}
		if depth == 0 {
			endIdx = i
			break
		}
	}

	// Remove trailing comma from line after block, or leading comma handling
	// Check if next line exists and starts the next block — if so, trim trailing comma
	// from endIdx line
	if endIdx+1 < len(lines) {
		trimmed := strings.TrimSpace(lines[endIdx])
		if strings.HasSuffix(trimmed, "},") {
			// Keep the comma handling simple: just remove the block lines
		}
	}

	// Remove lines [startIdx..endIdx] inclusive
	result := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	result = append(result, lines[:startIdx]...)
	result = append(result, lines[endIdx+1:]...)

	return strings.Join(result, "\n")
}
```

**Step 4: Update `WriteOpenCodeProject` to call it**

Replace the existing `WriteOpenCodeProject` function:

```go
// WriteOpenCodeProject scaffolds .opencode/ project files: agent prompts and opencode.jsonc.
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	// Scaffold agent .md files (existing behavior)
	results, err := writePerRoleProject(dir, "opencode", agents, selectedTools, force)
	if err != nil {
		return nil, err
	}

	// Also scaffold the chat agent prompt (always, not wizard-dependent)
	toolsRef := loadFilteredToolsReference(selectedTools)
	chatContent, err := templates.ReadFile("templates/opencode/agents/chat.md")
	if err == nil {
		chatAgent := harness.AgentConfig{Role: "chat", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6"}
		rendered := renderTemplate(string(chatContent), chatAgent, toolsRef)
		chatDest := filepath.Join(dir, ".opencode", "agents", "chat.md")
		written, writeErr := writeFile(chatDest, []byte(rendered), force)
		if writeErr != nil {
			return nil, writeErr
		}
		rel, relErr := filepath.Rel(dir, chatDest)
		if relErr != nil {
			rel = chatDest
		}
		results = append(results, WriteResult{Path: rel, Created: written})
	}

	// Generate opencode.jsonc from template
	configContent, err := renderOpenCodeConfig(dir, agents)
	if err != nil {
		return nil, fmt.Errorf("render opencode.jsonc: %w", err)
	}

	configDest := filepath.Join(dir, ".opencode", "opencode.jsonc")
	written, err := writeFile(configDest, []byte(configContent), force)
	if err != nil {
		return nil, fmt.Errorf("write opencode.jsonc: %w", err)
	}
	rel, relErr := filepath.Rel(dir, configDest)
	if relErr != nil {
		rel = configDest
	}
	results = append(results, WriteResult{Path: rel, Created: written})

	return results, nil
}
```

**Step 5: Run tests**

```bash
go test ./internal/initcmd/scaffold/ -v
```

Expected: all new tests pass, all existing tests still pass.

**Step 6: Commit**

```bash
git add internal/initcmd/scaffold/templates/opencode/opencode.jsonc
git add internal/initcmd/scaffold/templates/opencode/agents/chat.md
git add internal/initcmd/scaffold/scaffold.go
git add internal/initcmd/scaffold/scaffold_test.go
git commit -m "feat(scaffold): generate opencode.jsonc from wizard selections

kq init now generates .opencode/opencode.jsonc with model, temperature,
and effort from wizard selections. Includes role-based permissions,
dynamic HOME/project dir paths, disabled build/plan agents, and a
static chat agent for codebase research."
```

---

## Task 4: Verify end-to-end with existing integration test

**Files:**
- Modify: `internal/initcmd/initcmd_test.go` (verify opencode.jsonc appears in scaffold output)

**Step 1: Read the existing integration test**

Read `internal/initcmd/initcmd_test.go` to understand how it simulates wizard output.

**Step 2: Add assertion for opencode.jsonc**

Add to the existing test (or create new test) that verifies when agents use the opencode harness,
the scaffold output includes `opencode.jsonc`:

```go
func TestRun_OpencodeConfigGenerated(t *testing.T) {
	// This is a scaffold-level check — the integration test in initcmd_test.go
	// already tests the TOML write path. Just verify the config file shows up
	// in ScaffoldAll results when opencode agents are present.
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6",
			Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
	}

	results, err := ScaffoldAll(dir, agents, nil, false)
	require.NoError(t, err)

	var hasConfig bool
	for _, r := range results {
		if r.Path == ".opencode/opencode.jsonc" {
			hasConfig = true
		}
	}
	assert.True(t, hasConfig, "ScaffoldAll should produce opencode.jsonc for opencode agents")
}
```

**Step 3: Run full test suite**

```bash
go test ./internal/initcmd/... -v
```

Expected: all pass.

**Step 4: Commit**

```bash
git add internal/initcmd/scaffold/scaffold_test.go
git commit -m "test(scaffold): verify opencode.jsonc in ScaffoldAll output"
```
