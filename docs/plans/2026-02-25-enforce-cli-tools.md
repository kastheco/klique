# Enforce CLI-tools implementation plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically install enforcement hooks during `kas init` that block banned CLI tools (grep, sed, awk, diff, wc) and tell agents which modern replacement to use instead.

**Architecture:** New `InstallEnforcement() error` method on the Harness interface. Claude uses a PreToolUse bash hook + settings.json merge. opencode uses a `tool.execute.before` plugin. Both share the same banned-tools table. Codex gets a no-op.

**Tech Stack:** Go (harness interface, settings.json merge), Bash (Claude hook script), JavaScript (opencode plugin)

**Design doc:** `docs/plans/2026-02-25-enforce-cli-tools-design.md`

---

### Task 1: Add InstallEnforcement to Harness interface

**Files:**
- Modify: `internal/initcmd/harness/harness.go:14-24`
- Modify: `internal/initcmd/harness/codex.go`
- Modify: `internal/initcmd/harness/harness_test.go`

**Step 1: Write the failing test**

Add to `internal/initcmd/harness/harness_test.go`:

```go
func TestCodexAdapter_InstallEnforcement(t *testing.T) {
	c := &Codex{}
	assert.NoError(t, c.InstallEnforcement())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/harness/ -run TestCodexAdapter_InstallEnforcement -v`
Expected: FAIL — `InstallEnforcement` not defined

**Step 3: Add method to Harness interface and Codex no-op**

In `internal/initcmd/harness/harness.go`, add to the `Harness` interface (line 20, after `InstallSuperpowers`):

```go
InstallEnforcement() error
```

In `internal/initcmd/harness/codex.go`, add:

```go
func (c *Codex) InstallEnforcement() error { return nil }
```

Add stub methods to `claude.go` and `opencode.go` so the project compiles:

```go
// claude.go
func (c *Claude) InstallEnforcement() error { return nil }

// opencode.go
func (o *OpenCode) InstallEnforcement() error { return nil }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/harness/ -v`
Expected: PASS — all existing tests plus new one

**Step 5: Commit**

```
feat(harness): add InstallEnforcement interface method with codex no-op
```

---

### Task 2: Embed Claude hook script template

**Files:**
- Create: `internal/initcmd/scaffold/templates/claude/enforce-cli-tools.sh`

**Step 1: Write the embedded bash script**

Create `internal/initcmd/scaffold/templates/claude/enforce-cli-tools.sh`:

```bash
#!/bin/bash
# PreToolUse hook: block legacy CLI tools, enforce modern replacements.
# Installed by kasmos init. Source of truth: cli-tools skill.
# Reads Bash tool_input.command from stdin JSON and rejects banned commands.

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

[ -z "$COMMAND" ] && exit 0

# grep -> rg (ripgrep)
# Word-boundary match avoids false positives (e.g. ast-grep)
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bgrep\b'; then
  echo "BLOCKED: 'grep' is banned. Use 'rg' (ripgrep) instead. rg is faster, respects .gitignore, and has better defaults." >&2
  exit 2
fi

# sed -> sd or comby
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bsed\b'; then
  echo "BLOCKED: 'sed' is banned. Use 'sd' for simple replacements or 'comby' for structural/multi-line rewrites." >&2
  exit 2
fi

# awk -> yq/jq, sd, or comby
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bawk\b'; then
  echo "BLOCKED: 'awk' is banned. Use 'yq'/'jq' for structured data, 'sd' for text, or 'comby' for code patterns." >&2
  exit 2
fi

# standalone diff (not git diff) -> difft
if echo "$COMMAND" | grep -qP '(^|[|;&`]\s*|\$\(\s*)\bdiff\b' && \
   ! echo "$COMMAND" | grep -qP '\bgit\s+diff\b'; then
  echo "BLOCKED: standalone 'diff' is banned. Use 'difft' (difftastic) for syntax-aware structural diffs. 'git diff' is allowed." >&2
  exit 2
fi

# wc -l -> scc
if echo "$COMMAND" | grep -qP '\bwc\s+(-\w*l|--lines)\b|\bwc\b.*\s-l\b'; then
  echo "BLOCKED: 'wc -l' is banned. Use 'scc' for language-aware line counts with complexity estimates." >&2
  exit 2
fi

