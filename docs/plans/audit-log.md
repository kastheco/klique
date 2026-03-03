# Audit Log Implementation Plan

**Goal:** Add a structured audit log that captures all agent lifecycle events, plan state transitions, and operational actions — persisted in the planstore SQLite database and surfaced in a real-time event pane below the sidebar.

**Architecture:** A new `audit_events` table in the planstore SQLite DB stores timestamped, typed events with optional associations to plans/instances. A thin `auditlog` package provides a `Logger` interface with `Emit(event)` and query methods. The app layer calls `Emit` at every significant callsite (spawn, kill, FSM transition, push, error, etc.). A new `AuditPane` UI component renders below the navigation panel, showing the most recent events filtered by the currently selected plan/instance, with a keybind to toggle visibility.

**Tech Stack:** Go, SQLite (modernc.org/sqlite via existing planstore), bubbletea/lipgloss/bubbles (viewport), existing planstore HTTP server/client pattern for remote access.

**Size:** Large (estimated ~8 hours, 9 tasks, 3 waves)

---

## Wave 1: Backend — Schema, Logger, and Store Integration

### Task 1: Audit Event Types and Logger Interface

**Files:**
- Create: `config/auditlog/event.go`
- Create: `config/auditlog/logger.go`
- Test: `config/auditlog/logger_test.go`

**Step 1: write the failing test**

```go
func TestEventKind_String(t *testing.T) {
    assert.Equal(t, "agent_spawned", EventAgentSpawned.String())
    assert.Equal(t, "plan_transition", EventPlanTransition.String())
}

func TestNopLogger_DoesNotPanic(t *testing.T) {
    l := NopLogger()
    assert.NotPanics(t, func() {
        l.Emit(Event{Kind: EventAgentSpawned})
    })
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/auditlog/... -run TestEventKind -v
```

expected: FAIL — package does not exist

**Step 3: write minimal implementation**

`event.go` defines:
- `EventKind` string type with constants for all tier-1 and tier-2 events:
  - **Lifecycle:** `EventAgentSpawned`, `EventAgentFinished`, `EventAgentKilled`, `EventAgentPaused`, `EventAgentResumed`
  - **Plan:** `EventPlanTransition`, `EventPlanCreated`, `EventPlanMerged`, `EventPlanCancelled`
  - **Wave:** `EventWaveStarted`, `EventWaveCompleted`, `EventWaveFailed`
  - **Operational:** `EventPromptSent`, `EventGitPush`, `EventPRCreated`, `EventPermissionDetected`, `EventPermissionAnswered`, `EventFSMError`, `EventError`
- `Event` struct: `ID int64`, `Kind EventKind`, `Timestamp time.Time`, `Project string`, `PlanFile string`, `InstanceTitle string`, `AgentType string`, `WaveNumber int`, `TaskNumber int`, `Message string`, `Detail string` (JSON-encoded extra data), `Level string` (info/warn/error)

`logger.go` defines:
- `Logger` interface: `Emit(Event)`, `Query(filter QueryFilter) ([]Event, error)`, `Close() error`
- `QueryFilter` struct: `Project string`, `PlanFile string`, `InstanceTitle string`, `Kinds []EventKind`, `Limit int`, `Before time.Time`, `After time.Time`
- `NopLogger()` returning a no-op implementation (used when planstore is unconfigured)

**Step 4: run test to verify it passes**

```bash
go test ./config/auditlog/... -run TestEventKind -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/auditlog/
git commit -m "feat(auditlog): define event types, Logger interface, and NopLogger"
```

### Task 2: SQLite Logger Implementation

**Files:**
- Create: `config/auditlog/sqlite.go`
- Test: `config/auditlog/sqlite_test.go`

**Step 1: write the failing test**

