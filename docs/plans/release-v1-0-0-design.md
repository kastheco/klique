# Release v1.0.0 — Design

## Goal

Ship the first proper GitHub release of kasmos with working Homebrew distribution via `kastheco/homebrew-tap`.

## Current State

- Tags `v0.0.1-alpha` through `v1.0.14` exist but **zero GitHub releases** were ever published
- `main.go` version is `0.2.0` — out of sync with tags
- GoReleaser configured but `draft: true` means nothing ever went live
- Homebrew tap repo exists (`kastheco/homebrew-tap`) but has no formula — only a `Casks/` dir
- Scoop bucket repo (`kastheco/scoop-bucket`) does not exist
- Release CI workflow pins Go 1.23 but go.mod requires 1.24.4 — build would fail
- Justfile has a `release` recipe that runs goreleaser locally (bypassing CI)

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Versioning | Clean slate — delete all orphaned tags, release as v1.0.0 | No published releases exist, old tags are noise |
| Release flow | CI-driven — tag push triggers GitHub Actions | Reproducible, doesn't depend on local env |
| Scoop | Drop from goreleaser config | Repo doesn't exist, YAGNI, add later if needed |
| Draft mode | Auto-publish (`draft: false`) | CI validates version match, changelog is auto-generated |
| Changelog | Keep current filters (feat/fix only) | Users care about features and fixes, not refactors |
| Go version in CI | Fix to `go-version-file: 'go.mod'` | Current hardcoded 1.23 won't build go 1.24 code |

## Changes

### Files to modify

- **`main.go`** — set `version = "1.0.0"`
- **`.goreleaser.yaml`** — remove scoop section, set `draft: false`, remove `replace_existing_draft`
- **`.github/workflows/release.yml`** — use `go-version-file: 'go.mod'` instead of hardcoded `go-version: '1.23'`
- **`Justfile`** — simplify `release` recipe to version bump + tag + push only (CI runs goreleaser)

### Files to delete

- **`bump-version.sh`** — superseded by Justfile recipe

### Git operations

- Delete all local tags (`v0.0.1-alpha` through `v1.0.14`)
- Delete all remote tags on origin
- After changes: tag `v1.0.0`, push tag to trigger CI release

### External repos

- `kastheco/scoop-bucket` — no action needed (doesn't exist, reference removed)
- `kastheco/homebrew-tap` — goreleaser will auto-push formula on release

## What stays the same

- GoReleaser Homebrew tap config (already correct → `kastheco/homebrew-tap`)
- `install.sh` (works with GitHub releases as-is)
- Build workflow (already uses `go-version-file`)
- Binary names and symlinks (`kasmos`, `kas`, `kms`)
- Archive naming template
- Changelog GitHub-native mode
