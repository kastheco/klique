# Global Skill Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure personal skills installed to `~/.agents/skills/` are automatically symlinked into every harness's global skill directory during `kq init`, and provide a standalone `kq skills sync` command for on-demand re-syncing.

**Architecture:** `~/.agents/skills/` is the canonical location for personal (non-project) skills. Each harness reads from a different global path:
- Claude Code: `~/.claude/skills/`
- OpenCode: `~/.config/opencode/skills/`
- Codex: `~/.agents/skills/` (native, no sync needed)

A new `SyncGlobalSkills` function in the `harness` package iterates `~/.agents/skills/`, creating relative symlinks in each harness's global skill dir. This runs after `InstallSuperpowers` during init, and is exposed as `kq skills sync` for manual use. Skills that are themselves symlinks in `~/.agents/skills/` (e.g. `superpowers/`) are skipped — they're managed by `InstallSuperpowers`.

**Tech Stack:** Go `os.Symlink`, `os.ReadDir`, existing harness interface, cobra subcommand.

---

### Task 1: Add `SyncGlobalSkills` to the Harness interface

**Files:**
- Modify: `internal/initcmd/harness/harness.go`

**Step 1: Write the failing test**

Add to a new file `internal/initcmd/harness/sync_test.go`:

```go
package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncGlobalSkills_Claude(t *testing.T) {
	home := t.TempDir()

	// Create canonical skills
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"cli-tools", "my-custom-skill"} {
		dir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))
	}

	// Create a symlink skill (simulating superpowers — should be skipped)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "superpowers-repo", "skills"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(home, "superpowers-repo", "skills"),
		filepath.Join(agentsSkills, "superpowers"),
	))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// Real skills should be symlinked
	for _, name := range []string{"cli-tools", "my-custom-skill"} {
		link := filepath.Join(home, ".claude", "skills", name)
		target, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked", name)
		assert.Equal(t, filepath.Join("..", "..", ".agents", "skills", name), target)
	}

	// superpowers symlink should NOT be created (already managed by InstallSuperpowers)
	_, err = os.Readlink(filepath.Join(home, ".claude", "skills", "superpowers"))
	assert.Error(t, err, "superpowers should not be synced")
}

func TestSyncGlobalSkills_OpenCode(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))

	err := SyncGlobalSkills(home, "opencode")
	require.NoError(t, err)

	link := filepath.Join(home, ".config", "opencode", "skills", "cli-tools")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	// OpenCode uses a different base path, so relative symlink differs
	assert.Contains(t, target, "cli-tools")

	// Symlink should resolve to actual content
	content, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

func TestSyncGlobalSkills_ReplacesStaleSymlinks(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("new"), 0o644))

	// Create stale symlink
	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "cli-tools")))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// Should resolve to new content
	content, err := os.ReadFile(filepath.Join(claudeSkills, "cli-tools", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}

func TestSyncGlobalSkills_SkipsNonSymlinkEntries(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))

	// Create a real directory (user-managed) in destination — should not be overwritten
	claudeSkills := filepath.Join(home, ".claude", "skills")
	userManaged := filepath.Join(claudeSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(userManaged, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userManaged, "SKILL.md"), []byte("custom"), 0o644))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// User-managed dir should be untouched
	content, err := os.ReadFile(filepath.Join(userManaged, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "custom", string(content))
}

func TestSyncGlobalSkills_NoSkillsDir(t *testing.T) {
	home := t.TempDir()

	// No ~/.agents/skills/ — should be a no-op, not an error
	err := SyncGlobalSkills(home, "claude")
	assert.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/harness/ -run TestSyncGlobalSkills -v`
Expected: FAIL — `SyncGlobalSkills` undefined.

**Step 3: Implement `SyncGlobalSkills`**

Create `internal/initcmd/harness/sync.go`:

