# Improved Info Tab Implementation Plan

**Goal:** Persist plan subtask statuses and phase timestamps in the database, then overhaul the info pane UI to show plan summary, lifecycle timeline, and all-waves progress with per-task detail.

**Architecture:** Add a `subtasks` table and phase-timestamp columns to the existing SQLite task store. Parse plan content during ingestion to populate subtask rows and extract the goal field. Hook FSM transitions to record phase timestamps and orchestrator state changes to update subtask status. Redesign the info pane to render the new data in a scrollable multi-section layout (metadata → goal → lifecycle → progress → instances).

**Tech Stack:** Go, SQLite (modernc.org/sqlite), bubbletea/lipgloss (charm v2), existing taskstore/taskstate/taskfsm packages.

**Size:** Medium (estimated ~2.5 hours, 4 tasks, 3 waves)

---

## Wave 1: Database Schema and SQLite Store

### Task 1: Subtask Table, Phase Timestamps, and SQLiteStore Methods

**Files:**
- Modify: `config/taskstore/store.go`
- Modify: `config/taskstore/sqlite.go`
- Modify: `config/taskstore/testing.go`
- Modify: `config/taskstore/sqlite_test.go`

**Step 1: write the failing test**

Add tests to `sqlite_test.go` that exercise the new subtask and phase-timestamp methods:

```go
func TestSQLiteStore_SubtaskCRUD(t *testing.T) {
    store := newTestStore(t)
    project := "test-project"

    // Create a plan first.
    store.Create(project, TaskEntry{Filename: "plan.md", Status: StatusReady})

    // SetSubtasks should insert subtask rows.
    subtasks := []SubtaskEntry{
        {PlanFilename: "plan.md", WaveNumber: 1, TaskNumber: 1, Title: "schema migration", Status: SubtaskPending},
        {PlanFilename: "plan.md", WaveNumber: 1, TaskNumber: 2, Title: "store methods", Status: SubtaskPending},
        {PlanFilename: "plan.md", WaveNumber: 2, TaskNumber: 3, Title: "UI overhaul", Status: SubtaskPending},
    }
    require.NoError(t, store.SetSubtasks(project, "plan.md", subtasks))

    // GetSubtasks should return them ordered by task_number.
    got, err := store.GetSubtasks(project, "plan.md")
    require.NoError(t, err)
    require.Len(t, got, 3)
    assert.Equal(t, "schema migration", got[0].Title)
    assert.Equal(t, SubtaskPending, got[0].Status)
    assert.Equal(t, 1, got[0].WaveNumber)

    // UpdateSubtaskStatus should change a single subtask's status.
    require.NoError(t, store.UpdateSubtaskStatus(project, "plan.md", 1, SubtaskComplete))
    got, _ = store.GetSubtasks(project, "plan.md")
    assert.Equal(t, SubtaskComplete, got[0].Status)

    // SetSubtasks again should replace (upsert) all rows.
    require.NoError(t, store.SetSubtasks(project, "plan.md", subtasks[:1]))
    got, _ = store.GetSubtasks(project, "plan.md")
    assert.Len(t, got, 1)
}

func TestSQLiteStore_PhaseTimestamps(t *testing.T) {
    store := newTestStore(t)
    project := "test-project"
    now := time.Now().UTC().Truncate(time.Millisecond)

    store.Create(project, TaskEntry{Filename: "plan.md", Status: StatusReady})
    require.NoError(t, store.SetPhaseTimestamp(project, "plan.md", "implementing", now))

    entry, err := store.Get(project, "plan.md")
    require.NoError(t, err)
    assert.Equal(t, now.Unix(), entry.ImplementingAt.Unix())
}

func TestSQLiteStore_PlanGoal(t *testing.T) {
    store := newTestStore(t)
    project := "test-project"

    store.Create(project, TaskEntry{Filename: "plan.md", Status: StatusReady})
    require.NoError(t, store.SetPlanGoal(project, "plan.md", "add dark mode"))

    entry, err := store.Get(project, "plan.md")
    require.NoError(t, err)
    assert.Equal(t, "add dark mode", entry.Goal)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run 'TestSQLiteStore_Subtask|TestSQLiteStore_Phase|TestSQLiteStore_PlanGoal' -v
```

