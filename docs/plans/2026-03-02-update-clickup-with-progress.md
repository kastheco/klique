# Update ClickUp with Progress Implementation Plan

**Goal:** Automatically post progress comments to ClickUp tasks as their associated kasmos plans advance through the lifecycle — plan ready, wave completion, review findings, and fixer results.

**Architecture:** Add a `clickup_task_id` column to the plan store schema so plans imported from ClickUp carry a structured reference to their source task. A new `internal/clickup/commenter.go` module wraps the MCP `clickup_create_task_comment` tool. The TUI's signal-processing and lifecycle hooks fire `tea.Cmd` goroutines that post comments at key transitions: plan finalized, wave completed (multi-wave only), review approved/changes-requested, and fixer spawned. All MCP calls run in background `tea.Cmd`s so the TUI stays responsive; failures are logged but never block the lifecycle.

**Tech Stack:** Go 1.24+, bubbletea, internal/clickup (MCP client), config/planstore (SQLite + HTTP), config/planstate

**Size:** Medium (estimated ~3 hours, 3 tasks, 2 waves)

---

## Wave 1: Data Layer — Store ClickUp Task ID with Plans

### Task 1: Add clickup_task_id to plan store schema, planstate, and ClickUp import

Add a `ClickUpTaskID` field to the plan store's data model, SQLite schema (with migration), planstate layer, and wire it into the ClickUp import flow. This is the foundational data plumbing that all subsequent tasks depend on.

The `importClickUpTask` method in `app_state.go` already has the `task.ID` — we add `SetClickUpTaskID` and `ClickUpTaskID` accessors to planstate and call them during import.

**Files:**
- Modify: `config/planstore/store.go`
- Modify: `config/planstore/sqlite.go`
- Modify: `config/planstate/planstate.go`
- Modify: `app/app_state.go`
- Test: `config/planstore/sqlite_test.go`
- Test: `config/planstate/planstate_test.go`
- Test: `app/clickup_import_test.go`

**Step 1: write the failing test**

```go
// In sqlite_test.go — verify the new field round-trips through Create/Get.
func TestClickUpTaskIDRoundTrip(t *testing.T) {
    store := newTestStore(t)
    entry := PlanEntry{
        Filename:      "test-plan.md",
        Status:        StatusReady,
        ClickUpTaskID: "abc123xyz",
    }
    require.NoError(t, store.Create("proj", entry))
    got, err := store.Get("proj", "test-plan.md")
    require.NoError(t, err)
    assert.Equal(t, "abc123xyz", got.ClickUpTaskID)
}

// In planstate_test.go — verify SetClickUpTaskID and ClickUpTaskID accessors.
func TestSetClickUpTaskID(t *testing.T) {
    ps := newTestPlanState(t)
    require.NoError(t, ps.Create("plan.md", "desc", "branch", "", time.Now()))
    require.NoError(t, ps.SetClickUpTaskID("plan.md", "cu_task_99"))
    entry, ok := ps.Entry("plan.md")
    require.True(t, ok)
    assert.Equal(t, "cu_task_99", entry.ClickUpTaskID)
    assert.Equal(t, "cu_task_99", ps.ClickUpTaskID("plan.md"))
}

func TestClickUpTaskIDEmpty(t *testing.T) {
    ps := newTestPlanState(t)
    assert.Equal(t, "", ps.ClickUpTaskID("nonexistent.md"))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/planstore/... -run TestClickUpTaskIDRoundTrip -v
go test ./config/planstate/... -run TestSetClickUpTaskID -v
```

expected: FAIL — `ClickUpTaskID` field and methods do not exist

**Step 3: write minimal implementation**

1. Add `ClickUpTaskID string` field to `planstore.PlanEntry` with JSON tag `"clickup_task_id,omitempty"`.
2. Add a migration in `sqlite.go` (following the `contentMigration` pattern): `ALTER TABLE plans ADD COLUMN clickup_task_id TEXT NOT NULL DEFAULT ''`. Add a `migrateAddClickUpTaskIDColumn` function called from `NewSQLiteStore`, using the same `PRAGMA table_info` check pattern as `migrateAddContentColumn`.
3. Update all SQL queries in `sqlite.go` to include `clickup_task_id` in INSERT, SELECT, and UPDATE statements. Update `scanPlanEntry` and `scanPlanEntries` to scan the new column.
4. Add `ClickUpTaskID string` field to `planstate.PlanEntry` with JSON tag `"clickup_task_id,omitempty"`. Update `toPlanstoreEntry` to copy it. Update `Load` to populate it from store entries.
5. Add `SetClickUpTaskID(filename, taskID string) error` to `planstate.PlanState` — reads entry, sets field, writes via `store.Update`.
6. Add `ClickUpTaskID(filename string) string` helper to `planstate.PlanState` — returns the task ID (empty string if none).
7. In `app/app_state.go` `importClickUpTask`, after `Register` succeeds, call `m.planState.SetClickUpTaskID(filename, task.ID)`.
8. The HTTP store and server serialize `PlanEntry` as JSON — the new field flows through automatically via JSON tags. No code changes needed in `http.go` or `server.go`.

