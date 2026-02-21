# Bubbletea Architecture Patterns

Reference for implementing bubbletea Model/Update/View patterns. Covers the non-obvious architectural decisions that separate production TUIs from toy examples.

---

## 1. The Elm Architecture in Practice

### Model Field Ordering

Group model fields by architectural layer. This ordering reflects dependency flow and makes the struct self-documenting:

```go
type model struct {
    // Layer 1: UI state -- drives rendering decisions
    width       int
    height      int
    focused     panel
    mode        AppMode
    ready       bool

    // Layer 2: Domain data -- the actual application state
    workers     map[string]*Worker
    tasks       []Task
    statusMsg   string
    lastError   error

    // Layer 3: Sub-model components -- bubbles widgets
    table       table.Model
    viewport    viewport.Model
    help        help.Model
    spinner     spinner.Model

    // Layer 4: Async/infrastructure state -- channels, flags, refs
    workerPipes map[string]io.ReadCloser
    program     *tea.Program  // for p.Send() from goroutines
    daemonMode  bool
}
```

Maintain this ordering consistently. When reading an unfamiliar model, Layer 1 fields immediately reveal the UI's state space.

### View() Is a Pure Function

`View()` runs after every `Update` call -- potentially up to 60fps with high-frequency messages. Treat it as strictly read-only over the model:

- **Never** mutate model fields in `View()`. The method has a value receiver `(m model)` precisely to prevent this, but pointer fields (maps, slices) can still be mutated through the copy. Do not.
- **Never** perform I/O, system calls, or channel operations in `View()`.
- **Never** compute layout dimensions in `View()`. Store computed widths/heights in the model during `Update`.
- If `View()` modifies state, the bug manifests as flickering, phantom state changes, or behavior that differs between fast and slow terminals. These are extremely difficult to reproduce.

```go
// WRONG: computing layout in View
func (m model) View() string {
    sidebarWidth := m.width / 3         // recomputed every frame
    mainWidth := m.width - sidebarWidth // wasteful, belongs in Update
    // ...
}

// CORRECT: use precomputed values from Update
func (m model) View() string {
    sidebar := sidebarStyle.Width(m.sidebarWidth).Render(m.renderSidebar())
    main := mainStyle.Width(m.mainWidth).Render(m.renderMain())
    return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
}
```

### Init() vs WindowSizeMsg

`Init()` fires once before the first `Update`. Use it exclusively for launching initial commands -- timers, goroutine watchers, initial data fetches. Do **not** compute layout in `Init()` because terminal dimensions are unknown at that point.

`tea.WindowSizeMsg` arrives immediately after program start (and on every subsequent resize). Use this message as the true initialization point for layout:

```go
func (m model) Init() tea.Cmd {
    return tea.Batch(
        m.spinner.Tick,                    // start spinner animation
        watchSignals(),                     // start signal handler
        fetchInitialState(m.configPath),   // async data load
    )
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.sidebarWidth = min(msg.Width/3, 40)
        m.mainWidth = msg.Width - m.sidebarWidth
        headerHeight := 3
        footerHeight := 1
        contentHeight := msg.Height - headerHeight - footerHeight

        if !m.ready {
            // First WindowSizeMsg: initialize viewport and table
            m.viewport = viewport.New(m.mainWidth, contentHeight)
            m.viewport.YPosition = headerHeight
            m.table.SetWidth(m.sidebarWidth)
            m.table.SetHeight(contentHeight)
            m.ready = true
        } else {
            // Subsequent resizes: update dimensions
            m.viewport.Width = m.mainWidth
            m.viewport.Height = contentHeight
            m.table.SetWidth(m.sidebarWidth)
            m.table.SetHeight(contentHeight)
        }
        return m, nil
    }
    // ...
}
```

---

## 2. Command Patterns for Subprocess Management

### The One-Shot Cmd Nature

`tea.Cmd` is a function that returns exactly one `tea.Msg`. For continuous streams (subprocess stdout, file watchers, network connections), re-issue the Cmd after handling each message:

