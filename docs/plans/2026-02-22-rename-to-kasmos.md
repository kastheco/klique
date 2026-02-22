# Rename klique → kasmos: Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.
> Use superpowers:dispatching-parallel-agents for Phase 2 (7 parallel agents).

**Goal:** Rename the project from `klique` to `kasmos` across all source, config, build, web, and documentation files.

**Architecture:** Mechanical bulk rename using `sd` for literal replacements, with targeted edits for structural changes (config dir migration, symlink aliases, cobra command). Organized into 4 phases with maximum parallelism in Phase 2.

**Tech Stack:** `sd` (string replacement), `go build`/`go test` (verification), `gh` (GitHub API for repo rename)

## Dependency Graph

```
Phase 0: GitHub rename (manual, user action)
    ↓
Phase 1: Go module path rename (sequential gate — all imports must change first)
    ↓
Phase 2: PARALLEL — 7 independent agents, zero file overlap
    ├── Agent A: Config directory (XDG migration)
    ├── Agent B: Binary name + Cobra command
    ├── Agent C: Build/release infrastructure
    ├── Agent D: Distribution artifacts
    ├── Agent E: Web frontend
    ├── Agent F: Documentation
    └── Agent G: Plan docs (cosmetic)
    ↓
Phase 3: Final verification + squash commit
```

**File ownership (NO overlap between agents):**

| Agent | Files |
|-------|-------|
| A | `config/config.go`, `config/config_test.go`, `config/toml.go`, `session/storage.go` |
| B | `main.go`, `session/instance.go`, `app/help.go`, `internal/initcmd/initcmd.go`, `internal/check/*.go`, `check_test.go`, `skills.go` |
| C | `Justfile`, `Makefile`, `.goreleaser.yaml`, `install.sh`, `clean.sh`, `clean_hard.sh` |
| D | `dist/homebrew/klique.rb`, `dist/scoop/klique.json`, `dist/config.yaml` |
| E | `web/**` |
| F | `README.md`, `CONTRIBUTING.md`, `CLA.md` |
| G | `docs/plans/*.md` |

---

## Phase 0: GitHub Repo Rename (Manual — User Action)

These steps must be done by the user before implementation begins.

**Step 1: Rename the old kasmos repo**

```bash
gh repo rename kasmold --repo kastheco/kasmos --yes
```

**Step 2: Rename klique → kasmos**

```bash
gh repo rename kasmos --repo kastheco/klique --yes
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

Replace all `github.com/kastheco/klique` → `github.com/kastheco/kasmos` in Go source and go.mod.

**Files:**
- Modify: `go.mod` (line 1)
- Modify: all 54 `.go` files with import paths

**Step 1: Replace module path in go.mod and all Go files**

```bash
sd 'github.com/kastheco/klique' 'github.com/kastheco/kasmos' go.mod $(rg -l 'kastheco/klique' --type go)
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

## Phase 2: Parallel Rename (7 Independent Agents)

**Dispatch ALL agents simultaneously.** Zero file overlap between agents.
Each agent works on its own file set, commits independently, then reports back.

---

### Agent A: Config Directory — XDG Migration

**Scope:** `config/config.go`, `config/config_test.go`, `config/toml.go`, `session/storage.go`

Change config dir from `~/.klique` to `~/.config/kasmos/` with three-generation migration chain.

**Step 1: Rewrite `GetConfigDir()` in `config/config.go`**

Replace the existing `GetConfigDir` function (lines 20-43) with:

```go
// GetConfigDir returns the path to the application's configuration directory.
// Uses XDG-compliant ~/.config/kasmos/. On first run after a rename, it migrates
// legacy directories: ~/.hivemind → ~/.klique → ~/.config/kasmos/.
func GetConfigDir() (string, error) {
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
}
```

**Step 2: Update `config/toml.go` comments**

```bash
sd 'LoadTOMLConfig loads the TOML config from the default location \(~/.klique/config.toml\)' \
   'LoadTOMLConfig loads the TOML config from the default location (~/.config/kasmos/config.toml)' \
   config/toml.go
sd 'SaveTOMLConfig writes to the default location \(~/.klique/config.toml\)' \
   'SaveTOMLConfig writes to the default location (~/.config/kasmos/config.toml)' \
   config/toml.go
sd '# Generated by kq init' '# Generated by kas init' config/toml.go
```

**Step 3: Update `session/storage.go` comment**

```bash
sd '\.klique/worktrees/' '.config/kasmos/worktrees/' session/storage.go
```

**Step 4: Update `config/config_test.go`**

