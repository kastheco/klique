# Tmux Session Browser Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a TUI overlay (`T` keybind) to browse, kill, and adopt orphaned tmux sessions that aren't tracked by any kasmos Instance.

**Architecture:** New `DiscoverOrphans` function in `session/tmux/` parses `tmux ls` output and subtracts known instance names. New `TmuxBrowserOverlay` in `ui/overlay/` renders a searchable list with kill/adopt/attach actions. App wiring adds `stateTmuxBrowser` state, `T` keybind, and action dispatch handlers.

**Tech Stack:** Go 1.24+, bubbletea v1.3.x, lipgloss v1.1.x

**Size:** Medium (estimated ~4 hours, 5 tasks, 2 waves)

**Design doc:** `docs/plans/2026-02-26-tmux-session-browser-design.md`

---

## Wave 1: Core components
> Foundation: discovery function + overlay component + conventions update. No app wiring yet — these are independently testable units.

### Task 1: Discovery function + conventions update

**Files:**
- Modify: `session/tmux/tmux.go` (add `OrphanSession` struct + `DiscoverOrphans` function after `CleanupSessions`)
- Test: `session/tmux/tmux_test.go` (add `TestDiscoverOrphans` table-driven tests)
- Modify: `CLAUDE.md:20-25` (add arrow-key navigation convention to Standards)
- Modify: `ui/overlay/contextMenu.go:145-169` (remove `"k"` and `"j"` from nav cases)

**Step 1: Write the failing tests**

Add to `session/tmux/tmux_test.go`:

```go
func TestDiscoverOrphans(t *testing.T) {
	tests := []struct {
		name       string
		tmuxOutput string
		tmuxErr    error
		knownNames []string
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "no sessions running",
			tmuxErr:    &exec.ExitError{},
			knownNames: nil,
			wantCount:  0,
		},
		{
			name:       "all sessions tracked",
			tmuxOutput: "kas_foo|1740000000|1|0|80|24\nkas_bar|1740000000|1|0|120|40\n",
			knownNames: []string{"kas_foo", "kas_bar"},
			wantCount:  0,
		},
		{
			name:       "one orphan among tracked",
			tmuxOutput: "kas_foo|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames: []string{"kas_foo"},
			wantCount:  1,
		},
		{
			name:       "non-kas sessions ignored",
			tmuxOutput: "myshell|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames: nil,
			wantCount:  1,
		},
		{
			name:       "attached session detected",
			tmuxOutput: "kas_orphan|1740000000|1|1|80|24\n",
			knownNames: nil,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec := cmd_test.NewMockExecutor()
			if tt.tmuxErr != nil {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return nil, tt.tmuxErr
				}
			} else {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tt.tmuxOutput), nil
				}
			}

			orphans, err := DiscoverOrphans(cmdExec, tt.knownNames)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, orphans, tt.wantCount)
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/tmux/ -run TestDiscoverOrphans -v`
Expected: FAIL — `DiscoverOrphans` not defined.

**Step 3: Implement DiscoverOrphans**

Add to `session/tmux/tmux.go` after `CleanupSessions`:

```go
// OrphanSession represents a kas_ tmux session not tracked by any kasmos Instance.
type OrphanSession struct {
	Name     string    // raw tmux session name, e.g. "kas_auth-refactor-implement"
	Title    string    // human name with "kas_" prefix stripped
	Created  time.Time // session creation time
	Windows  int       // window count
	Attached bool      // whether another client is attached
	Width    int       // pane columns
	Height   int       // pane rows
}

// DiscoverOrphans lists kas_-prefixed tmux sessions that are NOT in knownNames.
// knownNames should contain the sanitized tmux names of all current Instances.
func DiscoverOrphans(cmdExec cmd.Executor, knownNames []string) ([]OrphanSession, error) {
	lsCmd := exec.Command("tmux", "ls", "-F",
		"#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}")
	output, err := cmdExec.Output(lsCmd)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil // no tmux server running
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	known := make(map[string]bool, len(knownNames))
	for _, n := range knownNames {
		known[n] = true
	}

	var orphans []OrphanSession
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, TmuxPrefix) {
			continue
		}
		if known[name] {
			continue
		}

		var created time.Time
		if epoch, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			created = time.Unix(epoch, 0)
		}
		windows, _ := strconv.Atoi(parts[2])
		attached := parts[3] != "0"
		width, _ := strconv.Atoi(parts[4])
		height, _ := strconv.Atoi(parts[5])

		title := strings.TrimPrefix(name, TmuxPrefix)
		orphans = append(orphans, OrphanSession{
			Name:     name,
			Title:    title,
			Created:  created,
			Windows:  windows,
			Attached: attached,
			Width:    width,
			Height:   height,
		})
	}
	return orphans, nil
}

// ToKasTmuxNamePublic is the exported version of toKasTmuxName for use by the app layer.
func ToKasTmuxNamePublic(name string) string {
	return toKasTmuxName(name)
}
```