**Step 4: run test to verify it passes**

```bash
go test ./config/planstore/... -run TestClickUpTaskIDRoundTrip -v
go test ./config/planstate/... -run "TestSetClickUpTaskID|TestClickUpTaskIDEmpty" -v
go test ./app/... -run TestClickUp -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/planstore/ config/planstate/ app/app_state.go app/clickup_import_test.go
git commit -m "feat(planstore): add clickup_task_id column and wire into import flow"
```

## Wave 2: Comment Posting — Hook Lifecycle Events to ClickUp

> **depends on wave 1:** The commenter needs `ClickUpTaskID` from the plan store to know which ClickUp task to comment on. The `SetClickUpTaskID` method must exist for import to populate it.

### Task 2: Create commenter, formatters, wire lifecycle hooks, and add postClickUpComment helper

Build the full comment-posting pipeline: a `Commenter` type in `internal/clickup` that wraps the MCP `clickup_create_task_comment` tool, structured format functions for each lifecycle event, a `Client()` accessor on `Importer`, a `postClickUpComment` helper on `home`, and the 6 lifecycle hook points in the TUI signal handlers.

The hook points are:

1. **Plan ready** (`PlannerFinished` signal in `app.go` ~line 890) — `FormatPlanReady`
2. **Wave completed** (wave orchestrator advance in `app.go` wave-advance handler, multi-wave only) — `FormatWaveComplete`
3. **All waves complete** (`waveAllCompleteMsg` in `app.go` ~line 1440) — `FormatAllWavesComplete`
4. **Review approved** (`ReviewApproved` signal in `app.go` ~line 865) — `FormatReviewApproved`
5. **Review changes requested** (`ReviewChangesRequested` signal in `app.go` ~line 877) — `FormatReviewChangesRequested`
6. **Fixer spawned** (fixer creation in `app_state.go`) — `FormatFixerSpawned`

**Files:**
- Create: `internal/clickup/commenter.go`
- Create: `internal/clickup/commenter_test.go`
- Create: `internal/clickup/comments.go`
- Create: `internal/clickup/comments_test.go`
- Modify: `internal/clickup/import.go` (expose `Client()` accessor and `NewImporterWithClient` test constructor on `Importer`)
- Modify: `app/app.go` (signal processing, wave completion handlers)
- Modify: `app/app_state.go` (`postClickUpComment` helper, fixer spawn hook)
- Create: `app/clickup_comment_test.go`

**Step 1: write the failing test**

```go
// In commenter_test.go — verify PostComment calls the right MCP tool with correct args.
func TestCommenterPostComment(t *testing.T) {
    mock := &mockMCPCaller{
        toolName: "clickup_create_task_comment",
        response: &mcpclient.ToolResult{Content: []mcpclient.Content{{Type: "text", Text: `{"id":"123"}`}}},
    }
    c := NewCommenter(mock, "ws_123")
    err := c.PostComment("task_456", "plan ready: my-feature")
    require.NoError(t, err)
    assert.Equal(t, "clickup_create_task_comment", mock.calledTool)
    assert.Equal(t, "task_456", mock.calledArgs["task_id"])
    assert.Contains(t, mock.calledArgs["comment_text"].(string), "plan ready: my-feature")
    assert.Equal(t, "ws_123", mock.calledArgs["workspace_id"])
}

func TestCommenterNoTool(t *testing.T) {
    mock := &mockMCPCaller{toolNotFound: true}
    c := NewCommenter(mock, "ws_123")
    err := c.PostComment("task_456", "test")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "no comment tool")
}

// In comments_test.go — verify formatters produce expected content.
func TestFormatPlanReady(t *testing.T) {
    msg := FormatPlanReady("my-feature")
    assert.Contains(t, msg, "my-feature")
    assert.Contains(t, msg, "plan finalized")
}

func TestFormatWaveComplete(t *testing.T) {
    msg := FormatWaveComplete("my-feature", 2, 3)
    assert.Contains(t, msg, "wave 2/3")
}

func TestFormatReviewApproved(t *testing.T) {
    msg := FormatReviewApproved("my-feature")
    assert.Contains(t, msg, "review approved")
}

func TestFormatReviewChangesRequested(t *testing.T) {
    msg := FormatReviewChangesRequested("my-feature", "fix the error handling in auth.go")
    assert.Contains(t, msg, "changes requested")
    assert.Contains(t, msg, "fix the error handling")
}

func TestFormatFixerSpawned(t *testing.T) {
    msg := FormatFixerSpawned("my-feature", "stuck in implementing state")
    assert.Contains(t, msg, "fixer")
    assert.Contains(t, msg, "stuck in implementing")
}

// In clickup_comment_test.go — verify postClickUpComment returns nil gracefully.
func TestPostClickUpCommentNoPlan(t *testing.T) {
    m := newTestHome(t)
    cmd := m.postClickUpComment("nonexistent.md", "test")
    assert.Nil(t, cmd, "should return nil when plan has no clickup task ID")
}

func TestPostClickUpCommentNoImporter(t *testing.T) {
    m := newTestHome(t)
    require.NoError(t, m.planState.Create("test.md", "desc", "branch", "", time.Now()))
    require.NoError(t, m.planState.SetClickUpTaskID("test.md", "cu_123"))
    m.loadPlanState()
    cmd := m.postClickUpComment("test.md", "test")
    assert.Nil(t, cmd, "should return nil when no ClickUp importer available")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./internal/clickup/... -run "TestCommenter|TestFormat" -v
go test ./app/... -run TestPostClickUpComment -v
```