This file needs careful rewriting. Key changes:
- `HasSuffix(configDir, ".klique")` → `HasSuffix(configDir, filepath.Join(".config", "kasmos"))`
- `"migrates legacy .hivemind to .klique"` → `"migrates legacy .hivemind to .config/kasmos"`
- `"skips migration when .klique already exists"` → `"skips migration when .config/kasmos already exists"`
- Create test directories under `.config/kasmos` instead of `.klique`
- Add test: `"migrates legacy .klique to .config/kasmos"` (the new second-generation migration)

**Step 5: Verify**

Run: `go test ./config/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add config/ session/storage.go && git commit -m "refactor: migrate config dir from ~/.klique to ~/.config/kasmos (XDG)"
```

---

### Agent B: Binary Name and Cobra Command

**Scope:** `main.go`, `session/instance.go`, `app/help.go`, `internal/initcmd/initcmd.go`, `internal/check/check.go`, `internal/check/check_test.go`, `internal/check/project.go`, `check_test.go`, `skills.go`

Change Cobra `Use` to `kas`, update all CLI-facing strings, replace `kq` → `kas` in comments.

**Step 1: Update `main.go`**

```bash
sd 'Use:   "klique"' 'Use:   "kas"' main.go
sd 'Short: "klique - Manage multiple AI agents' 'Short: "kas - Manage multiple AI agents' main.go
```

**Step 2: Update notification sender in `session/instance.go`**

```bash
sd 'SendNotification\("klique"' 'SendNotification("kas"' session/instance.go
```

**Step 3: Update help screen title in `app/help.go`**

```bash
sd 'GradientText\("klique"' 'GradientText("kas"' app/help.go
```

**Step 4: Update `kq` → `kas` in comments and user-facing strings**

```bash
sd "'kq'" "'kas'" internal/initcmd/initcmd.go
sd "Run 'kq'" "Run 'kas'" internal/initcmd/initcmd.go
sd 'kq init' 'kas init' internal/initcmd/initcmd.go skills.go internal/check/project.go
sd 'kq project' 'kas project' internal/check/check.go internal/check/check_test.go check_test.go
sd 'kq check' 'kas check' internal/check/check.go
sd 'for kq init' 'for kas init' internal/initcmd/initcmd.go
```

**Step 5: Verify**

Run: `go build ./...`
Expected: SUCCESS

**Step 6: Commit**

```bash
git add main.go session/instance.go app/help.go internal/ check_test.go skills.go
git commit -m "refactor: rename CLI command to kas, replace kq references"
```

---

### Agent C: Build and Release Infrastructure

**Scope:** `Justfile`, `Makefile`, `.goreleaser.yaml`, `install.sh`, `clean.sh`, `clean_hard.sh`

**Step 1: Rewrite Justfile**

The Justfile needs a full rewrite rather than incremental `sd` — too many interleaved changes. Write the complete new file:

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
sd 'kastheco/klique' 'kastheco/kasmos' .goreleaser.yaml
sd '"klique - A TUI' '"kas - A TUI' .goreleaser.yaml
sd 'bin.install "klique"' 'bin.install "kasmos"' .goreleaser.yaml
sd 'klique", "version' 'kasmos", "version' .goreleaser.yaml
```

**Step 4: Update `install.sh`**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' install.sh
sd 'INSTALL_NAME="klique"' 'INSTALL_NAME="kasmos"' install.sh
sd '/klique\$\{extension\}' '/kasmos${extension}' install.sh
sd '"klique_' '"kasmos_' install.sh
```

**Step 5: Rewrite `clean.sh`**

```bash
rm -rf ~/.config/kasmos
```

**Step 6: Rewrite `clean_hard.sh`**

```bash
rm -rf ~/.config/kasmos
rm -rf ~/.klique       # legacy
rm -rf ~/.hivemind     # legacy
```

**Step 7: Commit**

```bash
git add Justfile Makefile .goreleaser.yaml install.sh clean.sh clean_hard.sh
git commit -m "refactor: update build infrastructure for kasmos rename"
```

---

### Agent D: Distribution Artifacts

**Scope:** `dist/homebrew/klique.rb`, `dist/scoop/klique.json`, `dist/config.yaml`

**Step 1: Rename and update homebrew formula**

```bash
mv dist/homebrew/klique.rb dist/homebrew/kasmos.rb
sd 'class Klique' 'class Kasmos' dist/homebrew/kasmos.rb
sd 'kastheco/klique' 'kastheco/kasmos' dist/homebrew/kasmos.rb
sd '"klique - A TUI' '"kas - A TUI' dist/homebrew/kasmos.rb
sd 'bin.install "klique"' 'bin.install "kasmos"' dist/homebrew/kasmos.rb
sd 'klique", "version' 'kasmos", "version' dist/homebrew/kasmos.rb
sd 'klique_' 'kasmos_' dist/homebrew/kasmos.rb
```

**Step 2: Rename and update scoop manifest**

