# Bubbles Component Guide

Selection matrix, deep dives on key components, and integration patterns for the Charm ecosystem. Load this file when choosing and configuring specific components for a bubbletea application.

---

## 1. Component Selection Matrix

| Need | Component | Key Notes |
|------|-----------|-----------|
| Navigable list, uniform items | `bubbles/list` | Built-in filtering, custom delegates, status messages |
| Navigable table, columnar data | `bubbles/table` | Fixed-width columns, keyboard nav, selection |
| Scrollable text content | `bubbles/viewport` | Output display, help overlays, log viewers |
| Single-line text input | `bubbles/textinput` | Placeholder, validation, masking, suggestions |
| Multi-line text input | `bubbles/textarea` | Prompt editor, notes, large text entry |
| Multi-field form | `charmbracelet/huh` | Far superior to hand-rolling with textinput |
| Unknown duration indicator | `bubbles/spinner` | 12 styles -- choose based on context |
| Known percentage progress | `bubbles/progress` | Only with real data, never fake |
| Page navigation | `bubbles/paginator` | Dot or Arabic numeral style |
| Timed operation display | `bubbles/timer` / `stopwatch` | Worker duration tracking |
| Key binding help | `bubbles/help` | Short/long toggle, grouped bindings |
| Markdown rendering | `charmbracelet/glamour` | README display, formatted help docs |
| Static styled table | `lipgloss/table` | Render-only, no interactivity, `StyleFunc` |

---

## 2. `bubbles/list` Deep Dive

### Custom Item Delegates

The `DefaultDelegate` works for basic lists, but `ItemDelegate` gives full rendering control per row. This is how polished TUIs like lazydocker achieve their appearance -- every character is intentional.

```go
type workerDelegate struct{}

func (d workerDelegate) Height() int                               { return 1 }
func (d workerDelegate) Spacing() int                              { return 0 }
func (d workerDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d workerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
    worker := item.(Worker)

    icon := stateIndicator(worker.State) // ⠋ ✓ ✗ ⊘ ○
    role := roleStyle.Render(worker.Role)
    dur := dimStyle.Render(worker.Duration())

    row := fmt.Sprintf("%s %s %s", icon, role, dur)

    if index == m.Index() {
        row = selectedRowStyle.Width(m.Width()).Render(row)
    }
    fmt.Fprint(w, row)
}
```

### Filtering

Enable fuzzy filtering on any list:

```go
l := list.New(items, workerDelegate{}, width, height)
l.SetFilteringEnabled(true)
l.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(palette.Primary)
```

