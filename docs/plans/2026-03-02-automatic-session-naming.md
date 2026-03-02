# Automatic Session Naming Implementation Plan

**Goal:** Set meaningful opencode session titles when kasmos spawns agent sessions, replacing the broken auto-generated titles (e.g. "I'll load the required skills first, then generate the title.") with descriptive names derived from the plan, agent type, and wave/task context.

**Architecture:** A new `internal/opencodesession` package provides two capabilities: (1) a pure `BuildTitle` function that maps instance metadata (plan name, agent type, wave/task number) to a human-readable title string, and (2) a `ClaimAndSetTitle` function that atomically claims an unclaimed opencode session in the SQLite DB and sets its title. The tmux `Start()` method records a pre-launch timestamp, then after detecting the "Ask anything" ready string, calls the title-setter. For parallel wave tasks sharing the same worktree directory, an atomic compare-and-swap pattern (`UPDATE ... WHERE title NOT LIKE 'kas: %'`) prevents two goroutines from claiming the same DB session. A `kas: ` prefix on set titles distinguishes kasmos-managed titles from opencode's auto-generated ones.

**Tech Stack:** Go, `database/sql` + `modernc.org/sqlite` (already a transitive dep via planstore), `session/tmux` package.

**Size:** Small (estimated ~1.5 hours, 3 tasks, 2 waves)

---

## Wave 1: independent components

### Task 1: title builder and DB updater

**Files:**
- Create: `internal/opencodesession/title.go`
- Create: `internal/opencodesession/title_test.go`

**Step 1: write the failing test**

```go
package opencodesession

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		name string
		opts TitleOpts
		want string
	}{
		{
			name: "planner session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "planner"},
			want: "kas: plan automatic-session-naming",
		},
		{
			name: "coder session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "coder"},
			want: "kas: implement automatic-session-naming",
		},
		{
			name: "reviewer session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "reviewer"},
			want: "kas: review automatic-session-naming",
		},
		{
			name: "wave task session",
			opts: TitleOpts{PlanName: "automatic-session-naming", AgentType: "coder", WaveNumber: 2, TaskNumber: 3},
			want: "kas: implement automatic-session-naming w2/t3",
		},
		{
			name: "fixer session",
			opts: TitleOpts{InstanceTitle: "fix-login-bug", AgentType: "fixer"},
			want: "kas: fix fix-login-bug",
		},
		{
			name: "ad-hoc session no agent type",
			opts: TitleOpts{InstanceTitle: "my-session"},
			want: "kas: my-session",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTitle(tt.opts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE session (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		title TEXT NOT NULL,
		directory TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	return db
}

func TestClaimAndSetTitle_SingleSession(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_1', 'proj_1', 'auto-generated garbage', '/work/dir', ?, ?)`, now, now)
	require.NoError(t, err)

	beforeStart := time.UnixMilli(now - 100)
	err = ClaimAndSetTitle(db, "/work/dir", beforeStart, "kas: plan my-feature")
	require.NoError(t, err)

	var title string
	err = db.QueryRow(`SELECT title FROM session WHERE id = 'ses_1'`).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "kas: plan my-feature", title)
}

