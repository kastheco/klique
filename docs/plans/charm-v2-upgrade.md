# Charm Ecosystem v2 Upgrade Implementation Plan

**Goal:** Migrate klique from Charm v1 (bubbletea v1.3, lipgloss v1.1, bubbles v0.21) to Charm v2 (bubbletea v2, lipgloss v2, bubbles v2) — adopting the new declarative View API, restructured key/mouse events, and getter/setter component patterns.

**Architecture:** All three core Charm libraries must upgrade atomically since they share types at module boundaries. The migration rewrites import paths (`charm.land/*/v2`), converts `View() string` to `View() tea.View`, replaces `tea.KeyMsg` struct matching with `tea.KeyPressMsg`, moves program options into declarative View fields, and updates bubbles component APIs to use getter/setter methods. huh v2 (experimental branch) replaces huh v0.8. glamour and harmonica remain at current versions.

**Tech Stack:** Go 1.24+, charm.land/bubbletea/v2, charm.land/lipgloss/v2, charm.land/bubbles/v2, charm.land/huh/v2 (v2-exp branch), charmbracelet/glamour v0.10, charmbracelet/harmonica v0.2

**Size:** Large (estimated ~5 hours, 5 tasks, 3 waves)

---

## Wave 1: Module Bootstrap and Batch Import Rewrite

### Task 1: Update go.mod and Batch-Rewrite All Import Paths

**Files:**
- Modify: `go.mod`
- Modify: all `*.go` files (import blocks only — ~70 files)

> TDD note: Steps 1-2 omitted — this is a config/import-only task with no testable logic. Module resolution and compilation in subsequent tasks serve as verification.

**Step 3: write minimal implementation**

1. Update `go.mod` — add charm.land v2 modules:
   ```bash
   go get charm.land/bubbletea/v2@latest
   go get charm.land/lipgloss/v2@latest
   go get charm.land/bubbles/v2@latest
   go get charm.land/huh/v2@latest   # use v2-exp tag if stable tag not yet available
   go mod tidy
   ```

2. Batch-rewrite all import paths using `sd`:
   ```bash
   sd 'github.com/charmbracelet/bubbletea' 'charm.land/bubbletea/v2' $(fd -e go)
   sd 'github.com/charmbracelet/lipgloss' 'charm.land/lipgloss/v2' $(fd -e go)
   sd 'github.com/charmbracelet/bubbles/' 'charm.land/bubbles/v2/' $(fd -e go)
   sd 'github.com/charmbracelet/huh' 'charm.land/huh/v2' $(fd -e go)
   ```

3. Do NOT change `glamour` or `harmonica` imports — they remain on `github.com/charmbracelet/`.

4. Run `go mod tidy` to resolve the dependency graph.

**Step 4: run test to verify it passes**

```bash
go mod tidy && echo "module resolution OK"
```