expected: FAIL — `SubtaskEntry` undefined, `SetSubtasks` undefined, etc.

**Step 3: write minimal implementation**

In `store.go`, add the subtask types and new Store interface methods:

```go
type SubtaskStatus string

const (
    SubtaskPending  SubtaskStatus = "pending"
    SubtaskRunning  SubtaskStatus = "running"
    SubtaskComplete SubtaskStatus = "complete"
    SubtaskFailed   SubtaskStatus = "failed"
)

type SubtaskEntry struct {
    PlanFilename string        `json:"plan_filename"`
    WaveNumber   int           `json:"wave_number"`
    TaskNumber   int           `json:"task_number"`
    Title        string        `json:"title"`
    Status       SubtaskStatus `json:"status"`
    UpdatedAt    time.Time     `json:"updated_at,omitempty"`
}
```

Add phase-timestamp fields to `TaskEntry`:

```go
PlanningAt     time.Time `json:"planning_at,omitempty"`
ImplementingAt time.Time `json:"implementing_at,omitempty"`
ReviewingAt    time.Time `json:"reviewing_at,omitempty"`
DoneAt         time.Time `json:"done_at,omitempty"`
Goal           string    `json:"goal,omitempty"`
```

Add new methods to the `Store` interface:

```go
// Subtask operations
SetSubtasks(project, planFilename string, subtasks []SubtaskEntry) error
GetSubtasks(project, planFilename string) ([]SubtaskEntry, error)
UpdateSubtaskStatus(project, planFilename string, taskNumber int, status SubtaskStatus) error

// Phase timestamps
SetPhaseTimestamp(project, planFilename, phase string, ts time.Time) error

// Plan metadata
SetPlanGoal(project, planFilename, goal string) error
```

In `sqlite.go`:
- Add the `subtasks` table to the schema constant.
- Add migrations: `subtasksTableMigration`, plus `ALTER TABLE tasks ADD COLUMN` for `planning_at`, `implementing_at`, `reviewing_at`, `done_at`, `goal`.
- Run migrations in `NewSQLiteStore`.
- Update `scanTaskEntry`/`scanTaskEntries` to include the 5 new columns.
- Update `Create`/`Update`/`Get`/`List`/`ListByStatus`/`ListByTopic` queries to include the new columns.
- Implement `SetSubtasks` (DELETE existing + INSERT new in a transaction).
- Implement `GetSubtasks` (SELECT ordered by task_number).
- Implement `UpdateSubtaskStatus` (UPDATE single row).
- Implement `SetPhaseTimestamp` (UPDATE the appropriate column by phase name).
- Implement `SetPlanGoal` (UPDATE goal column).

**Step 4: run test to verify it passes**

```bash
go test ./config/taskstore/... -run 'TestSQLiteStore_Subtask|TestSQLiteStore_Phase|TestSQLiteStore_PlanGoal' -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/taskstore/store.go config/taskstore/sqlite.go config/taskstore/sqlite_test.go config/taskstore/testing.go
git commit -m "feat: add subtasks table, phase timestamps, and goal column to task store"
```

## Wave 2: HTTP Transport and Data Ingestion Pipeline

> **depends on wave 1:** HTTPStore and server need the Store interface additions from Task 1. Data ingestion needs the SetSubtasks/SetPhaseTimestamp/SetPlanGoal methods.

### Task 2: HTTPStore Client and Server Endpoints

**Files:**
- Modify: `config/taskstore/http.go`
- Modify: `config/taskstore/server.go`
- Modify: `config/taskstore/http_test.go`
- Modify: `config/taskstore/server_test.go`

**Step 1: write the failing test**

Add round-trip tests in `http_test.go` using the test server pattern already established in the file:

```go
func TestHTTPStore_SubtaskRoundTrip(t *testing.T) {
    // Setup: in-memory SQLite behind HTTP server.
    sqlite := newTestStore(t).(*SQLiteStore)
    srv := httptest.NewServer(NewHandler(sqlite))
    defer srv.Close()
    client := NewHTTPStore(srv.URL, "proj")

    sqlite.Create("proj", TaskEntry{Filename: "plan.md", Status: StatusReady})

    subtasks := []SubtaskEntry{
        {PlanFilename: "plan.md", WaveNumber: 1, TaskNumber: 1, Title: "task one", Status: SubtaskPending},
        {PlanFilename: "plan.md", WaveNumber: 2, TaskNumber: 2, Title: "task two", Status: SubtaskPending},
    }
    require.NoError(t, client.SetSubtasks("proj", "plan.md", subtasks))

    got, err := client.GetSubtasks("proj", "plan.md")
    require.NoError(t, err)
    require.Len(t, got, 2)

    require.NoError(t, client.UpdateSubtaskStatus("proj", "plan.md", 1, SubtaskComplete))
    got, _ = client.GetSubtasks("proj", "plan.md")
    assert.Equal(t, SubtaskComplete, got[0].Status)
}

func TestHTTPStore_PhaseTimestampRoundTrip(t *testing.T) {
    sqlite := newTestStore(t).(*SQLiteStore)
    srv := httptest.NewServer(NewHandler(sqlite))
    defer srv.Close()
    client := NewHTTPStore(srv.URL, "proj")
    now := time.Now().UTC().Truncate(time.Millisecond)

    sqlite.Create("proj", TaskEntry{Filename: "plan.md", Status: StatusReady})

    require.NoError(t, client.SetPhaseTimestamp("proj", "plan.md", "implementing", now))
    entry, err := client.Get("proj", "plan.md")
    require.NoError(t, err)
    assert.Equal(t, now.Unix(), entry.ImplementingAt.Unix())
}

func TestHTTPStore_PlanGoalRoundTrip(t *testing.T) {
    sqlite := newTestStore(t).(*SQLiteStore)
    srv := httptest.NewServer(NewHandler(sqlite))
    defer srv.Close()
    client := NewHTTPStore(srv.URL, "proj")

    sqlite.Create("proj", TaskEntry{Filename: "plan.md", Status: StatusReady})

    require.NoError(t, client.SetPlanGoal("proj", "plan.md", "add dark mode"))
    entry, err := client.Get("proj", "plan.md")
    require.NoError(t, err)
    assert.Equal(t, "add dark mode", entry.Goal)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run 'TestHTTPStore_Subtask|TestHTTPStore_Phase|TestHTTPStore_PlanGoal' -v
```

expected: FAIL — `SetSubtasks` not implemented on HTTPStore

**Step 3: write minimal implementation**

In `server.go`, add 5 new endpoints mirroring existing patterns:
- `GET /v1/projects/{project}/tasks/{filename}/subtasks` → GetSubtasks
- `PUT /v1/projects/{project}/tasks/{filename}/subtasks` → SetSubtasks
- `PUT /v1/projects/{project}/tasks/{filename}/subtasks/{taskNumber}/status` → UpdateSubtaskStatus
- `PUT /v1/projects/{project}/tasks/{filename}/phase-timestamp` → SetPhaseTimestamp
- `PUT /v1/projects/{project}/tasks/{filename}/goal` → SetPlanGoal

In `http.go`, implement the corresponding HTTPStore methods using the same `do()`/`decodeError()` pattern as existing methods. Each method builds a URL, marshals JSON, sends the request, and checks the response status.

**Step 4: run test to verify it passes**

```bash
go test ./config/taskstore/... -run 'TestHTTPStore_Subtask|TestHTTPStore_Phase|TestHTTPStore_PlanGoal' -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/taskstore/http.go config/taskstore/server.go config/taskstore/http_test.go config/taskstore/server_test.go
git commit -m "feat: add HTTP transport for subtasks, phase timestamps, and plan goal"
```

### Task 3: Subtask Ingestion, Phase Timestamps, and Status Persistence

