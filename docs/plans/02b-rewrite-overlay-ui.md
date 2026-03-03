# Rewrite Overlay UI Implementation Plan

**Goal:** Clean-room rewrite of `ui/overlay/` to replace 8 ad-hoc overlay structs (each with different APIs) with a unified `Overlay` interface and `OverlayManager`. Eliminates the 27-state switch explosion in `app/app_input.go` and the 20-case render switch in `app/app.go:View()` by making overlays polymorphic. Preserves all existing overlay types and their behavior.

**Architecture:** A new `Overlay` interface defines the contract: `HandleKey(tea.KeyMsg) Result`, `View() string`, `SetSize(w, h int)`. The `Result` type carries dismiss/submit/action signals so the `home` struct doesn't need per-overlay type assertions. An `OverlayManager` holds the active overlay stack (modal + toast layer), handles `PlaceOverlay` compositing, and exposes `IsActive() bool` for the input guard. Each concrete overlay (confirmation, text input, picker, form, permission, context menu, tmux browser) is rewritten to implement the interface. The `home` struct replaces 8 pointer fields with one `OverlayManager`, and the giant state/render switches collapse to `if m.overlays.IsActive() { return m.overlays.HandleKey(msg) }`.

**Tech Stack:** Go 1.24, bubbletea v1.3, lipgloss v1.1, bubbles v0.20, huh v0.6, testify

**Size:** Large (estimated ~8 hours, 7 tasks, 3 waves)

---

## Wave 1: Interface, Manager, and PlaceOverlay

Establishes the foundational types that all concrete overlays will implement. No app-layer changes yet — the interface and manager are built and tested in isolation.

### Task 1: Define Overlay Interface and Result Type

**Files:**
- Create: `ui/overlay/iface.go`
- Test: `ui/overlay/iface_test.go`

**Step 1: write the failing test**

```go
// iface_test.go
package overlay

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
)

// stubOverlay is a minimal Overlay implementation for testing the interface contract.
type stubOverlay struct {
    dismissed bool
    rendered  string
    w, h      int
}

func (s *stubOverlay) HandleKey(msg tea.KeyMsg) Result {
    if msg.Type == tea.KeyEsc {
        s.dismissed = true
        return Result{Dismissed: true}
    }
    if msg.Type == tea.KeyEnter {
        return Result{Dismissed: true, Submitted: true, Value: "test-value"}
    }
    return Result{}
}

func (s *stubOverlay) View() string   { return s.rendered }
func (s *stubOverlay) SetSize(w, h int) { s.w = w; s.h = h }

func TestOverlayInterface_Dismiss(t *testing.T) {
    var o Overlay = &stubOverlay{rendered: "hello"}
    result := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}

func TestOverlayInterface_Submit(t *testing.T) {
    var o Overlay = &stubOverlay{rendered: "hello"}
    result := o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "test-value", result.Value)
}

func TestOverlayInterface_SetSize(t *testing.T) {
    s := &stubOverlay{}
    var o Overlay = s
    o.SetSize(80, 24)
    assert.Equal(t, 80, s.w)
    assert.Equal(t, 24, s.h)
}

func TestResult_ActionField(t *testing.T) {
    r := Result{Dismissed: true, Action: "kill"}
    assert.Equal(t, "kill", r.Action)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run TestOverlayInterface -v
```

expected: FAIL — `Overlay` and `Result` types undefined

**Step 3: write minimal implementation**

Define the `Overlay` interface and `Result` struct in `ui/overlay/iface.go`:

```go
package overlay

import tea "github.com/charmbracelet/bubbletea"

// Result is returned by Overlay.HandleKey to signal what happened.
type Result struct {
    // Dismissed is true when the overlay should be closed.
    Dismissed bool
    // Submitted is true when the user confirmed/submitted (not just dismissed).
    Submitted bool
    // Value carries the primary return value (text input content, selected item, etc.).
    Value string
    // Action carries a secondary action identifier (context menu action, browser action, etc.).
    Action string
}

// Overlay is the common interface for all modal overlay components.
// Every overlay type in the package implements this interface.
type Overlay interface {
    // HandleKey processes a key event and returns the result.
    HandleKey(msg tea.KeyMsg) Result
    // View renders the overlay content (without the PlaceOverlay compositing).
    View() string
    // SetSize updates the available dimensions for the overlay.
    SetSize(w, h int)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run TestOverlayInterface -v
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/iface.go ui/overlay/iface_test.go
git commit -m "feat(overlay): define Overlay interface and Result type"
```