```go
type WorkerOutputMsg struct {
    WorkerID string
    Line     string
}

type WorkerExitMsg struct {
    WorkerID string
    Code     int
    Err      error
}

// Read one line from a worker's output scanner. Returns nil when stream ends.
// IMPORTANT: Pass *bufio.Scanner, not io.Reader. The scanner must be created
// once (when the worker starts) and reused across Cmd re-invocations.
// Creating a new scanner each call loses buffered data.
func watchWorkerOutput(workerID string, scanner *bufio.Scanner) tea.Cmd {
    return func() tea.Msg {
        if scanner.Scan() {
            return WorkerOutputMsg{WorkerID: workerID, Line: scanner.Text()}
        }
        if err := scanner.Err(); err != nil {
            return WorkerExitMsg{WorkerID: workerID, Code: -1, Err: err}
        }
        return WorkerExitMsg{WorkerID: workerID, Code: 0, Err: nil}
    }
}
```

The critical pattern: re-issue after each message to maintain the stream.

```go
case WorkerOutputMsg:
    m.appendOutput(msg.WorkerID, msg.Line)
    // Re-issue the command to continue reading
    scanner := m.workerScanners[msg.WorkerID]
    if scanner != nil {
        return m, watchWorkerOutput(msg.WorkerID, scanner)
    }
    return m, nil

case WorkerExitMsg:
    m.markWorkerDone(msg.WorkerID, msg.Code)
    delete(m.workerScanners, msg.WorkerID)
    // Do NOT re-issue -- stream is finished
    return m, nil
```

### Monitoring Multiple Workers with tea.Batch

Launch watchers for all active workers simultaneously:

```go
func (m model) startAllWorkerWatchers() tea.Cmd {
    var cmds []tea.Cmd
    for id, pipe := range m.workerPipes {
        cmds = append(cmds, watchWorkerOutput(id, pipe))
    }
    return tea.Batch(cmds...)
}
```

Call this from `Init()` or from the handler that spawns workers. Each watcher runs concurrently in its own goroutine managed by the bubbletea runtime.

### External Message Injection with p.Send()

For goroutines that outlive a single Cmd cycle (long-running monitors, event subscribers), use `p.Send()` to inject messages from outside the Elm loop:

```go
func startExternalMonitor(p *tea.Program, workerID string) {
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            stats, err := collectWorkerStats(workerID)
            if err != nil {
                p.Send(WorkerErrorMsg{WorkerID: workerID, Err: err})
                return
            }
            p.Send(WorkerStatsMsg{WorkerID: workerID, Stats: stats})
        }
    }()
}
```

Store the `*tea.Program` reference outside the model. Because bubbletea copies the model by value, storing `p` as a field on the model before passing it to `NewProgram` creates a chicken-and-egg problem. Use a package-level variable or pass `p` via a pointer shared between the model and goroutines:

```go
// Option 1: Package-level variable (simplest for single-program apps)
var program *tea.Program

func main() {
    m := newModel()
    program = tea.NewProgram(m, tea.WithAltScreen())
    if _, err := program.Run(); err != nil {
        os.Exit(1)
    }
}

// Goroutines use the package-level `program` variable:
func monitorWorker(workerID string) {
    // ...
    program.Send(WorkerExitMsg{WorkerID: workerID, Code: exitCode})
}
```

```go
// Option 2: Shared pointer field (for multi-program or test scenarios)
type model struct {
    programRef **tea.Program // pointer-to-pointer survives value copy
}

func main() {
    var p *tea.Program
    m := model{programRef: &p}
    p = tea.NewProgram(m, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        os.Exit(1)
    }
}
```

**Caution**: `p.Send()` is safe to call after the program exits (no-op), but blocks if called before `Run()` starts. Avoid sending from `Init()` goroutines before the event loop is active.

### Channel-Based Cmd Pattern

For complex coordination, wrap a channel receive in a Cmd:

```go
type WorkerEvent struct {
    WorkerID string
    Type     string
    Data     any
}

func waitForEvent(ch <-chan WorkerEvent) tea.Cmd {
    return func() tea.Msg {
        event, ok := <-ch
        if !ok {
            return WorkerChannelClosedMsg{}
        }
        return event // WorkerEvent implements tea.Msg implicitly
    }
}
```

Re-issue after each receive, same as the scanner pattern. Close the channel to terminate the loop.

---

## 3. Sub-Model Component Delegation

### Forward Messages to the Focused Component