```go
package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

// globalSkillsDir returns the global skills directory for a given harness.
// Returns the absolute path where the harness expects to find skill symlinks.
func globalSkillsDir(home, harnessName string) string {
	switch harnessName {
	case "claude":
		return filepath.Join(home, ".claude", "skills")
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "skills")
	case "codex":
		return filepath.Join(home, ".agents", "skills") // native, same as canonical
	default:
		return filepath.Join(home, "."+harnessName, "skills")
	}
}

// SyncGlobalSkills creates symlinks from a harness's global skill directory
// to ~/.agents/skills/<skill> for each personal skill.
//
// Skips entries in ~/.agents/skills/ that are themselves symlinks (managed
// externally, e.g. superpowers). Replaces existing symlinks in the destination.
// Skips non-symlink entries in the destination (user-managed directories).
//
// For codex, this is a no-op since codex reads from ~/.agents/skills/ directly.
func SyncGlobalSkills(home, harnessName string) error {
	if harnessName == "codex" {
		return nil // codex reads ~/.agents/skills/ natively
	}

	canonicalDir := filepath.Join(home, ".agents", "skills")
	entries, err := os.ReadDir(canonicalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", canonicalDir, err)
	}

	destDir := globalSkillsDir(home, harnessName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s skills dir: %w", harnessName, err)
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip non-directories
		if !entry.IsDir() {
			// Could be a file like .skill-lock.json
			continue
		}

		// Skip entries that are themselves symlinks (e.g. superpowers/)
		// These are managed by InstallSuperpowers, not by us.
		srcPath := filepath.Join(canonicalDir, name)
		fi, err := os.Lstat(srcPath)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}

		link := filepath.Join(destDir, name)

		// Compute relative symlink target from destDir to canonicalDir
		relTarget, err := filepath.Rel(destDir, srcPath)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", name, err)
		}

		// Check if link already exists
		if lfi, err := os.Lstat(link); err == nil {
			if lfi.Mode()&os.ModeSymlink != 0 {
				// Replace existing symlink
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove existing symlink %s: %w", name, err)
				}
			} else {
				// Non-symlink entry (user-managed) — skip
				continue
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", link, err)
		}

		if err := os.Symlink(relTarget, link); err != nil {
			return fmt.Errorf("symlink %s skill %s: %w", harnessName, name, err)
		}
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/harness/ -run TestSyncGlobalSkills -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/initcmd/harness/sync.go internal/initcmd/harness/sync_test.go
git commit -m "feat(harness): add SyncGlobalSkills to sync ~/.agents/skills/ to harness dirs"
```

---

### Task 2: Call `SyncGlobalSkills` during `kq init`

**Files:**
- Modify: `internal/initcmd/initcmd.go`

**Step 1: Add sync call after InstallSuperpowers loop**

In `initcmd.go`, after the superpowers installation loop (line ~53), add global skill sync:

```go
	// Stage 4a-2: Sync personal skills to all harness global dirs
	fmt.Println("\nSyncing personal skills...")
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("  WARNING: could not get home dir: %v\n", err)
	} else {
		for _, name := range state.SelectedHarness {
			fmt.Printf("  %-12s ", name)
			if err := harness.SyncGlobalSkills(home, name); err != nil {
				fmt.Printf("FAILED: %v\n", err)
			} else {
				fmt.Println("OK")
			}
		}
	}
```

**Step 2: Import `os` if not already imported**

Check imports at top of `initcmd.go` — `os` is already imported (line 5). No change needed.

**Step 3: Run full test suite**

Run: `go test ./internal/initcmd/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/initcmd/initcmd.go
git commit -m "feat(init): sync personal skills to harness dirs during kq init"
```

---

### Task 3: Add `kq skills sync` subcommand

**Files:**
- Create: `cmd/skills.go` (or wherever cobra commands live — check project structure)

**Step 1: Find existing command structure**

Check where cobra commands are defined:

```bash
find . -name '*.go' -path '*/cmd/*' | head -20
ast-grep run -p 'cobra.Command' -l go cmd/
```

**Step 2: Write the failing test**

