# Plan State Machine Gateway Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace scattered plan state mutations with a centralized FSM that enforces valid transitions, serializes writes with flock, and accepts agent input via sentinel files instead of direct JSON edits.

**Architecture:** New `config/planfsm` package owns all plan state mutations. The existing `config/planstate` package becomes read-only (queries, display helpers). All 22+ `SetStatus` calls in the app layer are replaced with `fsm.Transition(planFile, event)`. Agents drop sentinel files in `docs/plans/.signals/` which the metadata tick scans and feeds to the FSM.

**Tech Stack:** Go, `syscall.Flock` for file locking, bubbletea message pattern for signal delivery

**Waves:** 4 (T1-T3 sequential → T4-T5 sequential → T6-T8 sequential → T9-T10 sequential)

---

## Wave 1

### Task 1: Create `config/planfsm` package with FSM core

**Files:**
- Create: `config/planfsm/fsm.go`
- Create: `config/planfsm/fsm_test.go`

**Step 1: Write the failing test**

Create `config/planfsm/fsm_test.go`:

```go
package planfsm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransition_ValidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
		to    Status
	}{
		{StatusReady, PlanStart, StatusPlanning},
		{StatusPlanning, PlannerFinished, StatusReady},
		{StatusReady, ImplementStart, StatusImplementing},
		{StatusImplementing, ImplementFinished, StatusReviewing},
		{StatusReviewing, ReviewApproved, StatusDone},
		{StatusReviewing, ReviewChangesRequested, StatusImplementing},
		{StatusDone, StartOver, StatusImplementing},
		{StatusReady, Cancel, StatusCancelled},
		{StatusPlanning, Cancel, StatusCancelled},
		{StatusImplementing, Cancel, StatusCancelled},
		{StatusReviewing, Cancel, StatusCancelled},
		{StatusCancelled, Reopen, StatusPlanning},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			result, err := ApplyTransition(tc.from, tc.event)
			require.NoError(t, err)
			assert.Equal(t, tc.to, result)
		})
	}
}

func TestTransition_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
	}{
		{StatusReady, PlannerFinished},      // not planning
		{StatusReady, ImplementFinished},    // not implementing
		{StatusReady, ReviewApproved},       // not reviewing
		{StatusPlanning, ImplementStart},    // must go through ready
		{StatusImplementing, PlanStart},     // can't go backwards
		{StatusDone, PlanStart},             // terminal
		{StatusDone, ImplementFinished},     // terminal
		{StatusCancelled, ImplementStart},   // must reopen first
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			_, err := ApplyTransition(tc.from, tc.event)
			assert.Error(t, err)
		})
	}
}

func TestIsUserOnly(t *testing.T) {
	assert.True(t, StartOver.IsUserOnly())
	assert.True(t, Cancel.IsUserOnly())
	assert.True(t, Reopen.IsUserOnly())
	assert.False(t, PlannerFinished.IsUserOnly())
	assert.False(t, ReviewApproved.IsUserOnly())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/planfsm/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement the FSM core**

Create `config/planfsm/fsm.go`:

```go
package planfsm

import "fmt"

// Status represents the lifecycle state of a plan.
type Status string

const (
	StatusReady        Status = "ready"
	StatusPlanning     Status = "planning"
	StatusImplementing Status = "implementing"
	StatusReviewing    Status = "reviewing"
	StatusDone         Status = "done"
	StatusCancelled    Status = "cancelled"
)

// Event represents a lifecycle transition trigger.
type Event string

const (
	PlanStart              Event = "plan_start"
	PlannerFinished        Event = "planner_finished"
	ImplementStart         Event = "implement_start"
	ImplementFinished      Event = "implement_finished"
	ReviewApproved         Event = "review_approved"
	ReviewChangesRequested Event = "review_changes_requested"
	StartOver              Event = "start_over"
	Cancel                 Event = "cancel"
	Reopen                 Event = "reopen"
)

// IsUserOnly returns true if this event can only be triggered from the TUI,
// never by agent sentinel files.
func (e Event) IsUserOnly() bool {
	switch e {
	case StartOver, Cancel, Reopen:
		return true
	}
	return false
}