expected: module resolution succeeds (code won't fully compile until wave 2)

**Step 5: commit**

```bash
git add go.mod go.sum $(fd -e go)
git commit -m "refactor: rewrite charm import paths from github.com to charm.land/v2"
```

---

## Wave 2: API Migration by Subsystem

> **depends on wave 1:** import paths must point to v2 modules before API calls can be updated to v2 signatures.

### Task 2: Migrate App Package — View, Keys, Mouse, and Program Options

All bubbletea v2 API changes in the `app/` and `keys/` packages: `View() string` → `View() tea.View`, `tea.KeyMsg` → `tea.KeyPressMsg`, `msg.Type` → `msg.Code`/`msg.String()`, mouse event restructuring, and program option removal.

**Files:**
- Modify: `app/app.go` (View signature, program options, mouse dispatch)
- Modify: `app/app_input.go` (key handling via msg.Type → msg.Code, mouse handling)
- Modify: `app/app_state.go` (.Render calls)
- Modify: `app/app_actions.go`
- Modify: `app/help.go`
- Modify: `app/task_title.go`
- Modify: `app/clickup_progress.go`
- Modify: `keys/keys.go` (key binding definitions — update if `key.Binding` API changed)

**Step 1: write the failing test**

```bash
go build ./app/... ./keys/... 2>&1 | head -20
```

expected: FAIL — type mismatches on View(), KeyMsg, MouseMsg, program options

**Step 2: run test to verify it fails**

```bash
go build ./app/... 2>&1 | head -5
```

expected: FAIL

**Step 3: write minimal implementation**

**3a. View() signature** — in `app/app.go`:
```go
// Before
func (m *home) View() string {
    // ... renders content string
    return result
}

// After
func (m *home) View() tea.View {
    // ... renders content string
    v := tea.NewView(result)
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    return v
}
```
Remove `tea.WithAltScreen()` and `tea.WithMouseCellMotion()` from `tea.NewProgram()`.

**3b. Key events** — in `app/app_input.go` (30+ msg.Type usages):
- All `case tea.KeyMsg:` → `case tea.KeyPressMsg:`
- `msg.Type == tea.KeyEnter` → `msg.Code == tea.KeyEnter`
- `msg.Type == tea.KeyEsc` / `tea.KeyEscape` → `msg.Code == tea.KeyEscape`
- `msg.Type == tea.KeyCtrlC` → `msg.String() == "ctrl+c"`
- `msg.Type == tea.KeyRunes` → `len(msg.Text) > 0`
- `msg.Type == tea.KeyBackspace` → `msg.Code == tea.KeyBackspace`
- `msg.Type == tea.KeySpace` → `msg.Code == ' '`
- `msg.Type == tea.KeyTab` → `msg.Code == tea.KeyTab`
- `msg.Type == tea.KeyShiftTab` → `msg.String() == "shift+tab"`
- `msg.Type == tea.KeyCtrlUp` → `msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModCtrl)`
- `msg.Type == tea.KeyCtrlDown` → `msg.Code == tea.KeyDown && msg.Mod.Contains(tea.ModCtrl)`
- `msg.Type == tea.KeyShiftUp` → `msg.Code == tea.KeyUp && msg.Mod.Contains(tea.ModShift)`
- `msg.Type == tea.KeyShiftDown` → `msg.Code == tea.KeyDown && msg.Mod.Contains(tea.ModShift)`
- `msg.Type == tea.KeyCtrlU` → `msg.Code == 'u' && msg.Mod.Contains(tea.ModCtrl)`
- `msg.Type == tea.KeyCtrlD` → `msg.Code == 'd' && msg.Mod.Contains(tea.ModCtrl)`
- `msg.Type == tea.KeyCtrlAt` → `msg.String() == "ctrl+@"` (or equivalent)
- `msg.Type == tea.KeyDelete` → `msg.Code == tea.KeyDelete`
- `msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ` → `msg.Mod.Contains(tea.ModCtrl) && msg.Code >= 'a' && msg.Code <= 'z'`
- `string(msg.Runes)` → `msg.Text`
- `[]byte{byte(msg.Type)}` → encode from `msg.Code` and `msg.Mod`

**3c. Mouse events** — in `app/app_input.go`:
```go
// Before
func (m *home) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
    if msg.Action != tea.MouseActionPress { return m, nil }
    if msg.Button == tea.MouseButtonLeft { ... }

// After
func (m *home) handleMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
    if msg.Button == tea.MouseLeft { ... }
```
Update the dispatch in `app/app.go` Update():
```go
// Before
case tea.MouseMsg:
    return m.handleMouse(msg)

// After
case tea.MouseClickMsg:
    return m.handleMouse(msg)
```

**3d. keys/keys.go** — verify `key.Binding` and `key.NewBinding` APIs are unchanged in bubbles v2. The `key` package is likely stable. Fix any type mismatches.

**Step 4: run test to verify it passes**

```bash
go build ./app/... ./keys/...
```

expected: PASS (app package compiles)

**Step 5: commit**

```bash
git add app/ keys/
git commit -m "refactor: migrate app package to bubbletea v2 View/Key/Mouse APIs"
```

### Task 3: Migrate UI, Overlay, and Wizard Packages — View, Keys, Viewport, huh

All bubbletea/bubbles v2 API changes in `ui/`, `ui/overlay/`, and `internal/initcmd/wizard/`: View signatures, key events, viewport constructor and getter/setters, textinput migration, and huh v2 form library.

**Files:**
- Modify: `ui/preview.go` (viewport.New, .Width/.Height/.YOffset, key.Matches)
- Modify: `ui/info_pane.go` (viewport.New, .Width/.Height)
- Modify: `ui/diff.go` (viewport.New, .Width/.Height)
- Modify: `ui/audit_pane.go` (viewport.New, .Width/.Height)
- Modify: `ui/tabbed_window.go` (viewport references, View if applicable)
- Modify: `ui/menu.go`
- Modify: `ui/navigation_panel.go`
- Modify: `ui/statusbar.go`
- Modify: `ui/theme.go`
- Modify: `ui/overlay/iface.go` (overlay View interface — keep as `View() string` since overlays return fragments)
- Modify: `ui/overlay/confirmationOverlay.go` (key events, View)
- Modify: `ui/overlay/contextMenu.go` (key events, View)
- Modify: `ui/overlay/formOverlay.go` (key events, huh v2 API, View)
- Modify: `ui/overlay/manager.go` (key event dispatch)
- Modify: `ui/overlay/permissionOverlay.go` (key events, View)
- Modify: `ui/overlay/pickerOverlay.go` (key events, View)
- Modify: `ui/overlay/textInput.go` (key events, textinput style)
- Modify: `ui/overlay/textOverlay.go` (key events, View)
- Modify: `ui/overlay/tmuxBrowserOverlay.go` (key events, View)
- Modify: `ui/overlay/toast.go` (View)
- Modify: `ui/overlay/overlay.go`
- Modify: `ui/overlay/theme.go` (huh theme API)
- Modify: `internal/initcmd/wizard/model.go` (View, key events, program options)
- Modify: `internal/initcmd/wizard/model_agents.go` (key events, textinput, `case " ":` → `case "space":`)
- Modify: `internal/initcmd/wizard/model_harness.go` (key events)
- Modify: `internal/initcmd/wizard/model_review.go` (key events, View)
- Modify: `internal/initcmd/wizard/styles.go` (lipgloss NewStyle — likely no change)
- Modify: `internal/initcmd/wizard/wizard.go` (program options removal)

**Step 1: write the failing test**

```bash
go build ./ui/... ./internal/initcmd/... 2>&1 | head -20
```

expected: FAIL — type mismatches across UI and wizard packages

**Step 2: run test to verify it fails**

```bash
go build ./ui/... 2>&1 | head -5
```

expected: FAIL

**Step 3: write minimal implementation**

**3a. Overlay View interface** — `ui/overlay/iface.go`:
The `Overlay` interface's `View() string` is NOT a `tea.Model.View()` — overlays return string fragments composed by the parent. Keep it as `View() string`. Only the root model (`app/app.go`, handled in Task 2) returns `tea.View`.

**3b. Overlay key events** — all overlay `Update` methods:
- `case tea.KeyMsg:` → `case tea.KeyPressMsg:`
- `msg.Type == tea.KeyEnter` → `msg.Code == tea.KeyEnter`
- `msg.Type == tea.KeyEsc` → `msg.Code == tea.KeyEscape`
- `msg.Type == tea.KeyRunes` → `len(msg.Text) > 0`
- `msg.String()` calls — unchanged (method exists on `tea.KeyPressMsg`)

**3c. Viewport constructors** — 4 sites:
```go
// Before: viewport.New(0, 0)
// After:  viewport.New()
```

**3d. Viewport getter/setters** — in preview, info_pane, diff, audit_pane:
```go
// Before                          // After
p.viewport.Width = w               p.viewport.SetWidth(w)
p.viewport.Height = h              p.viewport.SetHeight(h)
p.viewport.YOffset                 p.viewport.YOffset()
```

**3e. huh v2** — in `ui/overlay/formOverlay.go` and `ui/overlay/theme.go`:
- Update `huh.NewForm()`, `huh.NewGroup()`, `huh.NewInput()` to v2 API
- `huh.PrevField()` / `huh.NextField()` — verify still exist
- `huh.ThemeBase()` — check v2 name, may need `isDark` parameter
- Adapt theme construction in `ThemeRosePine()` for huh v2 style structs

**3f. Wizard** — `internal/initcmd/wizard/`:
- `model.go`: `View() string` → `View() tea.View` (wizard is a standalone program)
- `wizard.go`: remove `tea.WithAltScreen()` from `NewProgram()`, set in View
- `model_agents.go`: `case " ":` → `case "space":`, key event updates
- All wizard models: `tea.KeyMsg` → `tea.KeyPressMsg`

**3g. textinput migration** — `model_agents.go`:
- `textinput.New()` is unchanged in v2
- If style fields are set directly, migrate to `Styles` struct

**Step 4: run test to verify it passes**

```bash
go build ./ui/... ./internal/initcmd/...
```

expected: PASS

**Step 5: commit**

```bash
git add ui/ internal/initcmd/
git commit -m "refactor: migrate ui/overlay/wizard packages to charm v2 APIs"
```

---

## Wave 3: Test Migration and Final Verification

> **depends on wave 2:** all source files must compile before test files can be updated and run.

### Task 4: Migrate All Test Files

All test files construct `tea.KeyMsg` struct literals for input simulation. These must become `tea.KeyPressMsg` with v2 fields (`Code`, `Text`, `Mod` instead of `Type`, `Runes`, `Alt`). Also update viewport field access and mouse event construction in tests.

**Files:**
- Modify: `app/app_test.go` (38 tea.KeyMsg refs)
- Modify: `app/app_task_creation_test.go` (13 refs)
- Modify: `app/app_permission_test.go` (8 refs)
- Modify: `app/app_input_right_on_instance_test.go`
- Modify: `app/app_input_yes_keybind_test.go`
- Modify: `app/app_input_viewport_test.go`
- Modify: `app/app_input_keybytes_test.go`
- Modify: `app/app_audit_pane_test.go`
- Modify: `app/app_planner_signal_test.go`
- Modify: `app/app_wave_orchestration_flow_test.go`
- Modify: `app/task_cancel_rename_delay_test.go`
- Modify: `app/app_statusbar_integration_test.go`
- Modify: `app/clickup_progress_test.go`
- Modify: `app/app_solo_agent_test.go`
- Modify: `app/app_task_actions_test.go`
- Modify: `app/app_task_completion_test.go`
- Modify: `ui/overlay/formOverlay_test.go` (42 refs)
- Modify: `ui/overlay/tmuxBrowserOverlay_test.go` (19 refs)
- Modify: `ui/overlay/pickerOverlay_test.go`
- Modify: `ui/overlay/textInput_test.go`
- Modify: `ui/overlay/contextMenu_test.go`
- Modify: `ui/overlay/confirmationOverlay_test.go`
- Modify: `ui/overlay/permissionOverlay_test.go`
- Modify: `ui/overlay/iface_test.go`
- Modify: `ui/overlay/manager_test.go`
- Modify: `ui/overlay/textOverlay_test.go`
- Modify: `ui/overlay/toast_test.go`
- Modify: `ui/overlay/theme_test.go`
- Modify: `ui/preview_test.go`
- Modify: `ui/tabbed_window_test.go`
- Modify: `ui/nav_panel_test.go`
- Modify: `internal/initcmd/wizard/model_test.go`
- Modify: `internal/initcmd/wizard/model_agents_test.go`
- Modify: `internal/initcmd/wizard/model_review_test.go`

**Step 1: write the failing test**

```bash
go test ./... 2>&1 | head -30
```

expected: FAIL — test files won't compile due to tea.KeyMsg struct literal changes

**Step 2: run test to verify it fails**

```bash
go build ./... 2>&1 | rg '_test.go' | head -10
```

expected: FAIL

**Step 3: write minimal implementation**

Batch-migrate test key event construction using `comby`:

```go
// Before (v1 struct literals)
tea.KeyMsg{Type: tea.KeyEnter}
tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
tea.KeyMsg{Type: tea.KeyEsc}
tea.KeyMsg{Type: tea.KeyCtrlC}

// After (v2 — KeyPressMsg with Code/Text)
tea.KeyPressMsg{Code: tea.KeyEnter}
tea.KeyPressMsg{Code: 'q', Text: "q"}
tea.KeyPressMsg{Code: tea.KeyEscape}
tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
```

Use `comby` for consistent patterns:
```bash
comby 'tea.KeyMsg{Type: tea.KeyEnter}' 'tea.KeyPressMsg{Code: tea.KeyEnter}' .go -in-place
comby 'tea.KeyMsg{Type: tea.KeyEsc}' 'tea.KeyPressMsg{Code: tea.KeyEscape}' .go -in-place
# ... etc for each pattern
```

Then manually fix:
- `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(...)}` → `tea.KeyPressMsg{Code: rune, Text: string}`
- `tea.WindowSizeMsg` — field names unchanged, no migration needed
- Mouse event construction in tests:
  ```go
  // Before
  tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 5, Y: 10}
  // After
  tea.MouseClickMsg{Button: tea.MouseLeft, Mouse: tea.Mouse{X: 5, Y: 10}}
  ```
- Viewport YOffset in `ui/tabbed_window_test.go`:
  ```go
  preview.viewport.YOffset    →    preview.viewport.YOffset()
  ```

**Step 4: run test to verify it passes**

```bash
go test ./... -count=1
```

expected: PASS — all tests green

**Step 5: commit**

```bash
git add -A
git commit -m "test: migrate all test files to charm v2 key/mouse event constructors"
```

### Task 5: Final Build Verification and Full Test Suite

Complete verification of the migration — build, vet, race-detect test, and fix any remaining issues.

**Files:**
- Possibly modify: any file with remaining compilation or test failures

> TDD note: this is a verification/fixup task. Steps 1-2 are the verification itself.

**Step 1: write the failing test**

```bash
go build ./... && go vet ./... && go test ./... -count=1 -race
```

**Step 2: run test to verify it fails**

```bash
go build ./... && go vet ./...
```

expected: PASS (or FAIL with specific remaining issues to fix)

**Step 3: write minimal implementation**

Fix any remaining issues:
- Unused imports from removed v1 types (e.g., `cursor` package if no longer needed)
- Type assertion failures from interface changes
- Runtime panics from incorrect mouse/key event handling
- Verify unused v1 module references are cleaned from `go.sum`
- Run `go mod tidy` one final time

**Step 4: run test to verify it passes**

```bash
go build ./... && go vet ./... && go test ./... -count=1 -race
```

expected: PASS — all tests green, no data races, no vet warnings

**Step 5: commit**

```bash
git add -A
git commit -m "fix: resolve remaining charm v2 migration issues and verify full test suite"
```
