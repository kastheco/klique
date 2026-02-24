# `kq init` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the `kq init` multi-harness agent environment setup wizard that detects CLIs, configures agents via interactive huh forms, writes TOML config, installs superpowers, and scaffolds per-harness project files.

**Architecture:** A new `internal/init/` package tree with three sub-packages (harness, wizard, scaffold) plus a TOML config layer in `config/`. The harness package provides a `Harness` interface with adapters for claude/opencode/codex. The wizard package builds multi-stage huh forms. The scaffold package writes per-harness project files from Go `embed.FS` templates. A cobra subcommand in `main.go` ties it together.

**Tech Stack:** Go 1.24+, charmbracelet/huh (forms), BurntSushi/toml (config parsing), cobra (CLI), embed (templates)

**Design doc:** `docs/plans/2026-02-20-kq-init-design.md`

---

## Architecture Decisions

### AD-1: TOML library choice - BurntSushi/toml

**Decision:** Use `github.com/BurntSushi/toml` over `pelletier/go-toml/v2`.

**Rationale:** BurntSushi/toml has simpler API for marshal/unmarshal, handles `map[string]T` nested tables natively (which we need for `[agents.*]`), and is the de-facto standard Go TOML library. pelletier/go-toml/v2 is faster but we're parsing a tiny config file -- simplicity wins.

### AD-2: TOML config coexists with JSON config

**Decision:** TOML file at `~/.klique/config.toml` is a separate file from `~/.klique/config.json`. The TOML file is the authority for agent profiles and phase mappings. The JSON file remains for legacy settings (default_program, auto_yes, etc.).

**Rationale:** The design doc states "TOML parser is an additional config source, not a replacement." The task-orchestration branch already has `Profiles`/`PhaseRoles` on the JSON Config struct -- we add a `LoadTOMLConfig()` that populates those same fields. `LoadConfig()` merges: JSON first, then TOML overlays `Profiles` and `PhaseRoles` if the TOML file exists. This avoids breaking existing config consumers.

### AD-3: AgentProfile extended with new fields

**Decision:** Add `Model string`, `Temperature *float64`, `Effort string`, `Enabled bool` to `AgentProfile`. The `BuildCommand()` method delegates to the harness adapter's `BuildFlags()` to fold these into the flags slice.

**Rationale:** The design doc specifies these fields in the TOML config. Making them struct fields (not just opaque flags) enables the wizard to pre-populate forms from existing config and enables harness-specific flag generation.

### AD-4: huh form stages as separate functions

**Decision:** Each wizard stage (harness detection, agent config, phase mapping, summary) is a standalone function returning a `*huh.Form`. The orchestrator runs them sequentially, passing state between stages.

**Rationale:** huh forms are stateless once Run() completes -- values are written to bound variables. Separate functions allow independent testing and keep each form under ~50 lines. The alternative (one giant form with all groups) doesn't support the inter-stage logic we need (e.g., stage 2 options depend on stage 1 selections).

### AD-5: Shared tools-reference in all agent templates

**Decision:** All scaffolded agent instruction files include a "Available CLI Tools" section that documents enhanced developer tools (ast-grep, difftastic, sd, scc, yq, comby, typos) when-available patterns. This section is identical across all harness formats -- stored once in `templates/shared/tools-reference.md` and injected at scaffold time via a `{{TOOLS_REFERENCE}}` placeholder.

**Rationale:** These tools dramatically improve agent code quality: ast-grep and comby enable structural search/replace instead of regex hacks, difftastic gives semantic diffs instead of line-based noise, sd replaces sed/awk with sane syntax, scc gives instant codebase metrics, yq handles YAML/TOML/JSON manipulation natively, and typos catches spelling errors in identifiers. Making agents aware of them is the difference between "use sed to rename" and "use ast-grep to structurally refactor". The instructions are model-agnostic (no agent-CLI-specific APIs) so they work identically across claude, opencode, and codex.

### AD-6: embed.FS for scaffold templates

**Decision:** Canonical agent/skill templates live in `internal/init/scaffold/templates/` and are embedded via `//go:embed templates/*`. Each harness adapter's `ScaffoldProject()` reads from this embedded FS and writes to the appropriate project directories.

**Rationale:** Follows the kasmos setup precedent (`config.DefaultProfile` embed.FS pattern in `internal/setup/agents.go`). Keeps templates versioned with the binary, no runtime file discovery needed.

---

## Work Packages

### Dependency Graph

```
WP01 (harness interface + adapters)
  |
  v
WP02 (TOML config layer) --- depends on WP01 for AgentProfile fields
  |
  v
WP03 (huh wizard) ---------- depends on WP01 (Harness interface), WP02 (config types)
  |
  v
WP04 (scaffold + superpowers) - depends on WP01 (Harness.ScaffoldProject/InstallSuperpowers)
  |
  v
WP05 (cobra command + integration) - depends on all above
```

WP01 and WP02 can be developed in parallel (WP02 only needs the AgentProfile type additions, which WP01 defines). WP03 and WP04 depend on WP01+WP02. WP05 ties everything together.

---

## Task 1: Harness Interface and Adapters (WP01)

**Files:**
- Create: `internal/init/harness/harness.go`
- Create: `internal/init/harness/claude.go`
- Create: `internal/init/harness/opencode.go`
- Create: `internal/init/harness/codex.go`
- Create: `internal/init/harness/harness_test.go`
- Modify: `go.mod` (no new deps needed for this WP)

### Step 1: Write the Harness interface and Registry

Create `internal/init/harness/harness.go`:

```go
package harness

// AgentConfig holds the wizard-collected configuration for one agent role.
// This is the wizard's view -- it gets mapped to config.AgentProfile on write.
type AgentConfig struct {
	Role        string   // "coder", "reviewer", "planner", or custom
	Harness     string   // "claude", "opencode", "codex"
	Model       string
	Temperature *float64 // nil = harness default
	Effort      string   // "" = harness default
	Enabled     bool
	ExtraFlags  []string
}

// Harness defines the interface each supported CLI adapter must implement.
type Harness interface {
	Name() string
	Detect() (path string, found bool)
	ListModels() ([]string, error)
	BuildFlags(agent AgentConfig) []string
	InstallSuperpowers() error
	ScaffoldProject(dir string, agents []AgentConfig, force bool) error
	SupportsTemperature() bool
	SupportsEffort() bool
}

// Registry holds all known harness adapters keyed by name.
type Registry struct {
	harnesses map[string]Harness
}

// NewRegistry creates a registry with all built-in harness adapters.
func NewRegistry() *Registry {
	r := &Registry{harnesses: make(map[string]Harness)}
	r.Register(&Claude{})
	r.Register(&OpenCode{})
	r.Register(&Codex{})
	return r
}

// Register adds a harness adapter to the registry.
func (r *Registry) Register(h Harness) {
	r.harnesses[h.Name()] = h
}

// Get returns the harness adapter for the given name, or nil.
func (r *Registry) Get(name string) Harness {
	return r.harnesses[name]
}

// All returns all registered harness names in stable order.
func (r *Registry) All() []string {
	return []string{"claude", "opencode", "codex"}
}

// DetectAll probes each harness and returns detection results.
type DetectResult struct {
	Name  string
	Path  string
	Found bool
}

func (r *Registry) DetectAll() []DetectResult {
	var results []DetectResult
	for _, name := range r.All() {
		h := r.harnesses[name]
		path, found := h.Detect()
		results = append(results, DetectResult{Name: name, Path: path, Found: found})
	}
	return results
}
```

### Step 2: Write failing tests for the Registry

Create `internal/init/harness/harness_test.go`:

```go
package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	t.Run("registers all built-in harnesses", func(t *testing.T) {
		assert.NotNil(t, r.Get("claude"))
		assert.NotNil(t, r.Get("opencode"))
		assert.NotNil(t, r.Get("codex"))
		assert.Nil(t, r.Get("nonexistent"))
	})

	t.Run("All returns stable order", func(t *testing.T) {
		assert.Equal(t, []string{"claude", "opencode", "codex"}, r.All())
	})

	t.Run("DetectAll returns results for every harness", func(t *testing.T) {
		results := r.DetectAll()
		require.Len(t, results, 3)
		assert.Equal(t, "claude", results[0].Name)
		assert.Equal(t, "opencode", results[1].Name)
		assert.Equal(t, "codex", results[2].Name)
	})
}
```

### Step 3: Run tests to verify they fail

```bash
go test ./internal/init/harness/ -v -run TestNewRegistry
```

