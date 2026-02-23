# Rename klique → kasmos: Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.
> Use superpowers:dispatching-parallel-agents for Phase 2 (6 parallel agents).
> Use superpowers:cli-tools — prefer `sd` for string replacements, `comby` for structural Go edits.

**Goal:** Rename the project from `klique` to `kasmos` across all source, config, build, web, and documentation files. Binary becomes `kasmos`, CLI command becomes `kas`, tmux prefix becomes `kas_`, config dir moves to `~/.config/kasmos/`.

**Architecture:** Mechanical bulk rename using `sd` for literal replacements, `comby` for structural Go changes (function bodies, regex patterns), and targeted manual edits for non-trivial rewrites. Organized into 4 phases with maximum parallelism in Phase 2.

**Tech Stack:** `sd` (string replacement), `comby` (structural Go edits), `go build`/`go test` (verification), `gh` (GitHub API)

**Naming Table:**

| Thing | Old | New |
|-------|-----|-----|
| Go module | `github.com/kastheco/kasmos` | `github.com/kastheco/kasmos` |
| Binary | `klique` | `kasmos` |
| Cobra `Use` | `klique` | `kas` |
| Symlink aliases | `kq` | `kas`, `ks`, `km` |
| Tmux prefix | `klique_` | `kas_` |
| LazyGit prefix | `klique_lazygit_` | `kas_lazygit_` |
| Log file | `klique.log` | `kas.log` |
| Config dir | `~/.klique` | `~/.config/kasmos/` |
| Docs/comments | `klique`/`kq` | `kasmos`/`kas` |

---

## Dependency Graph

```
Phase 0: GitHub rename (manual, user action)
    ↓
Phase 1: Go module path rename (sequential gate — all imports must change first)
    ↓
Phase 2: PARALLEL — 6 independent agents, zero file overlap
    ├── Agent A: Session & Runtime (tmux prefix, lazygit, log, daemon, lifecycle)
    ├── Agent B: CLI, Config & Init (cobra, config dir migration, init wizard, check)
    ├── Agent C: Build, CI & Distribution (Justfile, goreleaser, workflows, dist artifacts)
    ├── Agent D: Web Frontend
    ├── Agent E: Scaffold Templates (agent config templates shipped to users)
    └── Agent F: Documentation & Project Agent Configs
    ↓
Phase 3: Final verification + commit
```

**File ownership (NO overlap between agents):**

| Agent | Files |
|-------|-------|
| A | `session/tmux/*.go`, `session/instance.go`, `session/instance_lifecycle.go`, `session/instance_session.go`, `session/instance_session_async_test.go`, `session/storage.go`, `session/git/*.go`, `daemon/daemon.go`, `log/log.go`, `ui/git_pane.go`, `ui/preview_test.go` |
| B | `main.go`, `config/config.go`, `config/config_test.go`, `config/toml.go`, `config/state.go`, `config/planstate/planstate_test.go`, `app/help.go`, `internal/initcmd/initcmd.go`, `internal/initcmd/initcmd_test.go`, `internal/initcmd/wizard/*.go`, `internal/check/*.go`, `check.go`, `check_test.go`, `skills.go` |
| C | `Justfile`, `Makefile`, `.goreleaser.yaml`, `install.sh`, `clean.sh`, `clean_hard.sh`, `.github/workflows/*.yml`, `dist/**` |
| D | `web/**` |
| E | `internal/initcmd/scaffold/scaffold.go`, `internal/initcmd/scaffold/scaffold_test.go`, `internal/initcmd/scaffold/templates/**` |
| F | `README.md`, `CONTRIBUTING.md`, `CLA.md`, `.claude/agents/*.md`, `.opencode/agents/planner.md`, `.opencode/opencode.jsonc`, `docs/plans/*.md` |

---

## Phase 0: GitHub Repo Rename (Manual — User Action)

These steps must be done by the user before implementation begins.

**Step 1: Rename the old kasmos repo**

```bash
gh repo rename kasmold --repo kastheco/kasmos --yes
```

**Step 2: Rename klique → kasmos**

```bash
gh repo rename kasmos --repo kastheco/kasmos --yes
```

**Step 3: Update local git remote**

```bash
git remote set-url origin git@github.com:kastheco/kasmos.git
```

**Step 4: Verify**

