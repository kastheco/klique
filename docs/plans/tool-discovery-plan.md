# Tool Discovery for kq init — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an interactive wizard stage that scans PATH for CLI tools in tools-reference.md and scaffolds only selected tools into agent files.

**Architecture:** Static tool catalog in wizard drives `exec.LookPath` detection + huh multi-select. Scaffold gets a `FilterToolsReference` state machine that strips tool entries, empty category headers, and table rows for unselected binaries. `State.SelectedTools` carries user selection from wizard to scaffold.

**Tech Stack:** Go 1.24+, charmbracelet/huh (already dep), os/exec, testify

---

### Task 1: Tool detection function with injectable lookup

**Files:**
- Create: `internal/initcmd/wizard/stage_tools.go`
- Test: `internal/initcmd/wizard/stage_tools_test.go`

**Step 1: Write the failing test**

Create `internal/initcmd/wizard/stage_tools_test.go`:

```go
package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectTools(t *testing.T) {
	// Fake lookup: sg and sd found, everything else not
	fakeLookup := func(binary string) (string, error) {
		switch binary {
		case "sg":
			return "/usr/bin/sg", nil
		case "sd":
			return "/usr/bin/sd", nil
		default:
			return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
		}
	}

	results := detectTools(toolCatalog, fakeLookup)

	t.Run("returns result for every catalog entry", func(t *testing.T) {
		assert.Len(t, results, len(toolCatalog))
	})

	t.Run("found tools have path and Found=true", func(t *testing.T) {
		for _, r := range results {
			if r.Binary == "sg" {
				assert.True(t, r.Found)
				assert.Equal(t, "/usr/bin/sg", r.Path)
			}
			if r.Binary == "sd" {
				assert.True(t, r.Found)
				assert.Equal(t, "/usr/bin/sd", r.Path)
			}
		}
	})

	t.Run("missing tools have Found=false", func(t *testing.T) {
		for _, r := range results {
			if r.Binary == "comby" {
				assert.False(t, r.Found)
				assert.Empty(t, r.Path)
			}
		}
	})
}
```

Note: You'll need `"os/exec"` in the import for `exec.Error` / `exec.ErrNotFound`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/wizard/ -run TestDetectTools -v`
Expected: FAIL — `detectTools` and `toolCatalog` undefined.

**Step 3: Write minimal implementation**

Create `internal/initcmd/wizard/stage_tools.go`:

```go
package wizard

import "os/exec"

// toolDef defines a CLI tool the wizard can detect.
type toolDef struct {
	Binary string // executable name to look up in PATH
	Name   string // human-friendly display name
}

// toolCatalog is the static list of tools from tools-reference.md.
var toolCatalog = []toolDef{
	{"sg", "ast-grep"},
	{"comby", "comby"},
	{"difft", "difftastic"},
	{"sd", "sd"},
	{"yq", "yq"},
	{"mlr", "miller"},
	{"glow", "glow"},
	{"typos", "typos"},
	{"scc", "scc"},
	{"tokei", "tokei"},
	{"watchexec", "watchexec"},
	{"hyperfine", "hyperfine"},
	{"procs", "procs"},
	{"mprocs", "mprocs"},
}

// toolDetectResult holds the result of looking up a single tool.
type toolDetectResult struct {
	Binary string
	Name   string
	Path   string
	Found  bool
}

// lookupFunc abstracts exec.LookPath for testing.
type lookupFunc func(binary string) (string, error)

// detectTools probes PATH for each tool in the catalog.
func detectTools(catalog []toolDef, lookup lookupFunc) []toolDetectResult {
	results := make([]toolDetectResult, 0, len(catalog))
	for _, t := range catalog {
		path, err := lookup(t.Binary)
		results = append(results, toolDetectResult{
			Binary: t.Binary,
			Name:   t.Name,
			Path:   path,
			Found:  err == nil,
		})
	}
	return results
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/initcmd/wizard/ -run TestDetectTools -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/stage_tools.go internal/initcmd/wizard/stage_tools_test.go
git commit -m "feat(init): add tool detection with injectable lookup"
```

