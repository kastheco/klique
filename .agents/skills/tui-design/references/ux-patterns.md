# UX Patterns for Terminal UIs

Keybinding grammar, focus management, destructive action confirmation, empty states, error presentation, help overlays, and responsive layout patterns. Load this file when designing interaction patterns and keybind maps.

---

## 1. Keybinding Design

### The Grammar

Every mode has a clear "go up one level" key. `Esc` always works as "back." Destructive actions use uppercase (capital letter = dangerous).

```
Global (always active):
  q / ctrl+c    Quit (confirm if workers are running)
  ?             Toggle help overlay
  ctrl+z        Suspend to background

Dashboard (normal mode):
  j / ↓         Next item
  k / ↑         Previous item
  enter / l     Focus detail panel
  space         Select item (for batch operations)
  s             Spawn / create new
  K             Kill selected (capital = destructive)
  r             Restart selected
  c             Continue selected completed worker
  g / G         Jump to first / last
  /             Filter
  tab           Cycle panels
  a             AI analysis of selected failed worker

Detail panel (focused):
  j / ↓         Scroll down
  k / ↑         Scroll up
  ctrl+f / b    Page down / up
  G             Jump to bottom
  gg            Jump to top
  esc / h       Return to list
  f             Toggle fullscreen

Task panel (focused):
  j / k         Navigate tasks
  enter         Assign selected task to spawn dialog
  esc           Return to worker list
```

### Terminal Hotkey Collisions

These key combinations are intercepted by terminal emulators before they reach the application. Never bind them:

| Combo | Terminal Action | Interceptor |
|-------|----------------|-------------|
| `Ctrl+W` | Close tab | Most terminals |
| `Ctrl+T` | New tab | Most terminals |
| `Ctrl+Shift+*` | Various | Most terminals |
| `Ctrl+S` | XOFF (freeze output) | Terminal driver |
| `Ctrl+Q` | XON (resume output) | Terminal driver |
| `Ctrl+C` | SIGINT | Terminal driver (usable via bubbletea) |
| `Ctrl+Z` | SIGTSTP (suspend) | Terminal driver |

`Ctrl+C` is special: bubbletea intercepts it as a `KeyMsg` before the terminal sends SIGINT, so it is safe to use. `Ctrl+Z` can be handled similarly but requires explicit handling.

### Multi-Key Sequences

For `gg` (jump to top), implement as a state machine:

```go
type model struct {
    lastKey     string
    lastKeyTime time.Time
}

case tea.KeyMsg:
    now := time.Now()
    if msg.String() == "g" {
        if m.lastKey == "g" && now.Sub(m.lastKeyTime) < 500*time.Millisecond {
            m.lastKey = ""
            // Execute gg action: jump to top
            m.viewport.GotoTop()
            return m, nil
        }
        m.lastKey = "g"
        m.lastKeyTime = now
        return m, nil
    }
    m.lastKey = ""
```

---

## 2. Focus Ring Design

### Option A: Header Color Change (Recommended)

Change the panel header color to indicate focus. Focused = primary, unfocused = muted. Zero layout changes, zero reflow.

```go
func (m model) renderPanelHeader(title string, focused bool) string {
    style := headerStyle.Foreground(palette.Muted)
    if focused {
        style = headerStyle.Foreground(palette.Primary).Bold(true)
    }
    return style.Render(title)
}
```

### Option B: Content Brightness

Unfocused panels get dimmed content. All text in unfocused panels renders through `dimStyle` instead of their normal styles. Creates a strong visual signal but requires threading focus state into every render function.

### Option C: Border Changes (Avoid)

Adding/removing borders on focus changes causes layout reflow -- panels physically jump as borders add/remove character width. This is visually disorienting and breaks spatial memory. Never use this approach.

### Focus Cycling

`Tab` cycles through panels in a fixed order. Track focus as an enum:

```go
type panel int
const (
    panelWorkers panel = iota
    panelOutput
    panelTasks
    panelCount // sentinel for wrapping
)

case tea.KeyMsg:
    if msg.String() == "tab" {
        m.focused = (m.focused + 1) % panelCount
        return m, nil
    }
```

---

## 3. Confirmation for Destructive Actions

### The Status Bar Prompt Pattern

