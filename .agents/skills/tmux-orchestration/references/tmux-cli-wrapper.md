# tmux CLI Wrapper — Go Patterns

## Table of Contents
- [Interface Design](#interface-design)
- [Command Execution](#command-execution)
- [Error Handling](#error-handling)
- [Parsing](#parsing)
- [Testing](#testing)

---

## Interface Design

The `TmuxClient` interface abstracts all tmux interactions behind a clean Go API.
This is the seam for testing — the real implementation calls `os/exec`, tests use a
mock/stub.

```go
package tmux

import "context"

// PaneInfo represents parsed output from list-panes.
type PaneInfo struct {
    PaneID     string // e.g., "%42"
    PanePID    int    // PID of the process in the pane
    Dead       bool   // true if pane process has exited
    DeadStatus int    // exit code (only meaningful when Dead)
}

// TmuxClient abstracts tmux CLI operations for testability.
type TmuxClient interface {
    // Lifecycle
    SplitWindow(ctx context.Context, opts SplitOpts) (paneID string, err error)
    KillPane(ctx context.Context, paneID string) error
    SelectPane(ctx context.Context, paneID string) error

    // Window management
    NewWindow(ctx context.Context, opts NewWindowOpts) (windowID string, err error)
    JoinPane(ctx context.Context, opts JoinOpts) error

    // Introspection
    ListPanes(ctx context.Context, target string) ([]PaneInfo, error)
    DisplayMessage(ctx context.Context, format string) (string, error)
    CapturePane(ctx context.Context, paneID string) (string, error)

    // Environment tagging
    SetEnvironment(ctx context.Context, key, value string) error
    ShowEnvironment(ctx context.Context) (map[string]string, error)
    UnsetEnvironment(ctx context.Context, key string) error

    // Meta
    Version(ctx context.Context) (string, error)
}

// SplitOpts configures a split-window operation.
type SplitOpts struct {
    Target     string   // target pane/window to split from
    Horizontal bool     // -h flag (vertical split, pane appears to the right)
    Size       string   // -l flag: "50%" or "80" (columns)
    Command    []string // command to run in the new pane
    Env        []string // environment variables as "KEY=VALUE"
}

// NewWindowOpts configures a new-window operation.
type NewWindowOpts struct {
    Detached bool   // -d flag: don't switch to the new window
    Name     string // -n flag: window name
}

// JoinOpts configures a join-pane operation.
type JoinOpts struct {
    Source     string // -s: source pane to move
    Target     string // -t: destination window/pane
    Horizontal bool   // -h: join as horizontal split
    Detached   bool   // -d: don't follow focus
    Size       string // -l: size specification
}
```

### Why an Interface?

- **Testability**: Mock the entire tmux layer in backend tests. No real tmux needed.
- **Future flexibility**: Could swap to a socket-protocol implementation without changing
  the backend code.
- **Documentation**: The interface IS the contract. Every tmux operation kasmos uses is
  visible in one place.

---

## Command Execution

The real implementation wraps `os/exec.Command`. Every tmux invocation follows the same
pattern: build args, execute, capture stdout+stderr, parse result.

### ExecClient Implementation

```go
package tmux

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "strings"
)

// ExecClient implements TmuxClient using os/exec.
type ExecClient struct {
    // TmuxPath is the path to the tmux binary. Defaults to "tmux".
    TmuxPath string

    // Session constrains all operations to a specific tmux session.
    // If empty, uses the current session (TMUX env var).
    Session string

    // Logger for debug-level command tracing. nil = no logging.
    Logger Logger
}

// Logger interface for command tracing.
type Logger interface {
    Debug(msg string, keysAndValues ...any)
}

// run executes a tmux command and returns stdout.
// This is the single execution chokepoint — all methods call this.
func (c *ExecClient) run(ctx context.Context, args ...string) (string, error) {
    bin := c.TmuxPath
    if bin == "" {
        bin = "tmux"
    }

    // Prepend session target if configured
    if c.Session != "" {
        args = append([]string{"-t", c.Session}, args...)
    }

    cmd := exec.CommandContext(ctx, bin, args...)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if c.Logger != nil {
        c.Logger.Debug("tmux exec", "cmd", fmt.Sprintf("%s %s", bin, strings.Join(args, " ")))
    }

    err := cmd.Run()
    if err != nil {
        stderrStr := strings.TrimSpace(stderr.String())
        if c.Logger != nil {
            c.Logger.Debug("tmux error", "stderr", stderrStr, "err", err)
        }
        return "", &TmuxError{
            Command: args,
            Stderr:  stderrStr,
            Err:     err,
        }
    }

    result := strings.TrimSpace(stdout.String())
    if c.Logger != nil {
        c.Logger.Debug("tmux result", "stdout", result)
    }
    return result, nil
}
```

### Method Implementations

Each method translates its typed opts into tmux CLI arguments:

```go
func (c *ExecClient) SplitWindow(ctx context.Context, opts SplitOpts) (string, error) {
    args := []string{"split-window"}
    if opts.Horizontal {
        args = append(args, "-h")
    }
    if opts.Target != "" {
        args = append(args, "-t", opts.Target)
    }
    if opts.Size != "" {
        args = append(args, "-l", opts.Size)
    }
    // -P -F to capture the new pane's ID
    args = append(args, "-P", "-F", "#{pane_id}")

    // Environment variables
    for _, env := range opts.Env {
        args = append(args, "-e", env)
    }

    if len(opts.Command) > 0 {
        args = append(args, opts.Command...)
    }

    return c.run(ctx, args...)
}

func (c *ExecClient) KillPane(ctx context.Context, paneID string) error {
    _, err := c.run(ctx, "kill-pane", "-t", paneID)
    if err != nil {
        // "can't find pane" is not a real error — pane already gone
        if IsNotFound(err) {
            return nil
        }
    }
    return err
}

func (c *ExecClient) SelectPane(ctx context.Context, paneID string) error {
    _, err := c.run(ctx, "select-pane", "-t", paneID)
    return err
}

func (c *ExecClient) NewWindow(ctx context.Context, opts NewWindowOpts) (string, error) {
    args := []string{"new-window"}
    if opts.Detached {
        args = append(args, "-d")
    }
    if opts.Name != "" {
        args = append(args, "-n", opts.Name)
    }
    args = append(args, "-P", "-F", "#{window_id}")
    return c.run(ctx, args...)
}

func (c *ExecClient) JoinPane(ctx context.Context, opts JoinOpts) error {
    args := []string{"join-pane"}
    if opts.Source != "" {
        args = append(args, "-s", opts.Source)
    }
    if opts.Target != "" {
        args = append(args, "-t", opts.Target)
    }
    if opts.Horizontal {
        args = append(args, "-h")
    }
    if opts.Detached {
        args = append(args, "-d")
    }
    if opts.Size != "" {
        args = append(args, "-l", opts.Size)
    }
    _, err := c.run(ctx, args...)
    return err
}

func (c *ExecClient) ListPanes(ctx context.Context, target string) ([]PaneInfo, error) {
    format := "#{pane_id} #{pane_pid} #{pane_dead} #{pane_dead_status}"
    args := []string{"list-panes", "-F", format}
    if target != "" {
        args = append(args, "-t", target)
    }

    out, err := c.run(ctx, args...)
    if err != nil {
        return nil, err
    }
    return ParsePaneList(out)
}

func (c *ExecClient) DisplayMessage(ctx context.Context, format string) (string, error) {
    return c.run(ctx, "display-message", "-p", format)
}

func (c *ExecClient) CapturePane(ctx context.Context, paneID string) (string, error) {
    // -p prints to stdout, -S - starts from beginning of scrollback
    return c.run(ctx, "capture-pane", "-p", "-t", paneID, "-S", "-")
}

func (c *ExecClient) SetEnvironment(ctx context.Context, key, value string) error {
    _, err := c.run(ctx, "set-environment", key, value)
    return err
}

func (c *ExecClient) ShowEnvironment(ctx context.Context) (map[string]string, error) {
    out, err := c.run(ctx, "show-environment")
    if err != nil {
        return nil, err
    }
    return ParseEnvironment(out)
}

func (c *ExecClient) UnsetEnvironment(ctx context.Context, key string) error {
    _, err := c.run(ctx, "set-environment", "-u", key)
    return err
}

func (c *ExecClient) Version(ctx context.Context) (string, error) {
    // Version doesn't take a session target, call tmux directly
    bin := c.TmuxPath
    if bin == "" {
        bin = "tmux"
    }
    cmd := exec.CommandContext(ctx, bin, "-V")
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("tmux not found or not executable: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}
```

### Command Construction Rules

- **Always use `-P -F '#{pane_id}'`** on commands that create panes/windows. This is how
  you capture the ID of the thing you just created.
- **Always use `-t <pane_id>`** with the `%`-prefixed pane ID (e.g., `%42`), not pane
  indices. Indices shift when panes are moved; IDs are stable.
- **Use `-d` (detached)** on `join-pane` when parking to prevent focus from chasing the
  pane into the hidden window.
- **Context for cancellation**: Pass `context.Context` to support timeout/cancellation
  on tmux operations. Particularly important for the poll loop.

---

## Error Handling

### Typed Errors

```go
package tmux

import (
    "fmt"
    "strings"
)

// TmuxError wraps a failed tmux command with its stderr output.
type TmuxError struct {
    Command []string
    Stderr  string
    Err     error // underlying exec error
}

func (e *TmuxError) Error() string {
    return fmt.Sprintf("tmux %s: %s (stderr: %s)",
        strings.Join(e.Command, " "), e.Err, e.Stderr)
}

func (e *TmuxError) Unwrap() error {
    return e.Err
}

// Sentinel checks on tmux stderr content.
// tmux doesn't use exit codes meaningfully — stderr text is the discriminator.

func IsNotFound(err error) bool {
    var te *TmuxError
    if errors.As(err, &te) {
        return strings.Contains(te.Stderr, "can't find") ||
            strings.Contains(te.Stderr, "no such") ||
            strings.Contains(te.Stderr, "not found")
    }
    return false
}

func IsSessionGone(err error) bool {
    var te *TmuxError
    if errors.As(err, &te) {
        return strings.Contains(te.Stderr, "no server running") ||
            strings.Contains(te.Stderr, "session not found") ||
            strings.Contains(te.Stderr, "no current session")
    }
    return false
}

func IsNoSpace(err error) bool {
    var te *TmuxError
    if errors.As(err, &te) {
        // tmux returns this when the terminal is too small to split
        return strings.Contains(te.Stderr, "no space for new pane")
    }
    return false
}
```

### Error Classification Strategy

tmux communicates failure through stderr text, not structured error codes. The pattern:

1. **Wrap**: Every `run()` call wraps errors in `TmuxError` with the full command and
   stderr captured.
2. **Classify**: `Is*` functions check stderr strings for known patterns.
3. **Handle at the call site**: Some errors are expected (killing an already-dead pane).
   Others need to propagate as bubbletea messages.

```go
// Example: KillPane swallows "not found" because the pane may already be dead
func (c *ExecClient) KillPane(ctx context.Context, paneID string) error {
    _, err := c.run(ctx, "kill-pane", "-t", paneID)
    if err != nil && IsNotFound(err) {
        return nil // pane already gone, not an error
    }
    return err
}

// Example: SplitWindow surfacing NoSpace as a specific error for the TUI to show
func handleSplitError(err error) tea.Msg {
    if IsNoSpace(err) {
        return workerSpawnFailedMsg{
            reason: "terminal too small to create worker pane — resize and retry",
        }
    }
    return workerSpawnFailedMsg{reason: err.Error()}
}
```

---

## Parsing

tmux format strings return structured text. Parse them into typed Go values.

### Pane List Parsing

```go
package tmux

import (
    "strconv"
    "strings"
)

// ParsePaneList parses the output of:
//   list-panes -F '#{pane_id} #{pane_pid} #{pane_dead} #{pane_dead_status}'
func ParsePaneList(output string) ([]PaneInfo, error) {
    if output == "" {
        return nil, nil
    }

    var panes []PaneInfo
    for _, line := range strings.Split(output, "\n") {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }

        fields := strings.Fields(line)
        if len(fields) < 4 {
            continue // malformed line, skip
        }

        pid, err := strconv.Atoi(fields[1])
        if err != nil {
            pid = 0 // best effort
        }

        deadStatus, err := strconv.Atoi(fields[3])
        if err != nil {
            deadStatus = -1
        }

        panes = append(panes, PaneInfo{
            PaneID:     fields[0],
            PanePID:    pid,
            Dead:       fields[2] == "1",
            DeadStatus: deadStatus,
        })
    }
    return panes, nil
}

// ParseEnvironment parses the output of show-environment.
// Format: "KEY=VALUE" per line, or "-KEY" for unset vars.
func ParseEnvironment(output string) (map[string]string, error) {
    env := make(map[string]string)
    for _, line := range strings.Split(output, "\n") {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "-") {
            continue
        }
        if idx := strings.IndexByte(line, '='); idx > 0 {
            env[line[:idx]] = line[idx+1:]
        }
    }
    return env, nil
}

// ParseVersion extracts the version number from "tmux X.Y" output.
func ParseVersion(output string) (major, minor int, err error) {
    // "tmux 3.4" or "tmux 3.3a"
    parts := strings.Fields(output)
    if len(parts) < 2 {
        return 0, 0, fmt.Errorf("unexpected version format: %q", output)
    }
    ver := strings.TrimRight(parts[1], "abcdefghijklmnopqrstuvwxyz")
    dotParts := strings.SplitN(ver, ".", 2)
    major, err = strconv.Atoi(dotParts[0])
    if err != nil {
        return 0, 0, fmt.Errorf("parse major version: %w", err)
    }
    if len(dotParts) > 1 {
        minor, _ = strconv.Atoi(dotParts[1])
    }
    return major, minor, nil
}
```

### Parsing Design Rules

- **Pure functions**: Parsers take strings, return typed values. No side effects, trivially
  testable.
- **Tolerant parsing**: Skip malformed lines rather than failing the entire parse. tmux
  output can include unexpected lines (warnings, etc.).
- **Separate from execution**: Keep parsers in `parser.go`, not inline in method
  implementations. This makes parser tests clean.

---

## Testing

### Mock Client

```go
package tmux_test

import (
    "context"
    "sync"
)

// MockClient implements TmuxClient for testing.
type MockClient struct {
    mu    sync.Mutex
    calls []MockCall
    // Configure responses per method
    SplitWindowFn     func(ctx context.Context, opts SplitOpts) (string, error)
    KillPaneFn        func(ctx context.Context, paneID string) error
    ListPanesFn       func(ctx context.Context, target string) ([]PaneInfo, error)
    // ... etc for each method
}

type MockCall struct {
    Method string
    Args   []any
}

func (m *MockClient) SplitWindow(ctx context.Context, opts SplitOpts) (string, error) {
    m.mu.Lock()
    m.calls = append(m.calls, MockCall{Method: "SplitWindow", Args: []any{opts}})
    m.mu.Unlock()
    if m.SplitWindowFn != nil {
        return m.SplitWindowFn(ctx, opts)
    }
    return "%99", nil // default: return a fake pane ID
}

// AssertCalled checks that a method was called with expected arguments.
func (m *MockClient) AssertCalled(t *testing.T, method string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, c := range m.calls {
        if c.Method == method {
            return
        }
    }
    t.Errorf("expected call to %s, but it was not called", method)
}
```

### Parser Test Examples

```go
func TestParsePaneList(t *testing.T) {
    tests := []struct {
        name   string
        input  string
        want   []PaneInfo
    }{
        {
            name:  "two panes one dead",
            input: "%0 12345 0 0\n%1 12346 1 137",
            want: []PaneInfo{
                {PaneID: "%0", PanePID: 12345, Dead: false, DeadStatus: 0},
                {PaneID: "%1", PanePID: 12346, Dead: true, DeadStatus: 137},
            },
        },
        {
            name:  "empty output",
            input: "",
            want:  nil,
        },
        {
            name:  "malformed line skipped",
            input: "%0 12345 0 0\nbadline\n%2 12347 0 0",
            want: []PaneInfo{
                {PaneID: "%0", PanePID: 12345, Dead: false, DeadStatus: 0},
                {PaneID: "%2", PanePID: 12347, Dead: false, DeadStatus: 0},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParsePaneList(tt.input)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            // deep equal check...
        })
    }
}

func TestParseEnvironment(t *testing.T) {
    input := "KASMOS_PANE_1=worker-abc\nKASMOS_PANE_2=worker-def\n-REMOVED_VAR\nPATH=/usr/bin"
    got, _ := ParseEnvironment(input)

    if got["KASMOS_PANE_1"] != "worker-abc" {
        t.Errorf("expected worker-abc, got %s", got["KASMOS_PANE_1"])
    }
    if _, ok := got["REMOVED_VAR"]; ok {
        t.Error("unset variable should not be in map")
    }
}
```

### Integration Testing (Optional)

For integration tests that hit a real tmux:

```go
//go:build integration

func TestRealTmuxSplitAndKill(t *testing.T) {
    if os.Getenv("TMUX") == "" {
        t.Skip("not running inside tmux")
    }

    client := &ExecClient{}
    ctx := context.Background()

    paneID, err := client.SplitWindow(ctx, SplitOpts{
        Horizontal: true,
        Command:    []string{"echo", "hello"},
    })
    if err != nil {
        t.Fatalf("split-window failed: %v", err)
    }

    t.Logf("created pane: %s", paneID)

    // Give process time to exit
    time.Sleep(200 * time.Millisecond)

    err = client.KillPane(ctx, paneID)
    if err != nil {
        t.Fatalf("kill-pane failed: %v", err)
    }
}
```

Guard integration tests with build tags. They require a real tmux session.
