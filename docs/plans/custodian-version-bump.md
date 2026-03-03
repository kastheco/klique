# Custodian Version Bump Awareness

**Goal:** teach the custodian agent to bump the `version` constant in `main.go` before tagging a release, so the CI/CD "Validate tag matches version in main.go" step doesn't fail.

**Architecture:** add a "Release version bump" section to the custodian agent prompt (`.opencode/agents/custodian.md`) and skill (`.opencode/skills/kasmos-custodian/SKILL.md`) documenting the version-tag contract and the exact steps to update `main.go` before pushing a new tag.

**Tech Stack:** custodian skill markdown, `sd` for version replacement, git tagging

**Size:** Trivial (estimated ~15 min, 1 task, 1 wave)

---

## Context

The GitHub Actions `Release` workflow (`.github/workflows/release.yml`) has a validation step that extracts the `version` constant from `main.go` and compares it to the pushed git tag. If they don't match, the build fails immediately:

```
Version in main.go: 1.1.0
Tag version: 1.1.1
ERROR: Tag version (1.1.1) does not match version in main.go (1.1.0)
Please ensure the tag matches the version defined in main.go
Error: Process completed with exit code 1.
```

This happened on run #16 ("fix: promote done plans with running instances to active section inst...") when tag `v1.1.1` was pushed but `main.go` still had `version = "1.1.0"`.

The custodian agent handles release operations (`/kas.finish-branch`, branch merging, tagging) but has no awareness of this version-tag contract. It needs explicit instructions to:

1. Update `version` in `main.go` to match the intended tag **before** creating the tag
2. Commit the version bump on main
3. Then create and push the tag

## Wave 1: Add version bump instructions to custodian

### Task 1: Update custodian agent prompt and skill with release version protocol

**Files:**
- Modify: `.opencode/agents/custodian.md`
- Modify: `.opencode/skills/kasmos-custodian/SKILL.md`

Add a "Release Version Bump" section to both files covering:

1. **The contract:** `main.go` line 25 has `version = "X.Y.Z"` — this MUST match the tag being pushed (without the `v` prefix). The CI step `Validate tag matches version in main.go` enforces this.

2. **The procedure** (when creating a release tag):
   ```bash
   # 1. determine new version
   NEW_VERSION="1.2.0"  # whatever the target is

   # 2. bump version in main.go
   sd 'version\s*=\s*"[^"]*"' "version     = \"${NEW_VERSION}\"" main.go

   # 3. commit the bump
   git add main.go
   git commit -m "chore: bump version to ${NEW_VERSION}"

   # 4. tag and push
   git tag "v${NEW_VERSION}"
   git push origin main "v${NEW_VERSION}"
   ```

3. **The check:** before pushing any `v*` tag, always verify:
   ```bash
   rg '^[[:space:]]*version[[:space:]]*=' main.go
   ```
   and confirm the version matches the tag.

**Step 1:** Add a `## Release Operations` section to `.opencode/agents/custodian.md` after the existing `## Slash Commands` section.

**Step 2:** Add a `## Release Version Bump` section to `.opencode/skills/kasmos-custodian/SKILL.md` after the `## Cleanup Protocol` section.

**Step 3:** Verify both files parse correctly and the instructions are clear.

**Step 4:** Commit.

```bash
git add .opencode/agents/custodian.md .opencode/skills/kasmos-custodian/SKILL.md
git commit -m "docs: add version bump protocol to custodian agent"
```