Never use modal dialogs for "are you sure?" -- they interrupt flow and feel heavy. Instead, transform the status bar in-place:

```
Normal:    ● 3 running  ○ 1 pending  │  ? help  q quit
After K:   Kill worker coder-2? K again to confirm, esc cancel
```

The bar text changes to an error/warning color. Press the same key again within 3 seconds to confirm. Auto-revert on timeout. `Esc` cancels immediately.

```go
type confirmAction struct {
    action    string
    targetID  string
    label     string
    expiresAt time.Time
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Check for active confirmation
        if m.pending != nil && time.Now().Before(m.pending.expiresAt) {
            if msg.String() == "K" && m.pending.action == "kill" {
                target := m.pending.targetID
                m.pending = nil
                return m, m.killWorker(target)
            }
            if msg.String() == "escape" {
                m.pending = nil
                return m, nil
            }
        }

        if msg.String() == "K" && m.pending == nil {
            m.pending = &confirmAction{
                action:    "kill",
                targetID:  m.selectedWorkerID(),
                label:     m.selectedWorkerLabel(),
                expiresAt: time.Now().Add(3 * time.Second),
            }
            return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
                return confirmExpiredMsg{}
            })
        }

    case confirmExpiredMsg:
        m.pending = nil
    }
}
```

### Rendering the Confirmation Status Bar

```go
func (m model) renderStatusBar() string {
    if m.pending != nil && time.Now().Before(m.pending.expiresAt) {
        prompt := fmt.Sprintf("Kill worker %s? K again to confirm, esc cancel", m.pending.label)
        return errorStyle.Width(m.width).Render(prompt)
    }
    // Normal status bar...
}
```

---

## 4. Empty States

Empty states are design opportunities, not edge cases. An empty worker list should never display a blank table.

```go
func (m model) renderEmptyState(width, height int) string {
    title := dimStyle.Render("No workers running")
    hint := lipgloss.JoinVertical(lipgloss.Center,
        "",
        mutedStyle.Render("Press "+accentStyle.Render("s")+" to spawn your first"),
        mutedStyle.Render("worker, or load a task source"),
        mutedStyle.Render("with "+accentStyle.Render("kasmos <spec-dir>")),
    )

    content := lipgloss.JoinVertical(lipgloss.Center, title, hint)
    return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
```

Key principles:
- Center with `lipgloss.Place()`, dim foreground
- Highlight the single most important action with accent color
- Instructional without being condescending
- Show this instead of the component, not inside it

---

## 5. Error Presentation Tiers

### Tier 1 -- Transient Errors

Failed file reads, network timeouts, recoverable issues. Display in the status bar with error color, auto-clear after 5 seconds.

```go
case errorMsg:
    m.statusError = errorStyle.Render("Error: " + msg.Error())
    return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
        return clearStatusMsg{}
    })

case clearStatusMsg:
    m.statusError = ""
```

### Tier 2 -- Item Failures

Worker exit code != 0, task failures. Reflected in the item's state indicator (red `✗` in the table row). Full error details appear in the detail viewport when the item is selected. Apply error foreground to the status column only -- not the entire row. Full-row red treatment is too aggressive for a state that may be common.

### Tier 3 -- Fatal Errors

Daemon crash, cannot spawn any processes, missing binary. Full-screen error panel centered with `lipgloss.Place()`:

```go
func renderFatalError(width, height int, err error) string {
    title := errorStyle.Bold(true).Render("Fatal Error")
    msg := textStyle.Render(err.Error())
    hint := dimStyle.Render("Press q to exit, r to retry")

    content := lipgloss.JoinVertical(lipgloss.Center, "", title, "", msg, "", hint, "")

    box := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(palette.Error).
        Padding(1, 3).
        Width(min(60, width-4)).
        Render(content)

    return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
```

---

## 6. Help Overlay Design

### Short Mode (Status Bar)

Always visible, 3-4 critical bindings only:

```go
func (k keyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Help, k.Quit}
}
```

### Full Mode (Overlay)

Press `?` to toggle. Render as a centered overlay with a distinct background:

```go
func (m model) renderHelpOverlay() string {
    content := m.help.View(m.keys)

    box := lipgloss.NewStyle().
        Padding(2, 4).
        Border(lipgloss.RoundedBorder()).
        BorderForeground(palette.Border).
        Background(palette.Surface).
        Width(min(70, m.width-10)).
        Render(content)

    return lipgloss.Place(
        m.width, m.height,
        lipgloss.Center, lipgloss.Center,
        box,
    )
}
```

### Logical Grouping

Organize bindings into semantic groups in `FullHelp()`:

```go
func (k keyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.Tab, k.Filter},         // Navigation
        {k.Spawn, k.Kill, k.Continue, k.Restart}, // Workers
        {k.ScrollDown, k.ScrollUp, k.Fullscreen}, // Output
        {k.Help, k.Quit},                         // System
    }
}
```

The help component renders each inner slice as a column with the bindings listed vertically. The result is a clean multi-column layout:

```
  Navigation      Workers         Output          System
  ↑/k  up         s  spawn        j/k  scroll     ?  help
  ↓/j  down       K  kill         G    bottom      q  quit
  tab  switch      c  continue    f    fullscreen
  /    filter      r  restart
```

---

## 7. Responsive Layout Implementation

### Breakpoint System

```go
type layoutMode int
const (
    layoutNarrow   layoutMode = iota // <100 cols: single column, tab switching
    layoutStandard                   // 100-140: master-detail 40/60
    layoutWide                       // >140: three-column 20/35/45
)
```

### The updateLayout Method

Recompute all derived dimensions when the terminal resizes:

```go
func (m *model) updateLayout() {
    const headerH = 1
    const statusH = 1
    contentH := m.height - headerH - statusH

    switch {
    case m.width < 100:
        m.layout = layoutNarrow
        m.table.SetWidth(m.width)
        m.table.SetHeight(contentH)
        m.viewport.Width = m.width
        m.viewport.Height = contentH

    case m.width < 140:
        m.layout = layoutStandard
        leftW := m.width * 40 / 100
        rightW := m.width - leftW // never add columns; subtract from total
        m.table.SetWidth(leftW)
        m.table.SetHeight(contentH)
        m.viewport.Width = rightW
        m.viewport.Height = contentH

    default:
        m.layout = layoutWide
        col1 := m.width * 20 / 100
        col2 := m.width * 35 / 100
        col3 := m.width - col1 - col2 // remainder absorbs rounding error
        m.taskList.SetSize(col1, contentH)
        m.table.SetWidth(col2)
        m.table.SetHeight(contentH)
        m.viewport.Width = col3
        m.viewport.Height = contentH
    }
}
```

**Critical detail**: always compute the last column as `total - sum(others)`. Integer division truncates, so adding computed column widths may not equal the total. The last column absorbs the remainder.

### Minimum Dimension Protection

```go
case tea.WindowSizeMsg:
    m.width = max(msg.Width, 80)
    m.height = max(msg.Height, 24)
    m.updateLayout()
```

### Tab-Based View Switching for Narrow Mode

When the terminal is too narrow for side-by-side panels, switch to tabbed navigation:

```go
type tab int
const (
    tabWorkers tab = iota
    tabOutput
    tabTasks
)

func (m model) View() string {
    if m.layout == layoutNarrow {
        tabs := m.renderTabBar()
        var content string
        switch m.activeTab {
        case tabWorkers:
            content = m.table.View()
        case tabOutput:
            content = m.viewport.View()
        case tabTasks:
            content = m.taskList.View()
        }
        return lipgloss.JoinVertical(lipgloss.Left,
            m.renderHeader(),
            tabs,
            content,
            m.renderStatusBar(),
        )
    }
    // standard/wide layout with JoinHorizontal...
}
```

### Tab Bar Rendering

```go
func (m model) renderTabBar() string {
    tabs := []string{"Workers", "Output", "Tasks"}
    var rendered []string

    for i, t := range tabs {
        if tab(i) == m.activeTab {
            rendered = append(rendered, accentStyle.Bold(true).Render(" "+t+" "))
        } else {
            rendered = append(rendered, dimStyle.Render(" "+t+" "))
        }
    }

    bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
    separator := dividerStyle.Render(strings.Repeat("─", m.width))
    return lipgloss.JoinVertical(lipgloss.Left, bar, separator)
}
```