exit 0
```

**Step 2: Verify the template embeds**

Run: `go build ./...`
Expected: compiles (embed.FS picks up new file automatically since `templates/` is already embedded)

**Step 3: Commit**

```
feat(scaffold): add claude enforce-cli-tools hook script template
```

---

### Task 3: Implement Claude InstallEnforcement

**Files:**
- Modify: `internal/initcmd/harness/claude.go`
- Modify: `internal/initcmd/harness/harness_test.go`

**Step 1: Write the failing test**

Add to `internal/initcmd/harness/harness_test.go`:

```go
func TestClaudeAdapter_InstallEnforcement(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	c := &Claude{}
	require.NoError(t, c.InstallEnforcement())

	// Hook script written and executable
	hookPath := filepath.Join(tmpHome, ".claude", "hooks", "enforce-cli-tools.sh")
	assert.FileExists(t, hookPath)
	info, err := os.Stat(hookPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0o111 != 0, "hook script must be executable")

	// settings.json has PreToolUse entry
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	assert.FileExists(t, settingsPath)
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "enforce-cli-tools.sh")
	assert.Contains(t, string(data), "PreToolUse")

	// Idempotent: running again doesn't duplicate
	require.NoError(t, c.InstallEnforcement())
	data2, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data2), "enforce-cli-tools.sh"),
		"must not duplicate hook entry on re-run")
}

func TestClaudeAdapter_InstallEnforcement_PreservesExisting(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Pre-populate settings.json with existing hooks
	claudeDir := filepath.Join(tmpHome, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	existing := `{
  "hooks": {
    "Notification": [
      {
        "matcher": "permission_prompt",
        "hooks": [{ "type": "command", "command": "notify.sh" }]
      }
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644))

	c := &Claude{}
	require.NoError(t, c.InstallEnforcement())

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)
	// Both old and new hooks present
	assert.Contains(t, string(data), "notify.sh")
	assert.Contains(t, string(data), "enforce-cli-tools.sh")
	assert.Contains(t, string(data), "PreToolUse")
	assert.Contains(t, string(data), "Notification")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/harness/ -run TestClaudeAdapter_InstallEnforcement -v`
Expected: FAIL — stub returns nil without writing files

**Step 3: Implement Claude.InstallEnforcement**

Replace the stub in `internal/initcmd/harness/claude.go` with the real implementation. The method must:

1. Get home dir
2. Read embedded `templates/claude/enforce-cli-tools.sh` from the scaffold package (import the scaffold embed or duplicate the embed — see note below)
3. Write to `~/.claude/hooks/enforce-cli-tools.sh` with 0o755
4. Read `~/.claude/settings.json` (create `{"hooks":{}}` if missing)
5. Parse as `map[string]any`
6. Navigate to `hooks.PreToolUse` (create if missing)
7. Check if any entry already references `enforce-cli-tools.sh`
8. If not, append the matcher group
9. Marshal back with `json.MarshalIndent` (2-space)
10. Write settings.json

**Important:** The hook script template is embedded in the `scaffold` package. To avoid a circular import, either:
- (a) Move the template to a shared `internal/initcmd/enforcement/` package that both `harness` and `scaffold` can import, or
- (b) Embed the script directly in `claude.go` as a const string, or
- (c) Have `InstallEnforcement` accept parameters from the caller (initcmd.Run) that passes the embedded content.

Option (b) is simplest — embed the script as a `const` or `var` in `claude.go`. The script is small and self-contained. If it diverges from the template, that's a sign to refactor later.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/harness/ -run TestClaudeAdapter_InstallEnforcement -v`
Expected: PASS

**Step 5: Commit**

```
feat(claude): implement InstallEnforcement — hook script + settings.json merge
```

---

### Task 4: Implement opencode InstallEnforcement

**Files:**
- Modify: `internal/initcmd/harness/opencode.go`
- Modify: `internal/initcmd/harness/harness_test.go`

**Step 1: Write the failing test**

Add to `internal/initcmd/harness/harness_test.go`:

```go
func TestOpenCodeAdapter_InstallEnforcement(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	o := &OpenCode{}
	require.NoError(t, o.InstallEnforcement())

	// Plugin file written
	pluginPath := filepath.Join(tmpHome, ".config", "opencode", "plugins", "enforce-cli-tools.js")
	assert.FileExists(t, pluginPath)
	data, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "tool.execute.before")
	assert.Contains(t, string(data), "grep")
	assert.Contains(t, string(data), "rg")

	// Idempotent: running again overwrites without error
	require.NoError(t, o.InstallEnforcement())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/harness/ -run TestOpenCodeAdapter_InstallEnforcement -v`
Expected: FAIL — stub returns nil without writing files

**Step 3: Implement OpenCode.InstallEnforcement**

Replace the stub in `internal/initcmd/harness/opencode.go`. The method must:

1. Get home dir
2. Create `~/.config/opencode/plugins/` if needed
3. Write the JS plugin file (always overwrite — we own this file)

The JS plugin content as a Go const string:

```js
/**
 * CLI-tools enforcement plugin for OpenCode.
 * Installed by kasmos init. Blocks banned CLI tools and suggests replacements.
 */
export const EnforceCLIToolsPlugin = async ({ client, directory }) => {
  const BANNED = [
    { pattern: /(^|[|;&`]\s*|\$\(\s*)\bgrep\b/, name: "grep", replacement: "rg", reason: "rg is faster, respects .gitignore, and has better defaults" },
    { pattern: /(^|[|;&`]\s*|\$\(\s*)\bsed\b/, name: "sed", replacement: "sd or comby", reason: "sd for simple replacements, comby for structural/multi-line rewrites" },
    { pattern: /(^|[|;&`]\s*|\$\(\s*)\bawk\b/, name: "awk", replacement: "yq/jq, sd, or comby", reason: "yq/jq for structured data, sd for text, comby for code patterns" },
    { pattern: /\bwc\s+(-\w*l|--lines)\b|\bwc\b.*\s-l\b/, name: "wc -l", replacement: "scc", reason: "scc provides language-aware line counts with complexity estimates" },
  ];

  // diff needs special handling (allow git diff)
  const DIFF_PATTERN = /(^|[|;&`]\s*|\$\(\s*)\bdiff\b/;
  const GIT_DIFF_PATTERN = /\bgit\s+diff\b/;

  const checkCommand = (cmd) => {
    for (const { pattern, name, replacement, reason } of BANNED) {
      if (pattern.test(cmd)) {
        return `BLOCKED: '${name}' is banned. Use '${replacement}' instead. ${reason}.`;
      }
    }
    if (DIFF_PATTERN.test(cmd) && !GIT_DIFF_PATTERN.test(cmd)) {
      return "BLOCKED: standalone 'diff' is banned. Use 'difft' (difftastic) for syntax-aware structural diffs. 'git diff' is allowed.";
    }
    return null;
  };

  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      const cmd = output.args?.command;
      if (!cmd) return;
      const blocked = checkCommand(cmd);
      if (blocked) {
        output.args.command = `echo '${blocked.replace(/'/g, "'\\''")}' >&2; exit 2`;
      }
    },
  };
};
```

Note: Uses the args-mutation fallback strategy (rewrite command to echo error + exit 2). This is guaranteed to work regardless of whether opencode supports throw-to-block.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/harness/ -run TestOpenCodeAdapter_InstallEnforcement -v`
Expected: PASS

