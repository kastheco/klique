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

Add tests to `sqlite_test.go` that exercise subtask CRUD, phase timestamps, and goal persistence using the same assert style already used in this file (`require.NoError`, then `assert.Equal`). Keep names stable so Step 2/4 regexes keep working:

```go
func TestSQLiteStore_SubtaskCRUD(t *testing.T)
func TestSQLiteStore_PhaseTimestamps(t *testing.T)
func TestSQLiteStore_PlanGoal(t *testing.T)
```

In `TestSQLiteStore_SubtaskCRUD`, assert all of these explicitly:
- `SetSubtasks` replaces prior rows for the same `(project, plan)` (delete + insert semantics).
- `GetSubtasks` ordering is deterministic by `wave_number`, then `task_number`.
- `UpdateSubtaskStatus` updates one task row only and persists `updated_at` non-zero.

In `TestSQLiteStore_PhaseTimestamps`, also assert unknown phase handling:

```go
err := store.SetPhaseTimestamp(project, "plan.md", "invalid-phase", now)
require.Error(t, err)
assert.Contains(t, err.Error(), "unknown phase")
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run 'TestSQLiteStore_Subtask|TestSQLiteStore_Phase|TestSQLiteStore_PlanGoal' -v
```

expected: FAIL — new types/methods/columns not implemented yet.

**Step 3: write minimal implementation**

Implement concrete signatures first, then wire SQL.

In `store.go`:
- Add `SubtaskStatus` and `SubtaskEntry`:

```go
type SubtaskStatus string

type SubtaskEntry struct {
    PlanFilename string        `json:"plan_filename"`
    WaveNumber   int           `json:"wave_number"`
    TaskNumber   int           `json:"task_number"`
    Title        string        `json:"title"`
    Status       SubtaskStatus `json:"status"`
    UpdatedAt    time.Time     `json:"updated_at,omitempty"`
}
```

- Add `TaskEntry` fields (all optional/zero-safe):

```go
PlanningAt     time.Time `json:"planning_at,omitempty"`
ImplementingAt time.Time `json:"implementing_at,omitempty"`
ReviewingAt    time.Time `json:"reviewing_at,omitempty"`
DoneAt         time.Time `json:"done_at,omitempty"`
Goal           string    `json:"goal,omitempty"`
```

- Extend `Store` with exact signatures:

```go
SetSubtasks(project, planFilename string, subtasks []SubtaskEntry) error
GetSubtasks(project, planFilename string) ([]SubtaskEntry, error)
UpdateSubtaskStatus(project, planFilename string, taskNumber int, status SubtaskStatus) error
SetPhaseTimestamp(project, planFilename, phase string, ts time.Time) error
SetPlanGoal(project, planFilename, goal string) error
```

In `sqlite.go`, follow existing patterns already used by this package:
- Error wrapping pattern: `fmt.Errorf("<operation>: %w", err)`.
- Not-found detection pattern: `RowsAffected()==0` -> `fmt.Errorf("plan not found: %s/%s", project, filename)`.
- Time storage pattern: `formatTime`/`parseTime` (RFC3339Nano text columns).

Add schema + migrations:
- Create `subtasks` table in `schema` with unique key `(project, plan_filename, task_number)`.
- Add tasks columns via `migrateAddColumn` constants:
  - `planning_at`, `implementing_at`, `reviewing_at`, `done_at`, `goal`.
- Add a `subtasksTableMigration` constant and execute `CREATE TABLE IF NOT EXISTS subtasks (...)` in `NewSQLiteStore` after base `schema` exec.

Update task queries/scanners:
- `Create`, `Get`, `Update`, `List`, `ListByStatus`, `ListByTopic`, `scanTaskEntry`, `scanTaskEntries` must include the new five task columns.
- Keep `Update` behavior consistent with existing content-preservation test: do not re-add `content` into `UPDATE ... SET ...`.

Implement methods with transactional safety:

```go
func (s *SQLiteStore) SetSubtasks(project, planFilename string, subtasks []SubtaskEntry) error
func (s *SQLiteStore) GetSubtasks(project, planFilename string) ([]SubtaskEntry, error)
func (s *SQLiteStore) UpdateSubtaskStatus(project, planFilename string, taskNumber int, status SubtaskStatus) error
func (s *SQLiteStore) SetPhaseTimestamp(project, planFilename, phase string, ts time.Time) error
func (s *SQLiteStore) SetPlanGoal(project, planFilename, goal string) error
```

`SetSubtasks` must:
- begin transaction,
- delete existing rows for `(project, plan_filename)`,
- insert all provided rows,
- commit.