Create `cmd/skills_test.go`:

```go
func TestSkillsSyncCommand(t *testing.T) {
	// Verify the command exists and has correct metadata
	cmd := newSkillsSyncCmd()
	assert.Equal(t, "sync", cmd.Use)
	assert.Contains(t, cmd.Short, "skill")
}
```

**Step 3: Implement the skills sync command**

Create `cmd/skills.go`:

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/kastheco/klique/internal/initcmd/harness"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage agent skills",
	}
	cmd.AddCommand(newSkillsSyncCmd())
	return cmd
}

func newSkillsSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync personal skills from ~/.agents/skills/ to all harness skill directories",
		Long: `Reads personal skills from ~/.agents/skills/ and creates symlinks in each
detected harness's global skill directory:

  Claude Code:  ~/.claude/skills/
  OpenCode:     ~/.config/opencode/skills/
  Codex:        (native, no sync needed)

Replaces stale symlinks. Skips user-managed directories and symlink-based
skills (e.g. superpowers/) which are managed by 'kq init'.`,
		RunE: runSkillsSync,
	}
}

func runSkillsSync(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	registry := harness.NewRegistry()
	synced := 0

	for _, name := range registry.All() {
		h := registry.Get(name)
		if _, found := h.Detect(); !found {
			fmt.Printf("  %-12s SKIP (not installed)\n", name)
			continue
		}

		fmt.Printf("  %-12s ", name)
		if err := harness.SyncGlobalSkills(home, name); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			fmt.Println("OK")
			synced++
		}
	}

	if synced == 0 {
		fmt.Println("\nNo harnesses detected. Install claude, opencode, or codex first.")
	}

	return nil
}
```

**Step 4: Wire into root command**

In the root command file, add `rootCmd.AddCommand(newSkillsCmd())`.

**Step 5: Run tests and build**

```bash
go test ./cmd/ -run TestSkillsSync -v
go build ./...
```

Expected: PASS and clean build.

**Step 6: Commit**

```bash
git add cmd/skills.go cmd/skills_test.go
git commit -m "feat(cli): add 'kq skills sync' command for on-demand skill syncing"
```

---

### Task 4: Add `kq skills list` for visibility

**Files:**
- Modify: `cmd/skills.go`

**Step 1: Implement skills list command**

Add to `cmd/skills.go`:

```go
func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List personal skills in ~/.agents/skills/",
		RunE:  runSkillsList,
	}
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	skillsDir := filepath.Join(home, ".agents", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No personal skills found. Install skills to ~/.agents/skills/")
			return nil
		}
		return err
	}

	fmt.Printf("Personal skills in %s:\n\n", skillsDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Check if it's a symlink (externally managed)
		fi, err := os.Lstat(filepath.Join(skillsDir, name))
		if err != nil {
			continue
		}
		managed := ""
		if fi.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(filepath.Join(skillsDir, name))
			managed = fmt.Sprintf(" -> %s (external)", target)
		}

		fmt.Printf("  %-30s%s\n", name, managed)
	}

	return nil
}
```

Add to `newSkillsCmd()`: `cmd.AddCommand(newSkillsListCmd())`.

**Step 2: Build and test manually**

```bash
go build ./...
./kq skills list
./kq skills sync
```

**Step 3: Commit**

```bash
git add cmd/skills.go
git commit -m "feat(cli): add 'kq skills list' to show personal skills"
```

---

### Task 5: Embed cli-tools skill as a project skill and make it mandatory in all agents

**Context:** The current `templates/shared/tools-reference.md` (60 lines) is a compact summary injected into every agent file via `{{TOOLS_REFERENCE}}`. The `cli-tools` skill at `~/.agents/skills/cli-tools/` (925 lines across 8 files — SKILL.md + 7 resource files) is its evolved replacement. Rather than inlining 925 lines into every agent file, we embed the cli-tools skill tree as a project skill (like tui-design, golang-pro, tmux-orchestration already are) and make every agent template unconditionally load it.

**Approach:**
1. Copy the cli-tools skill tree into `templates/skills/cli-tools/` (SKILL.md + resources/) — `WriteProjectSkills` already walks this directory and writes to `.agents/skills/`
2. Replace `{{TOOLS_REFERENCE}}` in every agent template with a mandatory skill-load directive
3. Remove `templates/shared/tools-reference.md` and the `FilterToolsReference` machinery — tool filtering is no longer needed since the skill is loaded at runtime, not inlined
4. Update the wizard's tool-selection stage accordingly (it currently drives which tools appear in `tools-reference.md`)

**Files:**
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/SKILL.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/ast-grep.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/comby.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/difftastic.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/sd.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/yq.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/typos.md`
- Create: `internal/initcmd/scaffold/templates/skills/cli-tools/resources/scc.md`
- Delete: `internal/initcmd/scaffold/templates/shared/tools-reference.md`
- Modify: `internal/initcmd/scaffold/scaffold.go` (remove `loadFilteredToolsReference`, `renderTemplate` drops `{{TOOLS_REFERENCE}}`)
- Delete or gut: `internal/initcmd/scaffold/tools_filter.go`
- Delete or gut: `internal/initcmd/scaffold/tools_filter_test.go`
- Modify: all agent templates (`claude/agents/*.md`, `opencode/agents/*.md`, `codex/AGENTS.md`) — replace `{{TOOLS_REFERENCE}}` with mandatory load directive
- Modify: `internal/initcmd/scaffold/scaffold_test.go` — update assertions