```bash
git remote -v
# Expected: origin git@github.com:kastheco/kasmos.git
gh repo view kastheco/kasmos --json name
# Expected: {"name":"kasmos"}
```

---

## Phase 1: Go Module Path Rename (Sequential Gate)

**MUST complete before Phase 2.** All Go import paths change here.

Replace all `github.com/kastheco/kasmos` → `github.com/kastheco/kasmos` in Go source and go.mod.

**Files:**
- Modify: `go.mod` (line 1)
- Modify: all ~65 `.go` files with import paths

**Step 1: Replace module path in go.mod and all Go files**

```bash
sd 'github.com/kastheco/kasmos' 'github.com/kastheco/kasmos' go.mod $(rg -l 'kastheco/kasmos' --type go)
```

**Step 2: Verify compilation**

Run: `go build ./...`
Expected: SUCCESS (no errors)

**Step 3: Run tests**

Run: `go test ./...`
Expected: PASS (all tests pass)

**Step 4: Commit**

```bash
git add -A && git commit -m "refactor: rename Go module from klique to kasmos"
```

---

## Phase 2: Parallel Rename (6 Independent Agents)

**Dispatch ALL agents simultaneously.** Zero file overlap between agents.

---

### Agent A: Session & Runtime

**Scope:** Tmux prefix, lazygit, log file, daemon, session lifecycle, git worktree comments.

**Files:**
- Modify: `session/tmux/tmux.go`, `session/tmux/tmux_test.go`
- Modify: `session/instance.go`, `session/instance_lifecycle.go`
- Modify: `session/instance_session_async_test.go`, `session/storage.go`
- Modify: `ui/git_pane.go`, `ui/preview_test.go`
- Modify: `daemon/daemon.go`, `log/log.go`
- Check (import-only, likely no-op): `session/tmux/tmux_attach.go`, `session/tmux/tmux_io.go`, `session/tmux/tmux_unix.go`, `session/tmux/tmux_windows.go`, `session/instance_session.go`, `session/git/*.go`

**Step 1: Rename TmuxPrefix and helper function in `session/tmux/tmux.go`**

```bash
sd 'const TmuxPrefix = "klique_"' 'const TmuxPrefix = "kas_"' session/tmux/tmux.go
```

Rename the helper function:

```bash
sd 'func toKliqueTmuxName' 'func toKasTmuxName' session/tmux/tmux.go
```

Update all callers of `toKliqueTmuxName` → `toKasTmuxName`:

```bash
sd 'toKliqueTmuxName' 'toKasTmuxName' $(rg -l 'toKliqueTmuxName' --type go)
```

**Step 2: Update cleanup regex to handle three-generation legacy**

The regex must match current `kas_` sessions plus legacy `klique_` and `hivemind_` sessions.

```bash
sd 'cleanupSessionsRe matches current klique_ sessions and legacy hivemind_ sessions' \
   'cleanupSessionsRe matches current kas_ sessions and legacy klique_/hivemind_ sessions' \
   session/tmux/tmux.go

sd '(?:klique_|hivemind_)' '(?:kas_|klique_|hivemind_)' session/tmux/tmux.go

sd 'CleanupSessions kills all tmux sessions that start with the klique prefix' \
   'CleanupSessions kills all tmux sessions that start with the kas prefix' \
   session/tmux/tmux.go
```

**Step 3: Update lazygit tmux prefix in `ui/git_pane.go`**

```bash
sd 'klique_lazygit_' 'kas_lazygit_' ui/git_pane.go
sd 'klique-lazygit-override' 'kas-lazygit-override' ui/git_pane.go
```

**Step 4: Update log file name in `log/log.go`**

```bash
sd '"klique.log"' '"kas.log"' log/log.go
```

**Step 5: Update notification sender in `session/instance.go`**

```bash
sd 'SendNotification("klique"' 'SendNotification("kas"' session/instance.go
```

**Step 6: Update commit message in `session/instance_lifecycle.go`**

```bash
sd '[klique] update from' '[kas] update from' session/instance_lifecycle.go
```

**Step 7: Update storage comment in `session/storage.go`**

```bash
sd '.klique/worktrees/' '.config/kasmos/worktrees/' session/storage.go
```

**Step 8: Update daemon comment in `daemon/daemon.go`**