expected: FAIL — `NewCommenter`, `FormatPlanReady`, `postClickUpComment` do not exist

**Step 3: write minimal implementation**

**commenter.go:**

```go
package clickup

import "fmt"

// Commenter posts progress comments to ClickUp tasks via MCP.
type Commenter struct {
    client      MCPCaller
    workspaceID string
}

// NewCommenter creates a Commenter with the given MCP client and workspace ID.
func NewCommenter(client MCPCaller, workspaceID string) *Commenter {
    return &Commenter{client: client, workspaceID: workspaceID}
}

// PostComment posts a comment to the given ClickUp task.
// Returns nil on success. Errors are informational — callers should log, not fail.
func (c *Commenter) PostComment(taskID, text string) error {
    tool, found := c.client.FindTool("clickup_create_task_comment")
    if !found {
        return fmt.Errorf("no comment tool found in MCP server")
    }
    args := map[string]interface{}{
        "task_id":      taskID,
        "comment_text": text,
    }
    if c.workspaceID != "" {
        args["workspace_id"] = c.workspaceID
    }
    _, err := c.client.CallTool(tool.Name, args)
    if err != nil {
        return fmt.Errorf("post comment: %w", err)
    }
    return nil
}
```

**comments.go:**

```go
package clickup

import "fmt"

// FormatPlanReady returns a comment for when a plan is finalized.
func FormatPlanReady(planName string) string {
    return fmt.Sprintf("**[kasmos] plan finalized:** `%s`\n\nthe implementation plan has been reviewed and is ready for execution.", planName)
}

// FormatWaveComplete returns a comment for wave completion in multi-wave plans.
func FormatWaveComplete(planName string, wave, totalWaves int) string {
    return fmt.Sprintf("**[kasmos] wave %d/%d complete:** `%s`\n\nmoving to the next wave of implementation.", wave, totalWaves, planName)
}

// FormatAllWavesComplete returns a comment when all waves finish.
func FormatAllWavesComplete(planName string, totalWaves int) string {
    return fmt.Sprintf("**[kasmos] all %d waves complete:** `%s`\n\nimplementation finished — review started.", totalWaves, planName)
}

// FormatReviewApproved returns a comment for review approval.
func FormatReviewApproved(planName string) string {
    return fmt.Sprintf("**[kasmos] review approved:** `%s`\n\nimplementation passed review — ready to merge.", planName)
}

// FormatReviewChangesRequested returns a comment with review feedback.
func FormatReviewChangesRequested(planName, feedback string) string {
    if len(feedback) > 500 {
        feedback = feedback[:500] + "..."
    }
    return fmt.Sprintf("**[kasmos] changes requested:** `%s`\n\nreviewer feedback:\n> %s", planName, feedback)
}

// FormatFixerSpawned returns a comment when a fixer agent is spawned.
func FormatFixerSpawned(planName, reason string) string {
    return fmt.Sprintf("**[kasmos] fixer agent spawned:** `%s`\n\nreason: %s", planName, reason)
}
```

**import.go** — add `Client()` accessor and `NewImporterWithClient` test constructor:

```go
// Client returns the underlying MCP caller for reuse by other modules (e.g. Commenter).
func (im *Importer) Client() MCPCaller {
    return im.client
}

// NewImporterWithClient creates an Importer with a pre-existing MCP client.
// Intended for testing — production code should use NewImporter.
func NewImporterWithClient(client MCPCaller) *Importer {
    return &Importer{client: client}
}
```