**Step 1: Copy cli-tools skill tree into embedded templates**

Copy the full directory structure from `~/.agents/skills/cli-tools/` into `internal/initcmd/scaffold/templates/skills/cli-tools/`:

```
templates/skills/cli-tools/
├── SKILL.md
└── resources/
    ├── ast-grep.md
    ├── comby.md
    ├── difftastic.md
    ├── sd.md
    ├── yq.md
    ├── typos.md
    └── scc.md
```

No content modifications needed — the existing `WriteProjectSkills` function walks `templates/skills/` and writes the tree verbatim to `.agents/skills/`. The cli-tools skill will be scaffolded alongside tui-design, golang-pro, and tmux-orchestration automatically.

**Step 2: Update every agent template**

Replace `{{TOOLS_REFERENCE}}` in all agent templates with a mandatory, unconditional skill-load directive. Unlike other project skills which say "load when relevant", cli-tools is always required.

For claude and opencode per-role templates (`coder.md`, `reviewer.md`, `planner.md`, `chat.md`):

```markdown
## CLI Tools (MANDATORY)

You MUST read the `cli-tools` skill (SKILL.md) at the start of every session.
It contains tool selection tables, quick references, and common mistakes for
ast-grep, comby, difftastic, sd, yq, typos, and scc. The deep-dive reference
files in `resources/` should be read when you need to use that specific tool —
you don't need to read all of them upfront.
```