### Task 2: Implement OverlayManager with PlaceOverlay Compositing

**Files:**
- Create: `ui/overlay/manager.go`
- Test: `ui/overlay/manager_test.go`

**Step 1: write the failing test**

```go
// manager_test.go
package overlay

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestManager_IsActive(t *testing.T) {
    mgr := NewManager()
    assert.False(t, mgr.IsActive())

    mgr.Show(&stubOverlay{rendered: "test"})
    assert.True(t, mgr.IsActive())
}

func TestManager_ShowAndDismiss(t *testing.T) {
    mgr := NewManager()
    mgr.Show(&stubOverlay{rendered: "overlay content"})
    require.True(t, mgr.IsActive())

    result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, mgr.IsActive())
}

func TestManager_Submit(t *testing.T) {
    mgr := NewManager()
    mgr.Show(&stubOverlay{rendered: "form"})

    result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Submitted)
    assert.Equal(t, "test-value", result.Value)
    assert.False(t, mgr.IsActive(), "overlay should be dismissed after submit")
}

func TestManager_HandleKeyWhenInactive(t *testing.T) {
    mgr := NewManager()
    result := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.False(t, result.Dismissed)
    assert.False(t, result.Submitted)
}

func TestManager_SetSize(t *testing.T) {
    s := &stubOverlay{}
    mgr := NewManager()
    mgr.Show(s)
    mgr.SetSize(100, 50)
    assert.Equal(t, 100, s.w)
    assert.Equal(t, 50, s.h)
}

func TestManager_Render_Inactive(t *testing.T) {
    mgr := NewManager()
    bg := "background content"
    assert.Equal(t, bg, mgr.Render(bg))
}

func TestManager_Render_Active(t *testing.T) {
    mgr := NewManager()
    mgr.SetSize(80, 24)
    mgr.Show(&stubOverlay{rendered: "OVERLAY"})
    bg := "background content here"
    result := mgr.Render(bg)
    assert.NotEqual(t, bg, result, "render should composite overlay onto background")
    assert.Contains(t, result, "OVERLAY")
}

func TestManager_Current(t *testing.T) {
    mgr := NewManager()
    assert.Nil(t, mgr.Current())

    s := &stubOverlay{rendered: "x"}
    mgr.Show(s)
    assert.Equal(t, s, mgr.Current())
}

func TestManager_Dismiss(t *testing.T) {
    mgr := NewManager()
    mgr.Show(&stubOverlay{rendered: "x"})
    require.True(t, mgr.IsActive())

    mgr.Dismiss()
    assert.False(t, mgr.IsActive())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run TestManager -v
```

expected: FAIL — `NewManager`, `Manager` undefined

**Step 3: write minimal implementation**

Implement `Manager` in `ui/overlay/manager.go`. The manager holds one active overlay, delegates key handling, and composites via `PlaceOverlay`:

```go
package overlay

import tea "github.com/charmbracelet/bubbletea"

// Manager manages the active modal overlay and composites it onto the background.
type Manager struct {
    active   Overlay
    centered bool // whether to center the overlay (true for modals)
    shadow   bool // whether to show shadow/fade effect
    w, h     int  // viewport dimensions
}

// NewManager creates an overlay manager with no active overlay.
func NewManager() *Manager {
    return &Manager{centered: true, shadow: true}
}

// Show activates an overlay. Any previously active overlay is replaced.
func (m *Manager) Show(o Overlay) {
    m.active = o
    if m.w > 0 || m.h > 0 {
        o.SetSize(m.w, m.h)
    }
}

// ShowAt activates an overlay at a specific position (for context menus).
func (m *Manager) ShowAt(o Overlay, centered, shadow bool) {
    m.active = o
    m.centered = centered
    m.shadow = shadow
    if m.w > 0 || m.h > 0 {
        o.SetSize(m.w, m.h)
    }
}

// Dismiss closes the active overlay without returning a result.
func (m *Manager) Dismiss() {
    m.active = nil
    m.centered = true
    m.shadow = true
}

// IsActive returns true if a modal overlay is currently displayed.
func (m *Manager) IsActive() bool {
    return m.active != nil
}

// Current returns the active overlay, or nil.
func (m *Manager) Current() Overlay {
    return m.active
}

// HandleKey delegates to the active overlay. Returns a zero Result if inactive.
func (m *Manager) HandleKey(msg tea.KeyMsg) Result {
    if m.active == nil {
        return Result{}
    }
    result := m.active.HandleKey(msg)
    if result.Dismissed {
        m.active = nil
        m.centered = true
        m.shadow = true
    }
    return result
}

// SetSize updates the viewport dimensions and propagates to the active overlay.
func (m *Manager) SetSize(w, h int) {
    m.w = w
    m.h = h
    if m.active != nil {
        m.active.SetSize(w, h)
    }
}

// Render composites the active overlay onto the background string.
// Returns the background unchanged if no overlay is active.
func (m *Manager) Render(bg string) string {
    if m.active == nil {
        return bg
    }
    return PlaceOverlay(0, 0, m.active.View(), bg, m.shadow, m.centered)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run TestManager -v
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/manager.go ui/overlay/manager_test.go
git commit -m "feat(overlay): implement OverlayManager with compositing"
```