// transitionTable defines all valid state transitions.
// Key: current status → event → new status.
var transitionTable = map[Status]map[Event]Status{
	StatusReady: {
		PlanStart:      StatusPlanning,
		ImplementStart: StatusImplementing,
		Cancel:         StatusCancelled,
	},
	StatusPlanning: {
		PlannerFinished: StatusReady,
		Cancel:          StatusCancelled,
	},
	StatusImplementing: {
		ImplementFinished: StatusReviewing,
		Cancel:            StatusCancelled,
	},
	StatusReviewing: {
		ReviewApproved:         StatusDone,
		ReviewChangesRequested: StatusImplementing,
		Cancel:                 StatusCancelled,
	},
	StatusDone: {
		StartOver: StatusImplementing,
	},
	StatusCancelled: {
		Reopen: StatusPlanning,
	},
}

// ApplyTransition returns the new status for the given current status and event.
// Returns an error if the transition is not valid.
func ApplyTransition(current Status, event Event) (Status, error) {
	events, ok := transitionTable[current]
	if !ok {
		return "", fmt.Errorf("no transitions defined for status %q", current)
	}
	next, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid transition: %q + %q", current, event)
	}
	return next, nil
}
```

**Step 4: Run tests**

Run: `go test ./config/planfsm/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add config/planfsm/
git commit -m "feat(planfsm): add FSM core with transition table and validation"
```

### Task 2: Add file-locking `PlanStateMachine` that wraps FSM + planstate I/O

**Files:**
- Modify: `config/planfsm/fsm.go` (add PlanStateMachine struct)
- Create: `config/planfsm/lock.go` (flock helper)
- Modify: `config/planfsm/fsm_test.go` (add integration tests)

**Step 1: Write the failing test**

Add to `config/planfsm/fsm_test.go`:

```go
func TestPlanStateMachine_TransitionWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	// Seed with a ready plan
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test.md", "test plan", "plan/test", time.Now()))

	fsm := New(plansDir)
	err = fsm.Transition("test.md", PlanStart)
	require.NoError(t, err)

	// Re-read from disk to verify persistence
	reloaded, err := planstate.Load(plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test.md")
	require.True(t, ok)
	assert.Equal(t, "planning", string(entry.Status))
}