**Step 5: Commit**

```
feat(opencode): implement InstallEnforcement — tool.execute.before plugin
```

---

### Task 5: Wire InstallEnforcement into kas init

**Files:**
- Modify: `internal/initcmd/initcmd.go:39-53`

**Step 1: Add the enforcement stage**

In `internal/initcmd/initcmd.go`, after the "Syncing personal skills..." block (line 69) and before "Writing config..." (line 72), add:

```go
// Stage 4a-3: Install CLI-tools enforcement hooks
fmt.Println("\nInstalling enforcement hooks...")
for _, name := range state.SelectedHarness {
	h := registry.Get(name)
	if h == nil {
		continue
	}
	fmt.Printf("  %-12s ", name)
	if err := h.InstallEnforcement(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		// Non-fatal: continue with other harnesses
	} else {
		fmt.Println("OK")
	}
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Run all tests**

Run: `go test ./internal/initcmd/... -v`
Expected: PASS

**Step 4: Commit**

```
feat(init): wire InstallEnforcement into kas init pipeline
```

---

### Task 6: End-to-end validation

**Step 1: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 2: Build binary**

Run: `go build -o /tmp/kas-test ./`
Expected: compiles

**Step 3: Manual smoke test (optional)**

In a temp directory with `HOME` overridden, run `kas init` (if feasible without interactive wizard), or verify the individual `InstallEnforcement` methods produce correct output by inspecting the files they write.

**Step 4: Commit (if any fixups needed)**

```
fix(enforcement): address test/build issues from integration
```