### Task 3: Rewrite Shared Theme and Base Styles

**Files:**
- Modify: `ui/overlay/theme.go`
- Test: `ui/overlay/theme_test.go`

This task consolidates the duplicated style definitions scattered across overlay files (every file defines its own border, title, hint, item, selected styles) into a shared `Styles` struct in `theme.go`. The concrete overlay rewrites in Wave 2 will use these shared styles.

**Step 1: write the failing test**

```go
// theme_test.go
package overlay

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestStyles_ModalBorder(t *testing.T) {
    s := DefaultStyles()
    // Modal border should use the iris color
    rendered := s.ModalBorder.Render("content")
    assert.NotEmpty(t, rendered)
}

func TestStyles_FloatingBorder(t *testing.T) {
    s := DefaultStyles()
    rendered := s.FloatingBorder.Render("content")
    assert.NotEmpty(t, rendered)
}

func TestStyles_Title(t *testing.T) {
    s := DefaultStyles()
    rendered := s.Title.Render("my title")
    assert.Contains(t, rendered, "my title")
}

func TestStyles_Hint(t *testing.T) {
    s := DefaultStyles()
    rendered := s.Hint.Render("press esc")
    assert.Contains(t, rendered, "press esc")
}

func TestStyles_SelectedItem(t *testing.T) {
    s := DefaultStyles()
    rendered := s.SelectedItem.Render("item")
    assert.Contains(t, rendered, "item")
}

func TestStyles_Item(t *testing.T) {
    s := DefaultStyles()
    rendered := s.Item.Render("item")
    assert.Contains(t, rendered, "item")
}

func TestStyles_DisabledItem(t *testing.T) {
    s := DefaultStyles()
    rendered := s.DisabledItem.Render("disabled")
    assert.Contains(t, rendered, "disabled")
}

func TestStyles_SearchBar(t *testing.T) {
    s := DefaultStyles()
    rendered := s.SearchBar.Render("query")
    assert.Contains(t, rendered, "query")
}

func TestStyles_WarningBorder(t *testing.T) {
    s := DefaultStyles()
    rendered := s.WarningBorder.Render("warning")
    assert.NotEmpty(t, rendered)
}

func TestStyles_DangerBorder(t *testing.T) {
    s := DefaultStyles()
    rendered := s.DangerBorder.Render("danger")
    assert.NotEmpty(t, rendered)
}

func TestThemeRosePine_NotNil(t *testing.T) {
    theme := ThemeRosePine()
    assert.NotNil(t, theme)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run TestStyles -v
```

expected: FAIL — `DefaultStyles`, `Styles` undefined

**Step 3: write minimal implementation**

Add the `Styles` struct and `DefaultStyles()` constructor to `theme.go`, keeping the existing `ThemeRosePine()` and color constants intact:

```go
// Styles holds the shared lipgloss styles used by all overlay types.
type Styles struct {
    // Border styles for different overlay contexts
    ModalBorder   lipgloss.Style // centered modals (double border, iris)
    FloatingBorder lipgloss.Style // floating overlays like context menus (rounded, iris)
    WarningBorder lipgloss.Style // warning modals (rounded, gold)
    DangerBorder  lipgloss.Style // danger modals (double border, love/red)

    // Text styles
    Title        lipgloss.Style // overlay title (bold, iris)
    Hint         lipgloss.Style // hint/help text at bottom (muted)
    Muted        lipgloss.Style // secondary text (muted foreground)

    // List item styles (picker, context menu, browser)
    Item         lipgloss.Style // normal list item
    SelectedItem lipgloss.Style // highlighted/selected item
    DisabledItem lipgloss.Style // disabled/greyed-out item
    NumberPrefix lipgloss.Style // numbered shortcut prefix

    // Search bar
    SearchBar    lipgloss.Style // search input container

    // Button styles
    Button       lipgloss.Style // unfocused button
    FocusedButton lipgloss.Style // focused/active button
}

// DefaultStyles returns the standard overlay style set using the Rosé Pine Moon palette.
func DefaultStyles() Styles {
    return Styles{
        ModalBorder: lipgloss.NewStyle().
            Border(lipgloss.DoubleBorder()).
            BorderForeground(colorIris).
            Padding(1, 2),
        FloatingBorder: lipgloss.NewStyle().
            Border(lipgloss.RoundedBorder()).
            BorderForeground(colorIris).
            Padding(1, 2),
        WarningBorder: lipgloss.NewStyle().
            Border(lipgloss.RoundedBorder()).
            BorderForeground(colorGold).
            Padding(1, 2),
        DangerBorder: lipgloss.NewStyle().
            Border(lipgloss.DoubleBorder()).
            BorderForeground(colorLove).
            Padding(1, 2),
        Title: lipgloss.NewStyle().
            Foreground(colorIris).
            Bold(true).
            MarginBottom(1),
        Hint: lipgloss.NewStyle().
            Foreground(colorMuted).
            MarginTop(1),
        Muted: lipgloss.NewStyle().
            Foreground(colorMuted),
        Item: lipgloss.NewStyle().
            Padding(0, 1).
            Foreground(colorText),
        SelectedItem: lipgloss.NewStyle().
            Padding(0, 1).
            Background(colorFoam).
            Foreground(colorBase),
        DisabledItem: lipgloss.NewStyle().
            Padding(0, 1).
            Foreground(colorOverlay),
        NumberPrefix: lipgloss.NewStyle().
            Foreground(colorIris),
        SearchBar: lipgloss.NewStyle().
            Border(lipgloss.RoundedBorder()).
            BorderForeground(colorFoam).
            Padding(0, 1).
            MarginBottom(1),
        Button: lipgloss.NewStyle().
            Foreground(colorSubtle),
        FocusedButton: lipgloss.NewStyle().
            Background(colorIris).
            Foreground(colorBase),
    }
}
```

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run TestStyles -v
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/theme.go ui/overlay/theme_test.go
git commit -m "feat(overlay): add shared Styles struct consolidating duplicated style definitions"
```

## Wave 2: Rewrite Concrete Overlay Types

> **depends on wave 1:** all concrete overlays implement the `Overlay` interface defined in Task 1 and use the shared `Styles` from Task 3.

Each overlay is rewritten to implement the `Overlay` interface. The old API methods are preserved as thin wrappers so the app layer continues to compile during the transition (Wave 3 removes the wrappers). Existing tests are updated to use the new `HandleKey` → `Result` pattern.

### Task 4: Rewrite ConfirmationOverlay, TextOverlay, and TextInputOverlay

**Files:**
- Modify: `ui/overlay/confirmationOverlay.go`
- Modify: `ui/overlay/textOverlay.go`
- Modify: `ui/overlay/textInput.go`
- Modify: `ui/overlay/textInput_test.go`
- Create: `ui/overlay/confirmationOverlay_test.go`
- Create: `ui/overlay/textOverlay_test.go`

These three are the simplest overlays — they don't have search/filter logic. Rewriting them together keeps the task cohesive.

**Step 1: write the failing test**

```go
// confirmationOverlay_test.go
package overlay

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
)

func TestConfirmationOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewConfirmationOverlay("are you sure?")
}

func TestConfirmationOverlay_HandleKey_Confirm(t *testing.T) {
    c := NewConfirmationOverlay("delete?")
    result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "y", result.Action)
}

func TestConfirmationOverlay_HandleKey_Cancel(t *testing.T) {
    c := NewConfirmationOverlay("delete?")
    result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
    assert.Equal(t, "n", result.Action)
}

func TestConfirmationOverlay_HandleKey_Esc(t *testing.T) {
    c := NewConfirmationOverlay("delete?")
    result := c.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}

func TestConfirmationOverlay_HandleKey_CustomKeys(t *testing.T) {
    c := NewConfirmationOverlay("retry?")
    c.ConfirmKey = "r"
    c.CancelKey = "n"
    result := c.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "r", result.Action)
}

func TestConfirmationOverlay_View(t *testing.T) {
    c := NewConfirmationOverlay("are you sure?")
    c.SetSize(60, 20)
    view := c.View()
    assert.Contains(t, view, "are you sure?")
}

