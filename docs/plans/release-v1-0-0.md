# Release v1.0.0 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship the first proper GitHub release of kasmos with working CI pipeline and Homebrew distribution.

**Architecture:** Fix release config files (goreleaser, CI workflow, Justfile), clean up orphaned tags, bump version to 1.0.0, and push tag to trigger CI-driven release via GitHub Actions + GoReleaser.

**Tech Stack:** GoReleaser 2.9, GitHub Actions, Homebrew tap (`kastheco/homebrew-tap`)

**Size:** Small (estimated ~45 min, 3 tasks, 2 waves)

---

## Wave 1: Fix release config and clean tags
> **No prior dependencies.** Tasks 1 and 2 are independent and can run in parallel.

### Task 1: Fix release infrastructure

Update goreleaser config, CI workflow, and Justfile to match the design decisions.

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/release.yml`
- Modify: `Justfile`
- Delete: `bump-version.sh`

**Step 1: Update `.goreleaser.yaml`**

Remove the entire `scoops:` section (lines 39-47), set `draft: false`, and remove `replace_existing_draft`:

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
  - format: tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        format: zip

brews:
  - repository:
      owner: kastheco
      name: homebrew-tap
      token: "{{ .Env.GH_PAT }}"
    name: kasmos
    homepage: "https://github.com/kastheco/kasmos"
    description: "kas - A TUI-based agent-driven IDE for managing multiple AI agents"
    license: "AGPL-3.0"
    install: |
      bin.install "kasmos"
      bin.install_symlink "kasmos" => "kas"
      bin.install_symlink "kasmos" => "kms"
    test: |
      system "#{bin}/kas", "version"

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

**Step 2: Fix `.github/workflows/release.yml`**

Replace the hardcoded Go 1.23 setup with `go-version-file`. Also bump `actions/setup-go` from v4 to v5 (matches the build workflow):

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    permissions: write-all
    name: Build Release
    runs-on: ubuntu-latest

    steps:
      - name: Check out code into the Go module directory
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Validate tag matches version in main.go
        run: |
          # Extract version from main.go
          VERSION=$(grep -E '^[[:space:]]*version[[:space:]]*=' main.go | sed -E 's/.*"([^"]+)".*/\1/')
          
          # Get the tag name (remove refs/tags/ prefix)
          TAG_NAME=${GITHUB_REF#refs/tags/}
          
          # Remove 'v' prefix from tag if present
          TAG_VERSION=${TAG_NAME#v}
          
          echo "Version in main.go: $VERSION"
          echo "Tag version: $TAG_VERSION"
          
          # Compare versions
          if [ "$VERSION" != "$TAG_VERSION" ]; then
            echo "ERROR: Tag version ($TAG_VERSION) does not match version in main.go ($VERSION)"
            echo "Please ensure the tag matches the version defined in main.go"
            exit 1
          fi
          
          echo "✅ Tag version matches main.go version: $VERSION"

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_PAT: ${{ secrets.GH_PAT }}
```

**Step 3: Simplify Justfile `release` recipe**

Remove the local goreleaser invocation — CI handles that. The recipe just bumps version, commits, tags, and pushes. Also remove `release-dry` (can run `goreleaser release --snapshot --clean` directly if needed):

Replace lines 42-88 with:

```just
# Tag and push a release (CI runs goreleaser): just release 1.0.0
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
    sd 'version\s*=\s*"[^"]*"' "version     = \"${VERSION}\"" main.go
    if [[ -n "$(git status --porcelain)" ]]; then
        git add main.go
        git commit -m "release: v${VERSION}"
        echo "    committed version bump"
    fi

    # 3. Tag
    git tag -a "${TAG}" -m "kasmos ${TAG}"
    echo "    tagged ${TAG}"

    # 4. Push commit + tag — CI takes it from here
    git push origin "${BRANCH}"
    git push origin "${TAG}"
    echo "==> Pushed ${TAG}. CI will build and publish the release."
    echo "    https://github.com/kastheco/kasmos/releases/tag/${TAG}"
```

**Step 4: Delete `bump-version.sh`**

```bash
rm bump-version.sh
```

**Step 5: Verify goreleaser config parses**

Run: `goreleaser check`
Expected: no errors

**Step 6: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml Justfile
git rm bump-version.sh
git commit -m "fix: release infrastructure — drop scoop, auto-publish, fix go version"
```

---

### Task 2: Clean up orphaned tags

Delete all existing local and remote tags. These were never backed by published releases.

**Tags to delete:** `v0.0.1-alpha`, `v1.0.0` through `v1.0.14` (16 tags total).

**Step 1: Delete remote tags**

```bash
git push origin --delete v0.0.1-alpha v1.0.0 v1.0.1 v1.0.2 v1.0.3 v1.0.4 v1.0.5 v1.0.6 v1.0.7 v1.0.8 v1.0.9 v1.0.10 v1.0.11 v1.0.12 v1.0.13 v1.0.14
```

**Step 2: Delete local tags**

```bash
git tag -d v0.0.1-alpha v1.0.0 v1.0.1 v1.0.2 v1.0.3 v1.0.4 v1.0.5 v1.0.6 v1.0.7 v1.0.8 v1.0.9 v1.0.10 v1.0.11 v1.0.12 v1.0.13 v1.0.14
```

**Step 3: Verify clean state**

Run: `git tag --list`
Expected: empty output

No commit needed — tag operations are immediate.

---

## Wave 2: Tag and release
> **Depends on Wave 1:** Orphaned tags must be deleted and release infrastructure must be correct before tagging v1.0.0 and pushing to trigger CI.

### Task 3: Bump version and trigger release

Set version to 1.0.0 in source, tag, and push to trigger CI.

**Files:**
- Modify: `main.go:25`

**Step 1: Bump version in `main.go`**

Change line 25 from:
```go
version     = "0.2.0"
```
to:
```go
version     = "1.0.0"
```

**Step 2: Verify build**

Run: `go build -o kasmos . && ./kasmos version`
Expected: `kas version 1.0.0`

**Step 3: Commit**

```bash
git add main.go
git commit -m "release: v1.0.0"
```

**Step 4: Tag**

```bash
git tag -a v1.0.0 -m "kasmos v1.0.0"
```

**Step 5: Push commit and tag**

```bash
git push origin main
git push origin v1.0.0
```

This triggers the release workflow. Monitor at: `https://github.com/kastheco/kasmos/actions`

**Step 6: Verify release published**

After CI completes (~2-3 min):

```bash
gh release view v1.0.0 --repo kastheco/kasmos
```

Expected: release with 6 archives (linux/darwin/windows × amd64/arm64, minus windows/arm64), checksums.txt, and auto-generated changelog.

**Step 7: Verify Homebrew formula pushed**

```bash
gh api repos/kastheco/homebrew-tap/contents/Formula/kasmos.rb --jq '.name'
```

Expected: `kasmos.rb`