`SetPhaseTimestamp` should map phase -> column with a closed switch (no dynamic SQL from user input):
- `planning` -> `planning_at`
- `implementing` -> `implementing_at`
- `reviewing` -> `reviewing_at`
- `done` -> `done_at`
- otherwise return `fmt.Errorf("unknown phase: %s", phase)`

For `config/taskstore/testing.go`, keep return type as `Store`, but ensure compile remains clean with the expanded interface (no behavior change needed).

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

Add round-trip tests to `http_test.go` and endpoint contract tests to `server_test.go`, following existing server setup pattern (`httptest.NewServer(taskstore.NewHandler(...))`).

Use these method signatures in tests so compiler drives implementation:

```go
func (s *HTTPStore) SetSubtasks(project, planFilename string, subtasks []SubtaskEntry) error
func (s *HTTPStore) GetSubtasks(project, planFilename string) ([]SubtaskEntry, error)
func (s *HTTPStore) UpdateSubtaskStatus(project, planFilename string, taskNumber int, status SubtaskStatus) error
func (s *HTTPStore) SetPhaseTimestamp(project, planFilename, phase string, ts time.Time) error
func (s *HTTPStore) SetPlanGoal(project, planFilename, goal string) error
```

Also add server-only tests that validate:
- `400` for malformed JSON,
- `404` when task/subtask not found,
- `200` for success,
- JSON error body shape remains `{ "error": "..." }` (existing `writeError` contract).

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstore/... -run 'TestHTTPStore_Subtask|TestHTTPStore_Phase|TestHTTPStore_PlanGoal' -v
```

expected: FAIL — HTTP methods/routes missing.

**Step 3: write minimal implementation**

Keep parity with existing transport conventions in `http.go` and `server.go`:
- client methods call `do(req)` and use `decodeError(resp)` for non-success status.
- handlers use `writeError(...)` and `writeJSON(...)`.
- not-found detection remains `isNotFound(err)`.

Add server endpoints in `server.go`:
- `GET /v1/projects/{project}/tasks/{filename}/subtasks`
- `PUT /v1/projects/{project}/tasks/{filename}/subtasks`
- `PUT /v1/projects/{project}/tasks/{filename}/subtasks/{taskNumber}/status`
- `PUT /v1/projects/{project}/tasks/{filename}/phase-timestamp`
- `PUT /v1/projects/{project}/tasks/{filename}/goal`

Request payloads (explicit structs in handler scope):

```go
type updateSubtaskStatusRequest struct {
    Status SubtaskStatus `json:"status"`
}

type setPhaseTimestampRequest struct {
    Phase string    `json:"phase"`
    TS    time.Time `json:"timestamp"`
}