Add `"strconv"` to the imports in `tmux.go` (it already imports `"strings"` and `"time"`).

**Step 4: Run tests to verify they pass**

Run: `go test ./session/tmux/ -run TestDiscoverOrphans -v`
Expected: PASS

**Step 5: Update conventions**

In `CLAUDE.md`, after the line about lowercase labels, add:
```
- **Arrow-key navigation in overlays**: use ↑↓ for navigation, not j/k vim bindings. Letter keys should always type into search/filter when present.
```

In `ui/overlay/contextMenu.go`, change `HandleKeyPress` navigation cases:
- Line ~145: change `case "up", "k":` to `case "up":`
- Line ~158: change `case "down", "j":` to `case "down":`

**Step 6: Run full test suite for modified packages**

Run: `go test ./session/tmux/ ./ui/overlay/ -v`
Expected: all PASS

**Step 7: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go CLAUDE.md ui/overlay/contextMenu.go
git commit -m "feat: add DiscoverOrphans function and arrow-key nav convention"
```

---

### Task 2: TmuxBrowserOverlay component

**Files:**
- Create: `ui/overlay/tmuxBrowserOverlay.go`
- Create: `ui/overlay/tmuxBrowserOverlay_test.go`

**Step 1: Write the failing tests**

Create `ui/overlay/tmuxBrowserOverlay_test.go`:

```go
package overlay

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxBrowserOverlay_Basic(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now().Add(-10 * time.Minute), Width: 80, Height: 24},
		{Name: "kas_bar", Title: "bar", Created: time.Now().Add(-3 * time.Hour), Width: 120, Height: 40, Attached: true},
	}

	b := NewTmuxBrowserOverlay(items)
	require.NotNil(t, b)

	// Render should not panic and should contain session titles
	rendered := b.Render()
	assert.Contains(t, rendered, "foo")
	assert.Contains(t, rendered, "bar")
}

func TestTmuxBrowserOverlay_Navigation(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	assert.Equal(t, 0, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, b.selectedIdx)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, b.selectedIdx)
}

func TestTmuxBrowserOverlay_SearchFilter(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_auth", Title: "auth", Created: time.Now()},
		{Name: "kas_db", Title: "db", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	assert.Len(t, b.filtered, 2)

	// Type "au" to filter
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	assert.Len(t, b.filtered, 1)
	assert.Equal(t, 0, b.filtered[0]) // index of "auth"
}

func TestTmuxBrowserOverlay_Actions(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}

	tests := []struct {
		name     string
		key      tea.KeyMsg
		expected BrowserAction
	}{
		{"esc dismisses", tea.KeyMsg{Type: tea.KeyEsc}, BrowserDismiss},
		{"enter attaches", tea.KeyMsg{Type: tea.KeyEnter}, BrowserAttach},
		{"k kills when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, BrowserKill},
		{"a adopts when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, BrowserAdopt},
		{"o attaches when search empty", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}, BrowserAttach},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewTmuxBrowserOverlay(items)
			action := b.HandleKeyPress(tt.key)
			assert.Equal(t, tt.expected, action)
		})
	}
}

func TestTmuxBrowserOverlay_ActionKeysTypeWhenSearchActive(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_foo", Title: "foo", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)

	// Type "x" to enter search mode
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.Equal(t, "x", b.searchQuery)

	// Now "k" should type into search, not kill
	action := b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	assert.Equal(t, BrowserNone, action)
	assert.Equal(t, "xk", b.searchQuery)
}

func TestTmuxBrowserOverlay_SelectedItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	assert.Equal(t, "kas_a", b.SelectedItem().Name)

	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "kas_b", b.SelectedItem().Name)
}