func TestPlanStateMachine_RejectsInvalidTransition(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test.md", "test plan", "plan/test", time.Now()))

	fsm := New(plansDir)
	err = fsm.Transition("test.md", ImplementFinished) // ready → implement_finished is invalid
	assert.Error(t, err)

	// Status must remain unchanged on disk
	reloaded, err := planstate.Load(plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test.md")
	require.True(t, ok)
	assert.Equal(t, "ready", string(entry.Status))
}

func TestPlanStateMachine_MissingPlanReturnsError(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	fsm := New(plansDir)
	err := fsm.Transition("nonexistent.md", PlanStart)
	assert.Error(t, err)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./config/planfsm/ -run TestPlanStateMachine -v`
Expected: FAIL — `New` and `Transition` method don't exist

**Step 3: Implement `PlanStateMachine`**

Add to `config/planfsm/fsm.go`:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/klique/config/planstate"
)

// PlanStateMachine is the sole writer of plan-state.json. All plan status
// mutations must flow through Transition(). File-level locking prevents
// concurrent writes from the TUI and goroutines.
type PlanStateMachine struct {
	dir string // docs/plans/ directory
}

// New creates a PlanStateMachine for the given plans directory.
func New(dir string) *PlanStateMachine {
	return &PlanStateMachine{dir: dir}
}

// Transition applies an event to a plan's current status. It reads the current
// state from disk, validates the transition, writes the new state, and returns.
// All I/O is serialized via flock.
func (m *PlanStateMachine) Transition(planFile string, event Event) error {
	return m.withLock(func() error {
		ps, err := planstate.Load(m.dir)
		if err != nil {
			return fmt.Errorf("load plan state: %w", err)
		}
		entry, ok := ps.Entry(planFile)
		if !ok {
			return fmt.Errorf("plan not found: %s", planFile)
		}
		currentStatus := mapLegacyStatus(entry.Status)
		newStatus, err := ApplyTransition(currentStatus, event)
		if err != nil {
			return err
		}
		entry.Status = planstate.Status(newStatus)
		ps.Plans[planFile] = entry
		return ps.Save()
	})
}

// mapLegacyStatus converts old planstate statuses to FSM statuses.
// Handles the consolidated aliases (in_progress → implementing, completed/finished → done).
func mapLegacyStatus(s planstate.Status) Status {
	switch s {
	case "in_progress":
		return StatusImplementing
	case "completed", "finished":
		return StatusDone
	default:
		return Status(s)
	}
}
```

Create `config/planfsm/lock.go`:

```go
package planfsm

import (
	"os"
	"path/filepath"
	"syscall"
)

const lockFile = ".plan-state.lock"

// withLock acquires an exclusive file lock, runs fn, then releases the lock.
func (m *PlanStateMachine) withLock(fn func() error) error {
	lockPath := filepath.Join(m.dir, lockFile)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fn() // fallback: run without lock if we can't create lock file
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fn() // fallback: run without lock
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return fn()
}
```

**Step 4: Run tests**

Run: `go test ./config/planfsm/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add config/planfsm/
git commit -m "feat(planfsm): add PlanStateMachine with flock and disk I/O"
```

### Task 3: Add sentinel file scanner

**Files:**
- Create: `config/planfsm/signals.go`
- Create: `config/planfsm/signals_test.go`

**Step 1: Write the failing test**

Create `config/planfsm/signals_test.go`:

```go
package planfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanSignals_ParsesValidSentinels(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-2026-02-22-foo.md"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "review-changes-2026-02-22-bar.md"),
		[]byte("fix the tests"), 0o644))

	signals := ScanSignals(dir)
	require.Len(t, signals, 2)

	// Sort by plan file for deterministic assertion
	if signals[0].PlanFile > signals[1].PlanFile {
		signals[0], signals[1] = signals[1], signals[0]
	}

	assert.Equal(t, PlannerFinished, signals[1].Event)
	assert.Equal(t, "2026-02-22-foo.md", signals[1].PlanFile)
	assert.Empty(t, signals[1].Body)

	assert.Equal(t, ReviewChangesRequested, signals[0].Event)
	assert.Equal(t, "2026-02-22-bar.md", signals[0].PlanFile)
	assert.Equal(t, "fix the tests", signals[0].Body)
}

func TestScanSignals_IgnoresInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "garbage-file.txt"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, ".hidden"),
		nil, 0o644))

	signals := ScanSignals(dir)
	assert.Empty(t, signals)
}

func TestScanSignals_EmptyDirReturnsNil(t *testing.T) {
	dir := t.TempDir()
	signals := ScanSignals(dir)
	assert.Nil(t, signals)
}

func TestScanSignals_RejectsUserOnlyEvents(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// An agent trying to drop a cancel sentinel — should be ignored
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "cancel-2026-02-22-foo.md"),
		nil, 0o644))

	signals := ScanSignals(dir)
	assert.Empty(t, signals)
}

func TestConsumeSignal_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	path := filepath.Join(signalsDir, "planner-finished-test.md")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	sig := Signal{Event: PlannerFinished, PlanFile: "test.md", filePath: path}
	ConsumeSignal(sig)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/planfsm/ -run TestScanSignals -v`
Expected: FAIL — `ScanSignals`, `Signal`, `ConsumeSignal` don't exist

**Step 3: Implement signal scanner**

Create `config/planfsm/signals.go`:

```go
package planfsm

import (
	"os"
	"path/filepath"
	"strings"
)

// Signal represents a parsed sentinel file from an agent.
type Signal struct {
	Event    Event
	PlanFile string
	Body     string   // file contents (e.g. review feedback)
	filePath string   // full path for deletion
}

// sentinelPrefixes maps filename prefixes to FSM events.
var sentinelPrefixes = []struct {
	prefix string
	event  Event
}{
	{"planner-finished-", PlannerFinished},
	{"implement-finished-", ImplementFinished},
	{"review-approved-", ReviewApproved},
	{"review-changes-", ReviewChangesRequested},
}

// ScanSignals reads docs/plans/.signals/ and returns parsed signals.
// Ignores invalid files and user-only events. Returns nil if directory missing.
func ScanSignals(plansDir string) []Signal {
	signalsDir := filepath.Join(plansDir, ".signals")
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}

	var signals []Signal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sig, ok := parseSignal(signalsDir, entry.Name())
		if !ok {
			continue
		}
		if sig.Event.IsUserOnly() {
			continue
		}
		signals = append(signals, sig)
	}
	return signals
}

// ConsumeSignal deletes the sentinel file after processing.
func ConsumeSignal(sig Signal) {
	_ = os.Remove(sig.filePath)
}

func parseSignal(dir, filename string) (Signal, bool) {
	for _, sp := range sentinelPrefixes {
		if strings.HasPrefix(filename, sp.prefix) {
			planFile := strings.TrimPrefix(filename, sp.prefix)
			if planFile == "" {
				return Signal{}, false
			}
			filePath := filepath.Join(dir, filename)
			body := ""
			if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
				body = strings.TrimSpace(string(data))
			}
			return Signal{
				Event:    sp.event,
				PlanFile: planFile,
				Body:     body,
				filePath: filePath,
			}, true
		}
	}
	return Signal{}, false
}
```

**Step 4: Run tests**

Run: `go test ./config/planfsm/ -v`
Expected: all PASS

**Step 5: Add `.signals/` to `.gitignore`**

Append `docs/plans/.signals/` to `.gitignore`.

**Step 6: Commit**

```bash
git add config/planfsm/signals.go config/planfsm/signals_test.go .gitignore
git commit -m "feat(planfsm): add sentinel file scanner and consumer"
```

## Wave 2

### Task 4: Wire FSM into the app layer — replace `SetStatus` calls

**Files:**
- Modify: `app/app.go` — add `fsm *planfsm.PlanStateMachine` field, initialize in `newHome`
- Modify: `app/app_actions.go` — replace all `SetStatus` calls with `fsm.Transition`
- Modify: `app/app_state.go` — replace `SetStatus` calls in `transitionToReview`, `promptPushBranchThenAdvance`
- Modify: `app/app.go` — replace `SetStatus` calls in metadata tick handler

This is the largest task. The key mapping:

| Old call | New call |
|----------|----------|
| `planState.SetStatus(f, StatusPlanning)` | `fsm.Transition(f, planfsm.PlanStart)` |
| `planState.SetStatus(f, StatusImplementing)` | `fsm.Transition(f, planfsm.ImplementStart)` |
| `planState.SetStatus(f, StatusReviewing)` | `fsm.Transition(f, planfsm.ImplementFinished)` |
| `planState.SetStatus(f, StatusCompleted)` | `fsm.Transition(f, planfsm.ReviewApproved)` |
| `planState.SetStatus(f, StatusCancelled)` | `fsm.Transition(f, planfsm.Cancel)` |

Special cases:
- `modify_plan` action sets `StatusPlanning` — this is `PlanStart` (ready → planning)
- `start_over_plan` resets to planning — this is `StartOver` then `PlanStart` won't work because `StartOver` goes to `implementing`. Instead, `start_over_plan` should use a dedicated reset path. For now, treat start-over as `Cancel` + `Reopen` (cancelled → planning). Or add the status directly since start-over is user-only and already does branch reset. **Decision: keep start-over as a direct status set on the FSM (add `StartOver` → `StatusPlanning` as a special case), OR do Cancel + Reopen as two transitions.** The cleaner approach: change `StartOver` target to `StatusPlanning` since the user is starting the whole cycle over. Update the FSM table:

```
done → planning (StartOver)     [user-only]
```

Update `config/planfsm/fsm.go` transition table and tests accordingly.

**Step 1: Update FSM transition table for StartOver**

Change `StartOver` target from `StatusImplementing` to `StatusPlanning` in `fsm.go` and update tests.

**Step 2: Add `fsm` field to `home` struct**

In `app/app.go`, add:
```go
	fsm *planfsm.PlanStateMachine
```

Initialize in `newHome`:
```go
	h.fsm = planfsm.New(h.planStateDir)
```

**Step 3: Replace all `SetStatus` calls in `app_actions.go`**

Work through each call site systematically. The `planStageStatus` helper function is deleted entirely — the FSM replaces it.

For the `isLocked` function: replace status checks with FSM-aware logic. The FSM itself rejects invalid transitions, so `isLocked` can be simplified or removed (the FSM error becomes the "locked" signal).

**Step 4: Replace `SetStatus` calls in `app_state.go`**

- `transitionToReview`: `SetStatus(planFile, StatusReviewing)` → `fsm.Transition(planFile, ImplementFinished)`
- `promptPushBranchThenAdvance`: `SetStatus(inst.PlanFile, StatusReviewing)` → `fsm.Transition(inst.PlanFile, ImplementFinished)`

**Step 5: Replace `SetStatus` calls in `app.go` metadata tick**

- Reviewer completion: `SetStatus(inst.PlanFile, StatusCompleted)` → `fsm.Transition(inst.PlanFile, ReviewApproved)`

**Step 6: Verify it compiles and tests pass**

Run: `go build ./... && go test ./app/ -v -count=1`
Expected: clean compile, all tests pass (some tests may need updating to use FSM)

**Step 7: Commit**

```bash
git add app/ config/planfsm/
git commit -m "refactor: replace SetStatus calls with FSM transitions"
```

### Task 5: Add signal scanning to metadata tick and process signals in Update

**Files:**
- Modify: `app/app.go` — scan signals in metadata goroutine, process in Update handler
- Modify: `config/planfsm/fsm.go` — (if needed) add method to process signal batch

**Step 1: Add signals field to `metadataResultMsg`**

In `app/app.go`, add to `metadataResultMsg`:
```go
	Signals []planfsm.Signal
```

**Step 2: Scan signals in metadata goroutine**

In the metadata tick goroutine (around line 461), after loading plan state, add:
```go
	var signals []planfsm.Signal
	if planStateDir != "" {
		signals = planfsm.ScanSignals(planStateDir)
	}
```

Include `signals` in the returned `metadataResultMsg`.

**Step 3: Process signals in the Update handler**

In the `metadataResultMsg` case (around line 474), before the existing instance processing, add signal processing:

```go
	// Process agent signals — feed to FSM and consume sentinel files.
	for _, sig := range msg.Signals {
		if err := m.fsm.Transition(sig.PlanFile, sig.Event); err != nil {
			log.WarningLog.Printf("signal %s for %s rejected: %v", sig.Event, sig.PlanFile, err)
		}
		planfsm.ConsumeSignal(sig)
		// If review-changes, store feedback for the next coder session.
		if sig.Event == planfsm.ReviewChangesRequested && sig.Body != "" {
			m.pendingReviewFeedback[sig.PlanFile] = sig.Body
		}
	}
	if len(msg.Signals) > 0 {
		m.loadPlanState() // refresh after signal processing
	}
```

Add `pendingReviewFeedback map[string]string` to the `home` struct and initialize in `newHome`.

**Step 4: Verify it compiles and tests pass**

Run: `go build ./... && go test ./app/ -v -count=1`

**Step 5: Commit**

```bash
git add app/app.go
git commit -m "feat: scan and process agent sentinel signals in metadata tick"
```

## Wave 3

### Task 6: Remove planner-exit, coder-exit, reviewer-exit ad-hoc detection

Now that signals exist, the detection patterns in the metadata tick can be simplified. The planner writes a `planner-finished-*` sentinel when done (instead of klique detecting tmux death + status check). Similarly for coder and reviewer.

**However**, this is a gradual migration. For now, keep both paths working:
- Sentinel signals are the preferred path (agents write them)
- Tmux-death detection remains as a fallback (catches agents that crash without writing a sentinel)

**Files:**
- Modify: `app/app.go` — simplify planner-exit detection to only handle the "no sentinel" fallback case

**Step 1: Refactor planner-exit detection**

The planner-exit block (lines 578-610) currently detects dead planner + StatusReady and shows a confirm dialog. With signals, the FSM handles `PlannerFinished` automatically. The tmux-death detection becomes a fallback:

- If a planner pane dies AND no `planner-finished-*` sentinel was processed this tick AND plan is still `StatusPlanning` (agent crashed without writing sentinel) → show error toast, leave plan in `planning` state
- If plan is `StatusReady` (sentinel was processed, FSM transitioned) → show the implement confirmation dialog (existing behavior)

The key insight: the planner-exit confirmation dialog stays, but it's triggered by the FSM transition to `ready` rather than by tmux death detection.

**Step 2: Verify existing tests still pass**

Run: `go test ./app/ -v -count=1`

**Step 3: Commit**

```bash
git add app/app.go
git commit -m "refactor: simplify planner-exit detection with signal fallback"
```

### Task 7: Consolidate status constants — remove duplicates from `planstate`

**Files:**
- Modify: `config/planstate/planstate.go` — remove duplicate statuses, add migration helper
- Modify: `config/planstate/planstate.go` — update query methods to use consolidated statuses
- Modify: `ui/sidebar.go` — update status comparisons

**Step 1: Remove duplicate status constants**

In `config/planstate/planstate.go`, remove:
- `StatusInProgress` (alias for `StatusImplementing`)
- `StatusCompleted` (alias for `StatusDone`)
- `StatusFinished` (alias for `StatusDone`)

Keep only: `StatusReady`, `StatusPlanning`, `StatusImplementing`, `StatusReviewing`, `StatusDone`, `StatusCancelled`.

**Step 2: Add migration in `Load`**

In the `Load` function, after parsing, normalize any legacy statuses:
```go
for filename, entry := range wrapped.Plans {
    switch entry.Status {
    case "in_progress":
        entry.Status = StatusImplementing
    case "completed", "finished":
        entry.Status = StatusDone
    }
    wrapped.Plans[filename] = entry
}
```

**Step 3: Update query methods**

Methods like `Unfinished()`, `Finished()`, `IsDone()` that check for multiple terminal statuses now only need to check `StatusDone` and `StatusCancelled`.

**Step 4: Update sidebar status comparisons**

In `ui/sidebar.go`, replace `StatusInProgress` with `StatusImplementing` and `StatusCompleted` with `StatusDone`.

**Step 5: Verify everything compiles and tests pass**

Run: `go build ./... && go test ./... -v -count=1`

**Step 6: Commit**

```bash
git add config/planstate/ ui/sidebar.go
git commit -m "refactor: consolidate plan status constants, add legacy migration"
```

### Task 8: Delete `planStageStatus` helper and `SetStatus` from planstate

**Files:**
- Modify: `config/planstate/planstate.go` — remove `SetStatus` method (FSM is sole writer)
- Modify: `app/app_actions.go` — delete `planStageStatus` function

**Step 1: Remove `SetStatus` from planstate**

The `PlanStateMachine.Transition` now owns all writes. Remove the `SetStatus` method. Keep `Save()` as it's still needed by the FSM internally and by `Create`/`Register`.

**Step 2: Delete `planStageStatus` function**

In `app/app_actions.go`, the `planStageStatus` function (lines 505-517) is no longer needed — all callers now use `fsm.Transition`.

**Step 3: Verify compile + tests**

Run: `go build ./... && go test ./... -v -count=1`

**Step 4: Commit**

```bash
git add config/planstate/ app/app_actions.go
git commit -m "refactor: remove SetStatus and planStageStatus — FSM is sole writer"
```

## Wave 4

### Task 9: Update agent instructions for sentinel files

**Files:**
- Modify: `.opencode/agents/planner.md`
- Modify: `.opencode/agents/coder.md`
- Modify: `.opencode/agents/reviewer.md`

**Step 1: Update planner agent**

Replace the plan-state.json registration instructions with:
```markdown
## Plan Registration (CRITICAL)

When you finish writing a plan:
1. Write the plan to `docs/plans/<date>-<name>.md`
2. Write a signal file: create `docs/plans/.signals/planner-finished-<date>-<name>.md`
   (empty file — just touch it). klique will detect this and register the plan.

**Never modify plan-state.json directly.** klique owns that file.
```

**Step 2: Update coder agent**

Replace the plan state instructions with:
```markdown
## Plan State

When you finish implementing a plan, write a signal file:
`docs/plans/.signals/implement-finished-<planfile>`
(empty file). klique will detect this and advance the plan to review.

**Never modify plan-state.json directly.** klique owns that file.
```

**Step 3: Update reviewer agent**

Add signal instructions:
```markdown
## Review Completion

When your review is complete:
- If approved: write `docs/plans/.signals/review-approved-<planfile>` (empty file)
- If changes needed: write `docs/plans/.signals/review-changes-<planfile>` with your feedback as the file body

**Never modify plan-state.json directly.** klique owns that file.
```

**Step 4: Commit**

```bash
git add .opencode/agents/
git commit -m "docs: update agent instructions for sentinel file protocol"
```

### Task 10: End-to-end verification

**Step 1: Run full test suite**

Run: `go test ./... -v -count=1`
Expected: all tests pass

**Step 2: Run full build**

Run: `go build ./...`
Expected: clean compile

**Step 3: Run linter**

Run: `go vet ./...`
Expected: no issues

**Step 4: Run typos check**

Run: `typos config/planfsm/ app/ .opencode/agents/`
Expected: no typos