```go
func TestSQLiteLogger_EmitAndQuery(t *testing.T) {
    logger, err := NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(Event{
        Kind:          EventAgentSpawned,
        Project:       "testproj",
        PlanFile:      "plan.md",
        InstanceTitle: "plan-coder",
        AgentType:     "coder",
        Message:       "spawned coder agent",
    })

    events, err := logger.Query(QueryFilter{Project: "testproj", Limit: 10})
    require.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, EventAgentSpawned, events[0].Kind)
    assert.Equal(t, "plan-coder", events[0].InstanceTitle)
    assert.False(t, events[0].Timestamp.IsZero())
}

func TestSQLiteLogger_QueryFilterByPlan(t *testing.T) {
    logger, err := NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(Event{Kind: EventAgentSpawned, Project: "p", PlanFile: "a.md"})
    logger.Emit(Event{Kind: EventAgentSpawned, Project: "p", PlanFile: "b.md"})

    events, err := logger.Query(QueryFilter{Project: "p", PlanFile: "a.md", Limit: 10})
    require.NoError(t, err)
    assert.Len(t, events, 1)
}

func TestSQLiteLogger_QueryFilterByKind(t *testing.T) {
    logger, err := NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(Event{Kind: EventAgentSpawned, Project: "p"})
    logger.Emit(Event{Kind: EventPlanTransition, Project: "p"})

    events, err := logger.Query(QueryFilter{
        Project: "p",
        Kinds:   []EventKind{EventPlanTransition},
        Limit:   10,
    })
    require.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, EventPlanTransition, events[0].Kind)
}

func TestSQLiteLogger_QueryOrderDesc(t *testing.T) {
    logger, err := NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(Event{Kind: EventAgentSpawned, Project: "p", Message: "first"})
    time.Sleep(time.Millisecond)
    logger.Emit(Event{Kind: EventAgentFinished, Project: "p", Message: "second"})

    events, err := logger.Query(QueryFilter{Project: "p", Limit: 10})
    require.NoError(t, err)
    require.Len(t, events, 2)
    assert.Equal(t, "second", events[0].Message) // newest first
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/auditlog/... -run TestSQLiteLogger -v
```

expected: FAIL — `NewSQLiteLogger` undefined

**Step 3: write minimal implementation**

`sqlite.go`:
- `SQLiteLogger` struct wrapping `*sql.DB`
- Schema: `CREATE TABLE IF NOT EXISTS audit_events (id INTEGER PRIMARY KEY, kind TEXT NOT NULL, timestamp TEXT NOT NULL, project TEXT NOT NULL DEFAULT '', plan_file TEXT NOT NULL DEFAULT '', instance_title TEXT NOT NULL DEFAULT '', agent_type TEXT NOT NULL DEFAULT '', wave_number INTEGER NOT NULL DEFAULT 0, task_number INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '', level TEXT NOT NULL DEFAULT 'info')`
- Index: `CREATE INDEX IF NOT EXISTS idx_audit_project_ts ON audit_events(project, timestamp DESC)`
- Index: `CREATE INDEX IF NOT EXISTS idx_audit_plan ON audit_events(plan_file, timestamp DESC)`
- `NewSQLiteLogger(dbPath string) (*SQLiteLogger, error)` — opens DB, runs schema, returns logger
- `Emit(e Event)` — inserts row, sets `Timestamp` to `time.Now()` if zero. **Non-blocking**: Emit runs the INSERT synchronously but is designed to be called from the main goroutine (bubbletea Update). If this becomes a bottleneck, a buffered channel can be added later.
- `Query(f QueryFilter) ([]Event, error)` — builds dynamic WHERE clause from filter fields, `ORDER BY timestamp DESC`, `LIMIT` capped at 500
- `Close() error`

Reuse `formatTime`/`parseTime` helpers from planstore (or duplicate the 10-line pair to avoid import cycles).

**Step 4: run test to verify it passes**

```bash
go test ./config/auditlog/... -run TestSQLiteLogger -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/auditlog/sqlite.go config/auditlog/sqlite_test.go
git commit -m "feat(auditlog): SQLite logger with emit, query, and filtering"
```

### Task 3: Wire Logger into App Initialization and Planstore Factory

**Files:**
- Modify: `config/planstore/factory.go`
- Modify: `app/app.go`
- Modify: `app/app_state.go`
- Test: `config/auditlog/sqlite_test.go` (add integration test)

**Step 1: write the failing test**

```go
func TestSQLiteLogger_SharedDB(t *testing.T) {
    // Verify the logger can be opened on the same DB path as planstore
    // (separate table, no conflicts)
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "store.db")

    store, err := planstore.NewSQLiteStore(dbPath)
    require.NoError(t, err)
    defer store.Close()

    logger, err := NewSQLiteLogger(dbPath)
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(Event{Kind: EventAgentSpawned, Project: "p", Message: "test"})
    events, err := logger.Query(QueryFilter{Project: "p", Limit: 1})
    require.NoError(t, err)
    assert.Len(t, events, 1)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/auditlog/... -run TestSQLiteLogger_SharedDB -v
```