// textOverlay_test.go
func TestTextOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewTextOverlay("content")
}

func TestTextOverlay_HandleKey_AnyKeyDismisses(t *testing.T) {
    o := NewTextOverlay("help text")
    result := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
    assert.True(t, result.Dismissed)
}

func TestTextOverlay_View(t *testing.T) {
    o := NewTextOverlay("help content here")
    o.SetSize(60, 20)
    view := o.View()
    assert.Contains(t, view, "help content here")
}
```

Update `textInput_test.go` to test the new `HandleKey` → `Result` API alongside the existing tests:

```go
func TestTextInputOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewTextInputOverlay("title", "")
}

func TestTextInputOverlay_HandleKey_Submit(t *testing.T) {
    ti := NewTextInputOverlay("title", "hello")
    result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "hello", result.Value)
}

func TestTextInputOverlay_HandleKey_Cancel(t *testing.T) {
    ti := NewTextInputOverlay("title", "")
    result := ti.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run "TestConfirmationOverlay_ImplementsOverlay|TestTextOverlay_ImplementsOverlay|TestTextInputOverlay_ImplementsOverlay" -v
```

expected: FAIL — types don't implement `Overlay` interface (missing `HandleKey` method returning `Result`)

**Step 3: write minimal implementation**

Rewrite each overlay to implement the `Overlay` interface. Keep the old `HandleKeyPress` methods as deprecated wrappers that call `HandleKey` internally, so the app layer doesn't break during the transition.

For `ConfirmationOverlay`: add `HandleKey(tea.KeyMsg) Result` and `View() string` (rename existing `Render` to `View`), add `SetSize(w, h int)`. The old `HandleKeyPress` calls `HandleKey` and returns `result.Dismissed`.

For `TextOverlay`: add `HandleKey` returning `Result{Dismissed: true}` on any key, rename `Render` to `View`, add `SetSize`.

For `TextInputOverlay`: add `HandleKey` that returns `Result{Value: textarea.Value(), Submitted: true}` on submit, keep old `HandleKeyPress` as wrapper.

All three use `DefaultStyles()` for their border/title/hint rendering instead of inline `lipgloss.NewStyle()` calls.

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run "TestConfirmation|TestTextOverlay|TestTextInput" -v
```

expected: PASS (both old and new tests)

**Step 5: commit**

```bash
git add ui/overlay/confirmationOverlay.go ui/overlay/confirmationOverlay_test.go ui/overlay/textOverlay.go ui/overlay/textOverlay_test.go ui/overlay/textInput.go ui/overlay/textInput_test.go
git commit -m "feat(overlay): rewrite confirmation, text, and text-input overlays to implement Overlay interface"
```

### Task 5: Rewrite PickerOverlay and ContextMenu

**Files:**
- Modify: `ui/overlay/pickerOverlay.go`
- Modify: `ui/overlay/contextMenu.go`
- Create: `ui/overlay/pickerOverlay_test.go`
- Create: `ui/overlay/contextMenu_test.go`

These two share search/filter logic. Both are rewritten to implement `Overlay` and use shared `Styles`.

**Step 1: write the failing test**

```go
// pickerOverlay_test.go
package overlay

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
)

func TestPickerOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewPickerOverlay("pick one", []string{"a", "b", "c"})
}

func TestPickerOverlay_HandleKey_Submit(t *testing.T) {
    p := NewPickerOverlay("pick", []string{"alpha", "beta"})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "alpha", result.Value)
}

func TestPickerOverlay_HandleKey_Navigate(t *testing.T) {
    p := NewPickerOverlay("pick", []string{"alpha", "beta"})
    p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "beta", result.Value)
}

func TestPickerOverlay_HandleKey_Filter(t *testing.T) {
    p := NewPickerOverlay("pick", []string{"alpha", "beta", "gamma"})
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "beta", result.Value)
}

func TestPickerOverlay_HandleKey_Cancel(t *testing.T) {
    p := NewPickerOverlay("pick", []string{"alpha"})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}

func TestPickerOverlay_AllowCustom(t *testing.T) {
    p := NewPickerOverlay("pick", []string{"alpha"})
    p.SetAllowCustom(true)
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Submitted)
    assert.Equal(t, "z", result.Value)
}

func TestPickerOverlay_View(t *testing.T) {
    p := NewPickerOverlay("select item", []string{"one", "two"})
    p.SetSize(50, 20)
    view := p.View()
    assert.Contains(t, view, "select item")
    assert.Contains(t, view, "one")
    assert.Contains(t, view, "two")
}

// contextMenu_test.go
func TestContextMenu_ImplementsOverlay(t *testing.T) {
    items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
    var _ Overlay = NewContextMenu(0, 0, items)
}

func TestContextMenu_HandleKey_Select(t *testing.T) {
    items := []ContextMenuItem{
        {Label: "kill", Action: "kill"},
        {Label: "rename", Action: "rename"},
    }
    cm := NewContextMenu(0, 0, items)
    result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.Equal(t, "kill", result.Action)
}

func TestContextMenu_HandleKey_Navigate(t *testing.T) {
    items := []ContextMenuItem{
        {Label: "kill", Action: "kill"},
        {Label: "rename", Action: "rename"},
    }
    cm := NewContextMenu(0, 0, items)
    cm.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
    result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_NumberShortcut(t *testing.T) {
    items := []ContextMenuItem{
        {Label: "kill", Action: "kill"},
        {Label: "rename", Action: "rename"},
    }
    cm := NewContextMenu(0, 0, items)
    result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
    assert.True(t, result.Dismissed)
    assert.Equal(t, "rename", result.Action)
}

func TestContextMenu_HandleKey_Dismiss(t *testing.T) {
    items := []ContextMenuItem{{Label: "kill", Action: "kill"}}
    cm := NewContextMenu(0, 0, items)
    result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.Empty(t, result.Action)
}

func TestContextMenu_HandleKey_DisabledSkipped(t *testing.T) {
    items := []ContextMenuItem{
        {Label: "disabled", Action: "disabled", Disabled: true},
        {Label: "enabled", Action: "enabled"},
    }
    cm := NewContextMenu(0, 0, items)
    result := cm.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "enabled", result.Action)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run "TestPickerOverlay_ImplementsOverlay|TestContextMenu_ImplementsOverlay" -v
```

expected: FAIL — types don't implement `Overlay` (missing `HandleKey` returning `Result`)

**Step 3: write minimal implementation**

Rewrite `PickerOverlay` and `ContextMenu` to implement `Overlay`:

- `PickerOverlay.HandleKey` returns `Result{Submitted: true, Value: selectedItem}` on enter, `Result{Dismissed: true}` on esc. The old `HandleKeyPress(tea.KeyMsg) bool` becomes a wrapper.
- `ContextMenu.HandleKey` returns `Result{Dismissed: true, Action: selectedAction}` on selection, `Result{Dismissed: true}` on esc. The old `HandleKeyPress(tea.KeyMsg) (string, bool)` becomes a wrapper.
- Both use `DefaultStyles()` for rendering instead of package-level `var` styles.
- `ContextMenu.View()` replaces `Render()`. `PickerOverlay.View()` replaces `Render()`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run "TestPickerOverlay|TestContextMenu" -v
```

expected: PASS (both old and new tests)

**Step 5: commit**

```bash
git add ui/overlay/pickerOverlay.go ui/overlay/pickerOverlay_test.go ui/overlay/contextMenu.go ui/overlay/contextMenu_test.go
git commit -m "feat(overlay): rewrite picker and context menu to implement Overlay interface"
```

### Task 6: Rewrite FormOverlay, PermissionOverlay, and TmuxBrowserOverlay

**Files:**
- Modify: `ui/overlay/formOverlay.go`
- Modify: `ui/overlay/formOverlay_test.go`
- Modify: `ui/overlay/permissionOverlay.go`
- Create: `ui/overlay/permissionOverlay_test.go`
- Modify: `ui/overlay/tmuxBrowserOverlay.go`
- Modify: `ui/overlay/tmuxBrowserOverlay_test.go`

These are the more complex overlays. Each is rewritten to implement `Overlay` with domain-specific result values carried in `Result.Value` and `Result.Action`.

**Step 1: write the failing test**

```go
// permissionOverlay_test.go
package overlay

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
)

func TestPermissionOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewPermissionOverlay("instance", "desc", "pattern")
}

func TestPermissionOverlay_HandleKey_Confirm(t *testing.T) {
    p := NewPermissionOverlay("inst", "run command", "*.sh")
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "allow_once", result.Action)
}

func TestPermissionOverlay_HandleKey_Navigate(t *testing.T) {
    p := NewPermissionOverlay("inst", "run command", "*.sh")
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "allow_always", result.Action)
}

func TestPermissionOverlay_HandleKey_Reject(t *testing.T) {
    p := NewPermissionOverlay("inst", "run command", "*.sh")
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.Equal(t, "reject", result.Action)
}

func TestPermissionOverlay_HandleKey_Dismiss(t *testing.T) {
    p := NewPermissionOverlay("inst", "run command", "*.sh")
    result := p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}

func TestPermissionOverlay_View(t *testing.T) {
    p := NewPermissionOverlay("inst", "run command", "*.sh")
    p.SetSize(60, 20)
    view := p.View()
    assert.Contains(t, view, "permission required")
    assert.Contains(t, view, "run command")
}
```

Add to `formOverlay_test.go`:

```go
func TestFormOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewFormOverlay("title", 60)
}

func TestFormOverlay_HandleKey_Submit(t *testing.T) {
    f := NewFormOverlay("new plan", 60)
    for _, r := range "test-name" {
        f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
    }
    result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.True(t, result.Submitted)
    assert.Equal(t, "test-name", result.Value)
}