**Files:**
- Modify: `config/taskstate/taskstate.go`
- Modify: `config/taskstate/taskstate_test.go`
- Modify: `config/taskfsm/fsm.go`
- Modify: `config/taskfsm/fsm_test.go`
- Modify: `app/wave_orchestrator.go`
- Modify: `app/wave_orchestrator_test.go`

**Step 1: write the failing test**

In `config/taskstate/taskstate_test.go`:

```go
func TestTaskState_IngestSubtasks(t *testing.T) {
    store := taskstore.NewTestSQLiteStore(t)
    ps, err := taskstate.Load(store, "proj", t.TempDir())
    require.NoError(t, err)

    require.NoError(t, ps.Create("plan.md", "desc", "plan/test", "", time.Now()))

    content := "# Plan\n\n**Goal:** add dark mode\n\n## Wave 1\n\n### Task 1: Schema\n\nbody\n\n### Task 2: Store\n\nbody\n\n## Wave 2\n\n### Task 3: UI\n\nbody\n"
    require.NoError(t, ps.IngestContent("plan.md", content))

    // Goal should be extracted.
    entry, _ := ps.Entry("plan.md")
    assert.Equal(t, "add dark mode", entry.Goal)

    // Subtasks should be created.
    subtasks, err := ps.GetSubtasks("plan.md")
    require.NoError(t, err)
    require.Len(t, subtasks, 3)
    assert.Equal(t, "Schema", subtasks[0].Title)
    assert.Equal(t, 1, subtasks[0].WaveNumber)
}
```

In `config/taskfsm/fsm_test.go`:

```go
func TestFSM_TransitionRecordsPhaseTimestamp(t *testing.T) {
    store := taskstore.NewTestSQLiteStore(t)
    store.Create("proj", taskstore.TaskEntry{Filename: "plan.md", Status: taskstore.StatusReady})
    fsm := New(store, "proj", t.TempDir())

    require.NoError(t, fsm.Transition("plan.md", ImplementStart))

    entry, err := store.Get("proj", "plan.md")
    require.NoError(t, err)
    assert.False(t, entry.ImplementingAt.IsZero(), "implementing_at should be set")
}
```

In `app/wave_orchestrator_test.go`, add a test for subtask status persistence:

```go
func TestWaveOrchestrator_PersistsSubtaskStatus(t *testing.T) {
    store := taskstore.NewTestSQLiteStore(t)
    store.Create("proj", taskstore.TaskEntry{Filename: "plan.md", Status: taskstore.StatusImplementing})
    plan := &taskparser.Plan{Waves: []taskparser.Wave{
        {Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "first"}}},
    }}
    orch := NewWaveOrchestrator("plan.md", plan)
    orch.SetStore(store, "proj")
    orch.StartNextWave()
    orch.MarkTaskComplete(1)

    subtasks, _ := store.GetSubtasks("proj", "plan.md")
    require.Len(t, subtasks, 1)
    assert.Equal(t, taskstore.SubtaskComplete, subtasks[0].Status)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstate/... -run TestTaskState_IngestSubtasks -v
go test ./config/taskfsm/... -run TestFSM_TransitionRecordsPhaseTimestamp -v
go test ./app/... -run TestWaveOrchestrator_PersistsSubtaskStatus -v
```

expected: FAIL — `IngestContent`, `GetSubtasks`, `SetStore` undefined

**Step 3: write minimal implementation**

**taskstate.go** additions:
- Add `Goal` field to `taskstate.TaskEntry`, and phase timestamp fields: `PlanningAt`, `ImplementingAt`, `ReviewingAt`, `DoneAt`.
- Update `Load()` to map new fields from `taskstore.TaskEntry`.
- Update `toTaskstoreEntry()` to include new fields.
- Add `IngestContent(filename, content string) error` — calls `taskparser.Parse()`, extracts goal, calls `store.SetPlanGoal()`, builds `[]SubtaskEntry` from parsed waves/tasks, calls `store.SetSubtasks()`, then calls `store.SetContent()`.
- Add `GetSubtasks(filename string) ([]taskstore.SubtaskEntry, error)` — delegates to store.
- Add `UpdateSubtaskStatus(filename string, taskNumber int, status taskstore.SubtaskStatus) error` — delegates to store.