For codex `AGENTS.md` (which has no skill-loading mechanism — it's a flat file):

```markdown
## CLI Tools
Read the `cli-tools` skill (SKILL.md) at session start. Read individual
resource files in `resources/` when using that specific tool.
```

**Step 3: Remove tools-reference.md and FilterToolsReference**

- Delete `templates/shared/tools-reference.md`
- Remove `loadFilteredToolsReference` from `scaffold.go`
- Remove `{{TOOLS_REFERENCE}}` replacement from `renderTemplate` and `WriteCodexProject`
- Delete or empty `tools_filter.go` and `tools_filter_test.go`
- Update `writePerRoleProject` and `WriteCodexProject` signatures to drop `selectedTools []string` if no longer used anywhere, or keep the parameter for future use

**Step 4: Update scaffold tests**

In `scaffold_test.go`:
- Remove assertions checking `{{TOOLS_REFERENCE}}` is absent (it no longer exists in templates)
- Remove tests for filtered tools reference content
- Add assertion: `cli-tools/SKILL.md` exists in `.agents/skills/` after scaffold
- Add assertion: `cli-tools/resources/ast-grep.md` (and other resources) exist after scaffold
- Add assertion: agent files contain the mandatory cli-tools load directive

**Step 5: Consider tool-selection wizard impact**

The wizard's tool-selection stage (`internal/initcmd/wizard/stage_tools.go`) currently lets users pick which CLI tools to include. With the cli-tools skill written as a complete tree, tool filtering happens differently — the skill itself is always present, but the user might not have all tools installed.

Options (decide during implementation):
- A) Keep the wizard stage but use it to generate a `.agents/skills/cli-tools/.tool-config` that the skill can reference
- B) Remove the wizard stage entirely — the skill's "Tool Selection by Task" table already guides agents to the right tool, and missing binaries produce clear errors
- C) Keep the wizard stage and filter the cli-tools SKILL.md content at write time (reintroduces filtering complexity)

Recommendation: Option B — simplest, and the tool-discovery design plan already handles missing tools gracefully.

**Step 6: Run tests**

```bash
go test ./internal/initcmd/scaffold/ -v
go test ./internal/initcmd/... -v
```

Expected: ALL PASS

**Step 7: Commit**

```bash
git add internal/initcmd/scaffold/templates/skills/cli-tools/ \
        internal/initcmd/scaffold/templates/claude/agents/*.md \
        internal/initcmd/scaffold/templates/opencode/agents/*.md \
        internal/initcmd/scaffold/templates/codex/AGENTS.md \
        internal/initcmd/scaffold/scaffold.go \
        internal/initcmd/scaffold/scaffold_test.go
git rm internal/initcmd/scaffold/templates/shared/tools-reference.md \
       internal/initcmd/scaffold/tools_filter.go \
       internal/initcmd/scaffold/tools_filter_test.go
git commit -m "feat(scaffold): replace tools-reference with cli-tools project skill

cli-tools is the evolved replacement for tools-reference.md. Instead of
inlining a 60-line summary into every agent file, the full skill tree
(SKILL.md + 7 resource files, 925 lines) is written to .agents/skills/
as a project skill. Every agent template now has a mandatory directive
to load cli-tools at session start — no exceptions, no conditional loading."
```

---

### Task 6: Build verification

**Files:**
- Verify: all modified files

**Step 1: Run full test suite**

```bash
go test ./... -count=1
```

Expected: ALL PASS

**Step 2: Build and smoke test**

```bash
go build -o kq .
./kq skills list
./kq skills sync
```

Expected: Lists skills (including cli-tools), syncs without error.

**Step 3: Verify symlinks are correct**

```bash
ls -la ~/.claude/skills/ | grep cli-tools
ls -la ~/.config/opencode/skills/ | grep cli-tools
```

Both should show symlinks to `~/.agents/skills/cli-tools`.

**Step 4: Verify cli-tools skill tree is scaffolded correctly**

```bash
tmpdir=$(mktemp -d)
./kq init --dir "$tmpdir" --harness claude --harness opencode --harness codex

# Skill tree written to .agents/skills/
ls "$tmpdir"/.agents/skills/cli-tools/SKILL.md
ls "$tmpdir"/.agents/skills/cli-tools/resources/

# Symlinked into harness skill dirs
ls -la "$tmpdir"/.claude/skills/cli-tools
ls -la "$tmpdir"/.opencode/skills/cli-tools

# Every agent file has the mandatory load directive
grep -l "cli-tools" "$tmpdir"/.claude/agents/*.md "$tmpdir"/.opencode/agents/*.md "$tmpdir"/.codex/AGENTS.md

rm -rf "$tmpdir"
```

Expected: Full cli-tools tree in `.agents/skills/`, symlinked into each harness, and every agent file references it as mandatory.

**Step 5: Final commit if any loose changes**

```bash
go test ./... -count=1 && git add -A && git status
```
