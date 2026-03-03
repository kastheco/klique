# Charm Ecosystem v2 Upgrade Implementation Plan

**Goal:** Upgrade all Charm ecosystem dependencies to their v2 stable releases (bubbletea v2.0.1, lipgloss v2.0.0, bubbles v2.0.0) to gain the new key handling system (shift+enter, modifier composability), declarative view model, built-in color downsampling, and the Cursed Renderer. This unblocks features that are impossible on v1 (e.g., shift+enter in focus mode).

**Architecture:** The migration touches every file that imports a Charm package (~65 source files). The core breaking changes are: (1) import path migration (`github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2`, same for lipgloss), (2) `View() string` → `View() tea.View`, (3) `tea.KeyMsg` struct → `tea.KeyPressMsg` with `Code`/`Text`/`Mod` fields replacing `Type`/`Runes`/`Alt`, (4) program options like `WithAltScreen` move to declarative `View` fields, (5) `tea.WindowSize()` → `tea.RequestWindowSize`, (6) `" "` → `"space"` in key string matching. The migration is mechanical and can be done subsystem-by-subsystem with tests verifying each step. huh and glamour stay on v1 (no v2 stable releases exist yet); harmonica has no v2.

**Tech Stack:** Go 1.24, bubbletea v2.0.1, lipgloss v2.0.0, bubbles v2.0.0, huh v0.8.0 (unchanged), glamour v0.10.0 (unchanged), harmonica v0.2.0 (unchanged)

**Size:** Large (estimated ~8 hours, 8 tasks, 4 waves)

---

## Wave 1: Foundation — Go Module and Import Paths

All import path changes and `go.mod` updates. This wave produces a non-compiling state that subsequent waves fix file-by-file.

### Task 1: Update go.mod and Migrate All Import Paths

**Files:**
- Modify: `go.mod`
- Modify: all `.go` files importing `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles/*`

**Step 1: write the failing test**

No new tests needed. Existing tests serve as the regression suite — they will fail after import path changes until the API migration is complete.

**Step 2: run test to verify baseline passes**

```bash
go test ./... -count=1
```

expected: PASS — all green before migration begins

**Step 3: write minimal implementation**

Update `go.mod` to replace the three core packages:

```bash
# Remove old dependencies
go mod edit -droprequire github.com/charmbracelet/bubbletea
go mod edit -droprequire github.com/charmbracelet/lipgloss
go mod edit -droprequire github.com/charmbracelet/bubbles

# Add new v2 dependencies
go get charm.land/bubbletea/v2@v2.0.1
go get charm.land/lipgloss/v2@v2.0.0
go get charm.land/bubbles/v2@v2.0.0
```

Then batch-replace all import paths across the codebase:

```bash
# bubbletea
sd '"github.com/charmbracelet/bubbletea"' '"charm.land/bubbletea/v2"' $(fd -e go)

# lipgloss
sd '"github.com/charmbracelet/lipgloss"' '"charm.land/lipgloss/v2"' $(fd -e go)

# bubbles subpackages (viewport, key, spinner, progress, etc.)
sd '"github.com/charmbracelet/bubbles/' '"charm.land/bubbles/v2/' $(fd -e go)
```

Run `go mod tidy` to resolve transitive dependencies.

Note: the codebase will NOT compile after this step — that's expected. The API changes in waves 2-4 fix compilation.

**Step 4: run test to verify it passes**

```bash
go build ./... 2>&1 | head -20
```