type setPlanGoalRequest struct {
    Goal string `json:"goal"`
}
```

Use `strconv.Atoi(r.PathValue("taskNumber"))` for the path param; return `400` on parse failure.

Add HTTPStore URL helpers in `http.go` to avoid duplicated path formatting:

```go
func (s *HTTPStore) taskSubtasksURL(project, filename string) string
func (s *HTTPStore) taskSubtaskStatusURL(project, filename string, taskNumber int) string
func (s *HTTPStore) taskPhaseTimestampURL(project, filename string) string
func (s *HTTPStore) taskGoalURL(project, filename string) string
```

Then implement the 5 Store methods using those helpers.

Edge cases to lock down:
- `SetSubtasks(..., nil)` should still clear existing subtasks (send empty JSON array).
- `SetPhaseTimestamp` with zero `time.Time` should still serialize and store empty-string timestamp via SQLite `formatTime`.
- `UpdateSubtaskStatus` must return not-found if row absent (propagate server error body).

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

`app/wave_orchestrator.go`/`app/wave_orchestrator_test.go` are legacy paths in this repo; wave orchestration currently lives in `orchestration/engine.go` and `orchestration/engine_test.go`. Keep plan metadata unchanged, but implement/test the orchestrator portion in those real files.

Add tests in `config/taskstate/taskstate_test.go`:

```go
func TestTaskState_IngestContent_PopulatesGoalAndSubtasks(t *testing.T)
func TestTaskState_IngestContent_ParseFailureStillStoresContent(t *testing.T)
```

`PopulateGoalAndSubtasks` should assert:
- `entry.Goal` is populated from `**Goal:**`.
- `GetSubtasks` returns wave/task/title/status rows (`pending` default).

`ParseFailureStillStoresContent` should assert:
- content is still persisted in store,
- method returns parse error for missing wave headers,
- subtasks are not overwritten on parse failure.

Add in `config/taskfsm/fsm_test.go`:

```go
func TestFSM_TransitionRecordsPhaseTimestamp(t *testing.T)
func TestFSM_TransitionSkipsTimestampForNonPhaseStatuses(t *testing.T)
```

For orchestrator persistence, add in `orchestration/engine_test.go`:

```go
func TestWaveOrchestrator_PersistsSubtaskStatus(t *testing.T)
```

This test should set a test store via `SetStore(...)`, then assert:
- `StartNextWave` marks tasks `running`.
- `MarkTaskComplete` writes `complete`.
- `MarkTaskFailed` writes `failed`.

**Step 2: run test to verify it fails**

```bash
go test ./config/taskstate/... -run TestTaskState_IngestContent -v
go test ./config/taskfsm/... -run TestFSM_TransitionRecordsPhaseTimestamp -v
go test ./orchestration/... -run TestWaveOrchestrator_PersistsSubtaskStatus -v
```

expected: FAIL — ingestion/status persistence/timestamp hooks missing.

**Step 3: write minimal implementation**

`config/taskstate/taskstate.go`:
1. Extend `TaskEntry` with:

```go
Goal           string
PlanningAt     time.Time
ImplementingAt time.Time
ReviewingAt    time.Time
DoneAt         time.Time
```

2. Update `Load(...)` and `toTaskstoreEntry(...)` mappings for those fields.
3. Add exact methods:

```go
func (ps *TaskState) IngestContent(filename, content string) error
func (ps *TaskState) GetSubtasks(filename string) ([]taskstore.SubtaskEntry, error)
func (ps *TaskState) UpdateSubtaskStatus(filename string, taskNumber int, status taskstore.SubtaskStatus) error
```

Implementation contract for `IngestContent`:
- validate plan exists in `ps.Plans` (same `plan not found` wording used elsewhere),
- always call `ps.store.SetContent(...)` first,
- parse with `taskparser.Parse(content)`,
- on parse success: call `SetPlanGoal`, build subtasks from parsed waves/tasks, call `SetSubtasks`, update in-memory `ps.Plans[filename]` goal,
- on parse error: return wrapped error `fmt.Errorf("parse plan content: %w", err)` while keeping content persisted.

Required imports:

```go
import (
    ...
    "github.com/kastheco/kasmos/config/taskparser"
)
```

`config/taskfsm/fsm.go`:
- add `time` import,
- after successful `ForceSetStatus`, set phase timestamp for phase statuses.

Add helper:

```go
func phaseNameForStatus(s Status) (string, bool)
```

and use:

```go
if phase, ok := phaseNameForStatus(newStatus); ok {
    if err := m.store.SetPhaseTimestamp(m.project, planFile, phase, time.Now().UTC()); err != nil {
        return fmt.Errorf("set phase timestamp: %w", err)
    }
}
```

`orchestration/engine.go` (actual wave orchestrator implementation):
- add optional persistence fields:

```go
store   taskstore.Store
project string
```

- add method:

```go
func (o *WaveOrchestrator) SetStore(store taskstore.Store, project string)
```

- add internal helper:

```go
func (o *WaveOrchestrator) persistTaskStatus(taskNumber int, status taskstore.SubtaskStatus)
```

and call it from:
- `StartNextWave` -> `running`
- `MarkTaskComplete` -> `complete`
- `MarkTaskFailed` -> `failed`
- `RetryFailedTasks` -> `running`

Use best-effort semantics (do not change existing method signatures): if persistence fails, leave in-memory wave state transitions intact.

Wire store into orchestrators at creation/recovery points in app flow (neighbor files):
- `app/app_actions.go` when creating orchestrators in `executeTaskStage` (`implement`, `implement_direct`),
- `app/app.go` when creating from wave signals,
- `app/app_state.go` in `rebuildOrphanedOrchestrators`.

Also switch planner ingestion to the new method in `app/app_state.go`:

```go
if err := m.taskState.IngestContent(planFile, string(data)); err != nil {
    log.WarningLog.Printf("ingestTaskContent: ... %v", err)
}
```

Then add one recovery path in `app/app_actions.go` (`implement` existing orchestrator case): if the orchestrator exists but has no store wired (e.g. after restart), call `SetStore(m.taskStore, m.taskStoreProject)` before resuming.

**Step 4: run test to verify it passes**

```bash
go test ./config/taskstate/... -run TestTaskState_IngestContent -v
go test ./config/taskfsm/... -run TestFSM_TransitionRecordsPhaseTimestamp -v
go test ./orchestration/... -run TestWaveOrchestrator_PersistsSubtaskStatus -v
go test ./app/... -run 'TestTriggerPlanStage_Implement|TestWaveMonitor' -v
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