```bash
sd 'Find the klique binary' 'Find the kasmos binary' daemon/daemon.go
```

**Step 9: Update temp file pattern in `session/instance_session_async_test.go`**

```bash
sd '"klique-pty-' '"kas-pty-' session/instance_session_async_test.go
```

**Step 10: Update tmux test assertions in `session/tmux/tmux_test.go`**

All test strings use the `klique_` prefix — bulk replace:

```bash
sd 'klique_' 'kas_' session/tmux/tmux_test.go
```

Then fix the cleanup test which should assert legacy `klique_` sessions are also killed. Find the test `"kills klique, legacy hivemind, and lazygit sessions"` and update:

```bash
sd 'kills klique, legacy hivemind, and lazygit sessions' \
   'kills kas, legacy klique/hivemind, and lazygit sessions' \
   session/tmux/tmux_test.go
```

The test's mock output needs to use `kas_` prefix for current sessions. After the bulk `sd` above, verify the test data is consistent: current sessions use `kas_` prefix, and the cleanup regex still matches legacy `klique_`/`hivemind_` prefixes. If the test also asserts cleanup of legacy sessions, add test data with `klique_` prefix entries.

**Step 11: Update `ui/preview_test.go`**

```bash
sd 'klique_' 'kas_' ui/preview_test.go
```

**Step 12: Verify remaining `klique` in agent A files**

```bash
rg 'klique' session/ daemon/ log/log.go ui/git_pane.go ui/preview_test.go
```

Expected: zero matches (except possibly inside the cleanup regex pattern string `klique_` which is intentionally kept for legacy matching).

**Step 13: Run tests**

```bash
go test ./session/... ./daemon/... ./log/... ./ui/... -v
```

Expected: PASS

**Step 14: Commit**

```bash
git add session/ daemon/ log/ ui/git_pane.go ui/preview_test.go
git commit -m "refactor: rename tmux prefix to kas_, update session/runtime layer"
```

---

### Agent B: CLI, Config & Init

**Scope:** Cobra command, config directory XDG migration, init wizard, check command, skills.

**Files:**
- Modify: `main.go`
- Modify: `config/config.go`, `config/config_test.go`, `config/toml.go`, `config/state.go`
- Modify: `config/planstate/planstate_test.go`
- Modify: `app/help.go`
- Modify: `internal/initcmd/initcmd.go`, `internal/initcmd/initcmd_test.go`
- Modify: `internal/initcmd/wizard/wizard.go`, `internal/initcmd/wizard/wizard_test.go`, `internal/initcmd/wizard/stage_agents.go`
- Modify: `internal/check/check.go`, `internal/check/check_test.go`, `internal/check/project.go`, `internal/check/global.go`
- Modify: `check.go`, `check_test.go`, `skills.go`

**Step 1: Update `main.go` — Cobra command, version, error messages**

```bash
sd 'Use:   "klique"' 'Use:   "kas"' main.go
sd 'Short: "klique - Manage multiple AI agents' 'Short: "kas - Manage multiple AI agents' main.go
sd 'error: klique must be run from' 'error: kas must be run from' main.go
sd '"Print the version number of klique"' '"Print the version number of kas"' main.go
sd 'klique version %s' 'kas version %s' main.go
sd 'kastheco/kasmos/releases' 'kastheco/kasmos/releases' main.go
sd 'Write ~/.klique/config.toml' 'Write ~/.config/kasmos/config.toml' main.go
sd 'kqInitCmd' 'kasInitCmd' main.go
```

**Step 2: Rewrite `GetConfigDir()` in `config/config.go`**

Use comby for the structural rewrite of the function body:

```bash
comby \
  'func GetConfigDir() (string, error) {:[body]}' \
  'func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	newDir := filepath.Join(homeDir, ".config", "kasmos")

	// Already exists — fast path
	if _, err := os.Stat(newDir); err == nil {
		return newDir, nil
	}

	// Try migrating from legacy directories (most recent first)
	legacyDirs := []string{
		filepath.Join(homeDir, ".klique"),
		filepath.Join(homeDir, ".hivemind"),
	}

	for _, oldDir := range legacyDirs {
		if _, err := os.Stat(oldDir); err == nil {
			// Ensure parent ~/.config/ exists
			if mkErr := os.MkdirAll(filepath.Dir(newDir), 0755); mkErr != nil {
				log.ErrorLog.Printf("failed to create %s: %v", filepath.Dir(newDir), mkErr)
				return oldDir, nil
			}
			if renameErr := os.Rename(oldDir, newDir); renameErr != nil {
				log.ErrorLog.Printf("failed to migrate %s to %s: %v", oldDir, newDir, renameErr)
				return oldDir, nil
			}
			return newDir, nil
		}
	}

	return newDir, nil
}' \
  config/config.go -in-place
```