---

### Task 2: Interactive tools wizard stage

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go:11-22` (add `SelectedTools` to `State`)
- Modify: `internal/initcmd/wizard/wizard.go:44-68` (add stage 4 call in `Run`)
- Modify: `internal/initcmd/wizard/stage_tools.go` (add `runToolsStage`)

**Step 1: Add `SelectedTools` to `State`**

In `wizard.go`, add to `State` struct after `PhaseMapping`:

```go
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

	// Stage 4 outputs
	SelectedTools []string // binary names of CLI tools to include in scaffolded agent files
}
```

**Step 2: Wire stage 4 into `Run`**

In `wizard.go`, update `Run` to call `runToolsStage` after phase stage:

```go
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

	// Stage 4: Tool discovery
	if err := runToolsStage(state); err != nil {
		return nil, err
	}

	return state, nil
}
```

**Step 3: Implement `runToolsStage` in `stage_tools.go`**

Append to `stage_tools.go`:

```go
import (
	"fmt"
	"os/exec"

	"github.com/charmbracelet/huh"
)

func runToolsStage(state *State) error {
	results := detectTools(toolCatalog, exec.LookPath)

	var options []huh.Option[string]
	var preSelected []string

	for _, r := range results {
		label := fmt.Sprintf("%s  (%s)", r.Name, r.Binary)
		if r.Found {
			label = fmt.Sprintf("%s  (%s)  detected: %s", r.Name, r.Binary, r.Path)
			preSelected = append(preSelected, r.Binary)
		} else {
			label = fmt.Sprintf("%s  (%s)  not found", r.Name, r.Binary)
		}
		options = append(options, huh.NewOption(label, r.Binary))
	}

	state.SelectedTools = preSelected

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which CLI tools should agents reference?").
				Description("Pre-selected tools were detected on your PATH. You can add tools you plan to install.").
				Options(options...).
				Value(&state.SelectedTools),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("tool discovery: %w", err)
	}

	return nil
}
```

Note: Merge the imports — the file will have a single import block with both `"os/exec"` and `"github.com/charmbracelet/huh"` and `"fmt"`.

**Step 4: Run existing wizard tests to verify nothing is broken**

Run: `go test ./internal/initcmd/wizard/ -v`
Expected: PASS (existing tests don't call `Run` interactively)

**Step 5: Commit**

```bash
git add internal/initcmd/wizard/wizard.go internal/initcmd/wizard/stage_tools.go
git commit -m "feat(init): add interactive tool discovery wizard stage"
```

---

### Task 3: FilterToolsReference state machine

**Files:**
- Create: `scaffold/tools_filter.go`
- Create: `scaffold/tools_filter_test.go`

**Step 1: Write the failing tests**

Create `internal/initcmd/scaffold/tools_filter_test.go`:

```go
package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// miniDoc is a minimal tools-reference for testing with 2 categories, 3 tools, and a table.
const miniDoc = `## Available CLI Tools

These tools are available in this environment. Prefer them over lower-level alternatives when they apply.

### Code Search & Refactoring

- **ast-grep** (` + "`sg`" + `): Structural code search and replace using AST patterns.
  - Find all calls: ` + "`sg --pattern 'fmt.Errorf($$$)' --lang go`" + `
  - Structural replace: ` + "`sg --pattern 'errors.New($MSG)' --rewrite 'fmt.Errorf($MSG)' --lang go`" + `
- **comby** (` + "`comby`" + `): Language-aware structural search/replace with hole syntax.
  - ` + "`comby 'if err != nil { return :[rest] }' 'if err != nil { return fmt.Errorf(\":[context]: %w\", err) }' .go`" + `

### Diff & Change Analysis

- **difftastic** (` + "`difft`" + `): Structural diff that understands syntax.
  - Compare files: ` + "`difft old.go new.go`" + `

### When to Use What

| Task | Preferred Tool | Fallback |
|------|---------------|----------|
| Rename symbol across files | ` + "`sg`" + ` (ast-grep) | ` + "`sd`" + ` for simple strings |
| Structural code rewrite | ` + "`sg`" + ` or ` + "`comby`" + ` | manual edit |
| Review code changes | ` + "`difft`" + ` | ` + "`git diff`" + ` |
`