Add tests in `ui/info_pane_test.go` that pin the new rendering contract and preserve lowercase section labels (project standard).

Add:

```go
func TestInfoPane_PlanSummaryWithGoalAndLifecycle(t *testing.T)
func TestInfoPane_InstanceWithTaskAssignment(t *testing.T)
func TestInfoPane_ProgressGroupsByWave(t *testing.T)
```

Assertions to include:
- plan header view contains section labels: `goal`, `lifecycle`, `progress`, `instances`.
- lifecycle shows reached/unreached glyphs (`●`, `○`) and timestamps/`—`.
- progress shows fraction (`2/4`), bar, and per-wave task lines with glyphs (`✓`, `●`, `✗`, `○`).
- instance view shows `goal` row and `task N of M: <title>` format.

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run 'TestInfoPane_PlanSummaryWithGoalAndLifecycle|TestInfoPane_InstanceWithTaskAssignment|TestInfoPane_ProgressGroupsByWave' -v
```

expected: FAIL — new `InfoData` fields/types/sections not implemented.

**Step 3: write minimal implementation**

`ui/info_pane.go`:
1. Extend `InfoData` with:

```go
PlanGoal string
TaskTitle string

PlanningAt     time.Time
ImplementingAt time.Time
ReviewingAt    time.Time
DoneAt         time.Time

AllWaveSubtasks []WaveSubtaskGroup
CompletedTasks  int
TotalSubtasks   int
```

2. Add display types:

```go
type SubtaskDisplay struct {
    Number int
    Title  string
    Status string // pending|running|complete|failed
}

type WaveSubtaskGroup struct {
    WaveNumber int
    Subtasks   []SubtaskDisplay
}
```

3. Keep existing behavior intact:
- `ZoneViewPlan` button still rendered in plan-header view.
- fallback `"no instance selected"` unchanged.
- existing status colors remain via `statusColor`.

4. Refactor `renderPlanSummary()` into ordered sections:
- metadata (existing rows),
- goal (`renderGoalSection()`),
- lifecycle (`renderLifecycleSection()`),
- progress (`renderProgressSection()`),
- instances summary,
- button.

5. Add helpers with exact signatures:

```go
func (p *InfoPane) renderGoalSection() string
func (p *InfoPane) renderLifecycleSection() string
func (p *InfoPane) renderProgressSection() string
func (p *InfoPane) subtaskGlyph(status string) (string, color.Color)
```

Use ASCII for bar body to avoid glyph width issues in viewport:
- e.g. `[####----]` with width based on available pane width.

6. Update `renderInstanceSection()`:
- insert `goal` row when `PlanGoal != ""`.
- render task row as `task 3 of 6: http endpoints` when `TaskTitle != ""`; fallback to old `3 of 6` format if title missing.

Required imports for `ui/info_pane.go`:

```go
import (
    ...
    "time"
)
```

`app/app_state.go`:
1. In `updateInfoPaneForPlanHeader()`:
- set `PlanGoal`, `PlanningAt`, `ImplementingAt`, `ReviewingAt`, `DoneAt` from `taskstate.TaskEntry`.
- fetch subtasks from store (`m.taskStore.GetSubtasks(...)`) when store is non-nil.
- group by wave into `[]ui.WaveSubtaskGroup` sorted ascending wave then task number.
- compute `CompletedTasks` and `TotalSubtasks`.
- keep existing instance counters (`PlanInstanceCount`, etc.).

2. In `updateInfoPane()` (instance-selected):
- set `data.PlanGoal = entry.Goal`.
- resolve `TaskTitle` from orchestrator plan (use `orch.Plan()` and search all waves by task number, not only current wave).

Add local helpers in `app/app_state.go` to keep logic testable and deterministic:

```go
func groupSubtasksByWave(subtasks []taskstore.SubtaskEntry) ([]ui.WaveSubtaskGroup, int)
func findTaskTitle(plan *taskparser.Plan, taskNumber int) string
```

Edge cases:
- if `GetSubtasks` errors, log warning and render without progress groups (do not clear pane).
- if `TotalSubtasks == 0`, render progress as `0/0` and a fully empty bar.
- unknown status strings map to pending glyph/color.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run 'TestInfoPane_PlanSummaryWithGoalAndLifecycle|TestInfoPane_InstanceWithTaskAssignment|TestInfoPane_ProgressGroupsByWave' -v
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
