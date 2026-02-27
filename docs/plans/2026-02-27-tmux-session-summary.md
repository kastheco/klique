# Tmux Session Summary Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show all `kas_`-prefixed tmux sessions in the `T` overlay (managed + orphaned) with contextual actions, and display a persistent session count in the status bar.

**Architecture:** Extend the existing `DiscoverOrphans` pattern into `DiscoverAll` that returns all `kas_` sessions with a `Managed` flag. Enrich managed entries at the app layer with plan/agent metadata. Add a `CountKasSessions` helper piggybacked into the metadata poll goroutine for the status bar count.

**Tech Stack:** Go, bubbletea, lipgloss, tmux CLI

**Size:** Medium (estimated ~3 hours, 4 tasks, no waves)

---

### Task 1: `DiscoverAll` + `CountKasSessions` in tmux package

**Files:**
- Modify: `session/tmux/tmux.go:460-527` (add new type + functions after existing `DiscoverOrphans`)
- Test: `session/tmux/tmux_test.go`

**Step 1: Write the failing tests**

Add to `session/tmux/tmux_test.go` after the existing `TestDiscoverOrphans` function:

```go
func TestDiscoverAll(t *testing.T) {
	tests := []struct {
		name       string
		tmuxOutput string
		tmuxErr    error
		knownNames []string
		wantTotal  int
		wantManaged int
		wantErr    bool
	}{
		{
			name:        "no sessions running",
			tmuxErr:     &exec.ExitError{},
			knownNames:  nil,
			wantTotal:   0,
			wantManaged: 0,
		},
		{
			name:        "all sessions managed",
			tmuxOutput:  "kas_foo|1740000000|1|0|80|24\nkas_bar|1740000000|1|0|120|40\n",
			knownNames:  []string{"kas_foo", "kas_bar"},
			wantTotal:   2,
			wantManaged: 2,
		},
		{
			name:        "mix of managed and orphaned",
			tmuxOutput:  "kas_foo|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames:  []string{"kas_foo"},
			wantTotal:   2,
			wantManaged: 1,
		},
		{
			name:        "non-kas sessions ignored",
			tmuxOutput:  "myshell|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames:  nil,
			wantTotal:   1,
			wantManaged: 0,
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

			sessions, err := DiscoverAll(cmdExec, tt.knownNames)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, sessions, tt.wantTotal)

			managedCount := 0
			for _, s := range sessions {
				if s.Managed {
					managedCount++
				}
			}
			assert.Equal(t, tt.wantManaged, managedCount)
		})
	}
}

func TestCountKasSessions(t *testing.T) {
	tests := []struct {
		name       string
		tmuxOutput string
		tmuxErr    error
		want       int
	}{
		{
			name:    "no tmux server",
			tmuxErr: &exec.ExitError{},
			want:    0,
		},
		{
			name:       "two kas sessions one foreign",
			tmuxOutput: "kas_foo:1 windows\nkas_bar:1 windows\nmyshell:2 windows\n",
			want:       2,
		},
		{
			name:       "no kas sessions",
			tmuxOutput: "myshell:1 windows\n",
			want:       0,
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

			count := CountKasSessions(cmdExec)
			assert.Equal(t, tt.want, count)
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/tmux/ -run 'TestDiscoverAll|TestCountKasSessions' -v`
Expected: FAIL — `DiscoverAll` and `CountKasSessions` undefined.

**Step 3: Implement `SessionInfo`, `DiscoverAll`, and `CountKasSessions`**

Add to `session/tmux/tmux.go` after `DiscoverOrphans` (line ~527):