expected: FAIL — `planstore` import not yet wired (or PASS if schema is independent — either way validates coexistence)

**Step 3: write minimal implementation**

- In `config/planstore/factory.go`: export the resolved SQLite DB path so the auditlog can reuse it. Add `func ResolvedDBPath(cfg Config) string` that returns the path the factory would use for SQLite (from TOML config or default `~/.config/kasmos/planstore.db`).
- In `app/app.go` (`newHome` or init path): after planstore initialization, create the audit logger:
  - If planstore is SQLite-backed: `auditlog.NewSQLiteLogger(planstore.ResolvedDBPath(cfg))`
  - If planstore is HTTP-backed: `auditlog.NopLogger()` for now (HTTP audit endpoints are a future enhancement)
  - If planstore is unconfigured: `auditlog.NopLogger()`
- Store `auditLogger auditlog.Logger` on the `home` struct.
- Add `defer m.auditLogger.Close()` in the teardown path.
- In `app/app_state.go`: add a helper `func (m *home) audit(kind auditlog.EventKind, msg string, opts ...auditlog.EventOption)` that fills in `Project` from `m.planStoreProject` and calls `m.auditLogger.Emit(...)`. Use functional options pattern for optional fields (`WithPlan`, `WithInstance`, `WithAgent`, `WithWave`, `WithDetail`, `WithLevel`).

**Step 4: run test to verify it passes**

```bash
go test ./config/auditlog/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/factory.go config/auditlog/ app/app.go app/app_state.go
git commit -m "feat(auditlog): wire SQLite logger into app init with audit() helper"
```

## Wave 2: Event Emission — Instrument All Callsites

> **depends on wave 1:** the `audit()` helper and `auditlog.Logger` must exist before callsites can emit events.

### Task 4: Lifecycle Events — Agent Spawn, Finish, Kill, Pause, Resume

**Files:**
- Modify: `app/app_state.go` (spawnPlanAgent, spawnAdHocAgent, spawnWaveTasks, spawnCoderWithFeedback, spawnReviewer, spawnChatAboutPlan)
- Modify: `app/app_actions.go` (kill, pause, resume, adopt)
- Modify: `app/app.go` (instanceStartedMsg handler, metadata tick — agent finished detection)
- Test: `app/app_audit_test.go`

**Step 1: write the failing test**

```go
func TestAuditEmit_AgentSpawned(t *testing.T) {
    logger, err := auditlog.NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    // Simulate what spawnPlanAgent does
    logger.Emit(auditlog.Event{
        Kind:          auditlog.EventAgentSpawned,
        Project:       "test",
        PlanFile:      "plan.md",
        InstanceTitle: "plan-coder",
        AgentType:     "coder",
        WaveNumber:    1,
        TaskNumber:    2,
        Message:       "spawned coder for wave 1 task 2",
    })

    events, err := logger.Query(auditlog.QueryFilter{Project: "test", Limit: 10})
    require.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "coder", events[0].AgentType)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestAuditEmit -v
```

expected: FAIL — test file doesn't exist yet

**Step 3: write minimal implementation**

Add `m.audit(...)` calls at each callsite:

**Spawn events** (in `app_state.go`):
- `spawnPlanAgent`: emit `EventAgentSpawned` with plan, agent type, instance title
- `spawnAdHocAgent`: emit `EventAgentSpawned` with agent type "fixer"
- `spawnWaveTasks`: emit `EventAgentSpawned` for each task with wave/task numbers
- `spawnCoderWithFeedback`: emit `EventAgentSpawned` with detail containing truncated feedback
- `spawnReviewer`: emit `EventAgentSpawned` with agent type "reviewer"
- `spawnChatAboutPlan`: emit `EventAgentSpawned` with agent type "custodian"

