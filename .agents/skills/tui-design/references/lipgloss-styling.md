# Lipgloss Styling Reference

Style composition patterns, layout recipes, state indicator design, table selection guidance, and border discipline for bubbletea applications. Load this file when writing `View()` functions and styling components.

---

## 1. Style Composition Patterns

### The Package-Level Style System

Define all styles as package-level variables. `lipgloss.NewStyle()` allocates; calling it inside `View()` (which runs up to 60 times per second) creates unnecessary GC pressure. Define once, use everywhere.

Start with a palette struct, then derive all application styles from it:

```go
// palette.go
type Palette struct {
    Base      lipgloss.AdaptiveColor
    Surface   lipgloss.AdaptiveColor
    Border    lipgloss.AdaptiveColor
    Primary   lipgloss.AdaptiveColor
    Muted     lipgloss.AdaptiveColor
    Text      lipgloss.AdaptiveColor
    Success   lipgloss.AdaptiveColor
    Warning   lipgloss.AdaptiveColor
    Error     lipgloss.AdaptiveColor
    Active    lipgloss.AdaptiveColor
    Inactive  lipgloss.AdaptiveColor
    SelectBg  lipgloss.AdaptiveColor
    SelectFg  lipgloss.AdaptiveColor
}

var P = Palette{
    Border:   lipgloss.AdaptiveColor{Light: "254", Dark: "237"},
    Muted:    lipgloss.AdaptiveColor{Light: "245", Dark: "241"},
    Text:     lipgloss.AdaptiveColor{Light: "235", Dark: "252"},
    Primary:  lipgloss.AdaptiveColor{Light: "#1A6BC4", Dark: "#5FB4E0"},
    Success:  lipgloss.AdaptiveColor{Light: "#4E8A3E", Dark: "#A8CC8C"},
    Warning:  lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#DBAB79"},
    Error:    lipgloss.AdaptiveColor{Light: "#C4384B", Dark: "#E88388"},
    Active:   lipgloss.AdaptiveColor{Light: "#2B6CB0", Dark: "#89B4FA"},
    Inactive: lipgloss.AdaptiveColor{Light: "250", Dark: "243"},
    SelectBg: lipgloss.AdaptiveColor{Light: "255", Dark: "237"},
    SelectFg: lipgloss.AdaptiveColor{Light: "232", Dark: "255"},
}
```

### Deriving Styles from Palette

Build a complete style system through composition. Define a base style, then derive semantic variants:

```go
// styles.go
var (
    // Base with standard padding
    base = lipgloss.NewStyle().Padding(0, 1)

    // Text roles
    TextDim     = base.Foreground(P.Muted)
    TextPrimary = base.Foreground(P.Text)
    TextBold    = base.Foreground(P.Text).Bold(true)
    TextAccent  = base.Foreground(P.Primary).Bold(true)
    TextError   = base.Foreground(P.Error)
    TextSuccess = base.Foreground(P.Success)
    TextWarning = base.Foreground(P.Warning)

    // Structural
    Divider     = lipgloss.NewStyle().Foreground(P.Border)
    SectionHead = lipgloss.NewStyle().Foreground(P.Primary).Bold(true).Padding(0, 1)

    // Interactive states
    RowNormal   = base.Foreground(P.Text)
    RowSelected = base.Background(P.SelectBg).Foreground(P.SelectFg)
    RowRunning  = base.Foreground(P.Active)
    RowFailed   = base.Foreground(P.Error)
    RowDone     = base.Foreground(P.Muted)

    // Labels (dim, ALL-CAPS applied in render, not in style)
    Label = lipgloss.NewStyle().Foreground(P.Muted)
    Value = lipgloss.NewStyle().Foreground(P.Text)

    // Status bar
    StatusLeft  = lipgloss.NewStyle().Foreground(P.Text).Padding(0, 1)
    StatusRight = lipgloss.NewStyle().Foreground(P.Muted).Padding(0, 1)
    StatusError = lipgloss.NewStyle().Foreground(P.Error).Padding(0, 1)
)
```

### Conditional Style Application

Apply styles conditionally without allocating new styles in hot paths:

```go
func rowStyle(state WorkerState, selected bool) lipgloss.Style {
    if selected {
        return RowSelected
    }
    switch state {
    case StateRunning:
        return RowRunning
    case StateFailed:
        return RowFailed
    case StateDone:
        return RowDone
    default:
        return RowNormal
    }
}
```

---

## 2. Layout Composition Recipes

### The Three Layout Primitives

**`JoinVertical(position, blocks...)`** -- Stack blocks top-to-bottom. The `position` controls horizontal alignment of blocks with different widths. Use `lipgloss.Left` for nearly everything (left-aligned panels). `lipgloss.Center` for centered content like empty states or splash screens.

**`JoinHorizontal(position, blocks...)`** -- Place blocks side-by-side. The `position` controls vertical alignment of blocks with different heights. Use `lipgloss.Top` for panel layouts (panels align at the top). Custom float values (0.0-1.0) for proportional alignment.

