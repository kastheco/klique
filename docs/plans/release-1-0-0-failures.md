# Release v1.0.0 Failures — Fix and Redeploy

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the three distinct CI/CD failures blocking the v1.0.0 release and redeploy.

**Architecture:** The release pipeline has three independent failure classes: (1) a missing struct field that broke compilation, (2) gofmt formatting drift, and (3) GoReleaser config using deprecated v2 syntax + an expired `GH_PAT` secret for Homebrew tap publishing. The existing v1.0.0 GitHub release has all binary assets uploaded correctly but the Homebrew formula was never published. We fix the code, update CI config, delete the stale release+tag, bump the tag onto the fixed commit, and re-release.

**Tech Stack:** Go 1.24, GoReleaser v2.14, GitHub Actions, golangci-lint

**Size:** Small (estimated ~45 min, 3 tasks, no waves)

---

## Failure Analysis

### Failure 1: Build workflow — compilation error
- **Run:** [22473879077](https://github.com/kastheco/kasmos/actions/runs/22473879077)
- **Error:** `app/app.go:867:32: inst.Exited undefined (type *session.Instance has no field or method Exited)`
- **Root cause:** Commit `1dba89f` references `inst.Exited` in `app/app.go` but the `Exited bool` field was never committed to `session/instance.go`. The field exists only in the local working tree.

### Failure 2: Lint workflow — gofmt formatting
- **Run:** [22473879082](https://github.com/kastheco/kasmos/actions/runs/22473879082)
- **Error:** `Check formatting` step fails — `gofmt -l` finds unformatted files
- **Files:** `app/app_plan_actions_test.go`, `app/app_plan_completion_test.go`, `app/app_solo_agent_test.go`, `app/app_wave_orchestration_flow_test.go`, `internal/mcpclient/types.go`

### Failure 3: Release workflow — two sub-issues
- **Run (tag push):** [22473179947](https://github.com/kastheco/kasmos/actions/runs/22473179947) — GoReleaser built + uploaded all 7 binary assets, then **failed on Homebrew formula push** with `401 Bad credentials` when accessing `kastheco/homebrew-tap` via `GH_PAT`.
- **Runs (retries):** [22473082494](https://github.com/kastheco/kasmos/actions/runs/22473082494), [22473379501](https://github.com/kastheco/kasmos/actions/runs/22473379501) — GoReleaser fails with `422 Validation Failed: already_exists` because release assets from the first run already exist.
- **GoReleaser deprecation warnings:** `archives.format` → `archives.formats`, `archives.format_overrides.format` → `archives.format_overrides.formats`, `brews` → `homebrew_casks`.
- **Lint workflow config:** Uses hardcoded `go-version: '1.23'` and `golangci-lint v1.60.1` — both outdated vs the project's Go 1.24.

---

## Task 1: Fix code defects — missing field + gofmt

**Files:**
- Modify: `session/instance.go` (uncommitted `Exited` field — already in working tree)
- Modify: `app/app_plan_actions_test.go` (gofmt)
- Modify: `app/app_plan_completion_test.go` (gofmt)
- Modify: `app/app_solo_agent_test.go` (gofmt)
- Modify: `app/app_wave_orchestration_flow_test.go` (gofmt)
- Modify: `internal/mcpclient/types.go` (gofmt)

**Step 1: Run gofmt on all affected files**

```bash
gofmt -w app/app_plan_actions_test.go app/app_plan_completion_test.go app/app_solo_agent_test.go app/app_wave_orchestration_flow_test.go internal/mcpclient/types.go
```

**Step 2: Verify the `Exited` field exists in session/instance.go**

The field is already in the working tree (line 77-79). Verify:

```bash
rg 'Exited bool' session/instance.go
```

Expected: `Exited bool` found.

**Step 3: Run build + tests to confirm everything passes**

```bash
go build ./... && go test ./...
```

Expected: all packages build and pass.

**Step 4: Commit**

There are also other uncommitted changes in the working tree (focus ring improvements in `app/app_state.go`, `app/app_test.go`, plan state reconciliation in `config/planstate/planstate.go`, solo header navigation in `ui/navigation_panel.go`, `ui/nav_panel_test.go`). Stage and commit everything — these are all completed features that were missed in the last push.

```bash
git add session/instance.go app/app_plan_actions_test.go app/app_plan_completion_test.go app/app_solo_agent_test.go app/app_wave_orchestration_flow_test.go internal/mcpclient/types.go app/app_state.go app/app_test.go config/planstate/planstate.go ui/navigation_panel.go ui/nav_panel_test.go
git commit -m "fix: commit missing Exited field, gofmt fixes, and uncommitted features"
```

---

## Task 2: Update CI/CD configuration

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/lint.yml`

**Step 1: Update `.goreleaser.yaml` to fix deprecations**

Three changes needed per [goreleaser deprecation docs](https://goreleaser.com/deprecations):

1. `archives.format` → `archives.formats` (accepts list or string)
2. `archives.format_overrides[].format` → `archives.format_overrides[].formats`
3. `brews` → `homebrew_casks` with `directory: Casks` (casks must be in `Casks/`)

The updated config:

```yaml
version: 2.9

builds:
  - binary: kasmos
    goos:
      - darwin
      - linux
      - windows
    goarch:
      - amd64
      - arm64
    env:
      - CGO_ENABLED=0

archives:
  - formats:
      - tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        formats:
          - zip

homebrew_casks:
  - repository:
      owner: kastheco
      name: homebrew-tap
      token: "{{ .Env.GH_PAT }}"
    name: kasmos
    homepage: "https://github.com/kastheco/kasmos"
    description: "kas - A TUI-based agent-driven IDE for managing multiple AI agents"
    license: "AGPL-3.0"
    binaries:
      - kasmos
    hooks:
      post:
        install: |
          if OS.mac?
            system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{staged_path}/kasmos"]
          end

release:
  prerelease: auto
  draft: false

checksum:
  name_template: 'checksums.txt'

changelog:
  use: github

  filters:
    exclude:
      - "^docs:"
      - typo
      - "^refactor"
      - "^chore"
```

Note: the `install`/`test` stanzas from `brews` don't map 1:1 to `homebrew_casks`. Casks use `binaries` (array) to declare which binaries to link, and `hooks.post.install` for post-install actions. The symlinks (`kas`, `kms`) need to be created via a post-install hook or by listing them as additional binaries if goreleaser supports it. Check `goreleaser check` output after editing.

**Step 2: Update `.github/workflows/lint.yml`**

Change from hardcoded Go version to `go-version-file` and bump golangci-lint:

```yaml
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
          cache: false

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.64
          args: --timeout=3m --out-format=line-number --fast --max-issues-per-linter=0 --max-same-issues=0
          skip-cache: false
          only-new-issues: true
```

**Step 3: Validate goreleaser config locally**

```bash
goreleaser check
```

If goreleaser isn't installed locally, at minimum verify the YAML is valid:

```bash
yq . .goreleaser.yaml > /dev/null
```

**Step 4: Commit**

```bash
git add .goreleaser.yaml .github/workflows/lint.yml
git commit -m "ci: fix goreleaser deprecations (formats, homebrew_casks) and update lint workflow"
```

---

## Task 3: Re-release v1.0.0

This task has a **manual prerequisite**: the `GH_PAT` repository secret must be valid.

**Step 1: Verify/fix GH_PAT (MANUAL — user action required)**

The first release run failed with `401 Bad credentials` when GoReleaser tried to push the Homebrew formula to `kastheco/homebrew-tap`. The `GH_PAT` secret needs to be a valid personal access token with `repo` scope (or fine-grained token with Contents write access to `kastheco/homebrew-tap`).

Go to: https://github.com/kastheco/kasmos/settings/secrets/actions

Verify `GH_PAT` is set and not expired. If expired, generate a new one at https://github.com/settings/tokens and update the secret.

**Step 2: Push the fix commits to main**

```bash
git push origin main
```

Wait for Build + Lint workflows to pass on the push. Check status:

```bash
gh run list --limit 4
```

Both Build and Lint must show `completed | success` before proceeding.

**Step 3: Delete the existing v1.0.0 release and tag**

```bash
gh release delete v1.0.0 --yes --cleanup-tag
```

This deletes both the GitHub release (with all uploaded assets) and the remote tag.

Also delete the local tag:

```bash
git tag -d v1.0.0
```

**Step 4: Re-tag and push**

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers the Release workflow on the tag push.

**Step 5: Monitor the release**

```bash
# Watch the run
gh run list --workflow release --limit 3

# Once complete, verify
gh release view v1.0.0
```

Expected: Release workflow completes successfully with all binary assets + Homebrew cask published to `kastheco/homebrew-tap`.

**Step 6: Verify Homebrew installation works**

```bash
brew tap kastheco/tap
brew install kastheco/tap/kasmos
kas version
```

Expected: prints `1.0.0`.