Bubbles components maintain internal state (cursor position, scroll offset, filter text). Always forward messages through their `Update` method:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Global keys: handle regardless of focus
        switch msg.String() {
        case "ctrl+c":
            return m, tea.Quit
        case "?":
            m.mode = ModeHelp
            return m, nil
        case "tab":
            m.focused = (m.focused + 1) % panelCount
            return m, nil
        }
    case tea.WindowSizeMsg:
        // ALL components get resize messages, not just focused one
        m.table.SetWidth(m.sidebarWidth)
        m.table.SetHeight(m.contentHeight)
        m.viewport.Width = m.mainWidth
        m.viewport.Height = m.contentHeight
        m.help.Width = msg.Width
    }

    // Route to focused component
    switch m.focused {
    case panelWorkers:
        m.table, cmd = m.table.Update(msg)
        cmds = append(cmds, cmd)
    case panelOutput:
        m.viewport, cmd = m.viewport.Update(msg)
        cmds = append(cmds, cmd)
    }

    // Non-focused components may still need certain messages (e.g., spinner tick)
    m.spinner, cmd = m.spinner.Update(msg)
    cmds = append(cmds, cmd)

    return m, tea.Batch(cmds...)
}
```

### The Ready Pattern

Viewport (and other components that need dimensions) cannot render meaningfully before receiving a `WindowSizeMsg`. Guard with a boolean flag:

```go
func (m model) View() string {
    if !m.ready {
        return "\n  Initializing..."
    }
    return m.renderDashboard()
}
```

Initialize the viewport only on the first `WindowSizeMsg`, not in `Init()` or the model constructor. This avoids zero-dimension rendering artifacts.

### Focus Management

Bubbles components have no concept of "focused" vs "unfocused" -- that is entirely the parent model's responsibility. Maintain focus state as an enum:

```go
type panel int

const (
    panelWorkers panel = iota
    panelOutput
    panelTasks
    panelCount // sentinel for modular arithmetic
)

type model struct {
    focused panel
    // ...
}
```

Route `tea.KeyMsg` only to the focused component. Route `tea.WindowSizeMsg` to all components. Route `spinner.TickMsg` to the spinner regardless of focus.

Apply visual differentiation for focus state in `View()`:

```go
func (m model) renderPanel(p panel, content string) string {
    style := panelStyle
    if p == m.focused {
        style = style.BorderForeground(palette.Primary)
    } else {
        style = style.BorderForeground(palette.Border)
    }
    return style.Render(content)
}
```

### Component Sizing Belongs in Update

Never call `SetWidth()`, `SetHeight()`, or `SetSize()` inside `View()`. These methods mutate the component's internal model. Compute and apply all dimensions in the `WindowSizeMsg` handler within `Update`.

---

## 4. Modal State Machine

### Mode Enum Pattern

Define application modes as an explicit state machine:

```go
type AppMode int

const (
    ModeNormal AppMode = iota
    ModeSpawnDialog
    ModeHelp
    ModeConfirmKill
    ModeFilter
)
```

Check mode before routing keypresses in `Update`:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Mode-specific handling takes priority
    switch m.mode {
    case ModeHelp:
        if msg, ok := msg.(tea.KeyMsg); ok {
            if msg.String() == "esc" || msg.String() == "?" || msg.String() == "q" {
                m.mode = ModeNormal
            }
            return m, nil // swallow all other keys in help mode
        }
    case ModeConfirmKill:
        if msg, ok := msg.(tea.KeyMsg); ok {
            switch msg.String() {
            case "y", "Y":
                m.mode = ModeNormal
                return m, m.killWorker(m.pendingKillID)
            case "n", "N", "esc":
                m.mode = ModeNormal
                m.pendingKillID = ""
            }
            return m, nil
        }
    case ModeSpawnDialog:
        return m.updateSpawnDialog(msg)
    }

    // ModeNormal: standard routing
    // ...
}
```

### Overlay Rendering

Render the background content in all modes -- it provides spatial context and avoids the jarring "blank screen behind dialog" effect. Layer the modal on top using `lipgloss.Place()`:

```go
func (m model) View() string {
    base := m.renderDashboard()

    switch m.mode {
    case ModeHelp:
        overlay := m.renderHelpOverlay()
        return lipgloss.Place(
            m.width, m.height,
            lipgloss.Center, lipgloss.Center,
            overlay,
            lipgloss.WithWhitespaceChars(" "),
            lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
        )

    case ModeConfirmKill:
        dialog := confirmStyle.Render(fmt.Sprintf(
            "Kill worker %s?\n\n  y/n",
            m.pendingKillID,
        ))
        return lipgloss.Place(
            m.width, m.height,
            lipgloss.Center, lipgloss.Center,
            dialog,
            lipgloss.WithWhitespaceChars(" "),
            lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
        )

    case ModeSpawnDialog:
        dialog := dialogStyle.Render(m.spawnForm.View())
        return lipgloss.Place(
            m.width, m.height,
            lipgloss.Center, lipgloss.Center,
            dialog,
            lipgloss.WithWhitespaceChars(" "),
            lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
        )
    }

    return base
}
```

The `lipgloss.WithWhitespaceChars(" ")` with a dark foreground creates a semi-transparent scrim effect -- the background content is replaced with uniform spacing that preserves the illusion of depth.

For dialogs containing interactive components (text inputs, forms), route their `Update` calls in the mode-specific branch and ensure `tea.KeyMsg` does not leak to the background components.

---

## 5. Daemon Mode (WithoutRenderer)

### Behavior Differences

When running with `tea.WithoutRenderer()`, the program operates headlessly:

- `View()` is **never called**. Define it to return `""` in daemon mode -- do not waste cycles building strings.
- `Update()` receives all messages normally. The Elm loop functions identically.
- All `tea.Cmd` patterns work unchanged -- subprocess watchers, timers, batching.
- Use `tea.Println()` and `tea.Printf()` to emit structured output (JSON lines, log entries) to stdout.
- Graceful shutdown requires explicit signal handling since there is no `q` key to press.

### Structured Output Pattern

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case WorkerOutputMsg:
        m.appendOutput(msg.WorkerID, msg.Line)
        cmds := []tea.Cmd{
            watchWorkerOutput(msg.WorkerID, m.workerPipes[msg.WorkerID]),
        }
        if m.daemonMode {
            cmds = append(cmds, tea.Println(
                fmt.Sprintf(`{"event":"output","worker":%q,"line":%q}`,
                    msg.WorkerID, msg.Line),
            ))
        }
        return m, tea.Batch(cmds...)

    case WorkerExitMsg:
        m.markWorkerDone(msg.WorkerID, msg.Code)
        if m.daemonMode {
            return m, tea.Println(
                fmt.Sprintf(`{"event":"exit","worker":%q,"code":%d}`,
                    msg.WorkerID, msg.Code),
            )
        }
        return m, nil
    }
    // ...
}
```

### Signal Handling Cmd

Wrap `os/signal` in a Cmd for clean integration with the Elm loop:

```go
type SignalMsg struct {
    Signal os.Signal
}

func watchSignals() tea.Cmd {
    return func() tea.Msg {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        sig := <-sigCh
        return SignalMsg{Signal: sig}
    }
}
```

Handle in `Update`:

```go
case SignalMsg:
    // Graceful shutdown: kill workers, flush output, then quit
    var cmds []tea.Cmd
    for id := range m.workers {
        cmds = append(cmds, m.killWorker(id))
    }
    cmds = append(cmds, tea.Quit)
    return m, tea.Batch(cmds...)
```

### Dual-Mode Program Initialization

```go
func newProgram(m model, daemon bool) *tea.Program {
    m.daemonMode = daemon
    opts := []tea.ProgramOption{}
    if daemon {
        opts = append(opts, tea.WithoutRenderer())
    } else {
        opts = append(opts, tea.WithAltScreen(), tea.WithMouseCellMotion())
    }
    return tea.NewProgram(m, opts...)
}
```

---

## 6. Performance Patterns

### Style Allocation

Define styles at package level in a `var` block. `lipgloss.NewStyle()` allocates; calling it inside `View()` creates garbage every frame:

```go
// CORRECT: package-level allocation, zero per-frame cost
var (
    headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
    panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
    mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
    statusStyle  = lipgloss.NewStyle().Background(lipgloss.Color("236")).Padding(0, 1)
)

