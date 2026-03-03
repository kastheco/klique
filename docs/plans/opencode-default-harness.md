# opencode-default-harness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make opencode the default harness everywhere — registry order, runtime default program, and command detection.

**Architecture:** Change 4 source files and their tests. The harness registry registers opencode first so it appears first in the wizard and becomes the default harness for agent roles. The runtime config falls back to opencode instead of claude. `GetClaudeCommand()` is renamed to `GetDefaultCommand()` and tries opencode first, then claude.

**Tech Stack:** Go, testify

---

### Task 1: Change registry order so opencode is first

**Files:**
- Modify: `internal/initcmd/harness/harness.go:35-37`
- Modify: `internal/initcmd/harness/harness_test.go:21,27-29`

**Step 1: Write the failing test update**

In `harness_test.go`, update the order assertions:

```go
t.Run("All returns stable order", func(t *testing.T) {
    assert.Equal(t, []string{"opencode", "claude", "codex"}, r.All())
})
```

And in the `DetectAll` subtest:

```go
assert.Equal(t, "opencode", results[0].Name)
assert.Equal(t, "claude", results[1].Name)
assert.Equal(t, "codex", results[2].Name)
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/harness/ -run TestNewRegistry -v`
Expected: FAIL — order is still claude, opencode, codex

**Step 3: Swap registration order**

In `harness.go` `NewRegistry()`, change:

```go
func NewRegistry() *Registry {
	r := &Registry{harnesses: make(map[string]Harness)}
	r.Register(&OpenCode{})
	r.Register(&Claude{})
	r.Register(&Codex{})
	return r
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/harness/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/harness/harness.go internal/initcmd/harness/harness_test.go
git commit -m "feat: register opencode as first harness in registry"
```

---

### Task 2: Rename GetClaudeCommand to GetDefaultCommand with multi-binary detection

**Files:**
- Modify: `config/config.go:15-18,84-89,126-172`
- Modify: `config/config_test.go:23-88`

**Step 1: Write the failing test**

Rename `TestGetClaudeCommand` to `TestGetDefaultCommand` and update assertions. The function should find opencode first, fall back to claude:

```go
func TestGetDefaultCommand(t *testing.T) {
	t.Run("finds opencode in PATH", func(t *testing.T) {
		originalPath := os.Getenv("PATH")
		tempDir := t.TempDir()
		opencodePath := filepath.Join(tempDir, "opencode")

		err := os.WriteFile(opencodePath, []byte("#!/bin/bash\necho 'mock opencode'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir+":"+originalPath)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "opencode"))
	})

	t.Run("falls back to claude when opencode not found", func(t *testing.T) {
		tempDir := t.TempDir()
		claudePath := filepath.Join(tempDir, "claude")

		err := os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "claude"))
	})

	t.Run("handles neither command found", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetDefaultCommand()

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("prefers opencode over claude when both exist", func(t *testing.T) {
		originalPath := os.Getenv("PATH")
		tempDir := t.TempDir()

		opencodePath := filepath.Join(tempDir, "opencode")
		err := os.WriteFile(opencodePath, []byte("#!/bin/bash\necho 'mock opencode'"), 0755)
		require.NoError(t, err)

		claudePath := filepath.Join(tempDir, "claude")
		err = os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir+":"+originalPath)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "opencode"))
	})

	t.Run("handles alias parsing", func(t *testing.T) {
		aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)

		output := "opencode: aliased to /usr/local/bin/opencode"
		matches := aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 2)
		assert.Equal(t, "/usr/local/bin/opencode", matches[1])

		output = "/usr/local/bin/opencode"
		matches = aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 0)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/ -run TestGetDefaultCommand -v`
Expected: FAIL — `GetDefaultCommand` does not exist

**Step 3: Implement GetDefaultCommand and update constants**

In `config.go`, replace the constant and function:

```go
const (
	ConfigFileName = "config.json"
	defaultProgram = "opencode"
)
```

Replace `GetClaudeCommand` with:

```go
// GetDefaultCommand finds the preferred agent CLI binary.
// Tries opencode first, falls back to claude. Checks shell aliases then PATH.
func GetDefaultCommand() (string, error) {
	// Try opencode first (preferred)
	if path, err := findCommand("opencode"); err == nil {
		return path, nil
	}

	// Fall back to claude
	if path, err := findCommand("claude"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("neither opencode nor claude command found in aliases or PATH")
}

// findCommand attempts to locate a CLI binary by name.
// Checks shell aliases first, then falls back to exec.LookPath.
func findCommand(name string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; which %s", name)
	} else if strings.Contains(shell, "bash") {
		shellCmd = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; which %s", name)
	} else {
		shellCmd = fmt.Sprintf("which %s", name)
	}

	cmd := exec.Command(shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	binPath, err := exec.LookPath(name)
	if err == nil {
		return binPath, nil
	}

	return "", fmt.Errorf("%s command not found in aliases or PATH", name)
}
```

Update `DefaultConfig()`:

```go
func DefaultConfig() *Config {
	program, err := GetDefaultCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get default command: %v", err)
		program = defaultProgram
	}
	// ... rest unchanged
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat: rename GetClaudeCommand to GetDefaultCommand, prefer opencode"
```

---

### Task 3: Run full test suite and fix any fallout

**Files:**
- Possibly: `internal/initcmd/wizard/wizard_test.go` (if any tests hardcode order assumptions)

**Step 1: Run full test suite**

Run: `go test ./... -count=1`

**Step 2: Fix any failures**

Scan output for failures related to harness ordering or default program assumptions. The wizard tests in `wizard_test.go` use explicit harness values in their test fixtures so they should be unaffected, but verify.

**Step 3: Run full suite again**

Run: `go test ./... -count=1`
Expected: All PASS

**Step 4: Commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: update tests for opencode-first default"
```