func TestFilterToolsReference(t *testing.T) {
	t.Run("all selected returns full document", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{"sg", "comby", "difft"})
		assert.Contains(t, result, "ast-grep")
		assert.Contains(t, result, "comby")
		assert.Contains(t, result, "difftastic")
		assert.Contains(t, result, "### Code Search & Refactoring")
		assert.Contains(t, result, "### Diff & Change Analysis")
		assert.Contains(t, result, "### When to Use What")
	})

	t.Run("none selected strips all tools and headers", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{})
		assert.Contains(t, result, "## Available CLI Tools")
		assert.NotContains(t, result, "### Code Search")
		assert.NotContains(t, result, "### Diff")
		assert.NotContains(t, result, "ast-grep")
		assert.NotContains(t, result, "comby")
		assert.NotContains(t, result, "difft")
		assert.NotContains(t, result, "### When to Use What")
	})

	t.Run("partial category keeps header and selected tool", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{"sg"})
		assert.Contains(t, result, "### Code Search & Refactoring")
		assert.Contains(t, result, "ast-grep")
		assert.NotContains(t, result, "comby")
	})

	t.Run("empty category header stripped", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{"sg"})
		// Diff category has only difft which is not selected
		assert.NotContains(t, result, "### Diff & Change Analysis")
		assert.NotContains(t, result, "difftastic")
	})

	t.Run("table rows filtered by binary presence", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{"difft"})
		assert.Contains(t, result, "Review code changes")
		assert.NotContains(t, result, "Rename symbol")
		assert.NotContains(t, result, "Structural code rewrite")
	})

	t.Run("table suppressed when all data rows filtered", func(t *testing.T) {
		// Only comby selected; table rows all reference sg or difft as primary
		result := FilterToolsReference(miniDoc, []string{"comby"})
		assert.NotContains(t, result, "### When to Use What")
		assert.NotContains(t, result, "| Task |")
	})

	t.Run("multi-line sub-bullets removed atomically", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, []string{"comby", "difft"})
		// sg is excluded — all its sub-bullets must be gone
		assert.NotContains(t, result, "sg --pattern")
		assert.NotContains(t, result, "ast-grep")
		// comby sub-bullets still present
		assert.Contains(t, result, "comby")
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		result := FilterToolsReference("", []string{"sg"})
		assert.Empty(t, result)
	})

	t.Run("nil selected treated as empty", func(t *testing.T) {
		result := FilterToolsReference(miniDoc, nil)
		assert.NotContains(t, result, "ast-grep")
	})
}