Each item must implement `FilterValue() string` -- return the text that filtering matches against (typically the item's display name).

### Status Messages

Provide ephemeral feedback without a dialog:

```go
m.list.NewStatusMessage(statusStyle.Render("Worker spawned"))
```

Messages auto-clear after a few seconds. Use for confirmations, transient errors, and state change notifications.

### Sizing

Always set size in the `WindowSizeMsg` handler, accounting for surrounding chrome:

```go
case tea.WindowSizeMsg:
    h, v := docStyle.GetFrameSize()
    m.list.SetSize(msg.Width-h, msg.Height-v)
```

### Title and Style Customization

```go
l.Title = "WORKERS"
l.Styles.Title = lipgloss.NewStyle().Foreground(palette.Primary).Bold(true).Padding(0, 1)
l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(palette.Primary)
l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(palette.Primary)
```

### Empty State

Disable the default empty state and render a custom one:

```go
l.SetShowStatusBar(false)
l.SetShowHelp(false)

// In View(), check if list is empty:
if len(m.list.Items()) == 0 {
    return renderEmptyState(width, height)
}
return m.list.View()
```

---

## 3. `bubbles/viewport` for Streaming Output

### The Ready Pattern

The viewport requires knowing its dimensions before rendering correctly. Use a `ready` flag:

```go
case tea.WindowSizeMsg:
    if !m.ready {
        m.viewport = viewport.New(msg.Width-leftPanelW, msg.Height-headerH-statusH)
        m.viewport.SetContent(m.outputBuf[m.selectedWorker])
        m.ready = true
    } else {
        m.viewport.Width = msg.Width - leftPanelW
        m.viewport.Height = msg.Height - headerH - statusH
    }
```

### Auto-Scroll for Streaming Content

Scroll to bottom while the user is at bottom. If the user scrolls up (viewing history), stop auto-scrolling. Resume when the user returns to bottom:

```go
case WorkerOutputMsg:
    m.outputBuf[msg.WorkerID] += msg.Line + "\n"
    if m.selectedWorker == msg.WorkerID {
        atBottom := m.viewport.AtBottom()
        m.viewport.SetContent(m.outputBuf[msg.WorkerID])
        if atBottom {
            m.viewport.GotoBottom()
        }
    }
```

### Styling

The viewport has no built-in border. Wrap its output with a lipgloss style:

```go
func (m model) renderOutputPanel(width, height int) string {
    header := m.renderPanelHeader("OUTPUT", m.focused == panelOutput)
    content := m.viewport.View()

    // Percentage indicator
    pct := fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)
    footer := dimStyle.Render(pct)

    return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}
```

### Mouse Scroll Support

Enable mouse cell motion tracking on the program for viewport scrolling:

```go
p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

The viewport handles mouse wheel events automatically when this is enabled.

---

## 4. `bubbles/spinner` Style Guide

| Style | Visual | Recommended Use |
|-------|--------|-----------------|
| `Dot` | `⣾⣽⣻⢿⡿⣟⣯⣷` | Dense braille, background activity |
| `Line` | `- \ \| /` | Classic, high-contrast terminals |
| `MiniDot` | `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` | Subtle braille, inline in table rows |
| `Jump` | `⢄⢂⢁⡁⡈⡐⡠` | Playful, not for professional tools |
| `Pulse` | `█▓▒░` | Breathing effect, "waiting" states |
| `Points` | `∙∙∙` | Minimal, ambient status bar indicator |
| `Globe` | `🌍🌎🌏` | Tier 1 terminals only, network operations |
| `Moon` | `🌑🌒🌓...` | Tier 1 terminals only, time-based tasks |
| `Monkey` | `🙈🙉🙊` | Never use in a professional tool |
| `Meter` | `▱▰` | Progress-like without percentage data |
| `Hamburger` | `☱☲☴` | Trigram, stylized aesthetic |
| `Ellipsis` | `.⁣.⁣.` | Simple dot loading, familiar |

**Professional TUI recommendations**: `MiniDot` for inline table cell status, `Points` for status bar ambient "N running" state, `Pulse` for focused "processing" modals.

### Initialization with Custom Styling

```go
s := spinner.New(
    spinner.WithSpinner(spinner.MiniDot),
    spinner.WithStyle(lipgloss.NewStyle().Foreground(palette.Active)),
)
```

### Shared Spinner for Multiple Items

A single spinner instance can serve all running items. Update it once per tick, render its `View()` in each running row:

```go
case spinner.TickMsg:
    var cmd tea.Cmd
    m.spinner, cmd = m.spinner.Update(msg)
    return m, cmd

// In the list delegate Render():
if worker.State == StateRunning {
    icon = runningStyle.Render(m.spinner.View()) // same spinner for all
}
```

---

## 5. `huh` for Multi-Field Dialogs

### Why huh Over Hand-Rolled textinput

`huh` handles tab navigation between fields, validation with error display, placeholder text, accessible mode, theming, and confirmation -- all of which require significant boilerplate to hand-roll with raw `textinput` components.

### Form Composition

```go
var spawnRole string
var spawnPrompt string

form := huh.NewForm(
    huh.NewGroup(
        huh.NewSelect[string]().
            Title("Agent Role").
            Options(
                huh.NewOption("Planner", "planner"),
                huh.NewOption("Coder", "coder"),
                huh.NewOption("Reviewer", "reviewer"),
                huh.NewOption("Release", "release"),
            ).
            Value(&spawnRole),
        huh.NewText().
            Title("Prompt").
            Placeholder("Describe the task...").
            CharLimit(2000).
            Value(&spawnPrompt),
    ),
)
```

### Embedding in Bubbletea

`huh.Form` implements `tea.Model` -- treat it as a sub-model:

```go
type Model struct {
    form *huh.Form
}

func (m Model) Init() tea.Cmd {
    return m.form.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    form, cmd := m.form.Update(msg)
    if f, ok := form.(*huh.Form); ok {
        m.form = f
    }

    if m.form.State == huh.StateCompleted {
        // Extract values, dismiss dialog, spawn worker
        role := m.form.GetString("role")
        prompt := m.form.GetString("prompt")
        return m, m.spawnWorker(role, prompt)
    }

    return m, cmd
}
```

### Theming huh to Match Application Palette

```go
theme := huh.ThemeBase() // start from base
theme.Focused.Title = lipgloss.NewStyle().Foreground(palette.Primary).Bold(true)
theme.Focused.SelectedOption = lipgloss.NewStyle().Foreground(palette.Primary)
theme.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(palette.Primary)
theme.Blurred.Title = lipgloss.NewStyle().Foreground(palette.Muted)

form := huh.NewForm(...).WithTheme(theme)
```

### Confirm Dialogs

```go
var confirmed bool
huh.NewConfirm().
    Title("Kill worker coder-2?").
    Affirmative("Kill").
    Negative("Cancel").
    Value(&confirmed)
```

---

## 6. `bubbles/help` for Keybinding Display

### Key Map Definition

Implement the `help.KeyMap` interface with grouped bindings:

```go
type keyMap struct {
    Up      key.Binding
    Down    key.Binding
    Spawn   key.Binding
    Kill    key.Binding
    Continue key.Binding
    Help    key.Binding
    Quit    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down},                    // Navigation
        {k.Spawn, k.Kill, k.Continue},     // Workers
        {k.Help, k.Quit},                  // System
    }
}

var keys = keyMap{
    Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
    Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
    Spawn:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "spawn")),
    Kill:     key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "kill")),
    Continue: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "continue")),
    Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
    Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

### Styling the Help Component

```go
h := help.New()
h.Styles.ShortKey = lipgloss.NewStyle().Foreground(palette.Primary)
h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(palette.Muted)
h.Styles.FullKey = lipgloss.NewStyle().Foreground(palette.Primary)
h.Styles.FullDesc = lipgloss.NewStyle().Foreground(palette.Muted)
h.Styles.FullSeparator = lipgloss.NewStyle().Foreground(palette.Border)
```

### Toggling Between Short and Full

```go
case tea.KeyMsg:
    if key.Matches(msg, m.keys.Help) {
        m.help.ShowAll = !m.help.ShowAll
    }

// In status bar (short help always visible):
helpView := m.help.View(m.keys)
```

### Disabling Bindings Contextually

Disable bindings that don't apply in the current mode:

```go
// When in output panel, disable worker-specific bindings
m.keys.Spawn.SetEnabled(m.focused == panelWorkers)
m.keys.Kill.SetEnabled(m.focused == panelWorkers)
```

Disabled bindings are automatically hidden from the help view.