func TestFormOverlay_HandleKey_Cancel(t *testing.T) {
    f := NewFormOverlay("new plan", 60)
    result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.False(t, result.Submitted)
}
```

Add to `tmuxBrowserOverlay_test.go`:

```go
func TestTmuxBrowserOverlay_ImplementsOverlay(t *testing.T) {
    var _ Overlay = NewTmuxBrowserOverlay(nil)
}

func TestTmuxBrowserOverlay_HandleKey_Dismiss(t *testing.T) {
    b := NewTmuxBrowserOverlay(nil)
    result := b.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
    assert.True(t, result.Dismissed)
    assert.Empty(t, result.Action)
}

func TestTmuxBrowserOverlay_HandleKey_Attach(t *testing.T) {
    items := []TmuxBrowserItem{{Name: "sess", Title: "my-session"}}
    b := NewTmuxBrowserOverlay(items)
    result := b.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
    assert.True(t, result.Dismissed)
    assert.Equal(t, "attach", result.Action)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/overlay/... -run "TestPermissionOverlay_ImplementsOverlay|TestFormOverlay_ImplementsOverlay|TestTmuxBrowserOverlay_ImplementsOverlay" -v
```

expected: FAIL — types don't implement `Overlay`

**Step 3: write minimal implementation**

Rewrite each overlay to implement `Overlay`:

- `FormOverlay.HandleKey` returns `Result{Submitted: true, Value: name}` on submit. The `Description()`, `Branch()`, `WorkPath()` methods remain as accessors for the app layer to read after submit.
- `PermissionOverlay.HandleKey` returns `Result{Submitted: true, Action: "allow_once"|"allow_always"|"reject"}` on confirm. The `Choice()`, `Pattern()`, `Description()` methods remain as accessors.
- `TmuxBrowserOverlay.HandleKey` returns `Result{Dismissed: true, Action: "attach"|"kill"|"adopt"|"dismiss"}`. The `SelectedItem()`, `RemoveSelected()`, `IsEmpty()` methods remain as accessors.
- All three rename `Render()` to `View()` and use `DefaultStyles()`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run "TestPermission|TestFormOverlay|TestTmuxBrowser" -v
```

expected: PASS (both old and new tests)

**Step 5: commit**

```bash
git add ui/overlay/formOverlay.go ui/overlay/formOverlay_test.go ui/overlay/permissionOverlay.go ui/overlay/permissionOverlay_test.go ui/overlay/tmuxBrowserOverlay.go ui/overlay/tmuxBrowserOverlay_test.go
git commit -m "feat(overlay): rewrite form, permission, and tmux browser overlays to implement Overlay interface"
```

## Wave 3: App-Layer Integration

> **depends on wave 2:** all overlay types now implement the `Overlay` interface. This wave replaces the 8 pointer fields and state switches in the app layer with the `OverlayManager`.

### Task 7: Integrate OverlayManager into App Model and Remove Deprecated Wrappers

**Files:**
- Modify: `app/app.go` (replace 8 overlay fields with `OverlayManager`, update `View()`)
- Modify: `app/app_input.go` (replace per-overlay state handling with `OverlayManager.HandleKey`)
- Modify: `app/app_state.go` (update `confirmAction`, `waveStandardConfirmAction`, etc.)
- Modify: `app/app_actions.go` (update overlay creation calls)
- Modify: `ui/overlay/confirmationOverlay.go` (remove deprecated wrappers)
- Modify: `ui/overlay/textOverlay.go` (remove deprecated wrappers)
- Modify: `ui/overlay/textInput.go` (remove deprecated wrappers)
- Modify: `ui/overlay/pickerOverlay.go` (remove deprecated wrappers)
- Modify: `ui/overlay/contextMenu.go` (remove deprecated wrappers)
- Modify: `ui/overlay/formOverlay.go` (remove deprecated wrappers)
- Modify: `ui/overlay/permissionOverlay.go` (remove deprecated wrappers)
- Modify: `ui/overlay/tmuxBrowserOverlay.go` (remove deprecated wrappers)

This is the largest task — it rewires the app layer to use the manager. The key changes:

1. Replace 8 overlay pointer fields (`textInputOverlay`, `formOverlay`, `textOverlay`, `confirmationOverlay`, `contextMenu`, `pickerOverlay`, `tmuxBrowser`, `permissionOverlay`) with one `overlays *overlay.Manager` field.
2. Collapse the 20-case render switch in `View()` to `result = m.overlays.Render(mainView)`.
3. Collapse the per-overlay `handleKeyPress` blocks to a single `if m.overlays.IsActive()` guard that dispatches `m.overlays.HandleKey(msg)` and then routes the `Result` to the appropriate action handler based on `m.state`.
4. Keep `m.state` for now — it still tracks which *kind* of overlay is active so the result handler knows what to do with `Result.Value`/`Result.Action`. A future cleanup can replace the state enum with overlay type assertions, but that's out of scope.

**Step 1: write the failing test**

The app layer tests in `app/app_test.go` and the various `app/*_test.go` files already exercise overlay flows end-to-end. The failing test here is a compilation check — after modifying the struct fields, existing tests must still compile and pass.

```bash
go build ./app/...
```

expected: FAIL — removed fields cause compilation errors

**Step 2: run test to verify it fails**

```bash
go test ./app/... -count=1 -v 2>&1 | head -50
```

expected: FAIL — compilation errors from removed overlay fields

**Step 3: write minimal implementation**

Systematic changes:

1. In `app/app.go` `home` struct: remove the 8 overlay pointer fields, add `overlays *overlay.Manager`. In `Init()`, initialize `m.overlays = overlay.NewManager()`.

2. In `app/app.go` `View()`: replace the 20-case switch with:
   ```go
   if m.overlays.IsActive() {
       result = m.overlays.Render(mainView)
   } else {
       result = mainView
   }
   ```
   Toast rendering remains separate (toasts are non-modal, managed by `ToastManager`).

3. In `app/app_input.go`: replace each `if m.state == stateXxx { ... m.xxxOverlay.HandleKeyPress(msg) ... }` block with a unified handler:
   ```go
   if m.overlays.IsActive() {
       result := m.overlays.HandleKey(msg)
       if result.Dismissed {
           return m.handleOverlayResult(result)
       }
       return m, nil
   }
   ```
   The `handleOverlayResult` method uses `m.state` to determine what to do with the result (create plan, send prompt, execute context action, etc.).

4. In `app/app_state.go` and `app/app_actions.go`: replace `m.textInputOverlay = overlay.NewTextInputOverlay(...)` with `m.overlays.Show(overlay.NewTextInputOverlay(...))`, and similarly for all other overlay creation sites.

5. For overlays that need post-dismiss data access (form fields, permission choice, browser selection), the `handleOverlayResult` method uses `m.overlays.Current()` type assertion *before* the dismiss clears it, or the `Result.Value`/`Result.Action` fields carry the needed data.

6. Once the app layer uses `HandleKey` → `Result` exclusively, remove the deprecated wrappers from each overlay file:
   - Remove old `HandleKeyPress` methods (replaced by `HandleKey`)
   - Remove old `Render` methods (replaced by `View`)
   - Remove package-level `var` style declarations (`pickerBorderStyle`, `contextMenuStyle`, `browserBorderStyle`, etc.) — now served by `DefaultStyles()`
   - Remove exported `Dismissed` / `submitted` / `canceled` fields that are no longer read by the app layer (the `Result` type carries this information)
   - Update existing tests that reference removed methods to use the new API

**Step 4: run test to verify it passes**

```bash
go test ./app/... -count=1 -v
go test ./ui/overlay/... -count=1 -v
go test ./... -count=1 2>&1 | tail -20
```

expected: PASS — all existing app and overlay tests pass with the new manager, no references to removed methods remain

**Step 5: commit**

```bash
git add app/app.go app/app_input.go app/app_state.go app/app_actions.go ui/overlay/
git commit -m "feat(overlay): integrate OverlayManager into app model, remove deprecated wrappers"
```