**`Place(width, height, hPos, vPos, content, opts...)`** -- Position content within a fixed-size region. Essential for centering modals, empty states, and right-aligned status bar content. Supports whitespace fill options for background styling.

### The Full-Screen Layout Recipe

The canonical top-to-bottom TUI structure:

```go
func (m model) View() string {
    header := m.renderHeader()
    statusBar := m.renderStatusBar()

    // Compute available height for content
    contentH := m.height - lipgloss.Height(header) - lipgloss.Height(statusBar)

    var content string
    switch m.layout {
    case layoutNarrow:
        content = m.renderActiveTab(m.width, contentH)
    case layoutStandard:
        leftW := m.width * 40 / 100
        rightW := m.width - leftW
        left := m.renderWorkerPanel(leftW, contentH)
        right := m.renderOutputPanel(rightW, contentH)
        content = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
    case layoutWide:
        col1W := m.width * 20 / 100
        col2W := m.width * 35 / 100
        col3W := m.width - col1W - col2W
        col1 := m.renderTaskPanel(col1W, contentH)
        col2 := m.renderWorkerPanel(col2W, contentH)
        col3 := m.renderOutputPanel(col3W, contentH)
        content = lipgloss.JoinHorizontal(lipgloss.Top, col1, col2, col3)
    }

    return lipgloss.JoinVertical(lipgloss.Left, header, content, statusBar)
}
```

### The Status Bar Gap-Fill Recipe

Place left-aligned and right-aligned content on the same line with a dynamic gap:

```go
func (m model) renderStatusBar() string {
    left := StatusLeft.Render("● 3 running  ○ 1 pending")
    right := StatusRight.Render("? help  q quit")

    gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
    if gap < 0 {
        gap = 0
    }

    return left + strings.Repeat(" ", gap) + right
}
```

For status bars with a middle section (mode indicator):

```go
func (m model) renderStatusBar() string {
    left := StatusLeft.Render(m.statusCounts())
    middle := TextAccent.Render(m.modeLabel())
    right := StatusRight.Render("? help  q quit")

    leftW := lipgloss.Width(left)
    midW := lipgloss.Width(middle)
    rightW := lipgloss.Width(right)

    gapLeft := (m.width-midW)/2 - leftW
    gapRight := m.width - leftW - gapLeft - midW - rightW
    if gapLeft < 1 { gapLeft = 1 }
    if gapRight < 1 { gapRight = 1 }

    return left + strings.Repeat(" ", gapLeft) + middle + strings.Repeat(" ", gapRight) + right
}
```

### The Modal Overlay Recipe

Render a dialog centered over the existing content:

```go
func (m model) View() string {
    base := m.renderDashboard()

    if m.mode == ModeHelp {
        overlay := lipgloss.NewStyle().
            Padding(2, 4).
            Border(lipgloss.RoundedBorder()).
            BorderForeground(P.Border).
            Width(60).
            Render(m.renderHelpContent())

        return lipgloss.Place(
            m.width, m.height,
            lipgloss.Center, lipgloss.Center,
            overlay,
            lipgloss.WithWhitespaceChars(" "),
        )
    }

    return base
}
```

Note: the `Place` approach replaces the base content with whitespace. To show the base dimmed behind the overlay, render both and compose manually -- but for most TUIs the full-overlay approach is cleaner and simpler.

### Fixed-Height Panel Recipe

Ensure a panel fills exactly its allocated height:

```go
func (m model) renderWorkerPanel(width, height int) string {
    header := SectionHead.Render("WORKERS")
    headerH := lipgloss.Height(header)

    tableContent := m.table.View()
    tableH := lipgloss.Height(tableContent)

    // Pad to fill remaining height
    padH := height - headerH - tableH
    if padH < 0 { padH = 0 }
    padding := strings.Repeat("\n", padH)

    panel := lipgloss.JoinVertical(lipgloss.Left, header, tableContent, padding)

    return lipgloss.NewStyle().Width(width).Height(height).Render(panel)
}
```

---

## 3. State Indicator Design

### The Indicator Vocabulary

Design a consistent set of Unicode indicators for process/item states:

| State | Glyph | Color | Rationale |
|---|---|---|---|
| Running | `⠋` (animated spinner) | Active blue | Spinner draws eye to active work |
| Done (success) | `✓` | Success green | Universal completion symbol |
| Failed | `✗` | Error red (muted) | Universal failure, not aggressive |
| Killed | `⊘` | Inactive gray | "Prohibited/stopped" reads clearer than skull |
| Pending | `○` | Dim gray | Visually quieter than `◻` -- pending items should not demand attention |
| Waiting | `⋯` | Warning amber | Waiting for external input |

### Implementation Pattern

```go
func stateIndicator(s *spinner.Model, state WorkerState) string {
    switch state {
    case StateRunning:
        return lipgloss.NewStyle().Foreground(P.Active).Render(s.View())
    case StateDone:
        return TextSuccess.Render("✓")
    case StateFailed:
        return TextError.Render("✗")
    case StateKilled:
        return TextDim.Render("⊘")
    case StatePending:
        return TextDim.Render("○")
    case StateWaiting:
        return TextWarning.Render("⋯")
    default:
        return TextDim.Render("·")
    }
}
```