Update the doc comment above:

```bash
sd 'On first run after a rename, it migrates ~/.hivemind to ~/.klique.' \
   'Uses XDG-compliant ~/.config/kasmos/. On first run, migrates legacy\n// directories: ~/.klique → or ~/.hivemind → ~/.config/kasmos/.' \
   config/config.go
```

**Step 3: Update `config/toml.go` comments**

```bash
sd '~/.klique/config.toml' '~/.config/kasmos/config.toml' config/toml.go
sd '# Generated by kq init' '# Generated by kas init' config/toml.go
```

**Step 4: Rewrite `config/config_test.go`**

The tests need substantial rewriting for three-generation migration. Key changes:

```bash
sd 'HasSuffix(configDir, ".klique")' 'HasSuffix(configDir, filepath.Join(".config", "kasmos"))' config/config_test.go
sd '"migrates legacy .hivemind to .klique"' '"migrates legacy .hivemind to .config/kasmos"' config/config_test.go
sd '"skips migration when .klique already exists"' '"skips migration when .config/kasmos already exists"' config/config_test.go
```

Update test directory creation — replace `.klique` with `.config/kasmos`:

```bash
sd 'filepath.Join(tempHome, ".klique")' 'filepath.Join(tempHome, ".config", "kasmos")' config/config_test.go
```

Add a new test case for `.klique` → `.config/kasmos` migration. After the `sd` replacements, manually add:

```go
t.Run("migrates legacy .klique to .config/kasmos", func(t *testing.T) {
    tempHome := t.TempDir()
    t.Setenv("HOME", tempHome)
    oldDir := filepath.Join(tempHome, ".klique")
    require.NoError(t, os.MkdirAll(oldDir, 0755))
    require.NoError(t, os.WriteFile(filepath.Join(oldDir, "config.json"), []byte("{}"), 0644))

    configDir, err := GetConfigDir()
    require.NoError(t, err)
    assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))
    assert.NoFileExists(t, oldDir)
})
```

**Step 5: Update `config/planstate/planstate_test.go` comments**

```bash
sd -F 'klique transitions' 'kas transitions' config/planstate/planstate_test.go
sd -F 'klique marks' 'kas marks' config/planstate/planstate_test.go
sd -F 'klique wrote' 'kas wrote' config/planstate/planstate_test.go
sd -F 'klique spawns' 'kas spawns' config/planstate/planstate_test.go
```

**Step 6: Update help screen title in `app/help.go`**

```bash
sd 'GradientText("klique"' 'GradientText("kas"' app/help.go
```

**Step 7: Update `kq` → `kas` in internal/initcmd and check**

```bash
sd "'kq'" "'kas'" internal/initcmd/initcmd.go
sd "Run 'kq'" "Run 'kas'" internal/initcmd/initcmd.go
sd 'kq init' 'kas init' internal/initcmd/initcmd.go skills.go internal/check/project.go
sd 'kq project' 'kas project' internal/check/check.go internal/check/check_test.go check_test.go
sd 'kq check' 'kas check' internal/check/check.go
sd 'for kq init' 'for kas init' internal/initcmd/initcmd.go
```

**Step 8: Update `internal/initcmd/initcmd_test.go`**

```bash
sd '.klique' '.config/kasmos' internal/initcmd/initcmd_test.go
```

Verify the test creates the parent `.config` dir properly — may need `os.MkdirAll` instead of `os.Mkdir`.

**Step 9: Verify remaining `klique`/`kq` in agent B files**

```bash
rg 'klique' main.go config/ app/help.go internal/ check.go check_test.go skills.go
rg '\bkq\b' main.go config/ app/help.go internal/ check.go check_test.go skills.go
```

Expected: zero matches

**Step 10: Run tests**