```bash
mv dist/scoop/klique.json dist/scoop/kasmos.json
sd 'kastheco/klique' 'kastheco/kasmos' dist/scoop/kasmos.json
sd '"klique - A TUI' '"kas - A TUI' dist/scoop/kasmos.json
sd 'klique_' 'kasmos_' dist/scoop/kasmos.json
sd 'klique.exe' 'kasmos.exe' dist/scoop/kasmos.json
```

**Step 3: Update `dist/config.yaml`**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' dist/config.yaml
```

**Step 4: Commit**

```bash
git add dist/ && git commit -m "refactor: rename distribution artifacts for kasmos"
```

---

### Agent E: Web Frontend

**Scope:** `web/package.json`, `web/next.config.ts`, `web/src/app/layout.tsx`, `web/src/app/components/Header.tsx`, `web/src/app/components/PageContent.tsx`, `web/src/app/components/InstallTabs.tsx`

**Step 1: Update `web/package.json`**

```bash
sd '"name": "klique"' '"name": "kasmos"' web/package.json
```

**Step 2: Update `web/next.config.ts`**

```bash
sd '"/klique"' '"/kasmos"' web/next.config.ts
```

**Step 3: Update layout.tsx**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' web/src/app/layout.tsx
sd 'title: "klique' 'title: "kas' web/src/app/layout.tsx
sd '"klique"' '"kas"' web/src/app/layout.tsx
```

**Step 4: Update Header.tsx**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' web/src/app/components/Header.tsx
sd '>klique<' '>kas<' web/src/app/components/Header.tsx
```

**Step 5: Update PageContent.tsx**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' web/src/app/components/PageContent.tsx
sd 'klique can manage' 'kas can manage' web/src/app/components/PageContent.tsx
sd 'klique\n' 'kas\n' web/src/app/components/PageContent.tsx
sd 'Why klique' 'Why kas' web/src/app/components/PageContent.tsx
sd 'Install klique' 'Install kas' web/src/app/components/PageContent.tsx
sd 'klique by' 'kas by' web/src/app/components/PageContent.tsx
```

**Step 6: Update InstallTabs.tsx**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' web/src/app/components/InstallTabs.tsx
sd 'kastheco/tap/klique' 'kastheco/tap/kasmos' web/src/app/components/InstallTabs.tsx
sd 'scoop install klique' 'scoop install kasmos' web/src/app/components/InstallTabs.tsx
```

**Step 7: Regenerate package-lock.json**

```bash
cd web && npm install
```

**Step 8: Commit**

```bash
git add web/ && git commit -m "refactor: rename web frontend from klique to kasmos"
```

---

### Agent F: Documentation

**Scope:** `README.md`, `CONTRIBUTING.md`, `CLA.md`

**Step 1: Update README.md**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' README.md
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

Verify no remaining `klique` in README: `rg 'klique' README.md`

**Step 2: Update CONTRIBUTING.md**

```bash
sd 'klique' 'kasmos' CONTRIBUTING.md
```

**Step 3: Update CLA.md**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' CLA.md
sd '\[klique\]' '[kas]' CLA.md
```

**Step 4: Commit**

```bash
git add README.md CONTRIBUTING.md CLA.md && git commit -m "docs: update documentation for kasmos rename"
```

---

### Agent G: Plan Docs (Cosmetic)

**Scope:** `docs/plans/*.md` files containing `kastheco/klique`

**Step 1: Replace repo URLs only**

```bash
sd 'kastheco/klique' 'kastheco/kasmos' $(rg -l 'kastheco/klique' docs/plans/)
```

Do NOT replace bare `klique` — these are historical records.

**Step 2: Commit**

```bash
git add docs/plans/ && git commit -m "docs: update repo URLs in historical plan docs"
```

---

## Phase 3: Final Verification (Sequential)

**Wait for ALL Phase 2 agents to complete.** Then verify the combined result.

**Step 1: Full grep for remaining `klique` references**

```bash
rg 'klique' --type go
```

Expected: zero matches

```bash
rg 'klique' --type-not go -g '!web/package-lock.json' -g '!docs/plans/*.md' -g '!dist/checksums.txt' -g '!dist/artifacts.json'
```

Expected: zero matches (plan docs retain historical bare `klique`, dist generated files excluded)

**Step 2: Full build and test**

Run: `go build ./... && go test ./...`
Expected: all pass

**Step 3: Verify binary name and aliases**

```bash
go build -o kasmos . && ./kasmos version
```

Expected: prints version

**Step 4: Grep for remaining `kq` references (should be zero outside plan docs)**

```bash
rg '\bkq\b' --type go --type yaml
```

Expected: zero matches

**Step 5: Fix any stragglers**

```bash
git add -A && git diff --cached --stat
# If changes exist:
git commit -m "refactor: fix remaining klique references"
```