**Finish/kill events** (in `app_actions.go` and `app.go`):
- `executeContextAction` → `kill_instance`: emit `EventAgentKilled`
- `executeContextAction` → `pause_instance`: emit `EventAgentPaused`
- `executeContextAction` → `resume_instance`: emit `EventAgentResumed`
- `handleTmuxBrowserAction` → `BrowserAdopt`: emit `EventAgentSpawned` (adopted)
- In `app.go` metadata tick handler where `Running→Ready` is detected: emit `EventAgentFinished` with success/failure inferred from context

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestAuditEmit -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_state.go app/app_actions.go app/app.go app/app_audit_test.go
git commit -m "feat(auditlog): emit lifecycle events for spawn, finish, kill, pause, resume"
```

### Task 5: Plan and Wave Events — FSM Transitions, Wave Orchestration

**Files:**
- Modify: `config/planfsm/fsm.go` (add hook for transition logging)
- Modify: `app/app_actions.go` (plan merge, cancel, mark done, set status)
- Modify: `app/app.go` (wave started/completed/failed handlers)
- Modify: `app/app_state.go` (startNextWave, wave completion detection)
- Test: `app/app_audit_test.go` (extend)

**Step 1: write the failing test**

```go
func TestAuditEmit_PlanTransition(t *testing.T) {
    logger, err := auditlog.NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(auditlog.Event{
        Kind:     auditlog.EventPlanTransition,
        Project:  "test",
        PlanFile: "plan.md",
        Message:  "ready → implementing",
    })

    events, err := logger.Query(auditlog.QueryFilter{
        Project: "test",
        Kinds:   []auditlog.EventKind{auditlog.EventPlanTransition},
        Limit:   10,
    })
    require.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Contains(t, events[0].Message, "implementing")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestAuditEmit_PlanTransition -v
```

expected: FAIL — test doesn't exist yet

**Step 3: write minimal implementation**

**FSM transitions** — add audit calls around every `m.fsm.Transition()` call:
- `executePlanStage`: emit `EventPlanTransition` for each stage (plan, implement, review, finished)
- `fsmSetImplementing`, `fsmSetReviewing`: emit `EventPlanTransition` for intermediate steps
- `executeContextAction` → `merge_plan`: emit `EventPlanMerged`
- `executeContextAction` → `cancel_plan`: emit `EventPlanCancelled`
- `executeContextAction` → `mark_plan_done`: emit `EventPlanTransition` to done
- `executeContextAction` → `set_status`: emit `EventPlanTransition` with "manual override" detail
- `executeContextAction` → `start_over_plan`: emit `EventPlanTransition` with "start over" detail
- Plan creation (in `app_input.go` where new plans are created): emit `EventPlanCreated`

**Wave events** (in `app_state.go` and `app.go`):
- `startNextWave`: emit `EventWaveStarted` with wave number and task count
- Wave completion detection (metadata tick): emit `EventWaveCompleted` or `EventWaveFailed`
- All-waves-complete handler: emit `EventWaveCompleted` with "all waves complete" message

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestAuditEmit_PlanTransition -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planfsm/fsm.go app/app_actions.go app/app.go app/app_state.go app/app_audit_test.go
git commit -m "feat(auditlog): emit plan transitions, wave start/complete/fail events"
```

### Task 6: Operational Events — Prompts, Push, PR, Permissions, Errors

**Files:**
- Modify: `app/app_state.go` (push, PR creation)
- Modify: `app/app_actions.go` (push actions)
- Modify: `app/app_input.go` (prompt sent, permission answered)
- Modify: `app/app.go` (permission detection, error handler)
- Test: `app/app_audit_test.go` (extend)

**Step 1: write the failing test**

```go
func TestAuditEmit_PromptSent(t *testing.T) {
    logger, err := auditlog.NewSQLiteLogger(":memory:")
    require.NoError(t, err)
    defer logger.Close()

    logger.Emit(auditlog.Event{
        Kind:          auditlog.EventPromptSent,
        Project:       "test",
        InstanceTitle: "my-agent",
        Message:       "implement the feature",
    })

    events, err := logger.Query(auditlog.QueryFilter{
        Project: "test",
        Kinds:   []auditlog.EventKind{auditlog.EventPromptSent},
        Limit:   10,
    })
    require.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "implement the feature", events[0].Message)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestAuditEmit_PromptSent -v
```

expected: FAIL — test doesn't exist yet

**Step 3: write minimal implementation**

**Prompt events** (in `app_input.go`):
- When a prompt is sent to an agent (send-keys path and QueuedPrompt path): emit `EventPromptSent` with truncated prompt text (first 200 chars) as message

**Push events** (in `app_actions.go`):
- `pushSelectedInstance` success callback: emit `EventGitPush` with branch name
- `push_plan_branch` success callback: emit `EventGitPush`

**PR events** (in `app_input.go` or `app_state.go`):
- PR creation success: emit `EventPRCreated` with PR URL in detail

**Permission events** (in `app.go`):
- Permission prompt detected: emit `EventPermissionDetected` with instance title
- Permission answered (from overlay): emit `EventPermissionAnswered` with choice (allow once/always/reject)

**Error events** (in `app_state.go`):
- `handleError`: emit `EventError` with error message, level "error"
- FSM transition errors: emit `EventFSMError` with the transition that failed

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestAuditEmit_PromptSent -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app_state.go app/app_actions.go app/app_input.go app/app.go app/app_audit_test.go
git commit -m "feat(auditlog): emit operational events for prompts, push, PR, permissions, errors"
```

## Wave 3: TUI — Audit Pane Below Sidebar

> **depends on wave 2:** the audit logger must be emitting events before the pane can display them.

### Task 7: AuditPane UI Component

**Files:**
- Create: `ui/audit_pane.go`
- Test: `ui/audit_pane_test.go`

**Step 1: write the failing test**

```go
func TestAuditPane_RenderEmpty(t *testing.T) {
    pane := NewAuditPane()
    pane.SetSize(60, 10)
    output := pane.String()
    assert.Contains(t, output, "no events")
}

func TestAuditPane_RenderEvents(t *testing.T) {
    pane := NewAuditPane()
    pane.SetSize(60, 10)
    pane.SetEvents([]AuditEventDisplay{
        {Time: "12:34", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam},
        {Time: "12:35", Kind: "agent_finished", Icon: "✓", Message: "coder finished", Color: ColorGold},
    })
    output := pane.String()
    assert.Contains(t, output, "spawned coder")
    assert.Contains(t, output, "coder finished")
}

func TestAuditPane_ScrollDown(t *testing.T) {
    pane := NewAuditPane()
    pane.SetSize(60, 3) // very small — forces scroll
    events := make([]AuditEventDisplay, 20)
    for i := range events {
        events[i] = AuditEventDisplay{
            Time: fmt.Sprintf("12:%02d", i), Kind: "test", Icon: "·",
            Message: fmt.Sprintf("event %d", i), Color: ColorText,
        }
    }
    pane.SetEvents(events)
    pane.ScrollDown(5)
    output := pane.String()
    // Should show events from the scrolled position, not the top
    assert.NotContains(t, output, "event 0")
}

func TestAuditPane_ToggleVisibility(t *testing.T) {
    pane := NewAuditPane()
    assert.True(t, pane.Visible())
    pane.ToggleVisible()
    assert.False(t, pane.Visible())
    pane.ToggleVisible()
    assert.True(t, pane.Visible())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run TestAuditPane -v
```

expected: FAIL — `NewAuditPane` undefined

**Step 3: write minimal implementation**

`ui/audit_pane.go`:
- `AuditEventDisplay` struct: `Time string`, `Kind string`, `Icon string`, `Message string`, `Color lipgloss.Color`, `Level string`
- `AuditPane` struct with:
  - `events []AuditEventDisplay`
  - `viewport viewport.Model` for scrolling
  - `width, height int`
  - `visible bool` (default true)
  - `filterLabel string` (shows what's being filtered — plan name, instance title, or "all")
- `NewAuditPane() *AuditPane`
- `SetSize(w, h int)` — sets viewport dimensions
- `SetEvents(events []AuditEventDisplay)` — updates content, rebuilds viewport
- `SetFilter(label string)` — updates the filter label shown in the header
- `ScrollDown(n int)` / `ScrollUp(n int)` — delegates to viewport
- `Visible() bool` / `ToggleVisible()`
- `String() string` — renders:
  - 1-line header: `"── log ──"` (left) + filter label (right), styled with `ColorMuted`
  - Viewport body: each event as `"HH:MM icon message"` with icon colored by event kind
  - Empty state: centered `"no events"` in muted text
- Event icon mapping: `◆` spawn (foam), `✓` finish (gold), `✕` kill (rose), `⟳` transition (iris), `→` push (foam), `⚡` wave (gold), `!` error (rose), `·` default (muted)

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run TestAuditPane -v
```

expected: PASS

**Step 5: commit**

```bash
git add ui/audit_pane.go ui/audit_pane_test.go
git commit -m "feat(ui): add AuditPane component with scroll, filter, and toggle"
```

### Task 8: Layout Integration — Split Sidebar with Audit Pane

**Files:**
- Modify: `app/app.go` (home struct, View, WindowSize, init)
- Modify: `app/app_state.go` (audit pane data refresh)
- Modify: `app/app_input.go` (keybind for toggle, scroll passthrough)
- Modify: `keys/keys.go` (add audit toggle key)
- Test: `app/app_audit_pane_test.go`

**Step 1: write the failing test**

```go
func TestAuditPaneToggle(t *testing.T) {
    h := newTestHome()
    // Audit pane should be visible by default
    assert.True(t, h.auditPane.Visible())

    // Simulate toggle keybind
    h, _ = h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
    assert.False(t, h.(*home).auditPane.Visible())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestAuditPaneToggle -v
```

expected: FAIL — `auditPane` field doesn't exist

**Step 3: write minimal implementation**

**Layout changes** (`app.go`):
- Add `auditPane *ui.AuditPane` to `home` struct
- In `newHome`: create `ui.NewAuditPane()`, set visible by default
- In `updateHandleWindowSizeEvent`:
  - When audit pane is visible: split `contentHeight` for the sidebar column — nav gets 55%, audit pane gets 45% of sidebar height
  - `m.nav.SetSize(navWidth, navHeight)` where `navHeight = contentHeight * 55 / 100`
  - `m.auditPane.SetSize(navWidth, contentHeight - navHeight)`
  - When audit pane is hidden: nav gets full `contentHeight` as before
- In `View()`:
  - When visible: `sidebarCol = lipgloss.JoinVertical(lipgloss.Left, m.nav.String(), m.auditPane.String())`
  - When hidden: `sidebarCol = m.nav.String()` (unchanged)

**Keybind** (`keys/keys.go` and `app_input.go`):
- Add `KeyAuditToggle` mapped to `L`
- In `app_input.go` default state handler: toggle audit pane visibility and trigger `tea.WindowSize()` to recalculate layout

**Data refresh** (`app_state.go`):
- Add `refreshAuditPane()` method that:
  - Queries `m.auditLogger.Query(filter)` with filter based on current selection (selected plan → filter by plan, selected instance → filter by instance, nothing → last 50 global events)
  - Converts `[]auditlog.Event` to `[]ui.AuditEventDisplay` (format timestamp as HH:MM, map kind to icon/color)
  - Calls `m.auditPane.SetEvents(displays)` and `m.auditPane.SetFilter(label)`
- Call `refreshAuditPane()` from:
  - Every `m.audit(...)` call (so new events appear immediately)
  - Navigation selection changes (so filter updates when user selects different plan/instance)
  - `instanceChangedMsg` handler

**Scroll passthrough** (`app_input.go`):
- When `focusSlot == slotNav` and audit pane is visible: `ctrl+j`/`ctrl+k` scroll the audit pane (or mouse scroll when cursor is in the audit pane region)

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestAuditPaneToggle -v
```

expected: PASS

**Step 5: commit**

```bash
git add app/app.go app/app_state.go app/app_input.go keys/keys.go app/app_audit_pane_test.go
git commit -m "feat(app): integrate audit pane below sidebar with toggle and contextual filtering"
```

### Task 9: Menu Hint and Help Screen Updates

**Files:**
- Modify: `ui/menu.go` (add L hint)
- Modify: `app/app_state.go` (help screen text)
- Test: `ui/menu_test.go` (extend if menu has tests)

**Step 1: write the failing test**

No testable logic — this is a UI text update. Skip Steps 1-2.

**Step 3: write minimal implementation**

- In `ui/menu.go`: add `L log` to the keybind hints row (next to existing hints like `s spawn`, `? help`)
- In help screen text (if one exists for keybinds): add `L` — toggle audit log pane
- Ensure the hint only shows when the audit pane feature is active (always, since it's on by default)

**Step 4: verify manually**

```bash
go build ./... && go test ./ui/... -v
```

expected: builds and existing tests pass

**Step 5: commit**

```bash
git add ui/menu.go app/app_state.go
git commit -m "feat(ui): add audit log toggle hint to menu bar"
```
