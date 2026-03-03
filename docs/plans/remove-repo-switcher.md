# Remove Repo Switcher Implementation Plan

**Goal:** Remove the repo switcher UI component and all supporting code — users who want to work on another project can quit and cd to the directory instead.

**Architecture:** Delete the repo switcher button from the navigation panel, remove the `stateRepoSwitch` app state and picker overlay handling, remove the `R` keybinding, delete `folder_picker.go`, remove `RecentRepos` from persisted config state, and remove `RepoName` from the status bar. The `allInstances` master list and `activeRepoPath` remain since they serve persistence and plan-state loading — but the repo-filtering methods (`rebuildInstanceList`, `getKnownRepos`, `buildRepoPickerItems`, `switchToRepo`) are deleted.

**Tech Stack:** Go, bubbletea, lipgloss, bubblezone

**Size:** Small (estimated ~1 hour, 2 tasks, 2 waves)

---

## Wave 1: Remove all repo switcher code

### Task 1: Remove repo switcher UI, state, keybinding, input handling, and help text

This is a single coordinated removal across the `ui`, `app`, and `keys` packages. The changes are tightly coupled — removing the UI fields and removing their callers must happen atomically to keep the build green.

**Files:**
- Modify: `ui/navigation_panel.go`
- Modify: `ui/zones.go`
- Modify: `ui/statusbar.go`
- Modify: `ui/statusbar_test.go`
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/app_state.go`
- Modify: `app/help.go`
- Modify: `app/app_state_sidebar_status_test.go`
- Modify: `keys/keys.go`
- Delete: `app/folder_picker.go`

**Step 1: write the failing test**

No new test needed — this is a pure removal. Existing tests will be updated to remove references to deleted symbols.

**Step 2: run test to verify baseline**

```bash
go test ./ui/... ./app/... ./keys/... -count=1
```

expected: PASS (baseline before changes)

**Step 3: implement the removal**

**ui/navigation_panel.go:**
- Remove `repoName string` and `repoHovered bool` fields from `NavigationPanel` struct
- Remove `SetRepoName` and `SetRepoHovered` methods
- Remove the "Repo switcher" rendering block in `String()` (lines 1380-1394): the `repoSection` variable, `zone.Mark(ZoneNavRepo, ...)`, and the conditional that builds `legendSection` with `repoSection`
- Simplify `legendSection` assignment to just `legendSection = legend`

**ui/zones.go:**
- Remove `ZoneNavRepo = "zone-nav-repo"` constant

**ui/statusbar.go:**
- Remove `RepoName string` field from `StatusBarData`
- Remove `statusBarPlanNameStyle` (only used for repo name)
- Remove the `if s.data.RepoName != ""` block and `right` variable usage in `String()`
- Remove `rightWidth`, `rightStart`, and `writeAt(rightStart, ...)` since there's no right-aligned content

**ui/statusbar_test.go:**
- Remove `RepoName` from all `StatusBarData` literals
- Remove assertions about repo name positioning (`repoIdx`, `HasSuffix(trimmedRight, "my-repo")`)
- Remove or rewrite `TestStatusBar_VeryLongRepoName` (tests repo name truncation — no longer relevant)
- Remove or rewrite `TestStatusBar_BranchGroupCenteredAndRepoRightAligned` — keep the branch centering assertion but remove repo right-alignment assertion

**keys/keys.go:**
- Remove `KeyRepoSwitch` from the `KeyName` iota
- Remove `"R": KeyRepoSwitch` from the key-to-name map
- Remove the `KeyRepoSwitch: key.NewBinding(...)` entry

**app/app.go:**
- Remove `stateRepoSwitch` from the state iota and its comment
- Remove `repoPickerMap map[string]string` field from `home` struct
- Remove `h.nav.SetRepoName(filepath.Base(activeRepoPath))` in `New()` (line 425)
- Remove `state.AddRecentRepo(activeRepoPath)` block in `New()` (lines 456-458)
- Remove `folderPickedMsg` case from `Update()` (lines 1551-1565)
- Remove `stateRepoSwitch` overlay rendering case from `View()` (lines 1667-1674)

**app/app_input.go:**
- Remove `stateRepoSwitch` from the long state-check condition (line 29)
- Remove `ZoneNavRepo` hover tracking (lines 56-58)
- Remove `stateRepoSwitch` click-outside dismiss (lines 84-88)
- Remove `ZoneNavRepo` click handler (lines 108-113)
- Remove `stateRepoSwitch` picker overlay handling block (lines 963-984)
- Remove `KeyRepoSwitch` case from `handleDefaultKey` (lines 1473-1476)

**app/app_state.go:**
- Remove `RepoName` from `computeStatusBarData()` (line 62)
- Remove `rebuildInstanceList()` method (lines 274-287)
- Remove `getKnownRepos()` method (lines 289-311)
- Remove `buildRepoPickerItems()` method (lines 313-351)
- Remove `switchToRepo()` method (lines 353-365)

**app/help.go:**
- Remove the `R` / "switch repo" line from `helpTypeGeneral.toContent()` (line 54)

**app/app_state_sidebar_status_test.go:**
- Remove `h.nav.SetRepoName("kasmos")` call
- Remove `assert.Equal(t, "kasmos", data.RepoName)` assertion

**Delete `app/folder_picker.go`** — entirely dead code (the `openFolderPicker` method was only called from the repo switch handler).

**Step 4: run tests to verify**

```bash
go build ./... && go test ./ui/... ./app/... ./keys/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git rm app/folder_picker.go
git add ui/navigation_panel.go ui/zones.go ui/statusbar.go ui/statusbar_test.go app/app.go app/app_input.go app/app_state.go app/help.go app/app_state_sidebar_status_test.go keys/keys.go
git commit -m "refactor: remove repo switcher UI, state, keybinding, and input handling"
```

## Wave 2: Clean up config state

> **depends on wave 1:** `GetRecentRepos` and `AddRecentRepo` callers in `app/app.go` and `app/app_state.go` were removed in wave 1.

### Task 2: Remove RecentRepos from persisted config state

**Files:**
- Modify: `config/state.go`

**Step 1: write the failing test**

No new test — this removes unused config fields and methods. The build will confirm no remaining callers.

**Step 2: run test to verify baseline**

```bash
go test ./config/... -v -count=1
```

expected: PASS

**Step 3: implement the removal**

In `config/state.go`:
- Remove `RecentRepos []string` field from `State` struct (line 47)
- Remove `GetRecentRepos()` method (lines 143-146)
- Remove `AddRecentRepo()` method (lines 148-157)

Note: existing `state.json` files on disk will silently ignore the now-unknown `recent_repos` key during JSON unmarshalling — no migration needed.

**Step 4: run tests to verify**

```bash
go build ./... && go test ./... -count=1
```

expected: PASS — full build and test suite green

**Step 5: commit**

```bash
git add config/state.go
git commit -m "refactor: remove RecentRepos from persisted config state"
```