```go
// SessionInfo represents any kas_ tmux session (managed or orphaned).
type SessionInfo struct {
	Name     string    // raw tmux session name, e.g. "kas_auth-refactor-implement"
	Title    string    // human name with "kas_" prefix stripped
	Created  time.Time // session creation time
	Windows  int       // window count
	Attached bool      // whether another client is attached
	Width    int       // pane columns
	Height   int       // pane rows
	Managed  bool      // true if matched a known instance name
}

// DiscoverAll lists all kas_-prefixed tmux sessions, marking each as Managed
// if its name appears in knownNames. knownNames should contain the sanitized
// tmux names of all current Instances (e.g. from ToKasTmuxNamePublic).
func DiscoverAll(cmdExec cmd.Executor, knownNames []string) ([]SessionInfo, error) {
	lsCmd := exec.Command("tmux", "ls", "-F",
		"#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}")
	output, err := cmdExec.Output(lsCmd)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	known := make(map[string]bool, len(knownNames))
	for _, n := range knownNames {
		known[n] = true
	}

	var sessions []SessionInfo
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

		var created time.Time
		if epoch, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			created = time.Unix(epoch, 0)
		}
		windows, _ := strconv.Atoi(parts[2])
		attached := parts[3] != "0"
		width, _ := strconv.Atoi(parts[4])
		height, _ := strconv.Atoi(parts[5])

		sessions = append(sessions, SessionInfo{
			Name:     name,
			Title:    strings.TrimPrefix(name, TmuxPrefix),
			Created:  created,
			Windows:  windows,
			Attached: attached,
			Width:    width,
			Height:   height,
			Managed:  known[name],
		})
	}
	return sessions, nil
}

// CountKasSessions returns the number of kas_-prefixed tmux sessions.
// Returns 0 if no tmux server is running or on any error.
func CountKasSessions(cmdExec cmd.Executor) int {
	lsCmd := exec.Command("tmux", "ls")
	output, err := cmdExec.Output(lsCmd)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.HasPrefix(line, TmuxPrefix) {
			count++
		}
	}
	return count
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./session/tmux/ -run 'TestDiscoverAll|TestCountKasSessions' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add session/tmux/tmux.go session/tmux/tmux_test.go
git commit -m "feat: add DiscoverAll and CountKasSessions to tmux package"
```

---

### Task 2: Unified overlay — extend `TmuxBrowserOverlay` and item type

**Files:**
- Modify: `ui/overlay/tmuxBrowserOverlay.go` (add fields to `TmuxBrowserItem`, update rendering + action logic)
- Test: `ui/overlay/tmuxBrowserOverlay_test.go`

**Step 1: Write the failing tests**

Add to `ui/overlay/tmuxBrowserOverlay_test.go`:

```go
func TestTmuxBrowserOverlay_ManagedItemBlocksAdopt(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_managed", Title: "managed", Created: time.Now(), Managed: true, AgentType: "coder"},
	}
	b := NewTmuxBrowserOverlay(items)

	// "a" should be a no-op for managed items
	action := b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	assert.Equal(t, BrowserNone, action)
}

func TestTmuxBrowserOverlay_OrphanItemAllowsAdopt(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_orphan", Title: "orphan", Created: time.Now(), Managed: false},
	}
	b := NewTmuxBrowserOverlay(items)

	action := b.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	assert.Equal(t, BrowserAdopt, action)
}

func TestTmuxBrowserOverlay_ManagedItemRendersAgentType(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_auth", Title: "auth", Created: time.Now(), Managed: true, AgentType: "coder", PlanFile: "auth-plan"},
	}
	b := NewTmuxBrowserOverlay(items)
	rendered := b.Render()
	assert.Contains(t, rendered, "coder")
}

func TestTmuxBrowserOverlay_MixedItems(t *testing.T) {
	items := []TmuxBrowserItem{
		{Name: "kas_managed", Title: "managed", Created: time.Now(), Managed: true, AgentType: "planner"},
		{Name: "kas_orphan", Title: "orphan", Created: time.Now(), Managed: false},
	}
	b := NewTmuxBrowserOverlay(items)
	rendered := b.Render()
	assert.Contains(t, rendered, "managed")
	assert.Contains(t, rendered, "orphan")
	assert.Contains(t, rendered, "planner")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/overlay/ -run 'TestTmuxBrowserOverlay_Managed|TestTmuxBrowserOverlay_Orphan|TestTmuxBrowserOverlay_Mixed' -v`
Expected: FAIL — `Managed`, `AgentType`, `PlanFile` fields don't exist on `TmuxBrowserItem`.

**Step 3: Implement the overlay changes**

In `ui/overlay/tmuxBrowserOverlay.go`:

1. Add enrichment fields to `TmuxBrowserItem`:

```go
type TmuxBrowserItem struct {
	Name      string
	Title     string
	Created   time.Time
	Windows   int
	Attached  bool
	Width     int
	Height    int
	Managed   bool   // true = tracked by a kasmos instance
	PlanFile  string // plan filename (managed only)
	AgentType string // "planner"/"coder"/"reviewer" (managed only)
	Status    string // "running"/"ready"/"loading"/"paused" (managed only)
}
```

2. Update `HandleKeyPress` — the `"a"` (adopt) case should check whether the selected item is managed:

```go
case "a":
	if len(b.filtered) > 0 && !b.SelectedItem().Managed {
		return BrowserAdopt
	}
	return BrowserNone
```

3. Update `Render` — show agent type badge for managed items and adjust the hint bar:

In the item rendering loop, after computing `label`:

```go
// Badge for managed items
badge := ""
if item.Managed {
	badgeText := "managed"
	if item.AgentType != "" {
		badgeText = item.AgentType
	}
	badge = browserMutedStyle.Render(" [" + badgeText + "]")
}

label := fmt.Sprintf("%-28s %8s %s%s",
	truncateStr(item.Title, 28), age, attachedIndicator, dims) + badge
```

For the hint bar, render contextually:

```go
hint := "↑↓ navigate · k kill · o attach · esc close"
if len(b.filtered) > 0 && !b.SelectedItem().Managed {
	hint = "↑↓ navigate · k kill · a adopt · o attach · esc close"
}
s.WriteString(browserHintStyle.Render(hint))
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/overlay/ -run TestTmuxBrowserOverlay -v`
Expected: ALL PASS (new tests + existing tests)

**Step 5: Commit**

```bash
git add ui/overlay/tmuxBrowserOverlay.go ui/overlay/tmuxBrowserOverlay_test.go
git commit -m "feat: extend tmux browser overlay to show managed sessions"
```

---

### Task 3: Wire unified discovery + session count into the app layer

**Files:**
- Modify: `app/app.go:1456-1469` (rename msg type, add `TmuxSessionCount` to metadata msg)
- Modify: `app/app_state.go:1618-1630` (rename `discoverTmuxOrphans` → `discoverTmuxSessions`, enrich items)
- Modify: `app/app.go:543-614` (add `CountKasSessions` to metadata poll goroutine)
- Modify: `app/app.go:1138-1163` (handle new msg type, enrich from `allInstances`)
- Test: `app/app_test.go:1244-1278` (update existing test names + add new test for managed sessions)

**Step 1: Write the failing tests**

Update existing tests in `app/app_test.go` and add new ones. Rename `tmuxOrphansMsg` references to `tmuxSessionsMsg`:

```go
func TestTmuxBrowserActions(t *testing.T) {
	t.Run("tmuxSessionsMsg with no sessions shows toast", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxSessionsMsg{sessions: nil}
		model, _ := h.Update(msg)
		hm := model.(*home)
		assert.Nil(t, hm.tmuxBrowser)
		assert.Equal(t, stateDefault, hm.state)
	})

	t.Run("tmuxSessionsMsg with sessions opens browser", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxSessionsMsg{
			sessions: []tmux.SessionInfo{
				{Name: "kas_test", Title: "test", Width: 80, Height: 24, Managed: false},
			},
		}
		model, _ := h.Update(msg)
		hm := model.(*home)
		assert.NotNil(t, hm.tmuxBrowser)
		assert.Equal(t, stateTmuxBrowser, hm.state)
	})

	t.Run("managed sessions are enriched with instance metadata", func(t *testing.T) {
		h := newTestHome()
		inst, _ := session.NewInstance(session.InstanceOptions{
			Title:   "auth-impl",
			Path:    "/tmp",
			Program: "claude",
		})
		inst.PlanFile = "2026-02-27-auth.md"
		inst.AgentType = session.AgentTypeCoder
		inst.MarkStartedForTest()
		inst.SetTmuxSession(tmux.NewTmuxSession("auth-impl", "claude", false))
		h.allInstances = append(h.allInstances, inst)

		msg := tmuxSessionsMsg{
			sessions: []tmux.SessionInfo{
				{Name: "kas_auth-impl", Title: "auth-impl", Width: 80, Height: 24, Managed: true},
			},
		}
		model, _ := h.Update(msg)
		hm := model.(*home)
		require.NotNil(t, hm.tmuxBrowser)
		item := hm.tmuxBrowser.SelectedItem()
		assert.True(t, item.Managed)
		assert.Equal(t, "coder", item.AgentType)
		assert.Equal(t, "2026-02-27-auth.md", item.PlanFile)
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

Run: `go test ./app/ -run TestTmuxBrowserActions -v`
Expected: FAIL — `tmuxSessionsMsg` type doesn't exist yet.

**Step 3: Implement the app layer changes**

1. **Rename message type** in `app/app.go` (~line 1456):

Replace:
```go
type tmuxOrphansMsg struct {
	sessions []tmux.OrphanSession
	err      error
}
```
With:
```go
type tmuxSessionsMsg struct {
	sessions []tmux.SessionInfo
	err      error
}
```

2. **Add `TmuxSessionCount` to `metadataResultMsg`** in `app/app.go` (~line 1529):

```go
type metadataResultMsg struct {
	Results          []instanceMetadata
	PlanState        *planstate.PlanState
	Signals          []planfsm.Signal
	TmuxSessionCount int
}
```

3. **Add `tmuxSessionCount` field to `home` struct** in `app/app.go` (~after line 124):

```go
// tmuxSessionCount is the latest count of kas_-prefixed tmux sessions.
tmuxSessionCount int
```

4. **Piggyback count into metadata poll** in `app/app.go` (~line 613, inside the metadata goroutine). Before the `time.Sleep`:

```go
tmuxCount := tmux.CountKasSessions(cmd2.MakeExecutor())
```

And in the return:
```go
return metadataResultMsg{Results: results, PlanState: ps, Signals: signals, TmuxSessionCount: tmuxCount}
```

5. **Store count in Update handler** (`app/app.go`, in `case metadataResultMsg:` block, ~line 616):

```go
m.tmuxSessionCount = msg.TmuxSessionCount
```

6. **Rename `discoverTmuxOrphans` → `discoverTmuxSessions`** in `app/app_state.go` (~line 1618):

```go
func (m *home) discoverTmuxSessions() tea.Cmd {
	knownNames := make([]string, 0, len(m.allInstances))
	for _, inst := range m.allInstances {
		if inst.Started() && inst.TmuxAlive() {
			knownNames = append(knownNames, tmux.ToKasTmuxNamePublic(inst.Title))
		}
	}
	return func() tea.Msg {
		sessions, err := tmux.DiscoverAll(cmd2.MakeExecutor(), knownNames)
		return tmuxSessionsMsg{sessions: sessions, err: err}
	}
}
```

7. **Update call site** in `app/app_input.go` (~line 1313):

Replace `m.discoverTmuxOrphans()` with `m.discoverTmuxSessions()`.

8. **Update `tmuxSessionsMsg` handler** in `app/app.go` (~line 1138). Replace the `tmuxOrphansMsg` case:

```go
case tmuxSessionsMsg:
	if msg.err != nil {
		return m, m.handleError(msg.err)
	}
	if len(msg.sessions) == 0 {
		if m.toastManager != nil {
			m.toastManager.Info("no kas tmux sessions found")
			return m, m.toastTickCmd()
		}
		return m, nil
	}
	// Build instance lookup for enrichment.
	instMap := make(map[string]*session.Instance, len(m.allInstances))
	for _, inst := range m.allInstances {
		if inst.Started() {
			instMap[tmux.ToKasTmuxNamePublic(inst.Title)] = inst
		}
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
			Managed:  s.Managed,
		}
		if inst, ok := instMap[s.Name]; ok {
			items[i].PlanFile = inst.PlanFile
			items[i].AgentType = inst.AgentType
			items[i].Status = statusString(inst.Status)
		}
	}
	m.tmuxBrowser = overlay.NewTmuxBrowserOverlay(items)
	m.state = stateTmuxBrowser
	return m, nil