// WRONG: allocating inside View -- runs every frame
func (m model) View() string {
    header := lipgloss.NewStyle().Bold(true).Render("Title") // allocation per frame
    // ...
}
```

For styles that depend on runtime dimensions, use `.Width()` and `.Height()` on the pre-allocated style -- these return a copy without heap allocation in the common case:

```go
func (m model) renderSidebar() string {
    return panelStyle.Width(m.sidebarWidth).Height(m.contentHeight).Render(m.table.View())
}
```

### Memoize Static Renders

Cache rendered strings when their inputs have not changed. Use simple invalidation keys:

```go
type model struct {
    // ...

    // Render cache
    headerCache   string
    headerCacheW  int  // invalidation: width changed

    statusCache   string
    statusCacheW  int
    statusCacheMsg string
}

func (m *model) renderHeader() string {
    if m.width == m.headerCacheW && m.headerCache != "" {
        return m.headerCache
    }
    rendered := headerStyle.Width(m.width).Render(
        lipgloss.JoinHorizontal(lipgloss.Center,
            titleStyle.Render("KASMOS"),
            lipgloss.NewStyle().Width(m.width-20).Render(""),
            versionStyle.Render("v0.1.0"),
        ),
    )
    m.headerCacheW = m.width
    m.headerCache = rendered
    return rendered
}
```

Note: this requires a pointer receiver on the render method (or updating the cache in `Update`). Since `View()` has a value receiver, either pre-render in `Update` and store, or accept that the cache check runs but cannot update. The cleaner pattern is to compute and cache in the `WindowSizeMsg` handler:

```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    // Recompute cached renders that depend on width
    m.headerCache = renderHeaderString(m.width)
    m.footerCache = renderFooterString(m.width)
```

### Avoid Redundant String Measurement

`lipgloss.Width()` and `lipgloss.Height()` scan the entire string counting runes and newlines. Call once per layout pass, store the result:

```go
// WRONG: measuring inside a loop
for _, row := range rows {
    rendered := rowStyle.Render(row)
    totalHeight += lipgloss.Height(rendered) // O(n) per row
}

// CORRECT: measure once or compute from known inputs
rowHeight := 1 // known: single-line rows
totalHeight := len(rows) * rowHeight
```

### High-Frequency Message Batching

When workers produce output faster than the terminal can render (hundreds of lines per second), batching prevents frame drops and CPU saturation. Accumulate lines and flush on a tick:

```go
type OutputFlushMsg struct{}

func scheduleFlush() tea.Cmd {
    return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
        return OutputFlushMsg{}
    })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case WorkerOutputMsg:
        m.outputBuffer = append(m.outputBuffer, msg.Line)
        // Do NOT update viewport content here -- just buffer
        cmd := watchWorkerOutput(msg.WorkerID, m.workerPipes[msg.WorkerID])
        if !m.flushScheduled {
            m.flushScheduled = true
            return m, tea.Batch(cmd, scheduleFlush())
        }
        return m, cmd

    case OutputFlushMsg:
        if len(m.outputBuffer) > 0 {
            m.viewport.SetContent(
                m.viewport.View() + "\n" + strings.Join(m.outputBuffer, "\n"),
            )
            m.outputBuffer = m.outputBuffer[:0]
            m.viewport.GotoBottom()
        }
        m.flushScheduled = false
        return m, nil
    }
    // ...
}
```

This coalesces potentially dozens of output lines into a single render cycle, keeping the UI responsive even under heavy I/O.

### Delegate to Bubbles Components

`bubbles/table`, `bubbles/list`, and `bubbles/viewport` handle their own efficient rendering internally (virtual scrolling, visible-row-only rendering). Do not replace them with hand-rolled `strings.Builder` loops unless there is a measured performance reason. The bubbles implementations have been optimized for the common cases.

### Summary of Hot-Path Rules

| Rule | Rationale |
|---|---|
| Define styles in `var` blocks | Avoid allocation per frame |
| Compute layout in `Update`, not `View` | `View` is hot path; keep it cheap |
| Call `lipgloss.Width()`/`Height()` once | O(n) string scan per call |
| Cache static renders keyed on dimensions | Headers/footers rarely change |
| Batch high-frequency messages with tick | Prevents frame-per-line rendering |
| Use bubbles components over hand-rolled | Pre-optimized virtual scrolling |