Expected: FAIL (Claude/OpenCode/Codex structs don't exist yet)

### Step 4: Implement the Claude adapter

Create `internal/init/harness/claude.go`:

```go
package harness

import (
	"fmt"
	"os/exec"
	"strings"
)

// Claude implements Harness for the Claude Code CLI.
type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) Detect() (string, bool) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels returns the static set of Claude models.
// Claude Code doesn't have a CLI command to list models dynamically.
func (c *Claude) ListModels() ([]string, error) {
	return []string{
		"claude-sonnet-4-6",
		"claude-opus-4-6",
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
	}, nil
}

func (c *Claude) BuildFlags(agent AgentConfig) []string {
	var flags []string
	if agent.Model != "" {
		flags = append(flags, "--model", agent.Model)
	}
	if agent.Effort != "" {
		flags = append(flags, "--effort", agent.Effort)
	}
	flags = append(flags, agent.ExtraFlags...)
	return flags
}

func (c *Claude) InstallSuperpowers() error {
	// Check if already installed
	out, err := exec.Command("claude", "plugin", "list").Output()
	if err == nil && strings.Contains(string(out), "superpowers") {
		return nil // already installed
	}

	// Add marketplace
	cmd := exec.Command("claude", "plugin", "marketplace", "add", "obra/superpowers-marketplace")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add marketplace: %s: %w", string(out), err)
	}

	// Install plugin
	cmd = exec.Command("claude", "plugin", "install", "superpowers@superpowers-marketplace")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install plugin: %s: %w", string(out), err)
	}

	return nil
}

func (c *Claude) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Implemented in WP04
	return nil
}

func (c *Claude) SupportsTemperature() bool { return false }
func (c *Claude) SupportsEffort() bool      { return true }
```

### Step 5: Implement the OpenCode adapter

Create `internal/init/harness/opencode.go`:

```go
package harness

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenCode implements Harness for the OpenCode CLI.
type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) Detect() (string, bool) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels shells out to `opencode models` and parses the output line-by-line.
func (o *OpenCode) ListModels() ([]string, error) {
	cmd := exec.Command("opencode", "models")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("opencode models: %w", err)
	}

	var models []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			models = append(models, line)
		}
	}
	return models, scanner.Err()
}

func (o *OpenCode) BuildFlags(agent AgentConfig) []string {
	// opencode uses project config (opencode.json), not CLI flags for model/temp/effort
	return agent.ExtraFlags
}

func (o *OpenCode) InstallSuperpowers() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")

	// Clone or pull
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		cmd := exec.Command("git", "clone",
			"https://github.com/obra/superpowers.git", repoDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone superpowers: %s: %w", string(out), err)
		}
	} else {
		cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
		_ = cmd.Run() // best-effort update
	}

	// Symlink plugin
	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}
	pluginLink := filepath.Join(pluginDir, "superpowers.js")
	pluginSrc := filepath.Join(repoDir, ".opencode", "plugins", "superpowers.js")
	os.Remove(pluginLink) // remove existing link/file
	if err := os.Symlink(pluginSrc, pluginLink); err != nil {
		return fmt.Errorf("symlink plugin: %w", err)
	}

	// Symlink skills
	skillsDir := filepath.Join(home, ".config", "opencode", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	skillsLink := filepath.Join(skillsDir, "superpowers")
	skillsSrc := filepath.Join(repoDir, "skills")
	os.Remove(skillsLink) // remove existing link
	if err := os.Symlink(skillsSrc, skillsLink); err != nil {
		return fmt.Errorf("symlink skills: %w", err)
	}

	return nil
}

func (o *OpenCode) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Implemented in WP04
	return nil
}

func (o *OpenCode) SupportsTemperature() bool { return true }
func (o *OpenCode) SupportsEffort() bool      { return true }
```

### Step 6: Implement the Codex adapter

Create `internal/init/harness/codex.go`:

```go
package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Codex implements Harness for the Codex CLI.
type Codex struct{}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) Detect() (string, bool) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return "", false
	}
	return path, true
}

// ListModels returns a default suggestion. Codex accepts free-text model names.
func (c *Codex) ListModels() ([]string, error) {
	return []string{"gpt-5.3-codex"}, nil
}

func (c *Codex) BuildFlags(agent AgentConfig) []string {
	var flags []string
	if agent.Model != "" {
		flags = append(flags, "-m", agent.Model)
	}
	if agent.Effort != "" {
		flags = append(flags, "-c", fmt.Sprintf("reasoning.effort=%s", agent.Effort))
	}
	if agent.Temperature != nil {
		flags = append(flags, "-c", fmt.Sprintf("temperature=%g", *agent.Temperature))
	}
	flags = append(flags, agent.ExtraFlags...)
	return flags
}

func (c *Codex) InstallSuperpowers() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	repoDir := filepath.Join(home, ".codex", "superpowers")

	// Clone or pull
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return fmt.Errorf("create codex dir: %w", err)
		}
		cmd := exec.Command("git", "clone",
			"https://github.com/obra/superpowers.git", repoDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone superpowers: %s: %w", string(out), err)
		}
	} else {
		cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
		_ = cmd.Run() // best-effort update
	}

	// Symlink skills
	skillsDir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	skillsLink := filepath.Join(skillsDir, "superpowers")
	skillsSrc := filepath.Join(repoDir, "skills")
	os.Remove(skillsLink)
	if err := os.Symlink(skillsSrc, skillsLink); err != nil {
		return fmt.Errorf("symlink skills: %w", err)
	}

	return nil
}

func (c *Codex) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Implemented in WP04
	return nil
}

func (c *Codex) SupportsTemperature() bool { return true }
func (c *Codex) SupportsEffort() bool      { return true }
```

### Step 7: Write adapter-specific tests

Add to `internal/init/harness/harness_test.go`:

```go
func TestClaudeAdapter(t *testing.T) {
	c := &Claude{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "claude", c.Name())
	})

	t.Run("ListModels returns static list", func(t *testing.T) {
		models, err := c.ListModels()
		require.NoError(t, err)
		assert.Contains(t, models, "claude-sonnet-4-6")
		assert.Contains(t, models, "claude-opus-4-6")
		assert.Len(t, models, 4)
	})

	t.Run("BuildFlags with model and effort", func(t *testing.T) {
		flags := c.BuildFlags(AgentConfig{
			Model:  "claude-opus-4-6",
			Effort: "high",
		})
		assert.Equal(t, []string{"--model", "claude-opus-4-6", "--effort", "high"}, flags)
	})

	t.Run("BuildFlags skips empty fields", func(t *testing.T) {
		flags := c.BuildFlags(AgentConfig{})
		assert.Empty(t, flags)
	})

	t.Run("SupportsTemperature is false", func(t *testing.T) {
		assert.False(t, c.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, c.SupportsEffort())
	})
}

func TestCodexAdapter(t *testing.T) {
	c := &Codex{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "codex", c.Name())
	})

	t.Run("ListModels returns default", func(t *testing.T) {
		models, err := c.ListModels()
		require.NoError(t, err)
		assert.Equal(t, []string{"gpt-5.3-codex"}, models)
	})

	t.Run("BuildFlags with all fields", func(t *testing.T) {
		temp := 0.3
		flags := c.BuildFlags(AgentConfig{
			Model:       "gpt-5.3-codex",
			Effort:      "high",
			Temperature: &temp,
		})
		assert.Equal(t, []string{
			"-m", "gpt-5.3-codex",
			"-c", "reasoning.effort=high",
			"-c", "temperature=0.3",
		}, flags)
	})

	t.Run("SupportsTemperature is true", func(t *testing.T) {
		assert.True(t, c.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, c.SupportsEffort())
	})
}

func TestOpenCodeAdapter(t *testing.T) {
	o := &OpenCode{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "opencode", o.Name())
	})

	t.Run("BuildFlags returns only extra flags", func(t *testing.T) {
		flags := o.BuildFlags(AgentConfig{
			Model:      "anthropic/claude-sonnet-4-6",
			ExtraFlags: []string{"--verbose"},
		})
		// opencode uses project config, not CLI flags for model
		assert.Equal(t, []string{"--verbose"}, flags)
	})

	t.Run("SupportsTemperature is true", func(t *testing.T) {
		assert.True(t, o.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, o.SupportsEffort())
	})
}
```

### Step 8: Run tests to verify they pass

```bash
go test ./internal/init/harness/ -v
```

Expected: PASS

### Step 9: Commit

```bash
git add internal/init/harness/
git commit -m "feat(init): add harness interface and claude/opencode/codex adapters"
```

---

## Task 2: TOML Config Layer (WP02)

**Files:**
- Modify: `config/config.go` (add TOML loading, extend AgentProfile)
- Modify: `config/profile.go` (add Model, Temperature, Effort, Enabled fields)
- Create: `config/toml.go` (TOML-specific types and parser)
- Create: `config/toml_test.go`
- Modify: `go.mod` / `go.sum` (add BurntSushi/toml)

**Depends on:** WP01 conceptually (AgentProfile field additions), but can be developed in parallel since the fields are defined here in config/, not in harness/.

### Step 1: Add the TOML dependency

```bash
go get github.com/BurntSushi/toml@latest
```

### Step 2: Write failing tests for TOML config loading

Create `config/toml_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTOMLConfig(t *testing.T) {
	t.Run("parses valid TOML with agents and phases", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")

		content := `
[phases]
implementing = "coder"
spec_review = "reviewer"
quality_review = "reviewer"
planning = "planner"

[agents.coder]
enabled = true
program = "opencode"
model = "anthropic/claude-sonnet-4-6"
temperature = 0.7
effort = "high"
flags = []

[agents.reviewer]
enabled = true
program = "claude"
model = "claude-opus-4-6"
effort = "high"
flags = ["--agent", "reviewer"]

[agents.planner]
enabled = false
program = "codex"
model = "gpt-5.3-codex"
flags = []
`
		err := os.WriteFile(tomlPath, []byte(content), 0o644)
		require.NoError(t, err)

		tc, err := LoadTOMLConfigFrom(tomlPath)
		require.NoError(t, err)

		// Verify phases
		assert.Equal(t, "coder", tc.PhaseRoles["implementing"])
		assert.Equal(t, "reviewer", tc.PhaseRoles["spec_review"])
		assert.Equal(t, "planner", tc.PhaseRoles["planning"])

		// Verify agent profiles
		coder, ok := tc.Profiles["coder"]
		require.True(t, ok)
		assert.Equal(t, "opencode", coder.Program)
		assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
		assert.NotNil(t, coder.Temperature)
		assert.InDelta(t, 0.7, *coder.Temperature, 0.001)
		assert.Equal(t, "high", coder.Effort)
		assert.True(t, coder.Enabled)

		// Verify disabled agent
		planner, ok := tc.Profiles["planner"]
		require.True(t, ok)
		assert.False(t, planner.Enabled)

		// Verify flags preserved
		reviewer, ok := tc.Profiles["reviewer"]
		require.True(t, ok)
		assert.Equal(t, []string{"--agent", "reviewer"}, reviewer.Flags)
	})

	t.Run("returns error on missing file", func(t *testing.T) {
		_, err := LoadTOMLConfigFrom("/nonexistent/config.toml")
		assert.Error(t, err)
	})

	t.Run("returns error on invalid TOML", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")
		err := os.WriteFile(tomlPath, []byte("[invalid toml\n"), 0o644)
		require.NoError(t, err)

		_, err = LoadTOMLConfigFrom(tomlPath)
		assert.Error(t, err)
	})
}

func TestSaveTOMLConfig(t *testing.T) {
	t.Run("round-trips through save and load", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")

		temp := 0.5
		original := &TOMLConfig{
			Phases: map[string]string{
				"implementing": "coder",
				"planning":     "planner",
			},
			Agents: map[string]TOMLAgent{
				"coder": {
					Enabled:     true,
					Program:     "opencode",
					Model:       "anthropic/claude-sonnet-4-6",
					Temperature: &temp,
					Effort:      "high",
					Flags:       []string{},
				},
			},
		}

		err := SaveTOMLConfigTo(original, tomlPath)
		require.NoError(t, err)

		loaded, err := LoadTOMLConfigFrom(tomlPath)
		require.NoError(t, err)

		assert.Equal(t, original.Phases, loaded.PhaseRoles)
		coder := loaded.Profiles["coder"]
		assert.Equal(t, "opencode", coder.Program)
		assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
		assert.InDelta(t, 0.5, *coder.Temperature, 0.001)
	})
}

func TestResolveProfileWithDisabledAgent(t *testing.T) {
	t.Run("disabled agent falls back to default", func(t *testing.T) {
		cfg := &Config{
			PhaseRoles: map[string]string{"planning": "planner"},
			Profiles: map[string]AgentProfile{
				"planner": {Program: "codex", Enabled: false},
			},
		}
		profile := cfg.ResolveProfile("planning", "claude")
		assert.Equal(t, "claude", profile.Program)
	})

	t.Run("enabled agent resolves normally", func(t *testing.T) {
		cfg := &Config{
			PhaseRoles: map[string]string{"implementing": "coder"},
			Profiles: map[string]AgentProfile{
				"coder": {Program: "opencode", Enabled: true},
			},
		}
		profile := cfg.ResolveProfile("implementing", "claude")
		assert.Equal(t, "opencode", profile.Program)
	})
}
```

### Step 3: Run tests to verify they fail

```bash
go test ./config/ -v -run "TestLoadTOML|TestSaveTOML|TestResolveProfileWithDisabled"
```

Expected: FAIL (TOMLConfig, LoadTOMLConfigFrom, SaveTOMLConfigTo don't exist)

### Step 4: Extend AgentProfile with new fields

Modify `config/profile.go` -- add fields after existing `Flags`:

```go
// AgentProfile defines the program and flags for an agent in a specific role.
type AgentProfile struct {
	Program     string   `json:"program"     toml:"program"`
	Flags       []string `json:"flags,omitempty" toml:"flags,omitempty"`
	Model       string   `json:"model,omitempty" toml:"model,omitempty"`
	Temperature *float64 `json:"temperature,omitempty" toml:"temperature,omitempty"`
	Effort      string   `json:"effort,omitempty" toml:"effort,omitempty"`
	Enabled     bool     `json:"enabled,omitempty" toml:"enabled,omitempty"`
}
```

Update `ResolveProfile` to check `Enabled`:

```go
func (c *Config) ResolveProfile(phase string, defaultProgram string) AgentProfile {
	if c.PhaseRoles == nil || c.Profiles == nil {
		return AgentProfile{Program: defaultProgram}
	}
	roleName, ok := c.PhaseRoles[phase]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	profile, ok := c.Profiles[roleName]
	if !ok {
		return AgentProfile{Program: defaultProgram}
	}
	if profile.Program == "" || !profile.Enabled {
		return AgentProfile{Program: defaultProgram}
	}
	return profile
}
```

### Step 5: Implement TOML parser

Create `config/toml.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const TOMLConfigFileName = "config.toml"

// TOMLAgent is the TOML-level representation of an agent.
// Maps directly to [agents.*] tables in config.toml.
type TOMLAgent struct {
	Enabled     bool     `toml:"enabled"`
	Program     string   `toml:"program"`
	Model       string   `toml:"model,omitempty"`
	Temperature *float64 `toml:"temperature,omitempty"`
	Effort      string   `toml:"effort,omitempty"`
	Flags       []string `toml:"flags,omitempty"`
}

// TOMLConfig is the top-level TOML file structure.
type TOMLConfig struct {
	Phases map[string]string       `toml:"phases"`
	Agents map[string]TOMLAgent    `toml:"agents"`
}

// TOMLConfigResult holds the parsed config in terms of internal types.
type TOMLConfigResult struct {
	Profiles   map[string]AgentProfile
	PhaseRoles map[string]string
}

// LoadTOMLConfigFrom reads and parses a TOML config file,
// returning the result mapped to internal types.
func LoadTOMLConfigFrom(path string) (*TOMLConfigResult, error) {
	var tc TOMLConfig
	if _, err := toml.DecodeFile(path, &tc); err != nil {
		return nil, fmt.Errorf("decode TOML config: %w", err)
	}

	result := &TOMLConfigResult{
		Profiles:   make(map[string]AgentProfile),
		PhaseRoles: tc.Phases,
	}

	for name, agent := range tc.Agents {
		result.Profiles[name] = AgentProfile{
			Program:     agent.Program,
			Model:       agent.Model,
			Temperature: agent.Temperature,
			Effort:      agent.Effort,
			Enabled:     agent.Enabled,
			Flags:       agent.Flags,
		}
	}

	return result, nil
}

// LoadTOMLConfig loads the TOML config from the default location (~/.klique/config.toml).
// Returns nil, nil if the file does not exist.
func LoadTOMLConfig() (*TOMLConfigResult, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, TOMLConfigFileName)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil // no TOML config is valid
	}

	return LoadTOMLConfigFrom(path)
}

// SaveTOMLConfigTo writes a TOMLConfig to the given path.
func SaveTOMLConfigTo(tc *TOMLConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	// Write header comment
	if _, err := fmt.Fprintln(f, "# Generated by kq init"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f); err != nil {
		return err
	}

	enc := toml.NewEncoder(f)
	if err := enc.Encode(tc); err != nil {
		return fmt.Errorf("encode TOML: %w", err)
	}
	return nil
}

// SaveTOMLConfig writes to the default location (~/.klique/config.toml).
func SaveTOMLConfig(tc *TOMLConfig) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return SaveTOMLConfigTo(tc, filepath.Join(configDir, TOMLConfigFileName))
}

// GetTOMLConfigPath returns the path to the TOML config file.
func GetTOMLConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, TOMLConfigFileName), nil
}
```

### Step 6: Add TOML overlay to LoadConfig

Modify `config/config.go` -- add `Profiles` and `PhaseRoles` fields to Config struct, and overlay TOML values in `LoadConfig()`:

Add fields to Config struct (if not already present from task-orchestration merge):

```go
// Add to Config struct:
	Profiles   map[string]AgentProfile `json:"profiles,omitempty"`
	PhaseRoles map[string]string       `json:"phase_roles,omitempty"`
```

Add TOML overlay at the end of `LoadConfig()`, before `return &config`:

```go
	// Overlay TOML config if it exists
	tomlResult, tomlErr := LoadTOMLConfig()
	if tomlErr != nil {
		log.WarningLog.Printf("failed to load TOML config: %v", tomlErr)
	} else if tomlResult != nil {
		config.Profiles = tomlResult.Profiles
		config.PhaseRoles = tomlResult.PhaseRoles
	}
```

### Step 7: Run all tests

```bash
go test ./config/ -v
```

Expected: PASS (all existing tests + new TOML tests)

### Step 8: Commit

```bash
git add config/ go.mod go.sum
git commit -m "feat(config): add TOML config parser for agent profiles and phase roles"
```

---

## Task 3: Interactive Wizard (WP03)

**Files:**
- Create: `internal/init/wizard/wizard.go`
- Create: `internal/init/wizard/stage_harness.go`
- Create: `internal/init/wizard/stage_agents.go`
- Create: `internal/init/wizard/stage_phases.go`
- Create: `internal/init/wizard/wizard_test.go`
- Modify: `go.mod` / `go.sum` (add charmbracelet/huh)

**Depends on:** WP01 (Harness interface, Registry, DetectResult), WP02 (TOMLConfig, TOMLAgent, AgentProfile)

### Step 1: Add the huh dependency

```bash
go get github.com/charmbracelet/huh@latest
```

### Step 2: Write the wizard state type and orchestrator

Create `internal/init/wizard/wizard.go`:

```go
package wizard

import (
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/internal/init/harness"
)

// State holds all wizard-collected values across stages.
type State struct {
	// Stage 1 outputs
	Registry        *harness.Registry
	DetectResults   []harness.DetectResult
	SelectedHarness []string // names of harnesses user selected

	// Stage 2 outputs
	Agents []AgentState

	// Stage 3 outputs
	PhaseMapping map[string]string
}

// AgentState holds the wizard form values for one agent role.
type AgentState struct {
	Role        string
	Harness     string
	Model       string
	Temperature string // "" means default; parsed to *float64 on save
	Effort      string // "" means default
	Enabled     bool
}

// DefaultPhases returns the default lifecycle phase names.
func DefaultPhases() []string {
	return []string{"implementing", "spec_review", "quality_review", "planning"}
}

// DefaultAgentRoles returns the built-in agent role names.
func DefaultAgentRoles() []string {
	return []string{"coder", "reviewer", "planner"}
}

// Run executes all wizard stages in sequence.
// If existing is non-nil, pre-populates forms from existing config.
func Run(registry *harness.Registry, existing *config.TOMLConfigResult) (*State, error) {
	state := &State{
		Registry:      registry,
		DetectResults: registry.DetectAll(),
	}

	// Stage 1: Harness selection
	if err := runHarnessStage(state); err != nil {
		return nil, err
	}

	// Stage 2: Agent configuration
	if err := runAgentStage(state, existing); err != nil {
		return nil, err
	}

	// Stage 3: Phase mapping
	if err := runPhaseStage(state, existing); err != nil {
		return nil, err
	}

	return state, nil
}

// ToTOMLConfig converts wizard state to the TOML config structure.
func (s *State) ToTOMLConfig() *config.TOMLConfig {
	tc := &config.TOMLConfig{
		Phases: s.PhaseMapping,
		Agents: make(map[string]config.TOMLAgent),
	}

	for _, a := range s.Agents {
		agent := config.TOMLAgent{
			Enabled: a.Enabled,
			Program: a.Harness,
			Model:   a.Model,
			Effort:  a.Effort,
			Flags:   []string{},
		}

		if a.Temperature != "" {
			// Parse temperature string to float64
			var temp float64
			if _, err := fmt.Sscanf(a.Temperature, "%f", &temp); err == nil {
				agent.Temperature = &temp
			}
		}

		tc.Agents[a.Role] = agent
	}

	return tc
}

// ToAgentConfigs converts wizard state to harness.AgentConfig slice
// for use by scaffold and superpowers install.
func (s *State) ToAgentConfigs() []harness.AgentConfig {
	var configs []harness.AgentConfig
	for _, a := range s.Agents {
		if !a.Enabled {
			continue
		}
		ac := harness.AgentConfig{
			Role:    a.Role,
			Harness: a.Harness,
			Model:   a.Model,
			Effort:  a.Effort,
			Enabled: a.Enabled,
		}
		if a.Temperature != "" {
			var temp float64
			if _, err := fmt.Sscanf(a.Temperature, "%f", &temp); err == nil {
				ac.Temperature = &temp
			}
		}
		configs = append(configs, ac)
	}
	return configs
}
```

**Note:** Add `"fmt"` to imports.

### Step 3: Implement Stage 1 - Harness Detection & Selection

Create `internal/init/wizard/stage_harness.go`:

```go
package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

func runHarnessStage(state *State) error {
	// Build options from detection results
	var options []huh.Option[string]
	var preSelected []string

	for _, d := range state.DetectResults {
		label := d.Name
		if d.Found {
			label = fmt.Sprintf("%s  (detected: %s)", d.Name, d.Path)
			preSelected = append(preSelected, d.Name)
		} else {
			label = fmt.Sprintf("%s  (not found)", d.Name)
		}
		options = append(options, huh.NewOption(label, d.Name))
	}

	state.SelectedHarness = preSelected

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which agent harnesses do you want to configure?").
				Options(options...).
				Value(&state.SelectedHarness),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("harness selection: %w", err)
	}

	if len(state.SelectedHarness) == 0 {
		return fmt.Errorf("no harnesses selected")
	}

	return nil
}
```

### Step 4: Implement Stage 2 - Agent Configuration

Create `internal/init/wizard/stage_agents.go`:

```go
package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/kastheco/klique/config"
)

func runAgentStage(state *State, existing *config.TOMLConfigResult) error {
	roles := DefaultAgentRoles()

	// Initialize agent states with defaults or existing values
	for _, role := range roles {
		as := AgentState{
			Role:    role,
			Harness: state.SelectedHarness[0], // default to first selected
			Enabled: true,
		}

		// Pre-populate from existing config
		if existing != nil {
			if profile, ok := existing.Profiles[role]; ok {
				as.Harness = profile.Program
				as.Model = profile.Model
				as.Effort = profile.Effort
				as.Enabled = profile.Enabled
				if profile.Temperature != nil {
					as.Temperature = fmt.Sprintf("%g", *profile.Temperature)
				}
			}
		}

		state.Agents = append(state.Agents, as)
	}

	// Build a form group for each agent role
	for i := range state.Agents {
		if err := runSingleAgentForm(state, i); err != nil {
			return err
		}
	}

	return nil
}

func runSingleAgentForm(state *State, idx int) error {
	agent := &state.Agents[idx]

	// Build harness options (only selected harnesses)
	var harnessOpts []huh.Option[string]
	for _, name := range state.SelectedHarness {
		harnessOpts = append(harnessOpts, huh.NewOption(name, name))
	}

	// Get models for the current harness
	h := state.Registry.Get(agent.Harness)
	models, _ := h.ListModels()

	// Build model options
	var modelOpts []huh.Option[string]
	for _, m := range models {
		modelOpts = append(modelOpts, huh.NewOption(m, m))
	}

	// Determine which fields to show based on harness capabilities
	supportsTemp := h.SupportsTemperature()
	supportsEffort := h.SupportsEffort()

	// Build form fields
	var fields []huh.Field

	// Harness selector
	fields = append(fields,
		huh.NewSelect[string]().
			Title(fmt.Sprintf("Configure agent: %s - Harness", agent.Role)).
			Options(harnessOpts...).
			Value(&agent.Harness),
	)

	// Model: use Select for harnesses with known models, Input for free-text
	if len(models) > 1 {
		fields = append(fields,
			huh.NewSelect[string]().
				Title("Model").
				Options(modelOpts...).
				Value(&agent.Model),
		)
	} else {
		// Free-text input (codex or single-model harness)
		defaultModel := ""
		if len(models) > 0 {
			defaultModel = models[0]
		}
		if agent.Model == "" {
			agent.Model = defaultModel
		}
		fields = append(fields,
			huh.NewInput().
				Title("Model").
				Value(&agent.Model),
		)
	}

	// Temperature (conditional)
	if supportsTemp {
		fields = append(fields,
			huh.NewInput().
				Title("Temperature (empty = harness default)").
				Placeholder("e.g. 0.7").
				Value(&agent.Temperature),
		)
	}

	// Effort (conditional)
	if supportsEffort {
		effortOpts := []huh.Option[string]{
			huh.NewOption("default", ""),
			huh.NewOption("low", "low"),
			huh.NewOption("medium", "medium"),
			huh.NewOption("high", "high"),
		}
		fields = append(fields,
			huh.NewSelect[string]().
				Title("Effort").
				Options(effortOpts...).
				Value(&agent.Effort),
		)
	}

	// Enabled toggle
	fields = append(fields,
		huh.NewConfirm().
			Title("Enabled").
			Value(&agent.Enabled),
	)

	form := huh.NewForm(
		huh.NewGroup(fields...),
	)

	return form.Run()
}
```

### Step 5: Implement Stage 3 - Phase Mapping

Create `internal/init/wizard/stage_phases.go`:

```go
package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/kastheco/klique/config"
)

func runPhaseStage(state *State, existing *config.TOMLConfigResult) error {
	// Collect enabled agent names for dropdown options
	var enabledAgents []huh.Option[string]
	for _, a := range state.Agents {
		if a.Enabled {
			enabledAgents = append(enabledAgents, huh.NewOption(a.Role, a.Role))
		}
	}

	if len(enabledAgents) == 0 {
		return fmt.Errorf("no agents enabled; cannot map phases")
	}

	// Initialize phase mapping with defaults or existing values
	phases := DefaultPhases()
	state.PhaseMapping = make(map[string]string)

	defaults := map[string]string{
		"implementing":   "coder",
		"spec_review":    "reviewer",
		"quality_review": "reviewer",
		"planning":       "planner",
	}

	// Pre-populate from existing config or defaults
	for _, phase := range phases {
		if existing != nil && existing.PhaseRoles != nil {
			if role, ok := existing.PhaseRoles[phase]; ok {
				state.PhaseMapping[phase] = role
				continue
			}
		}
		state.PhaseMapping[phase] = defaults[phase]
	}

	// Build form fields for each phase
	var fields []huh.Field
	for _, phase := range phases {
		// Capture loop variable for closure
		p := phase
		val := state.PhaseMapping[p]

		// We need a local variable for huh to bind to
		fields = append(fields,
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Phase: %s", p)).
				Options(enabledAgents...).
				Value(func() *string {
					// Ensure the value pointer stays valid
					v := state.PhaseMapping[p]
					return &v
				}()),
		)
		// The above closure approach won't work cleanly with huh's value binding.
		// Instead, use a slice of string pointers.
		_ = val // suppress unused
	}

	// Better approach: use indexed slice for value binding
	phaseValues := make([]string, len(phases))
	for i, phase := range phases {
		phaseValues[i] = state.PhaseMapping[phase]
	}

	fields = nil
	for i, phase := range phases {
		fields = append(fields,
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Phase: %s", phase)).
				Options(enabledAgents...).
				Value(&phaseValues[i]),
		)
	}

	form := huh.NewForm(
		huh.NewGroup(fields...).
			Title("Map lifecycle phases to agents"),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("phase mapping: %w", err)
	}

	// Write back to state
	for i, phase := range phases {
		state.PhaseMapping[phase] = phaseValues[i]
	}

	return nil
}
```

### Step 6: Write wizard unit tests

Create `internal/init/wizard/wizard_test.go`:

```go
package wizard

import (
	"testing"

	"github.com/kastheco/klique/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateToTOMLConfig(t *testing.T) {
	temp := "0.7"
	state := &State{
		Agents: []AgentState{
			{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6",
				Temperature: temp, Effort: "high", Enabled: true},
			{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6",
				Temperature: "", Effort: "high", Enabled: true},
			{Role: "planner", Harness: "codex", Model: "gpt-5.3-codex",
				Temperature: "", Effort: "", Enabled: false},
		},
		PhaseMapping: map[string]string{
			"implementing":   "coder",
			"spec_review":    "reviewer",
			"quality_review": "reviewer",
			"planning":       "planner",
		},
	}

	tc := state.ToTOMLConfig()

	// Verify phases
	assert.Equal(t, "coder", tc.Phases["implementing"])
	assert.Equal(t, "reviewer", tc.Phases["spec_review"])

	// Verify agents
	coder, ok := tc.Agents["coder"]
	require.True(t, ok)
	assert.Equal(t, "opencode", coder.Program)
	assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
	assert.NotNil(t, coder.Temperature)
	assert.InDelta(t, 0.7, *coder.Temperature, 0.001)
	assert.True(t, coder.Enabled)

	// Verify disabled agent
	planner := tc.Agents["planner"]
	assert.False(t, planner.Enabled)

	// Verify nil temperature when empty
	reviewer := tc.Agents["reviewer"]
	assert.Nil(t, reviewer.Temperature)
}

func TestStateToAgentConfigs(t *testing.T) {
	state := &State{
		Agents: []AgentState{
			{Role: "coder", Harness: "opencode", Model: "model-1", Enabled: true},
			{Role: "reviewer", Harness: "claude", Model: "model-2", Enabled: true},
			{Role: "planner", Harness: "codex", Model: "model-3", Enabled: false},
		},
	}

	configs := state.ToAgentConfigs()

	// Only enabled agents
	assert.Len(t, configs, 2)
	assert.Equal(t, "coder", configs[0].Role)
	assert.Equal(t, "reviewer", configs[1].Role)
}

func TestDefaultPhases(t *testing.T) {
	phases := DefaultPhases()
	assert.Equal(t, []string{"implementing", "spec_review", "quality_review", "planning"}, phases)
}

func TestDefaultAgentRoles(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Equal(t, []string{"coder", "reviewer", "planner"}, roles)
}

func TestPrePopulateFromExisting(t *testing.T) {
	// This tests the pre-population logic without running the interactive form.
	// We test the data flow, not the huh form itself.
	temp := 0.5
	existing := &config.TOMLConfigResult{
		Profiles: map[string]config.AgentProfile{
			"coder": {
				Program:     "opencode",
				Model:       "anthropic/claude-sonnet-4-6",
				Temperature: &temp,
				Effort:      "high",
				Enabled:     true,
			},
		},
		PhaseRoles: map[string]string{
			"implementing": "coder",
		},
	}

	// Simulate what runAgentStage does for pre-population
	roles := DefaultAgentRoles()
	var agents []AgentState
	for _, role := range roles {
		as := AgentState{Role: role, Harness: "claude", Enabled: true}
		if profile, ok := existing.Profiles[role]; ok {
			as.Harness = profile.Program
			as.Model = profile.Model
			as.Effort = profile.Effort
			as.Enabled = profile.Enabled
			if profile.Temperature != nil {
				as.Temperature = "0.5"
			}
		}
		agents = append(agents, as)
	}

	assert.Equal(t, "opencode", agents[0].Harness) // coder got pre-populated
	assert.Equal(t, "claude", agents[1].Harness)    // reviewer got default
}
```

### Step 7: Run tests

```bash
go test ./internal/init/wizard/ -v
```

Expected: PASS (unit tests don't invoke interactive huh.Run())

### Step 8: Commit

```bash
git add internal/init/wizard/ go.mod go.sum
git commit -m "feat(init): add interactive wizard with harness/agent/phase stages"
```

---

## Task 4: Scaffold & Superpowers Install (WP04)

**Files:**
- Create: `internal/init/scaffold/scaffold.go`
- Create: `internal/init/scaffold/templates/shared/tools-reference.md` (harness-agnostic tool docs)
- Create: `internal/init/scaffold/templates/claude/agents/coder.md`
- Create: `internal/init/scaffold/templates/claude/agents/planner.md`
- Create: `internal/init/scaffold/templates/claude/agents/reviewer.md`
- Create: `internal/init/scaffold/templates/opencode/agents/coder.md`
- Create: `internal/init/scaffold/templates/opencode/agents/planner.md`
- Create: `internal/init/scaffold/templates/opencode/agents/reviewer.md`
- Create: `internal/init/scaffold/templates/codex/AGENTS.md`
- Create: `internal/init/scaffold/scaffold_test.go`
- Modify: `internal/init/harness/claude.go` (implement ScaffoldProject)
- Modify: `internal/init/harness/opencode.go` (implement ScaffoldProject)
- Modify: `internal/init/harness/codex.go` (implement ScaffoldProject)

**Depends on:** WP01 (Harness interface, AgentConfig)

### Step 1: Write failing tests for scaffold

Create `internal/init/scaffold/scaffold_test.go`:

```go
package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/klique/internal/init/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaffoldClaudeProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, false)
	require.NoError(t, err)

	// Verify agent files exist
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "coder.md"))
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	// Planner not created (not in agents list for claude)
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "agents", "planner.md"))
}

func TestScaffoldOpenCodeProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
	}

	err := WriteOpenCodeProject(dir, agents, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "coder.md"))
}

func TestScaffoldCodexProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
	}

	err := WriteCodexProject(dir, agents, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".codex", "AGENTS.md"))
}

func TestScaffoldSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	// Pre-create file with custom content
	existing := filepath.Join(agentDir, "coder.md")
	require.NoError(t, os.WriteFile(existing, []byte("custom content"), 0o644))

	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, false) // force=false
	require.NoError(t, err)

	// Verify custom content preserved
	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "custom content", string(content))
}

func TestScaffoldForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	existing := filepath.Join(agentDir, "coder.md")
	require.NoError(t, os.WriteFile(existing, []byte("old content"), 0o644))

	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, true) // force=true
	require.NoError(t, err)

	// Verify content was overwritten (not "old content")
	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.NotEqual(t, "old content", string(content))
}

func TestToolsReferenceInjected(t *testing.T) {
	t.Run("claude agents include tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		err := WriteClaudeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		// Verify tools reference was injected (not the placeholder)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
		assert.Contains(t, string(content), "difft")
		assert.Contains(t, string(content), "comby")
		assert.Contains(t, string(content), "typos")
		assert.Contains(t, string(content), "scc")
		assert.Contains(t, string(content), "yq")
		assert.Contains(t, string(content), "sd")
	})

	t.Run("opencode agents include tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
		}

		err := WriteOpenCodeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
	})

	t.Run("codex AGENTS.md includes tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
		}

		err := WriteCodexProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".codex", "AGENTS.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
	})

	t.Run("model placeholder is substituted", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
		}

		err := WriteClaudeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{MODEL}}")
		assert.Contains(t, string(content), "claude-opus-4-6")
	})
}
```

### Step 2: Run tests to verify they fail

```bash
go test ./internal/init/scaffold/ -v
```

Expected: FAIL

### Step 3: Create shared tools-reference template

This file is embedded into every agent template via `{{TOOLS_REFERENCE}}`. It teaches agents about available enhanced CLI tools in a model-agnostic way. The language uses conditional phrasing ("if available, prefer X") so it degrades gracefully on systems without all tools installed.

Create `internal/init/scaffold/templates/shared/tools-reference.md`:

```markdown
## Available CLI Tools

These tools are available in this environment. Prefer them over lower-level alternatives when they apply.

### Code Search & Refactoring

- **ast-grep** (`sg`): Structural code search and replace using AST patterns. Prefer over regex-based grep/sed for any code transformation that involves language syntax (renaming symbols, changing function signatures, rewriting patterns). Examples:
  - Find all calls: `sg --pattern 'fmt.Errorf($$$)' --lang go`
  - Structural replace: `sg --pattern 'errors.New($MSG)' --rewrite 'fmt.Errorf($MSG)' --lang go`
  - Interactive rewrite: `sg --pattern '$A != nil' --rewrite '$A == nil' --lang go --interactive`
- **comby** (`comby`): Language-aware structural search/replace with hole syntax. Use for multi-line pattern matching and complex rewrites that span statement boundaries. Examples:
  - `comby 'if err != nil { return :[rest] }' 'if err != nil { return fmt.Errorf(":[context]: %w", err) }' .go`
  - `comby 'func :[name](:[args]) {:[body]}' 'func :[name](:[args]) error {:[body]}' .go -d src/`

### Diff & Change Analysis

- **difftastic** (`difft`): Structural diff that understands syntax. Use for reviewing changes, comparing files, and understanding code modifications. Produces dramatically cleaner output than line-based diff for refactors and moves. Examples:
  - Compare files: `difft old.go new.go`
  - Git integration: `GIT_EXTERNAL_DIFF=difft git diff`
  - Single file history: `GIT_EXTERNAL_DIFF=difft git log -p -- path/to/file.go`

### Text Processing

- **sd**: Find-and-replace tool (modern sed alternative). Use for string replacements in files. Simpler syntax than sed -- no need to escape delimiters. Examples:
  - In-place replace: `sd 'old_name' 'new_name' src/**/*.go`
  - Regex with groups: `sd 'version = "(\d+)\.(\d+)"' 'version = "$1.$(($2+1))"' Cargo.toml`
  - Preview (dry run): `sd -p 'foo' 'bar' file.txt`
- **yq**: YAML/JSON/TOML processor (like jq for structured data). Use for querying and modifying config files, frontmatter, and structured data. Examples:
  - Read YAML field: `yq '.metadata.name' file.yaml`
  - Modify TOML: `yq -t '.agents.coder.model = "new-model"' config.toml`
  - Convert formats: `yq -o json file.yaml`
  - Query JSON: `yq '.dependencies | keys' package.json`

### Code Quality

- **typos** (`typos`): Fast source code spell checker. Finds and fixes typos in identifiers, strings, filenames, and comments. Run before commits. Examples:
  - Check project: `typos`
  - Check specific path: `typos src/`
  - Auto-fix: `typos --write-changes`
  - Check single file: `typos path/to/file.go`
- **scc**: Fast source code counter. Use for codebase metrics -- line counts, language breakdown, complexity estimates. Useful for understanding project scope. Examples:
  - Full project: `scc`
  - Specific directory: `scc internal/`
  - By file: `scc --by-file --sort lines`
  - Exclude tests: `scc --not-match '_test.go$'`

### When to Use What

| Task | Preferred Tool | Fallback |
|------|---------------|----------|
| Rename symbol across files | `sg` (ast-grep) | `sd` for simple strings |
| Structural code rewrite | `sg` or `comby` | manual edit |
| Find pattern in code | `sg --pattern` | `rg` (ripgrep) for literal strings |
| Replace string in files | `sd` | `sed` |
| Read/modify YAML/TOML/JSON | `yq` | manual edit |
| Review code changes | `difft` | `git diff` |
| Spell check code | `typos` | manual review |
| Count lines / project metrics | `scc` | `wc -l` |
```

### Step 4: Create per-harness agent templates

Each template includes `{{TOOLS_REFERENCE}}` which the scaffold code replaces with the shared tools-reference content at write time.

Create `internal/init/scaffold/templates/claude/agents/coder.md`:

```markdown
---
name: coder
description: Implementation agent for writing and modifying code
model: {{MODEL}}
---

You are the coder agent for klique. Your role is to implement features, fix bugs,
and write tests according to the spec-kitty work package you've been assigned.

Follow the constitution at `.kittify/memory/constitution.md`.
Follow TDD: write failing test, implement, verify pass, commit.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/claude/agents/reviewer.md`:

```markdown
---
name: reviewer
description: Code review agent for quality and spec compliance
model: {{MODEL}}
---

You are the reviewer agent for klique. Your role is to review code changes
for quality, security, spec compliance, and test coverage.

Follow the constitution at `.kittify/memory/constitution.md`.
Be specific about issues. Cite line numbers. Use `difft` for structural diffs when reviewing changes.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/claude/agents/planner.md`:

```markdown
---
name: planner
description: Planning agent for specifications and architecture
model: {{MODEL}}
---

You are the planner agent for klique. Your role is to write specifications,
create implementation plans, and decompose work into packages.

Follow the constitution at `.kittify/memory/constitution.md`.
Research before making architecture decisions. Use `scc` for codebase metrics when scoping work.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/opencode/agents/coder.md`:

```markdown
---
description: Implementation agent - writes code, fixes bugs, runs tests
mode: agent
---

You are the coder agent. Implement features according to spec-kitty work packages.
Follow TDD. Follow the constitution at `.kittify/memory/constitution.md`.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/opencode/agents/reviewer.md`:

```markdown
---
description: Review agent - checks quality, security, spec compliance
mode: agent
---

You are the reviewer agent. Review code for quality, security, and spec compliance.
Follow the constitution at `.kittify/memory/constitution.md`.
Use `difft` for structural diffs when reviewing changes.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/opencode/agents/planner.md`:

```markdown
---
description: Planning agent - specs, plans, task decomposition
mode: agent
---

You are the planner agent. Write specs, plans, and decompose work into packages.
Follow the constitution at `.kittify/memory/constitution.md`.
Use `scc` for codebase metrics when scoping work.

{{TOOLS_REFERENCE}}
```

Create `internal/init/scaffold/templates/codex/AGENTS.md`:

```markdown
# klique Agents

## Coder
Implementation agent. Writes code, fixes bugs, runs tests.
Follow TDD. Follow the constitution at `.kittify/memory/constitution.md`.

## Reviewer
Review agent. Checks quality, security, spec compliance.
Use `difft` for structural diffs when reviewing changes.
Follow the constitution at `.kittify/memory/constitution.md`.

## Planner
Planning agent. Writes specs, plans, decomposes work into packages.
Use `scc` for codebase metrics when scoping work.
Follow the constitution at `.kittify/memory/constitution.md`.

{{TOOLS_REFERENCE}}
```

### Step 5: Implement the scaffold package

Create `internal/init/scaffold/scaffold.go`:

```go
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/klique/internal/init/harness"
)

//go:embed templates/*
var templates embed.FS

// loadToolsReference reads the shared tools-reference template once.
// Returns empty string on error (non-fatal -- agents work without it).
func loadToolsReference() string {
	content, err := templates.ReadFile("templates/shared/tools-reference.md")
	if err != nil {
		return ""
	}
	return string(content)
}

// renderTemplate applies all placeholder substitutions to a template.
func renderTemplate(content string, agent harness.AgentConfig, toolsRef string) string {
	rendered := content
	rendered = strings.ReplaceAll(rendered, "{{MODEL}}", agent.Model)
	rendered = strings.ReplaceAll(rendered, "{{TOOLS_REFERENCE}}", toolsRef)
	return rendered
}

// WriteResult tracks scaffold output for summary display.
type WriteResult struct {
	Path    string
	Created bool // true=created, false=skipped
}

// WriteClaudeProject scaffolds .claude/ project files.
func WriteClaudeProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	agentDir := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create .claude/agents: %w", err)
	}

	// Only write templates for roles that use claude
	for _, agent := range agents {
		if agent.Harness != "claude" {
			continue
		}
		templatePath := fmt.Sprintf("templates/claude/agents/%s.md", agent.Role)
		content, err := templates.ReadFile(templatePath)
		if err != nil {
			// No template for this role - skip
			continue
		}

		rendered := renderTemplate(string(content), agent, toolsRef)

		dest := filepath.Join(agentDir, agent.Role+".md")
		if err := writeFile(dest, []byte(rendered), force); err != nil {
			return err
		}
	}

	return nil
}

// WriteOpenCodeProject scaffolds .opencode/ project files.
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	agentDir := filepath.Join(dir, ".opencode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create .opencode/agents: %w", err)
	}

	for _, agent := range agents {
		if agent.Harness != "opencode" {
			continue
		}
		templatePath := fmt.Sprintf("templates/opencode/agents/%s.md", agent.Role)
		content, err := templates.ReadFile(templatePath)
		if err != nil {
			continue
		}

		rendered := renderTemplate(string(content), agent, toolsRef)

		dest := filepath.Join(agentDir, agent.Role+".md")
		if err := writeFile(dest, []byte(rendered), force); err != nil {
			return err
		}
	}

	return nil
}

// WriteCodexProject scaffolds .codex/ project files.
func WriteCodexProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return fmt.Errorf("create .codex: %w", err)
	}

	content, err := templates.ReadFile("templates/codex/AGENTS.md")
	if err != nil {
		return fmt.Errorf("read codex template: %w", err)
	}

	// Codex uses a single AGENTS.md; apply tools ref but no per-agent model sub
	rendered := strings.ReplaceAll(string(content), "{{TOOLS_REFERENCE}}", toolsRef)

	dest := filepath.Join(codexDir, "AGENTS.md")
	return writeFile(dest, []byte(rendered), force)
}

// ScaffoldAll writes project files for all harnesses that have at least one enabled agent.
func ScaffoldAll(dir string, agents []harness.AgentConfig, force bool) ([]WriteResult, error) {
	var results []WriteResult

	// Group agents by harness
	byHarness := make(map[string][]harness.AgentConfig)
	for _, a := range agents {
		byHarness[a.Harness] = append(byHarness[a.Harness], a)
	}

	scaffolders := map[string]func(string, []harness.AgentConfig, bool) error{
		"claude":   WriteClaudeProject,
		"opencode": WriteOpenCodeProject,
		"codex":    WriteCodexProject,
	}

	for harnessName, harnessAgents := range byHarness {
		scaffolder, ok := scaffolders[harnessName]
		if !ok {
			continue
		}
		if err := scaffolder(dir, harnessAgents, force); err != nil {
			return results, fmt.Errorf("scaffold %s: %w", harnessName, err)
		}
	}

	// Walk the created files for summary
	for harnessName := range byHarness {
		prefix := "." + harnessName
		if harnessName == "codex" {
			prefix = ".codex"
		}
		target := filepath.Join(dir, prefix)
		_ = filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			results = append(results, WriteResult{Path: rel, Created: true})
			return nil
		})
	}

	return results, nil
}

// writeFile writes content to path. If force is false and the file exists, skip.
func writeFile(path string, content []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil // skip existing
		}
	}
	return os.WriteFile(path, content, 0o644)
}
```

### Step 5: Wire ScaffoldProject into harness adapters

Update `internal/init/harness/claude.go` -- replace the stub ScaffoldProject:

```go
func (c *Claude) ScaffoldProject(dir string, agents []AgentConfig, force bool) error {
	// Delegate to scaffold package - imported at the command level
	// This is a pass-through; the actual work is done by scaffold.WriteClaudeProject
	return nil
}
```

**Note:** The actual scaffolding is called from the orchestrator (WP05), not from the harness adapter. The harness interface declares ScaffoldProject for extensibility, but the current implementation delegates to the scaffold package directly. This avoids a circular dependency between harness and scaffold.

### Step 6: Run tests

```bash
go test ./internal/init/scaffold/ -v
```

Expected: PASS

### Step 7: Commit

```bash
git add internal/init/scaffold/
git commit -m "feat(init): add project scaffolding with embedded templates for all harnesses"
```

---

## Task 5: Cobra Command & Integration (WP05)

**Files:**
- Create: `internal/init/init.go`
- Modify: `main.go` (register `initCmd`)
- Create: `internal/init/init_test.go`

**Depends on:** WP01, WP02, WP03, WP04

### Step 1: Write the orchestrator

Create `internal/init/init.go`:

```go
package init

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/internal/init/harness"
	"github.com/kastheco/klique/internal/init/scaffold"
	"github.com/kastheco/klique/internal/init/wizard"
)

// Options holds the CLI flags for kq init.
type Options struct {
	Force bool // overwrite existing project scaffold files
	Clean bool // ignore existing config, start with factory defaults
}

// Run executes the kq init workflow.
func Run(opts Options) error {
	registry := harness.NewRegistry()

	// Load existing config unless --clean
	var existing *config.TOMLConfigResult
	if !opts.Clean {
		var err error
		existing, err = config.LoadTOMLConfig()
		if err != nil {
			fmt.Printf("Warning: could not load existing config: %v\n", err)
		}
	}

	// Run interactive wizard
	state, err := wizard.Run(registry, existing)
	if err != nil {
		return fmt.Errorf("wizard: %w", err)
	}

	// Stage 4a: Install superpowers into selected harnesses
	fmt.Println("\nInstalling superpowers...")
	for _, name := range state.SelectedHarness {
		h := registry.Get(name)
		if h == nil {
			continue
		}
		fmt.Printf("  %-12s ", name)
		if err := h.InstallSuperpowers(); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			// Non-fatal: continue with other harnesses
		} else {
			fmt.Println("OK")
		}
	}

	// Stage 4b: Write TOML config
	fmt.Println("\nWriting config...")
	tc := state.ToTOMLConfig()
	if err := config.SaveTOMLConfig(tc); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	tomlPath, _ := config.GetTOMLConfigPath()
	fmt.Printf("  %s\n", tomlPath)

	// Stage 4c: Scaffold project files
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	agentConfigs := state.ToAgentConfigs()
	fmt.Printf("\nScaffolding project: %s\n", projectDir)
	results, err := scaffold.ScaffoldAll(projectDir, agentConfigs, opts.Force)
	if err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}
	for _, r := range results {
		fmt.Printf("  %-40s OK\n", r.Path)
	}

	fmt.Println("\nDone! Run 'kq' to start.")
	return nil
}
```

**Note:** The package name `init` conflicts with Go's built-in `init()` function at the call site. Rename the package to `initialize` or `setup`:

Rename: `internal/init/` -> `internal/initialize/`
Update: all imports from `internal/init/...` to `internal/initialize/...`

Actually, the design doc uses `internal/init/`. To avoid the Go keyword conflict, we'll use the package declaration `package initialize` inside the `internal/init/` directory, or rename the directory. The cleaner approach: **rename directory to `internal/initcmd/`**.

```
internal/initcmd/
  initcmd.go           -- orchestrator (package initcmd)
  harness/
    harness.go
    claude.go
    opencode.go
    codex.go
    harness_test.go
  wizard/
    wizard.go
    stage_harness.go
    stage_agents.go
    stage_phases.go
    wizard_test.go
  scaffold/
    scaffold.go
    templates/
    scaffold_test.go
```

### Step 2: Register the cobra command

Modify `main.go` -- add in the `init()` function:

```go
	// Import at top:
	// initcmd "github.com/kastheco/klique/internal/initcmd"

	var forceFlag bool
	var cleanFlag bool

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Configure agent harnesses, install superpowers, and scaffold project files",
		Long: `Run an interactive wizard to:
  1. Detect and select agent CLIs (claude, opencode, codex)
  2. Configure agent roles (coder, reviewer, planner) with model and tuning
  3. Map lifecycle phases to agent roles
  4. Install superpowers skills into each harness
  5. Write ~/.klique/config.toml and scaffold project-level agent files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return initcmd.Run(initcmd.Options{
				Force: forceFlag,
				Clean: cleanFlag,
			})
		},
	}

	initCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing project scaffold files")
	initCmd.Flags().BoolVar(&cleanFlag, "clean", false, "Ignore existing config, start with factory defaults")

	rootCmd.AddCommand(initCmd)
```

### Step 3: Write integration test

Create `internal/initcmd/initcmd_test.go`:

```go
package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/klique/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsDefaults(t *testing.T) {
	opts := Options{}
	assert.False(t, opts.Force)
	assert.False(t, opts.Clean)
}

// Integration test: verify the full flow writes config and scaffold files.
// This test uses a temp HOME to avoid touching real config.
// It does NOT run the interactive wizard (that requires a terminal).
// Instead it tests the post-wizard write path.
func TestWritePhase(t *testing.T) {
	// Set up temp HOME
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create config dir
	configDir := filepath.Join(tmpHome, ".klique")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	// Simulate wizard output
	temp := 0.7
	tc := &config.TOMLConfig{
		Phases: map[string]string{
			"implementing": "coder",
			"planning":     "planner",
		},
		Agents: map[string]config.TOMLAgent{
			"coder": {
				Enabled: true,
				Program: "opencode",
				Model:   "anthropic/claude-sonnet-4-6",
				Temperature: &temp,
				Effort:  "high",
				Flags:   []string{},
			},
			"planner": {
				Enabled: true,
				Program: "claude",
				Model:   "claude-opus-4-6",
				Flags:   []string{},
			},
		},
	}

	// Write TOML config
	err := config.SaveTOMLConfig(tc)
	require.NoError(t, err)

	// Verify TOML file exists
	tomlPath := filepath.Join(configDir, "config.toml")
	assert.FileExists(t, tomlPath)

	// Verify it can be loaded back
	result, err := config.LoadTOMLConfigFrom(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, "coder", result.PhaseRoles["implementing"])
	assert.Equal(t, "opencode", result.Profiles["coder"].Program)
}
```

### Step 4: Run all tests

```bash
go test ./... -v
```

Expected: PASS

### Step 5: Manual smoke test

```bash
go build -o kq . && ./kq init --help
```

Verify help text shows `--force` and `--clean` flags.

```bash
./kq init
```

Walk through the wizard manually. Verify:
1. Stage 1 detects installed CLIs
2. Stage 2 shows model lists per harness
3. Stage 3 shows phase mapping with enabled agents only
4. Config written to `~/.klique/config.toml`
5. Project files scaffolded in `.claude/`, `.opencode/`, `.codex/` as appropriate
6. All scaffolded agent .md files contain the tools-reference section (grep for "ast-grep" in any agent file)
7. No `{{TOOLS_REFERENCE}}` or `{{MODEL}}` placeholders remain in output files

### Step 6: Commit

```bash
git add internal/initcmd/ main.go
git commit -m "feat: add kq init command with interactive wizard and project scaffolding"
```

---

## Verification Checklist

After all tasks are complete, verify:

- [ ] `go test ./...` passes
- [ ] `go build .` succeeds
- [ ] `go vet ./...` clean
- [ ] `kq init --help` shows correct usage
- [ ] `kq init` runs the full wizard flow
- [ ] `kq init --clean` ignores existing config
- [ ] `kq init --force` overwrites project files
- [ ] `~/.klique/config.toml` is written with correct structure
- [ ] TOML config loads correctly via `config.LoadConfig()` (JSON + TOML overlay)
- [ ] Disabled agents are not selectable in phase mapping
- [ ] `ResolveProfile()` falls back to default for disabled agents
- [ ] Project scaffold files are correct per harness
- [ ] Existing files are preserved without `--force`
- [ ] Superpowers install runs for each selected harness
- [ ] All scaffolded agent files contain tools-reference section (no raw `{{TOOLS_REFERENCE}}` placeholder)
- [ ] Tools reference mentions all 7 tools: ast-grep, difftastic, sd, scc, yq, comby, typos
- [ ] Tools reference is identical across claude, opencode, and codex agent files
- [ ] Constitution compliance: Go 1.24+, tests present, no manager AI agent pattern

---

## Implementation Notes

### Package naming

The `internal/init/` directory cannot use `package init` (Go keyword). Use `package initcmd` with directory `internal/initcmd/`.

### huh form testing

huh forms require a terminal (TTY) to run interactively. Unit tests should test the data transformation logic (State -> TOMLConfig, pre-population) but not invoke `form.Run()`. For interactive testing, use the manual smoke test in WP05.

### TOML field ordering

BurntSushi/toml's encoder writes fields in struct definition order. The `[phases]` section should come before `[agents.*]` to match the design doc's example. The `TOMLConfig` struct field order controls this.

### Circular dependency avoidance

The `harness` package defines the interface and adapters. The `scaffold` package handles file writing with embedded templates. The `initcmd` package orchestrates both. Neither `harness` nor `scaffold` imports the other.

### Shared tools-reference pattern

The `templates/shared/tools-reference.md` file is the single source of truth for CLI tool documentation. All per-harness templates include `{{TOOLS_REFERENCE}}` which `renderTemplate()` replaces at scaffold time. This means:
- Updating tool docs requires editing ONE file, not 7 templates
- Adding a new tool is a one-line addition to the table + a section
- The tools-reference content is model-agnostic -- no harness-specific APIs referenced
- If the shared file is missing (embed error), the placeholder is replaced with empty string (graceful degradation)

The tools documented are those available on the developer's system: **ast-grep** (structural code search/replace), **difftastic** (structural diffs), **sd** (find-replace), **scc** (code metrics), **yq** (structured data processing), **comby** (multi-line structural rewrite), **typos** (spell checking). All instructions use conditional "prefer X over Y when available" language so they work even if a specific tool isn't installed.

### Error handling philosophy

Superpowers installation failures are non-fatal (print warning, continue). Config write failures are fatal. Scaffold failures are fatal. This matches the design doc's behavior where undetected harnesses show a warning but don't block.