```

Add a small helper near the handler (or in `app_state.go`):

```go
func statusString(s session.Status) string {
	switch s {
	case session.Running:
		return "running"
	case session.Ready:
		return "ready"
	case session.Loading:
		return "loading"
	case session.Paused:
		return "paused"
	default:
		return ""
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./app/ -run TestTmuxBrowserActions -v`
Expected: PASS

Then run the full test suite:
Run: `go test ./...`
Expected: PASS (no regressions)

**Step 5: Commit**

```bash
git add app/app.go app/app_state.go app/app_input.go app/app_test.go
git commit -m "feat: wire unified tmux session discovery into app layer"
```

---

### Task 4: Session count in status bar

**Files:**
- Modify: `ui/statusbar.go:20-28` (add `TmuxSessionCount` to `StatusBarData`)
- Modify: `ui/statusbar.go:115-130` (render count in left group)
- Modify: `app/app_state.go:58-121` (feed count into `computeStatusBarData`)
- Test: `ui/statusbar_test.go`

**Step 1: Write the failing test**

Add to `ui/statusbar_test.go`:

```go
func TestStatusBar_TmuxSessionCount(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "main",
		TmuxSessionCount: 3,
	})

	plain := stripANSI(sb.String())
	assert.Contains(t, plain, "tmux:3")
}

func TestStatusBar_TmuxSessionCountZeroHidden(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(100)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "main",
		TmuxSessionCount: 0,
	})

	plain := stripANSI(sb.String())
	assert.NotContains(t, plain, "tmux:")
}

func TestStatusBar_TmuxSessionCountWithPlanStatus(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSize(120)
	sb.SetData(StatusBarData{
		RepoName:         "kasmos",
		Branch:           "feat/auth",
		PlanStatus:       "implementing",
		TmuxSessionCount: 5,
	})

	plain := stripANSI(sb.String())
	assert.Contains(t, plain, "implementing")
	assert.Contains(t, plain, "tmux:5")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run 'TestStatusBar_TmuxSession' -v`
Expected: FAIL — `TmuxSessionCount` field doesn't exist on `StatusBarData`.

**Step 3: Implement**

1. Add field to `StatusBarData` in `ui/statusbar.go` (~line 20):

```go
type StatusBarData struct {
	RepoName         string
	Branch           string
	PlanName         string
	PlanStatus       string
	WaveLabel        string
	TaskGlyphs       []TaskGlyph
	FocusMode        bool
	TmuxSessionCount int // total kas_ tmux sessions (0 = hide)
}
```

2. Add tmux count rendering to `leftStatusGroup()` in `ui/statusbar.go` (~line 115). After the existing returns, add the tmux count as a fallback or append. The cleanest approach is to build the left group as parts and join them:

Replace `leftStatusGroup()` with:

```go
func (s *StatusBar) leftStatusGroup() string {
	var parts []string

	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		glyphParts := make([]string, 0, len(s.data.TaskGlyphs))
		for _, g := range s.data.TaskGlyphs {
			glyphParts = append(glyphParts, taskGlyphStr(g))
		}
		glyphs := strings.Join(glyphParts, " ")
		parts = append(parts, glyphs+" "+statusBarWaveLabelStyle.Render(s.data.WaveLabel))
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	if s.data.TmuxSessionCount > 0 {
		tmuxLabel := fmt.Sprintf("tmux:%d", s.data.TmuxSessionCount)
		parts = append(parts, statusBarTmuxCountStyle.Render(tmuxLabel))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, statusBarSepStyle.Render(" · "))
}
```

Add the style variable near the other status bar styles:

```go
var statusBarTmuxCountStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Background(ColorSurface)
```

Add `"fmt"` to the import block if not already present.

3. Feed the count into `computeStatusBarData()` in `app/app_state.go` (~line 59):

After setting `FocusMode`:
```go
data.TmuxSessionCount = m.tmuxSessionCount
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/ -run TestStatusBar -v`
Expected: ALL PASS

Run full suite: `go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add ui/statusbar.go ui/statusbar_test.go app/app_state.go
git commit -m "feat: show tmux session count in status bar"
```