func TestTmuxBrowserOverlay_RemoveItem(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_a", Title: "a", Created: time.Now()},
		{Name: "kas_b", Title: "b", Created: time.Now()},
		{Name: "kas_c", Title: "c", Created: time.Now()},
	}
	b := NewTmuxBrowserOverlay(items)
	b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown}) // select "b"

	b.RemoveSelected()
	assert.Len(t, b.sessions, 2)
	assert.Equal(t, "kas_a", b.sessions[0].Name)
	assert.Equal(t, "kas_c", b.sessions[1].Name)
}

func TestTmuxBrowserOverlay_Empty(t *testing.T) {
	b := NewTmuxBrowserOverlay(nil)
	assert.True(t, b.IsEmpty())
	rendered := b.Render()
	assert.Contains(t, rendered, "no sessions")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/overlay/ -run TestTmuxBrowser -v`
Expected: FAIL — types not defined.

**Step 3: Implement TmuxBrowserOverlay**

Create `ui/overlay/tmuxBrowserOverlay.go`:

```go
package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BrowserAction represents what the user chose in the tmux browser.
type BrowserAction int

const (
	BrowserNone    BrowserAction = iota
	BrowserDismiss               // esc
	BrowserKill                  // k (search empty)
	BrowserAdopt                 // a (search empty)
	BrowserAttach                // enter or o (search empty)
)

// TmuxBrowserItem holds metadata for a single orphaned tmux session.
type TmuxBrowserItem struct {
	Name     string
	Title    string
	Created  time.Time
	Windows  int
	Attached bool
	Width    int
	Height   int
}

var browserBorderStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorIris).
	Padding(1, 2)

var browserTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorIris).
	MarginBottom(1)

var browserSearchStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorFoam).
	Padding(0, 1).
	MarginBottom(1)

var browserItemStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(colorText)

var browserSelectedStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Background(colorFoam).
	Foreground(colorBase)

var browserMutedStyle = lipgloss.NewStyle().
	Foreground(colorMuted)

var browserHintStyle = lipgloss.NewStyle().
	Foreground(colorMuted).
	MarginTop(1)

// TmuxBrowserOverlay shows orphaned tmux sessions with kill/adopt/attach actions.
type TmuxBrowserOverlay struct {
	sessions    []TmuxBrowserItem
	filtered    []int // indices into sessions
	selectedIdx int
	searchQuery string
	width       int
}

// NewTmuxBrowserOverlay creates a browser overlay from discovered orphan sessions.
func NewTmuxBrowserOverlay(items []TmuxBrowserItem) *TmuxBrowserOverlay {
	b := &TmuxBrowserOverlay{
		sessions: items,
		width:    56,
	}
	b.applyFilter()
	return b
}

func (b *TmuxBrowserOverlay) applyFilter() {
	b.filtered = nil
	query := strings.ToLower(b.searchQuery)
	for i, item := range b.sessions {
		if query == "" || strings.Contains(strings.ToLower(item.Title), query) {
			b.filtered = append(b.filtered, i)
		}
	}
	if b.selectedIdx >= len(b.filtered) {
		b.selectedIdx = len(b.filtered) - 1
	}
	if b.selectedIdx < 0 {
		b.selectedIdx = 0
	}
}

// HandleKeyPress processes input and returns the action to take.
func (b *TmuxBrowserOverlay) HandleKeyPress(msg tea.KeyMsg) BrowserAction {
	switch msg.Type {
	case tea.KeyEsc:
		if b.searchQuery != "" {
			b.searchQuery = ""
			b.applyFilter()
			return BrowserNone
		}
		return BrowserDismiss
	case tea.KeyEnter:
		if len(b.filtered) > 0 {
			return BrowserAttach
		}
		return BrowserNone
	case tea.KeyUp:
		if b.selectedIdx > 0 {
			b.selectedIdx--
		}
		return BrowserNone
	case tea.KeyDown:
		if b.selectedIdx < len(b.filtered)-1 {
			b.selectedIdx++
		}
		return BrowserNone
	case tea.KeyBackspace:
		if len(b.searchQuery) > 0 {
			runes := []rune(b.searchQuery)
			b.searchQuery = string(runes[:len(runes)-1])
			b.applyFilter()
		}
		return BrowserNone
	case tea.KeyRunes:
		r := string(msg.Runes)
		// Action keys only fire when search is empty
		if b.searchQuery == "" {
			switch r {
			case "k":
				if len(b.filtered) > 0 {
					return BrowserKill
				}
				return BrowserNone
			case "a":
				if len(b.filtered) > 0 {
					return BrowserAdopt
				}
				return BrowserNone
			case "o":
				if len(b.filtered) > 0 {
					return BrowserAttach
				}
				return BrowserNone
			}
		}
		// All other runes type into search
		b.searchQuery += r
		b.applyFilter()
		return BrowserNone
	}
	return BrowserNone
}