expected: FAIL — compilation errors from API changes (KeyMsg, View, etc.)

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: migrate charm ecosystem import paths to v2 vanity domains"
```

---

## Wave 2: Core API Migration — Key Handling and View Model

> **depends on wave 1:** import paths must point to v2 modules before API migration can begin.

### Task 2: Migrate Key Handling in keys/ and ui/overlay/

**Files:**
- Modify: `keys/keys.go`
- Modify: `ui/overlay/confirmationOverlay.go`
- Modify: `ui/overlay/contextMenu.go`
- Modify: `ui/overlay/formOverlay.go`
- Modify: `ui/overlay/permissionOverlay.go`
- Modify: `ui/overlay/pickerOverlay.go`
- Modify: `ui/overlay/textInput.go`
- Modify: `ui/overlay/textOverlay.go`
- Modify: `ui/overlay/tmuxBrowserOverlay.go`
- Test: `ui/overlay/formOverlay_test.go`, `ui/overlay/textInput_test.go`, `ui/overlay/tmuxBrowserOverlay_test.go`

**Step 1: write the failing test**

No new tests needed. Existing overlay tests construct `tea.KeyMsg` literals that will need updating to `tea.KeyPressMsg` / `Key` struct format.

**Step 2: run test to verify it fails**

```bash
go build ./ui/overlay/... 2>&1 | head -20
```

expected: FAIL — `tea.KeyMsg` is now an interface, not a struct

**Step 3: write minimal implementation**

In `keys/keys.go`:
- `key.NewBinding` / `key.WithKeys` / `key.Matches` — check if bubbles/v2 key package API changed. If the `key` package moved to `charm.land/bubbles/v2/key`, update imports. The binding API is expected to remain compatible.

In all overlay `HandleKeyPress` methods:
- Change parameter type from `tea.KeyMsg` to `tea.KeyPressMsg`
- Replace `msg.Type == tea.KeyRunes` with `len(msg.Text) > 0`
- Replace `msg.Runes` with `msg.Text` (now a `string`, not `[]rune`)
- Replace `msg.Type == tea.KeyEnter` with `msg.Code == tea.KeyEnter`
- Replace `msg.Type == tea.KeyEsc` / `tea.KeyEscape` with `msg.Code == tea.KeyEsc`
- Replace `msg.Type == tea.KeyBackspace` with `msg.Code == tea.KeyBackspace`
- Replace `msg.Type == tea.KeyTab` with `msg.Code == tea.KeyTab`
- Replace `msg.Type == tea.KeyDown` / `tea.KeyUp` with `msg.Code == tea.KeyDown` / `tea.KeyUp`
- Replace `msg.Type == tea.KeySpace` with `msg.Code == ' '` (or match `msg.String() == "space"`)
- Replace `string(msg.Runes)` with `msg.Text`

In all overlay test files:
- Replace `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}` with `tea.KeyPressMsg{Code: 'x', Text: "x"}`
- Replace `tea.KeyMsg{Type: tea.KeyEnter}` with `tea.KeyPressMsg{Code: tea.KeyEnter}`
- Replace `tea.KeyMsg{Type: tea.KeyEsc}` with `tea.KeyPressMsg{Code: tea.KeyEsc}`
- Replace `tea.KeyMsg{Type: tea.KeyTab}` with `tea.KeyPressMsg{Code: tea.KeyTab}`
- Replace `tea.KeyMsg{Type: tea.KeyDown}` with `tea.KeyPressMsg{Code: tea.KeyDown}`
- Replace `tea.KeyMsg{Type: tea.KeyUp}` with `tea.KeyPressMsg{Code: tea.KeyUp}`
- Replace `tea.KeyMsg{Type: tea.KeyBackspace}` with `tea.KeyPressMsg{Code: tea.KeyBackspace}`
- Replace `tea.KeyMsg{Type: tea.KeyShiftTab}` with `tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}`

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -v -count=1
go test ./keys/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add keys/ ui/overlay/
git commit -m "refactor: migrate key handling in keys/ and ui/overlay/ to bubbletea v2 API"
```

### Task 3: Migrate Key Handling in ui/ Panels

**Files:**
- Modify: `ui/preview.go`
- Modify: `ui/tabbed_window.go`
- Modify: `ui/diff.go`
- Modify: `ui/audit_pane.go`
- Modify: `ui/info_pane.go`
- Modify: `ui/navigation_panel.go`
- Modify: `ui/statusbar.go`
- Modify: `ui/menu.go`
- Modify: `ui/theme.go`
- Modify: `ui/spring.go`
- Test: `ui/preview_test.go`, `ui/tabbed_window_test.go`, `ui/nav_panel_test.go`

**Step 1: write the failing test**

No new tests needed. Existing tests exercise viewport, key matching, and rendering.

**Step 2: run test to verify it fails**

```bash
go build ./ui/... 2>&1 | head -20
```

expected: FAIL — `tea.KeyMsg` struct usage, viewport API changes