func TestClaimAndSetTitle_SkipsAlreadyClaimed(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	// First session already claimed by kasmos
	_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_1', 'proj_1', 'kas: implement other-plan', '/work/dir', ?, ?)`, now-50, now-50)
	require.NoError(t, err)
	// Second session unclaimed
	_, err = db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
		VALUES ('ses_2', 'proj_1', 'I will load skills first', '/work/dir', ?, ?)`, now, now)
	require.NoError(t, err)

	beforeStart := time.UnixMilli(now - 100)
	err = ClaimAndSetTitle(db, "/work/dir", beforeStart, "kas: implement my-feature w1/t2")
	require.NoError(t, err)

	var title string
	err = db.QueryRow(`SELECT title FROM session WHERE id = 'ses_2'`).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "kas: implement my-feature w1/t2", title)
}

func TestClaimAndSetTitle_NoMatchReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	// No sessions in DB
	err := ClaimAndSetTitle(db, "/work/dir", time.Now(), "kas: plan foo")
	assert.NoError(t, err) // best-effort, no error on miss
}

func TestClaimAndSetTitle_ParallelClaims(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UnixMilli()
	// Three sessions created nearly simultaneously (simulating parallel wave tasks)
	for i, id := range []string{"ses_1", "ses_2", "ses_3"} {
		_, err := db.Exec(`INSERT INTO session (id, project_id, title, directory, time_created, time_updated)
			VALUES (?, 'proj_1', 'garbage title', '/shared/worktree', ?, ?)`,
			id, now+int64(i*100), now+int64(i*100))
		require.NoError(t, err)
	}

	beforeStart := time.UnixMilli(now - 50)

	// Simulate three goroutines claiming sequentially (real parallelism tested by SQLite's single-writer lock)
	err := ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t1")
	require.NoError(t, err)
	err = ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t2")
	require.NoError(t, err)
	err = ClaimAndSetTitle(db, "/shared/worktree", beforeStart, "kas: implement plan w1/t3")
	require.NoError(t, err)

	// Each session should have a unique title
	titles := make(map[string]bool)
	rows, err := db.Query(`SELECT title FROM session WHERE directory = '/shared/worktree' ORDER BY time_created ASC`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var title string
		require.NoError(t, rows.Scan(&title))
		titles[title] = true
	}
	assert.Len(t, titles, 3)
	assert.True(t, titles["kas: implement plan w1/t1"])
	assert.True(t, titles["kas: implement plan w1/t2"])
	assert.True(t, titles["kas: implement plan w1/t3"])
}
```

**Step 2: run test to verify it fails**

```bash
go test ./internal/opencodesession/... -run 'TestBuildTitle|TestClaimAndSetTitle' -v
```

expected: FAIL — package does not exist

**Step 3: write minimal implementation**

`TitleOpts` struct holds plan name, agent type, wave/task numbers, and instance title. `BuildTitle` maps agent types to verb prefixes ("plan", "implement", "review", "fix") and appends wave/task context as `w2/t3` when present. All titles are prefixed with `kas: ` to mark them as kasmos-managed. Falls back to instance title for ad-hoc sessions.

`ClaimAndSetTitle(db *sql.DB, workDir string, beforeStart time.Time, title string) error` implements the atomic claim pattern:
1. Query all unclaimed sessions: `SELECT id FROM session WHERE directory = ? AND time_created >= ? AND title NOT LIKE 'kas: %' ORDER BY time_created ASC`
2. For each candidate, attempt atomic claim: `UPDATE session SET title = ?, time_updated = ? WHERE id = ? AND title NOT LIKE 'kas: %'`
3. Check `RowsAffected()` — if 1, claim succeeded, return. If 0, another goroutine claimed it, try next candidate.
4. If no candidates remain, return nil (best-effort, not an error).

`SetTitle(workDir string, beforeStart time.Time, opts TitleOpts) error` is the public entry point: builds the title, resolves the DB path from `$XDG_DATA_HOME/opencode/opencode.db` (or `~/.local/share/opencode/opencode.db`), opens the DB in WAL mode, and calls `ClaimAndSetTitle`. Opens the DB fresh each time (not pooled) since this runs once per session startup and the DB belongs to opencode.

**Step 4: run test to verify it passes**

```bash
go test ./internal/opencodesession/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add internal/opencodesession/title.go internal/opencodesession/title_test.go
git commit -m "feat: add opencode session title builder with atomic claim for parallel tasks"
```

### Task 2: wire title setting into tmux session startup

**Files:**
- Modify: `session/tmux/tmux.go`
- Create: `session/tmux/session_title_test.go`

**Step 1: write the failing test**

```go
package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTmuxSession_SessionTitle(t *testing.T) {
	ts := &TmuxSession{}

	// Default: no title, no callback
	assert.Nil(t, ts.titleFunc)

	// Set title callback
	called := false
	var capturedDir string
	var capturedBefore time.Time
	var capturedTitle string
	ts.SetTitleFunc(func(workDir string, beforeStart time.Time, title string) {
		called = true
		capturedDir = workDir
		capturedBefore = beforeStart
		capturedTitle = title
	})
	ts.sessionTitle = "kas: plan my-feature"

	assert.NotNil(t, ts.titleFunc)

	// Simulate the call that Start() would make
	ts.titleFunc("/work/dir", time.Now(), ts.sessionTitle)
	assert.True(t, called)
	assert.Equal(t, "/work/dir", capturedDir)
	assert.Equal(t, "kas: plan my-feature", capturedTitle)
	assert.False(t, capturedBefore.IsZero())
}

func TestTmuxSession_SessionTitle_SkippedWhenEmpty(t *testing.T) {
	ts := &TmuxSession{}
	// No title set — titleFunc should not be called even if set
	called := false
	ts.SetTitleFunc(func(string, time.Time, string) { called = true })
	// sessionTitle is empty, so Start() would skip the call
	assert.Empty(t, ts.sessionTitle)
	assert.False(t, called)
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/tmux/... -run TestTmuxSession_SessionTitle -v
```

expected: FAIL — `titleFunc` and `sessionTitle` fields do not exist

**Step 3: write minimal implementation**

Add two fields to `TmuxSession`:
- `sessionTitle string` — the desired title for the opencode session
- `titleFunc func(workDir string, beforeStart time.Time, title string)` — callback that performs the actual DB update (injected by the instance layer to avoid a direct dependency from tmux → opencodesession)

Add setter methods:
- `SetSessionTitle(title string)` — sets the title string
- `SetTitleFunc(fn func(string, time.Time, string))` — sets the callback

In `Start()`:
1. Record `beforeStart := time.Now()` just before the `exec.Command("tmux", "new-session", ...)` call
2. After the "Ask anything" detection loop (for opencode programs only), if `sessionTitle != ""` and `titleFunc != nil`, call `go t.titleFunc(workDir, beforeStart, t.sessionTitle)` in a goroutine (best-effort, non-blocking — title setting must not delay startup or block the UI)

The goroutine is fire-and-forget. Title setting is cosmetic — if it fails, the session still works fine with the auto-generated garbage title.

**Step 4: run test to verify it passes**

```bash
go test ./session/tmux/... -run TestTmuxSession_SessionTitle -v
```

expected: PASS

**Step 5: commit**

```bash
git add session/tmux/tmux.go session/tmux/session_title_test.go
git commit -m "feat: set opencode session title after startup via callback"
```

## Wave 2: integration wiring

> **depends on wave 1:** Task 3 imports `internal/opencodesession.BuildTitle` (Task 1) and calls `tmux.SetSessionTitle`/`SetTitleFunc` (Task 2). Cannot compile without both.

### Task 3: wire title from instance metadata through to opencode

**Files:**
- Modify: `session/instance_lifecycle.go`
- Modify: `session/instance.go` (add import for opencodesession)
- Create: `session/instance_title_test.go`

**Step 1: write the failing test**

```go
package session

import (
	"testing"

	"github.com/kastheco/kasmos/internal/opencodesession"
	"github.com/stretchr/testify/assert"
)

func TestBuildTitleOptsFromInstance(t *testing.T) {
	tests := []struct {
		name string
		inst *Instance
		want string
	}{
		{
			name: "planner with plan file",
			inst: &Instance{
				PlanFile:  "2026-03-02-automatic-session-naming.md",
				AgentType: AgentTypePlanner,
				Title:     "automatic-session-naming-plan",
			},
			want: "kas: plan automatic-session-naming",
		},
		{
			name: "coder wave task",
			inst: &Instance{
				PlanFile:   "2026-03-02-automatic-session-naming.md",
				AgentType:  AgentTypeCoder,
				WaveNumber: 2,
				TaskNumber: 3,
				Title:      "automatic-session-naming-W2-T3",
			},
			want: "kas: implement automatic-session-naming w2/t3",
		},
		{
			name: "reviewer",
			inst: &Instance{
				PlanFile:  "2026-03-02-automatic-session-naming.md",
				AgentType: AgentTypeReviewer,
				Title:     "automatic-session-naming-review",
			},
			want: "kas: review automatic-session-naming",
		},
		{
			name: "fixer ad-hoc",
			inst: &Instance{
				AgentType: AgentTypeFixer,
				Title:     "fix-login-bug",
			},
			want: "kas: fix fix-login-bug",
		},
		{
			name: "ad-hoc no agent type",
			inst: &Instance{
				Title: "my-session",
			},
			want: "kas: my-session",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildTitleOpts(tt.inst)
			got := opencodesession.BuildTitle(opts)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

**Step 2: run test to verify it fails**

```bash
go test ./session/... -run TestBuildTitleOptsFromInstance -v
```

expected: FAIL — `buildTitleOpts` undefined

**Step 3: write minimal implementation**

Add a `buildTitleOpts(inst *Instance) opencodesession.TitleOpts` helper in `instance_lifecycle.go` that extracts the plan display name (stripping the date prefix and `.md` suffix from `PlanFile`), agent type, wave/task numbers, and instance title.

Add a `configureSessionTitle()` method on `Instance` that:
1. Checks if the program is opencode (`tmux.IsOpenCodeProgram` — needs to be exported or use the existing `programSupportsCliPrompt` helper)
2. Builds `TitleOpts` via `buildTitleOpts`
3. Calls `tmuxSession.SetSessionTitle(opencodesession.BuildTitle(opts))`
4. Calls `tmuxSession.SetTitleFunc(opencodesession.SetTitleCallback())` — a factory function that returns the closure wrapping `SetTitle`

Call `configureSessionTitle()` from each Start variant (`Start`, `StartOnMainBranch`, `StartOnBranch`, `StartInSharedWorktree`) right after `setTmuxTaskEnv()` and before the tmux session actually starts.

The `opencodesession.SetTitleCallback()` factory returns `func(workDir string, beforeStart time.Time, title string)` that calls `SetTitle(workDir, beforeStart, TitleOpts{...})` — but since the title is already built, it just calls the lower-level `setTitleInDB` directly. Alternatively, the callback can simply call `opencodesession.SetTitleDirect(workDir, beforeStart, title)` which opens the DB and runs `ClaimAndSetTitle`.

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run TestBuildTitleOptsFromInstance -v
go test ./internal/opencodesession/... -v
```

expected: PASS

**Step 5: commit**

```bash
git add session/instance_lifecycle.go session/instance.go session/instance_title_test.go
git commit -m "feat: wire session title from instance metadata through to opencode DB"
```