// SelectedItem returns the currently highlighted session, or a zero value if empty.
func (b *TmuxBrowserOverlay) SelectedItem() TmuxBrowserItem {
	if len(b.filtered) == 0 || b.selectedIdx >= len(b.filtered) {
		return TmuxBrowserItem{}
	}
	return b.sessions[b.filtered[b.selectedIdx]]
}

// RemoveSelected removes the currently selected item from the list.
func (b *TmuxBrowserOverlay) RemoveSelected() {
	if len(b.filtered) == 0 || b.selectedIdx >= len(b.filtered) {
		return
	}
	idx := b.filtered[b.selectedIdx]
	b.sessions = append(b.sessions[:idx], b.sessions[idx+1:]...)
	b.applyFilter()
}

// IsEmpty returns true if there are no sessions to display.
func (b *TmuxBrowserOverlay) IsEmpty() bool {
	return len(b.sessions) == 0
}

// Render draws the browser overlay.
func (b *TmuxBrowserOverlay) Render() string {
	var s strings.Builder

	s.WriteString(browserTitleStyle.Render("tmux sessions"))
	s.WriteString("\n")

	// Search bar
	innerWidth := b.width - 8
	if innerWidth < 10 {
		innerWidth = 10
	}
	searchText := b.searchQuery
	if searchText == "" {
		searchText = browserMutedStyle.Render("\uf002 type to filter...")
	}
	s.WriteString(browserSearchStyle.Width(innerWidth).Render(searchText))
	s.WriteString("\n")

	// Items
	if len(b.filtered) == 0 {
		s.WriteString(browserMutedStyle.Render("  no sessions"))
		s.WriteString("\n")
	} else {
		for i, idx := range b.filtered {
			item := b.sessions[idx]
			age := relativeTime(item.Created)
			dims := fmt.Sprintf("%d×%d", item.Width, item.Height)

			attachedIndicator := "  "
			if item.Attached {
				attachedIndicator = "● "
			}

			label := fmt.Sprintf("%-28s %8s %s%s",
				truncateStr(item.Title, 28), age, attachedIndicator, dims)

			if i == b.selectedIdx {
				s.WriteString(browserSelectedStyle.Width(innerWidth).Render("▸ " + label))
			} else {
				s.WriteString(browserItemStyle.Width(innerWidth).Render("  " + label))
			}
			s.WriteString("\n")
		}
	}

	s.WriteString(browserHintStyle.Render("↑↓ navigate • k kill • a adopt • o attach • esc close"))

	return browserBorderStyle.Width(b.width).Render(s.String())
}

// SetSize updates the overlay width.
func (b *TmuxBrowserOverlay) SetSize(width, height int) {
	b.width = width
}