**app_state.go** — add `postClickUpComment` helper:

```go
// postClickUpComment returns a tea.Cmd that posts a comment to the ClickUp task
// associated with the given plan. Returns nil if the plan has no ClickUp task ID
// or no ClickUp MCP client is available. Errors are logged, never surfaced.
func (m *home) postClickUpComment(planFile, commentText string) tea.Cmd {
    if m.planState == nil {
        return nil
    }
    taskID := m.planState.ClickUpTaskID(planFile)
    if taskID == "" {
        return nil
    }
    if m.clickUpImporter == nil {
        return nil
    }
    workspaceID := ""
    if projCfg := clickup.LoadProjectConfig(m.activeRepoPath); projCfg.WorkspaceID != "" {
        workspaceID = projCfg.WorkspaceID
    }
    commenter := clickup.NewCommenter(m.clickUpImporter.Client(), workspaceID)
    return func() tea.Msg {
        if err := commenter.PostComment(taskID, commentText); err != nil {
            log.WarningLog.Printf("clickup comment for %s: %v", planFile, err)
        }
        return nil
    }
}
```

Then wire the 6 hook points in `app.go` and `app_state.go`. Each follows this pattern — add after the existing side-effect logic at each transition:

```go
// Example: PlannerFinished signal handler (~line 894)
if cmd := m.postClickUpComment(capturedPlanFile, clickup.FormatPlanReady(planstate.DisplayName(capturedPlanFile))); cmd != nil {
    signalCmds = append(signalCmds, cmd)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./internal/clickup/... -run "TestCommenter|TestFormat" -v
go test ./app/... -run TestPostClickUpComment -v
go test ./app/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add internal/clickup/commenter.go internal/clickup/commenter_test.go internal/clickup/comments.go internal/clickup/comments_test.go internal/clickup/import.go app/app.go app/app_state.go app/clickup_comment_test.go
git commit -m "feat(clickup): commenter, formatters, and lifecycle hook wiring"
```

### Task 3: Integration test — full lifecycle comment flow

Add an integration-style test that exercises the full flow: create a plan with a ClickUp task ID, set up a mock MCP caller on the importer (using `NewImporterWithClient` from Task 2), call `postClickUpComment` for each event type, and verify the mock received the correct task IDs and comment content.

**Files:**
- Create: `app/clickup_lifecycle_test.go`

**Step 1: write the failing test**

```go
// In clickup_lifecycle_test.go
func TestClickUpCommentLifecycle(t *testing.T) {
    // Setup: create a plan state with a ClickUp task ID.
    m := newTestHome(t)
    require.NoError(t, m.planState.Create("test.md", "desc", "plan/test", "", time.Now()))
    require.NoError(t, m.planState.SetClickUpTaskID("test.md", "cu_lifecycle"))
    m.loadPlanState()

    // Verify postClickUpComment produces a non-nil cmd when importer is set.
    mock := newMockClickUpMCPCaller()
    m.clickUpImporter = clickup.NewImporterWithClient(mock)

    planName := planstate.DisplayName("test.md")

    // Test each lifecycle comment format.
    cmd := m.postClickUpComment("test.md", clickup.FormatPlanReady(planName))
    assert.NotNil(t, cmd, "should return cmd when importer and task ID exist")

    // Execute the cmd to trigger the mock.
    cmd() // fire-and-forget returns nil msg

    assert.Equal(t, 1, mock.commentCount)
    assert.Equal(t, "cu_lifecycle", mock.lastTaskID)
    assert.Contains(t, mock.lastComment, "plan finalized")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run TestClickUpCommentLifecycle -v
```

expected: FAIL — mock infrastructure (`newMockClickUpMCPCaller`) doesn't exist yet

**Step 3: write minimal implementation**

1. Build `mockClickUpMCPCaller` in `app/clickup_lifecycle_test.go` — implements `MCPCaller`, records `CallTool` invocations, returns success. Tracks `commentCount`, `lastTaskID`, `lastComment` for assertions.
2. Write the lifecycle test covering plan-ready, wave-complete, review-approved, review-changes-requested, and fixer-spawned comment types.

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run TestClickUpCommentLifecycle -v
go test ./internal/clickup/... -v
go test ./config/planstore/... -v
go test ./config/planstate/... -v
```

expected: PASS — all tests pass including the new lifecycle test

**Step 5: commit**

```bash
git add app/clickup_lifecycle_test.go
git commit -m "test(clickup): integration test for lifecycle comment flow"
```