### Indicator Width Consistency

All indicators must occupy the same column width for alignment. Single-cell glyphs (`✓`, `✗`, `○`, `⊘`, `⋯`) are naturally 1 cell wide. The spinner view may vary -- wrap it with a fixed-width style or pad:

```go
func fixedWidthIndicator(s string, width int) string {
    return lipgloss.NewStyle().Width(width).Render(s)
}
```

---

## 4. `lipgloss/table` vs `bubbles/table`

### When to Use Each

**`lipgloss/table`** -- Static, render-only. No internal state, no keyboard handling. Call `.Render()` or `.String()` to get a styled string. Use for:
- Summary/stats panels (read-only data)
- Non-interactive info displays
- Tables that change only when data changes, not on user input

**`bubbles/table`** -- Interactive bubbletea component with its own `Update`/`View` cycle. Handles keyboard navigation, row selection, and scrolling internally. Use for:
- Worker lists the user navigates and selects
- Task lists with filtering
- Any table where the user moves a selection cursor

### `lipgloss/table` Example (Status Summary)

```go
func renderSummary(workers []Worker) string {
    t := table.New().
        Border(lipgloss.NormalBorder()).
        BorderStyle(lipgloss.NewStyle().Foreground(P.Border)).
        StyleFunc(func(row, col int) lipgloss.Style {
            if row == table.HeaderRow {
                return lipgloss.NewStyle().Foreground(P.Primary).Bold(true).Padding(0, 1)
            }
            return lipgloss.NewStyle().Foreground(P.Text).Padding(0, 1)
        }).
        Headers("METRIC", "COUNT").
        Row("Running", fmt.Sprintf("%d", countState(workers, StateRunning))).
        Row("Pending", fmt.Sprintf("%d", countState(workers, StatePending))).
        Row("Done", fmt.Sprintf("%d", countState(workers, StateDone))).
        Row("Failed", fmt.Sprintf("%d", countState(workers, StateFailed)))

    return t.Render()
}
```

### `bubbles/table` Example (Interactive Worker List)

```go
func newWorkerTable() table.Model {
    columns := []table.Column{
        {Title: "STATE", Width: 6},
        {Title: "ROLE", Width: 10},
        {Title: "TASK", Width: 30},
        {Title: "DURATION", Width: 10},
    }

    t := table.New(
        table.WithColumns(columns),
        table.WithFocused(true),
        table.WithHeight(10),
    )

    s := table.DefaultStyles()
    s.Header = s.Header.
        Bold(true).
        Foreground(P.Primary).
        BorderBottom(true).
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(P.Border)
    s.Selected = s.Selected.
        Background(P.SelectBg).
        Foreground(P.SelectFg).
        Bold(true)
    s.Cell = s.Cell.Foreground(P.Text)
    t.SetStyles(s)

    return t
}
```

---

## 5. Border Discipline

### The Principle

Borders mark the boundary of something important, not "everything is in a box." If every panel has a border, the borders lose meaning and become noise. Polished TUIs use borders sparingly.

### Recommended Approach

One outer border on the application container (or none if using alt screen, which already provides a visual boundary). No borders on internal panels. Separate sections with thin divider lines:

```go
func dividerLine(width int) string {
    return Divider.Render(strings.Repeat("─", width))
}
```

### Top-Only Border for Section Headers

A partial border creates separation without full boxing:

```go
var sectionHeaderStyle = lipgloss.NewStyle().
    Border(lipgloss.NormalBorder()).
    BorderTop(true).
    BorderBottom(false).
    BorderLeft(false).
    BorderRight(false).
    BorderForeground(P.Border).
    Bold(true).
    Foreground(P.Primary).
    Padding(0, 1)
```

### The Over-Bordered vs Clean Comparison

Avoid (border on every element):
```
╭──────────────╮╭──────────────╮
│╭────────────╮││╭────────────╮│
││ Workers    ││││ Output     ││
│╰────────────╯││╰────────────╯│
│╭────────────╮││              │
││ coder-1    ││││              │
│╰────────────╯││              │
╰──────────────╯╰──────────────╯
```

Prefer (structure through spacing and color):
```
 WORKERS          OUTPUT
 ────────────────────────────────
 ● coder-1  2m   Starting task...
 ○ planner  --   Analyzing spec
 ✓ reviewer 5m
                  > Building auth module
 ──────────────── > Running tests...
 ● 2 running  ○ 1 pending  ? help
```

The second version has the same information density with dramatically less visual noise. The section headers and divider lines provide structure; the state indicators and color provide hierarchy.

### When Borders ARE Appropriate

- **Modal dialogs/overlays**: A `RoundedBorder()` around a centered overlay creates clear visual separation from the background content. The border says "this is floating above everything else."
- **The single focused panel** in a multi-panel layout (if using border-based focus indication, which is Option C and generally not recommended).
- **Standalone splash screens or loading states** that need to draw the eye to a small region of an otherwise empty terminal.

Anywhere else, prefer divider lines, spacing, and color differentiation.