// truncateStr truncates s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// relativeTime returns a human-readable relative time string.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/overlay/ -run TestTmuxBrowser -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add ui/overlay/tmuxBrowserOverlay.go ui/overlay/tmuxBrowserOverlay_test.go
git commit -m "feat: add TmuxBrowserOverlay component"
```

---

## Wave 2: App integration
> **Depends on Wave 1:** `DiscoverOrphans` and `TmuxBrowserOverlay` must exist before wiring the keybind, state, and action handlers.

### Task 3: Keybind registration + state + discovery wiring

**Files:**
- Modify: `keys/keys.go` (add `KeyTmuxBrowser` constant + `"T"` mapping + binding)
- Modify: `app/app.go` (add `stateTmuxBrowser` to state enum, `tmuxBrowser` field on `home`, `tmuxOrphansMsg` type, render case in `View()`)
- Modify: `app/app_input.go` (add `stateTmuxBrowser` to menu highlighting guard, add state handler + `keys.KeyTmuxBrowser` case in default dispatch)
- Modify: `app/app_state.go` (add `discoverTmuxOrphans` method)
- Modify: `session/tmux/tmux.go` (export `ToKasTmuxNamePublic`)

**Step 1: Add keybind**

In `keys/keys.go`:

Add `KeyTmuxBrowser` to the `KeyName` const block (after `KeySpaceExpand`):
```go
KeyTmuxBrowser // T - browse orphaned tmux sessions
```

Add to `GlobalKeyStringsMap`:
```go
"T": KeyTmuxBrowser,
```

Add to `GlobalkeyBindings`:
```go
KeyTmuxBrowser: key.NewBinding(
    key.WithKeys("T"),
    key.WithHelp("T", "tmux sessions"),
),
```

**Step 2: Add state, field, and message type**

In `app/app.go`:

Add `stateTmuxBrowser` to the state enum (after `statePermission`):
```go
// stateTmuxBrowser is the state when the tmux session browser overlay is shown.
stateTmuxBrowser
```

Add `tmuxBrowser` field to `home` struct (near the other overlay fields like `pickerOverlay`, `contextMenu`):
```go
// tmuxBrowser is the tmux session browser overlay.
tmuxBrowser *overlay.TmuxBrowserOverlay
```

Add message type (near other message types):
```go
// tmuxOrphansMsg carries discovered orphaned tmux sessions.
type tmuxOrphansMsg struct {
    sessions []tmux.OrphanSession
    err      error
}
```

Add render case in `View()` switch (before the `default:` case):
```go
case m.state == stateTmuxBrowser && m.tmuxBrowser != nil:
    result = overlay.PlaceOverlay(0, 0, m.tmuxBrowser.Render(), mainView, true, true)
```

Handle the `tmuxOrphansMsg` in the `Update` switch:
```go
case tmuxOrphansMsg:
    if msg.err != nil {
        return m, m.handleError(msg.err)
    }
    if len(msg.sessions) == 0 {
        m.toastManager.Info("no orphaned tmux sessions found")
        return m, m.toastTickCmd()
    }
    items := make([]overlay.TmuxBrowserItem, len(msg.sessions))
    for i, s := range msg.sessions {
        items[i] = overlay.TmuxBrowserItem{
            Name:     s.Name,
            Title:    s.Title,
            Created:  s.Created,
            Windows:  s.Windows,
            Attached: s.Attached,
            Width:    s.Width,
            Height:   s.Height,
        }
    }
    m.tmuxBrowser = overlay.NewTmuxBrowserOverlay(items)
    m.state = stateTmuxBrowser
    return m, nil
```

**Step 3: Wire input handling**

In `app/app_input.go`:

Add `stateTmuxBrowser` to the menu highlighting guard (the long `if` on line ~27):
```go
m.state == stateTmuxBrowser
```

Add a `stateTmuxBrowser` handler block in `handleKeyPress` (before the `stateSearch` handler):
```go
if m.state == stateTmuxBrowser {
    if m.tmuxBrowser == nil {
        m.state = stateDefault
        return m, nil
    }
    action := m.tmuxBrowser.HandleKeyPress(msg)
    return m.handleTmuxBrowserAction(action)
}
```

Add `keys.KeyTmuxBrowser` case in the default state switch (near `keys.KeyRepoSwitch`):
```go
case keys.KeyTmuxBrowser:
    return m, m.discoverTmuxOrphans()