```bash
go test ./config/... ./internal/... ./app/... -v
go build ./...
```

Expected: PASS

**Step 11: Commit**

```bash
git add main.go config/ app/help.go internal/ check.go check_test.go skills.go
git commit -m "refactor: rename CLI to kas, migrate config to ~/.config/kasmos (XDG)"
```

---

### Agent C: Build, CI & Distribution

**Scope:** Justfile, Makefile, goreleaser, install script, clean scripts, GitHub workflows, dist artifacts.

**Files:**
- Rewrite: `Justfile`
- Modify: `Makefile`, `.goreleaser.yaml`, `install.sh`
- Rewrite: `clean.sh`, `clean_hard.sh`
- Modify: `.github/workflows/build.yml`, `.github/workflows/cla.yml`
- Rename+Modify: `dist/homebrew/klique.rb` → `dist/homebrew/kasmos.rb`
- Rename+Modify: `dist/scoop/klique.json` → `dist/scoop/kasmos.json`
- Modify: `dist/config.yaml`

**Step 1: Rewrite Justfile**

Write the complete new Justfile (too many interleaved changes for `sd`):

```just
set shell := ["bash", "-cu"]
set dotenv-load := true

# Build kasmos binary
build:
    go build -o kasmos .

# Install to GOPATH/bin (with kas, ks, km aliases)
install:
    go install .
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/kas"
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/ks"
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/km"

# Build + install
bi: build install

# run with no args
bin:
    kas

init:
    kas init --force

# Build + install + run
kas: build install bin

# Run tests
test:
    go test ./...

# Run linter
lint:
    go vet ./...

# Run kasmos (pass-through args)
run *ARGS:
    go run . {{ARGS}}

# Dry-run release (no publish)
release-dry v:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "==> Dry run for kasmos v{{v}}"
    goreleaser release --snapshot --clean
    echo "==> Artifacts in dist/"

# Full release: just release 0.2.1
release v:
    #!/usr/bin/env bash
    set -euo pipefail

    VERSION="{{v}}"
    TAG="v${VERSION}"

    echo "==> Releasing kasmos ${TAG}"

    # 1. Ensure clean working tree
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "ERROR: working tree is dirty, commit or stash first"
        exit 1
    fi

    BRANCH=$(git branch --show-current)
    echo "    branch: ${BRANCH}"

    # 2. Update version in source
    sed -i "s/var version = \".*\"/var version = \"${VERSION}\"/" main.go
    if [[ -n "$(git status --porcelain)" ]]; then
        git add main.go
        git commit -m "release: v${VERSION}"
        echo "    committed version bump"
    fi

    # 3. Tag
    git tag -a "${TAG}" -m "kasmos ${TAG}"
    echo "    tagged ${TAG}"

    # 4. Push commit + tag
    git push origin "${BRANCH}"
    git push origin "${TAG}"
    echo "    pushed to origin"

    # 5. Goreleaser builds, creates GH release, pushes homebrew formula
    GITHUB_TOKEN="${GH_PAT}" goreleaser release --clean
    echo "==> Done: https://github.com/kastheco/kasmos/releases/tag/${TAG}"

# Clean build artifacts
clean:
    rm -f kasmos
    rm -rf dist/
```

**Step 2: Update Makefile**

```bash
sd 'klique' 'kasmos' Makefile
```

**Step 3: Update `.goreleaser.yaml`**

```bash
sd 'binary: klique' 'binary: kasmos' .goreleaser.yaml
sd 'name: klique' 'name: kasmos' .goreleaser.yaml
sd 'kastheco/kasmos' 'kastheco/kasmos' .goreleaser.yaml
sd '"klique - A TUI' '"kas - A TUI' .goreleaser.yaml
sd 'bin.install "klique"' 'bin.install "kasmos"' .goreleaser.yaml
sd '"klique", "version"' '"kasmos", "version"' .goreleaser.yaml
```