**fsm.go** additions:
- After `ForceSetStatus()` succeeds in `Transition()`, call `store.SetPhaseTimestamp()` with the new status name and `time.Now().UTC()`. Only set for recognized phases: "planning", "implementing", "reviewing", "done".

**wave_orchestrator.go** additions:
- Add optional `store taskstore.Store` and `project string` fields.
- Add `SetStore(store taskstore.Store, project string)` method.
- In `MarkTaskComplete()` and `MarkTaskFailed()`, if `store` is non-nil, call `store.UpdateSubtaskStatus()` with the appropriate status.
- In `StartNextWave()`, if `store` is non-nil, call `store.UpdateSubtaskStatus()` for each task being started (set to `SubtaskRunning`).

**Step 4: run test to verify it passes**

```bash
go test ./config/taskstate/... -run TestTaskState_IngestSubtasks -v
go test ./config/taskfsm/... -run TestFSM_TransitionRecordsPhaseTimestamp -v
go test ./app/... -run TestWaveOrchestrator_PersistsSubtaskStatus -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/taskstate/taskstate.go config/taskstate/taskstate_test.go config/taskfsm/fsm.go config/taskfsm/fsm_test.go app/wave_orchestrator.go app/wave_orchestrator_test.go
git commit -m "feat: wire subtask ingestion, phase timestamps, and status persistence"
```

## Wave 3: Info Pane UI Overhaul

> **depends on wave 2:** The info pane reads subtask data and phase timestamps populated by the ingestion pipeline (Task 3). The app layer wiring calls store methods added in Tasks 1-2.

### Task 4: Redesign Info Pane with Lifecycle, Progress, and Goal Sections

**Files:**
- Modify: `ui/info_pane.go`
- Modify: `ui/info_pane_test.go`
- Modify: `app/app_state.go`

**Step 1: write the failing test**

Add tests to `ui/info_pane_test.go`:

```go
func TestInfoPane_PlanSummaryWithGoalAndLifecycle(t *testing.T) {
    pane := NewInfoPane()
    pane.SetSize(70, 40)
    now := time.Now()
    pane.SetData(InfoData{
        IsPlanHeaderSelected: true,
        PlanName:             "improved-info-tab",
        PlanStatus:           "implementing",
        PlanBranch:           "plan/improved-info-tab",
        PlanGoal:             "persist subtask statuses and redesign the info pane",
        PlanningAt:           now.Add(-2 * time.Hour),
        ImplementingAt:       now.Add(-1 * time.Hour),
        AllWaveSubtasks: []WaveSubtaskGroup{
            {WaveNumber: 1, Subtasks: []SubtaskDisplay{
                {Number: 1, Title: "schema migration", Status: "complete"},
                {Number: 2, Title: "store methods", Status: "complete"},
            }},
            {WaveNumber: 2, Subtasks: []SubtaskDisplay{
                {Number: 3, Title: "http endpoints", Status: "running"},
                {Number: 4, Title: "UI overhaul", Status: "pending"},
            }},
        },
        CompletedTasks: 2,
        TotalSubtasks:  4,
    })

    output := pane.String()
    assert.Contains(t, output, "persist subtask statuses")
    assert.Contains(t, output, "lifecycle")
    assert.Contains(t, output, "implementing")
    assert.Contains(t, output, "2/4")
    assert.Contains(t, output, "schema migration")
    assert.Contains(t, output, "✓")
    assert.Contains(t, output, "●")
    assert.Contains(t, output, "○")
}

func TestInfoPane_InstanceWithTaskAssignment(t *testing.T) {
    pane := NewInfoPane()
    pane.SetSize(70, 30)
    pane.SetData(InfoData{
        HasInstance:    true,
        HasPlan:        true,
        Title:          "my-feature-coder",
        Status:         "running",
        PlanName:       "my-feature",
        PlanGoal:       "add dark mode toggle",
        PlanStatus:     "implementing",
        AgentType:      "coder",
        TaskNumber:     3,
        TotalTasks:     6,
        TaskTitle:      "http endpoints",
    })

    output := pane.String()
    assert.Contains(t, output, "add dark mode toggle")
    assert.Contains(t, output, "task 3 of 6: http endpoints")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run 'TestInfoPane_PlanSummaryWithGoalAndLifecycle|TestInfoPane_InstanceWithTaskAssignment' -v
```