```

**Step 4: Add discovery command**

Add to `app/app_state.go`:
```go
// discoverTmuxOrphans returns a tea.Cmd that lists orphaned kas_ tmux sessions.
func (m *home) discoverTmuxOrphans() tea.Cmd {
    knownNames := make([]string, 0, len(m.allInstances))
    for _, inst := range m.allInstances {
        if inst.Started() && inst.TmuxAlive() {
            knownNames = append(knownNames, tmux.ToKasTmuxNamePublic(inst.Title))
        }
    }
    return func() tea.Msg {
        orphans, err := tmux.DiscoverOrphans(cmd2.MakeExecutor(), knownNames)
        return tmuxOrphansMsg{sessions: orphans, err: err}
    }
}
```

Note: `ToKasTmuxNamePublic` was already added in Task 1.

**Step 5: Verify compilation**

Run: `go build ./...`
Expected: compiles without errors.

**Step 6: Commit**

```bash
git add keys/keys.go app/app.go app/app_input.go app/app_state.go
git commit -m "feat: wire T keybind, stateTmuxBrowser, and orphan discovery"
```

---

### Task 4: Action handlers (kill, adopt, attach)

**Files:**
- Modify: `app/app_state.go` (add `handleTmuxBrowserAction`, `adoptOrphanSession`)
- Modify: `app/app.go` (add `tmuxKillResultMsg`, `tmuxAttachReturnMsg` types, handle them in `Update`)
- Modify: `session/instance_lifecycle.go` (add `AdoptOrphanTmuxSession`)
- Modify: `session/tmux/tmux.go` (add `NewTmuxSessionFromExisting`)
- Test: `app/app_test.go` (add tests for browser action dispatch)

**Step 1: Write tests**

Add to `app/app_test.go`:

```go
func TestTmuxBrowserActions(t *testing.T) {
	t.Run("tmuxOrphansMsg with no orphans shows toast", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxOrphansMsg{sessions: nil}
		model, _ := h.Update(msg)
		hm := model.(*home)
		assert.Nil(t, hm.tmuxBrowser)
		assert.Equal(t, stateDefault, hm.state)
	})

	t.Run("tmuxOrphansMsg with orphans opens browser", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxOrphansMsg{
			sessions: []tmux.OrphanSession{
				{Name: "kas_test", Title: "test", Width: 80, Height: 24},
			},
		}
		model, _ := h.Update(msg)
		hm := model.(*home)
		assert.NotNil(t, hm.tmuxBrowser)
		assert.Equal(t, stateTmuxBrowser, hm.state)
	})

	t.Run("dismiss returns to default state", func(t *testing.T) {
		h := newTestHome()
		h.tmuxBrowser = overlay.NewTmuxBrowserOverlay([]overlay.TmuxBrowserItem{
			{Name: "kas_test", Title: "test"},
		})
		h.state = stateTmuxBrowser
		model, _ := h.handleTmuxBrowserAction(overlay.BrowserDismiss)
		hm := model.(*home)
		assert.Nil(t, hm.tmuxBrowser)
		assert.Equal(t, stateDefault, hm.state)
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./app/ -run TestTmuxBrowser -v`
Expected: FAIL — `handleTmuxBrowserAction` not defined.

**Step 3: Implement action handlers**

Add message types to `app/app.go`:
```go
// tmuxKillResultMsg is sent after an orphaned tmux session is killed.
type tmuxKillResultMsg struct {
    name string
    err  error
}

// tmuxAttachReturnMsg is sent when the user detaches from a passively attached orphan session.
type tmuxAttachReturnMsg struct{}
```

Handle the result messages in `app.go` `Update` switch:
```go
case tmuxKillResultMsg:
    if msg.err != nil {
        m.toastManager.Error(fmt.Sprintf("failed to kill session: %v", msg.err))
    } else {
        m.toastManager.Success(fmt.Sprintf("killed session '%s'", msg.name))
    }
    return m, m.toastTickCmd()

case tmuxAttachReturnMsg:
    m.toastManager.Info("detached from tmux session")
    return m, tea.Batch(tea.WindowSize(), m.toastTickCmd())
```

Add to `app/app_state.go`:
```go
// handleTmuxBrowserAction dispatches the action from the tmux browser overlay.
func (m *home) handleTmuxBrowserAction(action overlay.BrowserAction) (tea.Model, tea.Cmd) {
    switch action {
    case overlay.BrowserDismiss:
        m.tmuxBrowser = nil
        m.state = stateDefault
        return m, nil

    case overlay.BrowserKill:
        selected := m.tmuxBrowser.SelectedItem()
        if selected.Name == "" {
            return m, nil
        }
        name := selected.Name
        m.tmuxBrowser.RemoveSelected()
        if m.tmuxBrowser.IsEmpty() {
            m.tmuxBrowser = nil
            m.state = stateDefault
        }
        return m, func() tea.Msg {
            killCmd := exec.Command("tmux", "kill-session", "-t", name)
            err := cmd2.MakeExecutor().Run(killCmd)
            return tmuxKillResultMsg{name: name, err: err}
        }

    case overlay.BrowserAdopt:
        selected := m.tmuxBrowser.SelectedItem()
        if selected.Name == "" {
            return m, nil
        }
        m.tmuxBrowser = nil
        m.state = stateDefault
        return m.adoptOrphanSession(selected)

    case overlay.BrowserAttach:
        selected := m.tmuxBrowser.SelectedItem()
        if selected.Name == "" {
            return m, nil
        }
        m.tmuxBrowser = nil
        m.state = stateDefault
        name := selected.Name
        return m, func() tea.Msg {
            attachCmd := exec.Command("tmux", "attach-session", "-t", name)
            attachCmd.Stdin = os.Stdin
            attachCmd.Stdout = os.Stdout
            attachCmd.Stderr = os.Stderr
            _ = attachCmd.Run()
            return tmuxAttachReturnMsg{}
        }
    }
    return m, nil
}

// adoptOrphanSession creates a new Instance backed by an existing orphaned tmux session.
func (m *home) adoptOrphanSession(item overlay.TmuxBrowserItem) (tea.Model, tea.Cmd) {
    inst, err := session.NewInstance(session.InstanceOptions{
        Title:   item.Title,
        Path:    m.activeRepoPath,
        Program: "unknown",
    })
    if err != nil {
        return m, m.handleError(err)
    }

    m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))
    m.nav.SelectInstance(inst)

    m.toastManager.Info(fmt.Sprintf("adopting session '%s'", item.Title))

    return m, func() tea.Msg {
        err := inst.AdoptOrphanTmuxSession(item.Name)
        return instanceStartedMsg{instance: inst, err: err}
    }
}
```

Add required imports to `app/app_state.go`:
```go
"os"
"os/exec"
```

Add `NewTmuxSessionFromExisting` to `session/tmux/tmux.go`:
```go
// NewTmuxSessionFromExisting creates a TmuxSession that wraps an already-running
// tmux session identified by its raw sanitized name. Used for adopting orphans.
func NewTmuxSessionFromExisting(sanitizedName string, program string, skipPermissions bool) *TmuxSession {
    return &TmuxSession{
        sanitizedName:   sanitizedName,
        program:         program,
        skipPermissions: skipPermissions,
        ptyFactory:      MakePtyFactory(),
        cmdExec:         cmd.MakeExecutor(),
    }
}
```

Add `AdoptOrphanTmuxSession` to `session/instance_lifecycle.go`:
```go
// AdoptOrphanTmuxSession connects this instance to an existing orphaned tmux session
// identified by its raw tmux name. No worktree is created — the session keeps its
// existing working directory.
func (i *Instance) AdoptOrphanTmuxSession(tmuxName string) error {
    ts := tmux.NewTmuxSessionFromExisting(tmuxName, i.Program, i.SkipPermissions)
    i.tmuxSession = ts
    if err := ts.Restore(); err != nil {
        return fmt.Errorf("failed to adopt orphan session %s: %w", tmuxName, err)
    }
    i.started = true
    i.SetStatus(Ready)
    return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./app/ -run TestTmuxBrowser -v`
Expected: all PASS

**Step 5: Run full build + test**

Run: `go build ./... && go test ./...`
Expected: compiles and tests pass.

**Step 6: Commit**

```bash
git add app/app.go app/app_state.go app/app_test.go session/instance_lifecycle.go session/tmux/tmux.go
git commit -m "feat: implement kill, adopt, attach action handlers for tmux browser"
```

---

### Task 5: Help screen update + final polish

**Files:**
- Modify: `app/help.go` (add `T` to help screen)
- Modify: `app/app_input.go` (add click-outside dismiss for `stateTmuxBrowser`)

**Step 1: Update help screen**

In `app/help.go`, in `helpTypeGeneral.toContent()`, add after the `keyStyle.Render("R")` line in the "sessions" section:
```go
keyStyle.Render("T")+descStyle.Render("             - browse orphaned tmux sessions"),
```

**Step 2: Add click-outside dismiss**

In `app/app_input.go`, in `handleMouse`, add alongside the existing click-outside handlers (near line ~77):
```go
if m.state == stateTmuxBrowser && msg.Button == tea.MouseButtonLeft {
    m.tmuxBrowser = nil
    m.state = stateDefault
    return m, nil
}
```

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 4: Run build**

Run: `go build ./...`
Expected: compiles cleanly.

**Step 5: Commit**

```bash
git add app/help.go app/app_input.go
git commit -m "feat: add tmux browser to help screen and click-outside dismiss"
```