**Step 3: write minimal implementation**

- `ui/preview.go` and `ui/tabbed_window.go`: change `ViewportHandlesKey(msg tea.KeyMsg)` to `tea.KeyPressMsg`. Update `key.Matches` calls if the bubbles/v2 key API changed.
- `ui/preview.go`: update `ViewportUpdate(msg tea.KeyMsg)` parameter type.
- All lipgloss usage: lipgloss v2 API is largely compatible but uses `charm.land/lipgloss/v2` import. The `Style` type, `NewStyle()`, `Render()`, color functions should remain the same. Verify `lipgloss.Color()` still works (it does in v2).
- viewport: bubbles/v2 viewport may have API changes — check if `viewport.Model` still has `SetContent`, `View`, `Update` with same signatures.
- spinner: check if `spinner.Model` API changed in bubbles/v2.

In test files:
- Same `tea.KeyMsg` → `tea.KeyPressMsg` migration as Task 2.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/
git commit -m "refactor: migrate ui/ panels to bubbletea v2 and lipgloss v2 APIs"
```

### Task 4: Migrate Key Handling in internal/initcmd/wizard/

**Files:**
- Modify: `internal/initcmd/wizard/wizard.go`
- Modify: `internal/initcmd/wizard/model.go`
- Modify: `internal/initcmd/wizard/model_agents.go`
- Modify: `internal/initcmd/wizard/model_harness.go`
- Modify: `internal/initcmd/wizard/model_review.go`
- Modify: `internal/initcmd/wizard/styles.go`
- Test: `internal/initcmd/wizard/model_test.go`, `internal/initcmd/wizard/model_agents_test.go`, `internal/initcmd/wizard/model_review_test.go`

**Step 1: write the failing test**

No new tests needed.

**Step 2: run test to verify it fails**

```bash
go build ./internal/initcmd/wizard/... 2>&1 | head -20
```

expected: FAIL — `tea.KeyMsg`, `View() string`, program options

**Step 3: write minimal implementation**

- `wizard.go`: remove `tea.WithAltScreen()` from `tea.NewProgram()` call. The wizard's `View()` method will set `v.AltScreen = true` instead.
- `model.go`: change `View() string` to `View() tea.View`. Wrap return value with `tea.NewView(...)` and set `v.AltScreen = true`. Update `tea.KeyMsg` → `tea.KeyPressMsg` in `Update()`.
- `model_agents.go`, `model_harness.go`, `model_review.go`: update `tea.KeyMsg` → `tea.KeyPressMsg`, `msg.Type` → `msg.Code`, `msg.Runes` → `msg.Text`.
- `styles.go`: lipgloss import path change only.

In test files:
- Same `tea.KeyMsg` → `tea.KeyPressMsg` migration pattern.

**Step 4: run test to verify it passes**

```bash
go test ./internal/initcmd/wizard/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add internal/initcmd/wizard/
git commit -m "refactor: migrate init wizard to bubbletea v2 declarative view and key API"
```

---

## Wave 3: App Layer Migration

> **depends on wave 2:** the app layer imports types from `ui/` and `keys/` that must already be migrated.

### Task 5: Migrate app/app.go — Model, View, and Program Setup

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Modify: `app/app_actions.go`
- Modify: `app/help.go`
- Modify: `app/task_title.go`
- Modify: `app/clickup_progress.go`
- Test: `app/app_test.go` (partial — test infrastructure updates)

**Step 1: write the failing test**

No new tests needed.

**Step 2: run test to verify it fails**

```bash
go build ./app/... 2>&1 | head -20
```

expected: FAIL — `View() string`, `tea.WindowSize()`, `tea.WithAltScreen`, `tea.KeyMsg`

**Step 3: write minimal implementation**

- `app/app.go`:
  - Change `View() string` to `View() tea.View`. Wrap the rendered string with `tea.NewView(...)`. Set `v.AltScreen = true` and `v.MouseMode = tea.MouseModeCellMotion` (or whatever the current program options specify).
  - Remove `tea.WithAltScreen()` and `tea.WithMouseCellMotion()` from `tea.NewProgram()`.
  - In `Update()`: change `case tea.KeyMsg:` to `case tea.KeyPressMsg:`.
  - Replace `tea.WindowSizeMsg` if the type name changed (check — it may still be `tea.WindowSizeMsg` in v2).

- `app/app_state.go`, `app/app_actions.go`, `app/help.go`:
  - Replace all `tea.WindowSize()` calls with `tea.RequestWindowSize`.
  - `help.go`: change `handleHelpState(msg tea.KeyMsg)` parameter to `tea.KeyPressMsg`.

- `app/task_title.go`, `app/clickup_progress.go`:
  - Import path changes, any `tea.Model`/`tea.Cmd` usage should be compatible.

**Step 4: run test to verify it passes**

```bash
go build ./app/... 2>&1 | head -5
```

expected: compiles (tests may still fail due to test file updates needed in Task 6)

**Step 5: commit**

```bash
git add app/app.go app/app_state.go app/app_actions.go app/help.go app/task_title.go app/clickup_progress.go
git commit -m "refactor: migrate app model, view, and program setup to bubbletea v2"
```

### Task 6: Migrate app/app_input.go — Input Routing and PTY Forwarding

**Files:**
- Modify: `app/app_input.go`
- Test: `app/app_input_keybytes_test.go`, `app/app_input_viewport_test.go`, `app/app_input_right_on_instance_test.go`, `app/app_input_yes_keybind_test.go`

**Step 1: write the failing test**

No new tests needed.

**Step 2: run test to verify it fails**

```bash
go build ./app/... 2>&1 | head -20
```

expected: FAIL — `tea.KeyMsg` struct, `msg.Type`, `msg.Runes`, `tea.WindowSize()`

**Step 3: write minimal implementation**

- `app_input.go`:
  - Change all `handleKeyPress(msg tea.KeyMsg)` and `handleMenuHighlighting(msg tea.KeyMsg)` signatures to `tea.KeyPressMsg`.
  - Replace all `msg.Type == tea.KeyXxx` with `msg.Code == tea.KeyXxx`.
  - Replace `case tea.KeyRunes:` with `default:` guarded by `len(msg.Text) > 0`.
  - Replace `msg.Runes` with `msg.Text` (string, not []rune).
  - Replace `string(msg.Runes)` with `msg.Text`.
  - Replace all `tea.WindowSize()` with `tea.RequestWindowSize`.
  - `keyToBytes()`: update parameter from `tea.KeyMsg` to `tea.KeyPressMsg`. Replace `msg.Type` switch with `msg.Code` switch. Replace `case tea.KeyRunes:` with a check on `len(msg.Text) > 0`. Replace `msg.Alt` with `msg.Mod.Contains(tea.ModAlt)`. Replace `msg.Runes` with `[]byte(msg.Text)`. Add `tea.KeyShiftEnter` handling (if the key exists) or detect shift+enter via `msg.Code == tea.KeyEnter && msg.Mod.Contains(tea.ModShift)` — forward as `\x1b[13;2u` or `\r` depending on terminal capability.

- Test files:
  - Same `tea.KeyMsg{...}` → `tea.KeyPressMsg{...}` migration pattern as previous tasks.

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run 'TestKeyToBytes|TestKeyPress|TestViewport|TestRightOnInstance|TestYesKeybind' -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_input.go app/app_input_keybytes_test.go app/app_input_viewport_test.go app/app_input_right_on_instance_test.go app/app_input_yes_keybind_test.go
git commit -m "refactor: migrate app input routing and PTY forwarding to bubbletea v2 key API"
```

### Task 7: Migrate Remaining App Tests

**Files:**
- Modify: `app/app_test.go`
- Modify: `app/app_task_creation_test.go`
- Modify: `app/app_wave_orchestration_flow_test.go`
- Modify: `app/app_planner_signal_test.go`
- Modify: `app/app_permission_test.go`
- Modify: `app/app_audit_pane_test.go`
- Modify: `app/app_statusbar_integration_test.go`
- Modify: `app/app_solo_agent_test.go`
- Modify: `app/app_task_actions_test.go`
- Modify: `app/app_task_completion_test.go`
- Modify: `app/app_audit_test.go`
- Modify: `app/task_cancel_rename_delay_test.go`
- Modify: `app/clickup_progress_test.go`

**Step 1: write the failing test**

No new tests needed — this task fixes existing tests to compile and pass with v2 types.

**Step 2: run test to verify it fails**

```bash
go build ./app/... 2>&1 | head -20
```

expected: FAIL — test files still use `tea.KeyMsg{Type: ...}` struct literals

**Step 3: write minimal implementation**

Batch-migrate all test files using `sd` and manual fixups:

- Replace `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}` → `tea.KeyPressMsg{Code: 'x', Text: "x"}` (for single-char runes)
- Replace `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}` → `tea.KeyPressMsg{Code: r, Text: string(r)}`
- Replace `tea.KeyMsg{Type: tea.KeyEnter}` → `tea.KeyPressMsg{Code: tea.KeyEnter}`
- Replace `tea.KeyMsg{Type: tea.KeyEsc}` / `tea.KeyEscape` → `tea.KeyPressMsg{Code: tea.KeyEsc}`
- Replace `tea.KeyMsg{Type: tea.KeyTab}` → `tea.KeyPressMsg{Code: tea.KeyTab}`
- Replace `tea.KeyMsg{Type: tea.KeyShiftTab}` → `tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}`
- Replace `tea.KeyMsg{Type: tea.KeyBackspace}` → `tea.KeyPressMsg{Code: tea.KeyBackspace}`
- Replace `tea.KeyMsg{Type: tea.KeyDelete}` → `tea.KeyPressMsg{Code: tea.KeyDelete}`
- Replace `tea.KeyMsg{Type: tea.KeyUp}` → `tea.KeyPressMsg{Code: tea.KeyUp}`
- Replace `tea.KeyMsg{Type: tea.KeyDown}` → `tea.KeyPressMsg{Code: tea.KeyDown}`
- Replace `tea.KeyMsg{Type: tea.KeyLeft}` → `tea.KeyPressMsg{Code: tea.KeyLeft}`
- Replace `tea.KeyMsg{Type: tea.KeyRight}` → `tea.KeyPressMsg{Code: tea.KeyRight}`
- Replace `tea.KeyMsg{Type: tea.KeyPgDown}` → `tea.KeyPressMsg{Code: tea.KeyPgDown}`
- Replace `tea.KeyMsg{Type: tea.KeyCtrlS}` → `tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}`
- Replace `tea.KeyMsg{Type: tea.KeyCtrlUp}` → `tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}`
- Replace `tea.KeyMsg{Type: tea.KeyCtrlDown}` → `tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}`
- Replace `tea.KeyMsg{Type: tea.KeyCtrlAt}` → check v2 equivalent (Ctrl+Space / Ctrl+@)

**Step 4: run test to verify it passes**

```bash
go test ./app/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add app/
git commit -m "refactor: migrate all app test files to bubbletea v2 KeyPressMsg literals"
```

---

## Wave 4: Session Layer and Final Verification

> **depends on wave 3:** session/terminal.go and session/permission_prompt.go import from bubbletea and must compile after the app layer is done.

### Task 8: Migrate Session Layer and Full Suite Verification

**Files:**
- Modify: `session/terminal.go`
- Modify: `session/permission_prompt.go`
- Modify: `session/tmux/tmux_io.go`
- Test: full suite

**Step 1: write the failing test**

No new tests needed.

**Step 2: run test to verify it fails**

```bash
go build ./session/... 2>&1 | head -10
```

expected: FAIL if any bubbletea types are used in session layer

**Step 3: write minimal implementation**

- `session/terminal.go`: update import path. This file uses the bubbletea terminal for PTY rendering — check if the `tea.Program` interaction API changed.
- `session/permission_prompt.go`: update import path and any `tea.KeyMsg` usage.
- `session/tmux/tmux_io.go`: update import path if it imports bubbletea.

Run `go mod tidy` to clean up any stale indirect dependencies.

Verify the entire codebase compiles and all tests pass:

```bash
go build ./...
go test ./... -count=1
```

Run `typos` to catch any spelling issues introduced during migration.

**Step 4: run test to verify it passes**

```bash
go test ./... -count=1
```

expected: PASS — all packages green

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: migrate session layer to v2 and verify full test suite"
```