**Step 4: Update `install.sh`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' install.sh
sd 'INSTALL_NAME="klique"' 'INSTALL_NAME="kasmos"' install.sh
sd '"klique_' '"kasmos_' install.sh
sd '/klique\$' '/kasmos$' install.sh
```

Verify: `rg 'klique' install.sh` → zero matches

**Step 5: Rewrite `clean.sh`**

```bash
tmux kill-server
rm -rf worktree*
rm -rf ~/.config/kasmos
rm -rf ~/.klique       # legacy
rm -rf ~/.hivemind     # legacy
```

**Step 6: Rewrite `clean_hard.sh`**

```bash
tmux kill-server
rm -rf worktree*
rm -rf ~/.config/kasmos
rm -rf ~/.klique       # legacy
rm -rf ~/.hivemind     # legacy
git worktree prune
```

**Step 7: Update `.github/workflows/build.yml`**

```bash
sd 'BINARY_NAME=klique' 'BINARY_NAME=kasmos' .github/workflows/build.yml
sd 'name: klique-' 'name: kasmos-' .github/workflows/build.yml
```

**Step 8: Update `.github/workflows/cla.yml`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' .github/workflows/cla.yml
sd "remote-repository-name: 'klique-clas'" "remote-repository-name: 'kasmos-clas'" .github/workflows/cla.yml
```

**Step 9: Rename and update homebrew formula**

```bash
mv dist/homebrew/klique.rb dist/homebrew/kasmos.rb
sd 'class Klique' 'class Kasmos' dist/homebrew/kasmos.rb
sd 'kastheco/kasmos' 'kastheco/kasmos' dist/homebrew/kasmos.rb
sd '"klique - A TUI' '"kas - A TUI' dist/homebrew/kasmos.rb
sd 'bin.install "klique"' 'bin.install "kasmos"' dist/homebrew/kasmos.rb
sd '"klique", "version"' '"kasmos", "version"' dist/homebrew/kasmos.rb
sd 'klique_' 'kasmos_' dist/homebrew/kasmos.rb
```

**Step 10: Rename and update scoop manifest**

```bash
mv dist/scoop/klique.json dist/scoop/kasmos.json
sd 'kastheco/kasmos' 'kastheco/kasmos' dist/scoop/kasmos.json
sd '"klique - A TUI' '"kas - A TUI' dist/scoop/kasmos.json
sd 'klique_' 'kasmos_' dist/scoop/kasmos.json
sd 'klique.exe' 'kasmos.exe' dist/scoop/kasmos.json
```

**Step 11: Update `dist/config.yaml`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' dist/config.yaml
sd 'project_name: klique' 'project_name: kasmos' dist/config.yaml
sd 'name: klique' 'name: kasmos' dist/config.yaml
sd 'binary: klique' 'binary: kasmos' dist/config.yaml
sd '"klique - A TUI' '"kas - A TUI' dist/config.yaml
sd 'bin.install "klique"' 'bin.install "kasmos"' dist/config.yaml
sd '"klique", "version"' '"kasmos", "version"' dist/config.yaml
sd 'id: klique' 'id: kasmos' dist/config.yaml
```

**Step 12: Verify**

```bash
rg 'klique' Justfile Makefile .goreleaser.yaml install.sh clean.sh clean_hard.sh .github/ dist/
```

Expected: zero matches (except `clean*.sh` which mention `~/.klique` as a legacy path)

**Step 13: Commit**

```bash
git add Justfile Makefile .goreleaser.yaml install.sh clean.sh clean_hard.sh .github/ dist/
git commit -m "refactor: update build/CI/distribution for kasmos rename"
```

---

### Agent D: Web Frontend

**Scope:** Package name, base path, metadata, all user-facing text.

**Files:**
- Modify: `web/package.json`, `web/next.config.ts`, `web/src/app/layout.tsx`
- Modify: `web/src/app/components/Header.tsx`, `web/src/app/components/PageContent.tsx`, `web/src/app/components/InstallTabs.tsx`

**Step 1: Update `web/package.json`**

```bash
sd '"name": "klique"' '"name": "kasmos"' web/package.json
```

**Step 2: Update `web/next.config.ts`**

```bash
sd '"/klique"' '"/kasmos"' web/next.config.ts
```

**Step 3: Update `web/src/app/layout.tsx`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' web/src/app/layout.tsx
sd 'title: "klique - Agent-Driven IDE' 'title: "kas - Agent-Driven IDE' web/src/app/layout.tsx
sd '"klique", "tui"' '"kasmos", "tui"' web/src/app/layout.tsx
sd 'title: "klique"' 'title: "kas"' web/src/app/layout.tsx
```