expected: FAIL — `PlanGoal`, `AllWaveSubtasks`, `WaveSubtaskGroup`, `SubtaskDisplay`, etc. undefined

**Step 3: write minimal implementation**

**ui/info_pane.go** changes:

Add new types to `InfoData`:

```go
// New fields on InfoData:
PlanGoal       string
TaskTitle      string // task title for the instance's assigned task

// Phase timestamps (zero = not reached yet)
PlanningAt     time.Time
ImplementingAt time.Time
ReviewingAt    time.Time
DoneAt         time.Time

// All-waves subtask progress
AllWaveSubtasks []WaveSubtaskGroup
CompletedTasks  int
TotalSubtasks   int
```

New types:

```go
type SubtaskDisplay struct {
    Number int
    Title  string
    Status string // "pending", "running", "complete", "failed"
}

type WaveSubtaskGroup struct {
    WaveNumber int
    Subtasks   []SubtaskDisplay
}
```

Rewrite `renderPlanSummary()` to match the approved Mockup A:
1. **Plan metadata section** — name, status, branch, review cycle (existing, keep).
2. **Goal section** — new section header "goal" + divider + wrapped goal text.
3. **Lifecycle section** — new section header "lifecycle" + divider + phase rows with filled/unfilled bullet and timestamp or "—".
4. **Progress section** — header "progress" with inline fraction + ASCII bar + divider + per-wave groups with per-task rows showing glyph + number + title.
5. **Instances section** — existing instance counts.
6. **View plan doc button** — existing, keep at bottom.

Add helper methods:
- `renderGoalSection()` — renders goal text wrapped to pane width.
- `renderLifecycleSection()` — renders phase timeline with ● for reached phases (with timestamp) and ○ for unreached.
- `renderProgressSection()` — renders ASCII progress bar + all-waves task list with status glyphs (✓/●/✗/○).

For the **instance view**, modify `renderInstanceSection()`:
- After "plan name" row, add a "goal" row showing `PlanGoal` (one-liner, truncated to width).
- Change the "task" row from `"4 of 6"` to `"task 4 of 6: http endpoints"` using the new `TaskTitle` field.

**app/app_state.go** changes:

Modify `updateInfoPaneForPlanHeader()`:
- Load subtasks from store: `subtasks, err := m.taskStore.GetSubtasks(m.taskStoreProject, planFile)`.
- Group subtasks by wave number into `[]ui.WaveSubtaskGroup`.
- Count completed tasks for `CompletedTasks`.
- Read phase timestamps from the `TaskEntry` (update `taskstate.TaskEntry` to carry them — already done in Task 3).
- Read goal from `TaskEntry.Goal` (already populated by ingestion in Task 3).
- Populate all new `InfoData` fields.

Modify `updateInfoPane()` (instance-selected):
- Add `data.PlanGoal = entry.Goal` when a plan is bound.
- Resolve `TaskTitle` from the orchestrator's parsed plan: `orch.plan.Waves[wave].Tasks[task].Title`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run 'TestInfoPane_PlanSummaryWithGoalAndLifecycle|TestInfoPane_InstanceWithTaskAssignment' -v
```

expected: PASS

Also run the full test suite to verify nothing is broken:

```bash
go test ./ui/... ./app/... ./config/... -v -count=1
```

**Step 5: commit**

```bash
git add ui/info_pane.go ui/info_pane_test.go app/app_state.go
git commit -m "feat: redesign info pane with goal, lifecycle timeline, and all-waves progress"
```