func TestFilterToolsReferenceWithRealTemplate(t *testing.T) {
	// Load the actual embedded tools-reference.md
	content, err := templates.ReadFile("templates/shared/tools-reference.md")
	if err != nil {
		t.Skip("embedded templates not available in test context")
	}
	src := string(content)

	t.Run("all 14 tools selected preserves full content", func(t *testing.T) {
		all := []string{"sg", "comby", "difft", "sd", "yq", "mlr", "glow", "typos", "scc", "tokei", "watchexec", "hyperfine", "procs", "mprocs"}
		result := FilterToolsReference(src, all)
		// Every category header must be present
		assert.Contains(t, result, "### Code Search & Refactoring")
		assert.Contains(t, result, "### Diff & Change Analysis")
		assert.Contains(t, result, "### Text Processing")
		assert.Contains(t, result, "### Code Quality")
		assert.Contains(t, result, "### Dev Workflow")
		assert.Contains(t, result, "### When to Use What")
		// No trailing excessive whitespace
		assert.False(t, strings.HasSuffix(result, "\n\n\n"))
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/initcmd/scaffold/ -run TestFilterToolsReference -v`
Expected: FAIL — `FilterToolsReference` undefined.

**Step 3: Implement `FilterToolsReference`**

Create `internal/initcmd/scaffold/tools_filter.go`:

```go
package scaffold

import (
	"regexp"
	"strings"
)

// toolEntryRe matches a tool bullet: `- **name** (`binary`):`.
// Captures the binary name in group 1.
var toolEntryRe = regexp.MustCompile("^- \\*\\*[^*]+\\*\\* \\(`([^`]+)`\\)")

// backtickTokenRe finds all backtick-quoted tokens in a string.
var backtickTokenRe = regexp.MustCompile("`([^`]+)`")

// FilterToolsReference filters tools-reference.md content to include only
// tools whose binary name appears in selected. Strips empty category headers
// and table rows referencing unselected tools.
func FilterToolsReference(content string, selected []string) string {
	if content == "" {
		return ""
	}

	sel := make(map[string]bool, len(selected))
	for _, s := range selected {
		sel[s] = true
	}

	lines := strings.Split(content, "\n")
	var out []string

	// Buffered category header — emitted only when first included tool confirms it
	var categoryBuf []string
	// Buffered tool entry — emitted or discarded based on binary match
	var toolBuf []string
	var toolIncluded bool

	// Table state
	var inTable bool
	var tableBuf []string // header + separator rows
	var tableDataRows []string

	flushTool := func() {
		if toolIncluded && len(toolBuf) > 0 {
			// Emit buffered category header if not yet emitted
			if len(categoryBuf) > 0 {
				out = append(out, categoryBuf...)
				categoryBuf = nil
			}
			out = append(out, toolBuf...)
		}
		toolBuf = nil
		toolIncluded = false
	}

	flushTable := func() {
		if len(tableDataRows) > 0 {
			// Emit category header if needed
			if len(categoryBuf) > 0 {
				out = append(out, categoryBuf...)
				categoryBuf = nil
			}
			out = append(out, tableBuf...)
			out = append(out, tableDataRows...)
		}
		tableBuf = nil
		tableDataRows = nil
		inTable = false
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect table start: "| Task |" header
		if strings.HasPrefix(line, "| Task ") {
			flushTool()
			inTable = true
			tableBuf = []string{line}
			tableDataRows = nil
			continue
		}

		if inTable {
			// Separator row: |------|
			if strings.HasPrefix(line, "|--") || strings.HasPrefix(line, "| --") {
				tableBuf = append(tableBuf, line)
				continue
			}
			// Data row
			if strings.HasPrefix(line, "|") && strings.Contains(line, "|") {
				if tableRowIncluded(line, sel) {
					tableDataRows = append(tableDataRows, line)
				}
				continue
			}
			// End of table (blank line or non-table line)
			flushTable()
			// Fall through to process this line normally
		}

		// Category header: ### ...
		if strings.HasPrefix(line, "### ") {
			flushTool()
			// Start new category buffer
			categoryBuf = []string{line}
			// Include the blank line after header if present
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				categoryBuf = append(categoryBuf, lines[i+1])
				i++
			}
			continue
		}

		// Tool entry start: - **name** (`binary`):
		if m := toolEntryRe.FindStringSubmatch(line); m != nil {
			flushTool()
			binary := m[1]
			toolBuf = []string{line}
			toolIncluded = sel[binary]
			continue
		}

		// Tool continuation: indented sub-bullet
		if len(toolBuf) > 0 && len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			toolBuf = append(toolBuf, line)
			continue
		}

		// Blank line inside a tool entry — could be trailing
		if len(toolBuf) > 0 && strings.TrimSpace(line) == "" {
			flushTool()
			out = append(out, line)
			continue
		}

		// Preamble and other lines (## header, intro paragraph, blank lines)
		flushTool()
		out = append(out, line)
	}

	// Flush remaining state
	flushTool()
	flushTable()

	result := strings.Join(out, "\n")
	// Clean up excessive trailing newlines
	for strings.HasSuffix(result, "\n\n\n") {
		result = strings.TrimSuffix(result, "\n")
	}
	return result
}

// tableRowIncluded returns true if all backtick-quoted tool binaries in the row
// are in the selected set. Tokens that aren't known tool binaries (like command
// examples or "git diff") are ignored.
func tableRowIncluded(row string, sel map[string]bool) bool {
	// Known tool binaries from the catalog
	knownBinaries := map[string]bool{
		"sg": true, "comby": true, "difft": true, "sd": true,
		"yq": true, "mlr": true, "glow": true, "typos": true,
		"scc": true, "tokei": true, "watchexec": true, "hyperfine": true,
		"procs": true, "mprocs": true,
	}

	matches := backtickTokenRe.FindAllStringSubmatch(row, -1)
	hasToolRef := false
	for _, m := range matches {
		token := m[1]
		// Extract first word (handles "sg (ast-grep)" → "sg")
		word := strings.Fields(token)[0]
		if knownBinaries[word] {
			hasToolRef = true
			if !sel[word] {
				return false
			}
		}
	}
	// Rows with no recognized tool references are kept
	return hasToolRef
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/scaffold/ -run TestFilterToolsReference -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/tools_filter.go internal/initcmd/scaffold/tools_filter_test.go
git commit -m "feat(init): add FilterToolsReference state machine for tool filtering"
```

---

### Task 4: Wire filtered tools into scaffold functions

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go:16-25` (`loadToolsReference` → `loadFilteredToolsReference`)
- Modify: `internal/initcmd/scaffold/scaffold.go:56-91` (`writePerRoleProject` signature)
- Modify: `internal/initcmd/scaffold/scaffold.go:93-101` (`WriteClaudeProject`, `WriteOpenCodeProject` signatures)
- Modify: `internal/initcmd/scaffold/scaffold.go:103-136` (`WriteCodexProject` signature)
- Modify: `internal/initcmd/scaffold/scaffold.go:138-169` (`ScaffoldAll` signature)
- Modify: `internal/initcmd/initcmd.go:72` (pass `state.SelectedTools`)

**Step 1: Update `loadToolsReference` to accept selected tools**

In `scaffold.go`, replace `loadToolsReference`:

```go
// loadFilteredToolsReference reads the shared tools-reference template and filters
// it to include only tools whose binary names appear in selectedTools.
// Returns empty string on error (non-fatal).
func loadFilteredToolsReference(selectedTools []string) string {
	content, err := templates.ReadFile("templates/shared/tools-reference.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: tools-reference template missing from binary: %v\n", err)
		return ""
	}
	if len(selectedTools) == 0 {
		return ""
	}
	return FilterToolsReference(string(content), selectedTools)
}
```

**Step 2: Add `selectedTools` parameter to scaffold functions**

Update `writePerRoleProject`:
```go
func writePerRoleProject(dir, harnessName string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	toolsRef := loadFilteredToolsReference(selectedTools)
	// ... rest unchanged
```

Update `WriteClaudeProject`:
```go
func WriteClaudeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	return writePerRoleProject(dir, "claude", agents, selectedTools, force)
}
```

Update `WriteOpenCodeProject`:
```go
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	return writePerRoleProject(dir, "opencode", agents, selectedTools, force)
}
```

Update `WriteCodexProject`:
```go
func WriteCodexProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	// ... validation loop unchanged ...
	toolsRef := loadFilteredToolsReference(selectedTools)
	// ... rest unchanged
```

Update `ScaffoldAll`:
```go
func ScaffoldAll(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	// ... grouping unchanged ...
	type scaffoldFn func(string, []harness.AgentConfig, []string, bool) ([]WriteResult, error)
	scaffolders := map[string]scaffoldFn{
		"claude":   WriteClaudeProject,
		"opencode": WriteOpenCodeProject,
		"codex":    WriteCodexProject,
	}
	for _, harnessName := range []string{"claude", "opencode", "codex"} {
		// ...
		harnessResults, err := scaffolders[harnessName](dir, harnessAgents, selectedTools, force)
		// ...
	}
	// ...
```

**Step 3: Wire `state.SelectedTools` in `initcmd.go`**

Update line 72 in `initcmd.go`:

```go
results, err := scaffold.ScaffoldAll(projectDir, agentConfigs, state.SelectedTools, opts.Force)
```

**Step 4: Run the full test suite to verify compilation**

Run: `go build ./...`
Expected: PASS (compiles cleanly)

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/scaffold.go internal/initcmd/initcmd.go
git commit -m "refactor(init): wire filtered tools through scaffold pipeline"
```

---

### Task 5: Update existing scaffold tests for new parameter

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold_test.go` (add `selectedTools` arg to all function calls)

**Step 1: Update all test call sites**

Every call to `WriteClaudeProject`, `WriteOpenCodeProject`, `WriteCodexProject`, and `ScaffoldAll` needs the new `selectedTools` parameter. For existing tests that expect the full tools-reference to be injected, pass all 14 tool binaries:

```go
var allTools = []string{"sg", "comby", "difft", "sd", "yq", "mlr", "glow", "typos", "scc", "tokei", "watchexec", "hyperfine", "procs", "mprocs"}
```

Replace every call like:
- `WriteClaudeProject(dir, agents, false)` → `WriteClaudeProject(dir, agents, allTools, false)`
- `WriteClaudeProject(dir, agents, true)` → `WriteClaudeProject(dir, agents, allTools, true)`
- `WriteOpenCodeProject(dir, agents, false)` → `WriteOpenCodeProject(dir, agents, allTools, false)`
- `WriteCodexProject(dir, agents, false)` → `WriteCodexProject(dir, agents, allTools, false)`
- `ScaffoldAll(dir, agents, false)` → `ScaffoldAll(dir, agents, allTools, false)`

**Step 2: Add a test verifying filtered injection**

Add to `TestToolsReferenceInjected`:

```go
t.Run("filtered tools reference omits unselected tools", func(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	_, err := WriteClaudeProject(dir, agents, []string{"sg", "difft"}, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "ast-grep")
	assert.Contains(t, string(content), "difft")
	assert.NotContains(t, string(content), "comby")
	assert.NotContains(t, string(content), "typos")
	assert.NotContains(t, string(content), "watchexec")
})

t.Run("empty tools selection produces no tools reference", func(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	_, err := WriteClaudeProject(dir, agents, []string{}, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "ast-grep")
	assert.NotContains(t, string(content), "Available CLI Tools")
})
```

**Step 3: Run the full scaffold test suite**

Run: `go test ./internal/initcmd/scaffold/ -v`
Expected: PASS

**Step 4: Run all initcmd tests**

Run: `go test ./internal/initcmd/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/scaffold_test.go
git commit -m "test(init): update scaffold tests for tool filtering parameter"
```

---

### Task 6: Final verification

**Step 1: Run all project tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 2: Build the binary**

Run: `go build -o /dev/null ./cmd/kq`
Expected: Success (clean compile)

**Step 3: Verify no lint issues**

Run: `golangci-lint run ./internal/initcmd/...` (if available, otherwise `go vet ./internal/initcmd/...`)
Expected: Clean

**Step 4: Commit any fixups if needed, otherwise done**