**Step 4: Update `web/src/app/components/Header.tsx`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' web/src/app/components/Header.tsx
sd '>klique<' '>kas<' web/src/app/components/Header.tsx
```

**Step 5: Update `web/src/app/components/PageContent.tsx`**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' web/src/app/components/PageContent.tsx
sd 'klique can manage' 'kas can manage' web/src/app/components/PageContent.tsx
sd 'Why klique' 'Why kas' web/src/app/components/PageContent.tsx
sd 'Install klique' 'Install kas' web/src/app/components/PageContent.tsx
sd 'klique by' 'kas by' web/src/app/components/PageContent.tsx
```

Find the large `klique` text (ASCII art / hero text) and replace:

```bash
sd '            klique' '            kas' web/src/app/components/PageContent.tsx
```

**Step 6: Update `web/src/app/components/InstallTabs.tsx`**

```bash
sd 'kastheco/tap/klique' 'kastheco/tap/kasmos' web/src/app/components/InstallTabs.tsx
sd 'scoop install klique' 'scoop install kasmos' web/src/app/components/InstallTabs.tsx
sd 'kastheco/kasmos@latest' 'kastheco/kasmos@latest' web/src/app/components/InstallTabs.tsx
sd 'kastheco/kasmos/main' 'kastheco/kasmos/main' web/src/app/components/InstallTabs.tsx
```

**Step 7: Regenerate package-lock.json**

```bash
cd web && npm install
```

**Step 8: Verify**

```bash
rg 'klique' web/ --glob '!web/package-lock.json' --glob '!web/node_modules/**'
```

Expected: zero matches

**Step 9: Commit**

```bash
git add web/ && git commit -m "refactor: rename web frontend from klique to kasmos"
```

---

### Agent E: Scaffold Templates

**Scope:** Agent config templates that `kas init` scaffolds into user projects.

**Files:**
- Modify: `internal/initcmd/scaffold/scaffold.go`, `internal/initcmd/scaffold/scaffold_test.go`
- Modify: `internal/initcmd/scaffold/templates/opencode/agents/planner.md`
- Modify: `internal/initcmd/scaffold/templates/codex/AGENTS.md`
- Modify: `internal/initcmd/scaffold/templates/claude/agents/planner.md`

**Step 1: Update scaffold templates — replace `klique` with `kasmos`/`kas` as appropriate**

```bash
sd 'klique TUI polls' 'kasmos TUI polls' \
   internal/initcmd/scaffold/templates/opencode/agents/planner.md \
   internal/initcmd/scaffold/templates/claude/agents/planner.md

sd 'managed by klique' 'managed by kasmos' \
   internal/initcmd/scaffold/templates/opencode/agents/planner.md \
   internal/initcmd/scaffold/templates/claude/agents/planner.md

sd '# klique Agents' '# kasmos Agents' \
   internal/initcmd/scaffold/templates/codex/AGENTS.md

sd 'klique sidebar' 'kasmos sidebar' \
   internal/initcmd/scaffold/templates/codex/AGENTS.md

sd 'Only klique transitions' 'Only kasmos transitions' \
   internal/initcmd/scaffold/templates/codex/AGENTS.md
```

**Step 2: Check scaffold.go and scaffold_test.go for bare `klique`**

After Phase 1 handled imports, check for remaining bare references:

```bash
rg 'klique' internal/initcmd/scaffold/scaffold.go internal/initcmd/scaffold/scaffold_test.go
```

If any remain, fix with `sd`.

**Step 3: Run tests**

```bash
go test ./internal/initcmd/scaffold/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/initcmd/scaffold/
git commit -m "refactor: update scaffold templates for kasmos rename"
```

---

### Agent F: Documentation & Project Agent Configs

**Scope:** README, CONTRIBUTING, CLA, agent configs, opencode config, plan docs.

**Files:**
- Modify: `README.md`, `CONTRIBUTING.md`, `CLA.md`
- Modify: `.claude/agents/planner.md`, `.claude/agents/coder.md`
- Modify: `.opencode/agents/planner.md`, `.opencode/opencode.jsonc`
- Modify: `docs/plans/*.md` (repo URLs only)

