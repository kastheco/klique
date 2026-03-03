# Rename klique → kasmos: Design

## Goal

Rename the project from `klique` to `kasmos`. The primary CLI command becomes `kas` (binary `kasmos`, aliases `kas`, `ks`, `km`). Config moves to XDG-compliant `~/.config/kasmos/`.

## Naming Table

| Thing | Old | New |
|-------|-----|-----|
| Go module | `github.com/kastheco/kasmos` | `github.com/kastheco/kasmos` |
| Binary | `klique` | `kasmos` |
| Cobra `Use` | `klique` | `kas` |
| Symlink aliases | `kq` | `kas`, `ks`, `km` |
| Config dir | `~/.klique` | `~/.config/kasmos/` |
| Docs/comments/Justfile | `klique`/`kq` | `kasmos`/`kas` |
| goreleaser binary | `klique` | `kasmos` |
| Homebrew formula | `klique` | `kasmos` |
| Scoop manifest | `klique` | `kasmos` |

## GitHub Repo Rename (Manual, Pre-Implementation)

1. Rename `kastheco/kasmos` → `kastheco/kasmold` (free up the name)
2. Rename `kastheco/kasmos` → `kastheco/kasmos`
3. Update local git remote: `git remote set-url origin git@github.com:kastheco/kasmos.git`

## Config Directory Migration

Three-generation migration chain: `~/.hivemind` → `~/.klique` → `~/.config/kasmos/`.

`GetConfigDir()` checks in reverse order:
1. If `~/.config/kasmos/` exists → use it
2. If `~/.klique` exists → rename to `~/.config/kasmos/`, use it
3. If `~/.hivemind` exists → rename to `~/.config/kasmos/`, use it
4. Otherwise → create `~/.config/kasmos/`

## Scope

~101 files need updating across these categories:

- **Go source** (~40 files): module import paths, string literals, cobra command
- **Build/release** (~5 files): Makefile, Justfile, `.goreleaser.yaml`, install.sh, clean scripts
- **Distribution** (~3 files): homebrew formula, scoop manifest, checksums
- **Web frontend** (~7 files): package.json, Next.js components
- **Docs** (~30 files): README, CONTRIBUTING, plan files (cosmetic)
- **Tests** (~15 files): import paths, string assertions

## What Does NOT Change

- `kastheco` GitHub org name
- Internal Go package names (`package config`, `package app`, etc.)
- Architecture, behavior, features
- `.agents/` directory name (project skills)