**Step 1: Update README.md**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' README.md
sd '# klique' '# kas' README.md
sd '!\[klique Screenshot\]' '![kas Screenshot]' README.md
sd 'kastheco/tap/klique' 'kastheco/tap/kasmos' README.md
sd 'scoop install klique' 'scoop install kasmos' README.md
sd 'the `klique` binary' 'the `kasmos` binary' README.md
sd -- '--name kq' '--name kas' README.md
sd '  klique \[flags\]' '  kas [flags]' README.md
sd '  klique \[command\]' '  kas [command]' README.md
sd 'version number of klique' 'version number of kas' README.md
sd 'help for klique' 'help for kas' README.md
sd 'Using klique' 'Using kas' README.md
sd 'klique -p' 'kas -p' README.md
sd 'klique debug' 'kas debug' README.md
sd 'klique is a fork' 'kas is a fork' README.md
```

Check for any remaining standalone `klique` in README:

```bash
rg 'klique' README.md
```

Fix any stragglers with targeted `sd`.

**Step 2: Update CONTRIBUTING.md**

```bash
sd 'klique' 'kasmos' CONTRIBUTING.md
```

**Step 3: Update CLA.md**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' CLA.md
sd '\[klique\]' '[kas]' CLA.md
```

**Step 4: Update `.claude/agents/planner.md`**

```bash
sd 'planner agent for klique' 'planner agent for kasmos' .claude/agents/planner.md
sd 'klique is a Go TUI' 'kasmos is a Go TUI' .claude/agents/planner.md
sd 'klique TUI polls' 'kasmos TUI polls' .claude/agents/planner.md
sd 'managed by klique' 'managed by kasmos' .claude/agents/planner.md
sd 'created by klique' 'created by kasmos' .claude/agents/planner.md
```

**Step 5: Update `.claude/agents/coder.md`**

```bash
sd 'coder agent for klique' 'coder agent for kasmos' .claude/agents/coder.md
sd 'klique is a Go TUI' 'kasmos is a Go TUI' .claude/agents/coder.md
```

**Step 6: Update `.opencode/agents/planner.md`**

```bash
sd 'created by klique' 'created by kasmos' .opencode/agents/planner.md
sd 'klique TUI polls' 'kasmos TUI polls' .opencode/agents/planner.md
sd 'managed by klique' 'managed by kasmos' .opencode/agents/planner.md
```

**Step 7: Update `.opencode/opencode.jsonc`**

```bash
sd '/home/kas/dev/klique' '/home/kas/dev/kasmos' .opencode/opencode.jsonc
```

**Step 8: Update repo URLs in plan docs (cosmetic)**

```bash
sd 'kastheco/kasmos' 'kastheco/kasmos' $(rg -l 'kastheco/kasmos' docs/plans/)
```

Do NOT replace bare `klique` in plan docs — these are historical records.

**Step 9: Commit**

```bash
git add README.md CONTRIBUTING.md CLA.md .claude/ .opencode/ docs/plans/
git commit -m "docs: update documentation and agent configs for kasmos rename"
```

---

## Phase 3: Final Verification (Sequential)

**Wait for ALL Phase 2 agents to complete.** Then verify the combined result.

**Step 1: Full grep for remaining `klique` references in Go**

```bash
rg 'klique' --type go
```

Expected: zero matches

**Step 2: Full grep for remaining `klique` in non-Go (excluding expected)**

```bash
rg 'klique' --type-not go \
   -g '!web/package-lock.json' \
   -g '!docs/plans/*.md' \
   -g '!dist/checksums.txt' \
   -g '!dist/artifacts.json' \
   -g '!dist/metadata.json' \
   -g '!go.sum'
```

Expected: only `clean.sh`/`clean_hard.sh` (legacy path comments) and the cleanup regex string in tmux.go (already checked in Step 1)

**Step 3: Full grep for remaining `kq` references**

```bash
rg '\bkq\b' --type go --type yaml
```

Expected: zero matches

**Step 4: Full build and test**

```bash
go build -o kasmos . && go test ./...
```

Expected: all pass

**Step 5: Verify binary name**

```bash
./kasmos version
```

Expected: `kas version 0.2.0`

**Step 6: Fix any stragglers**

```bash
# If any klique references found in Steps 1-3:
# Fix with targeted sd commands, then:
git add -A && git diff --cached --stat
git commit -m "refactor: fix remaining klique references"
```
