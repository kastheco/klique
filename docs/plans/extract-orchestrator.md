# Extract Orchestration Engine

**Goal:** Extract signal processing, agent spawning, and wave orchestration logic from `app/` into a standalone `orchestrator/` package so that both the TUI and a future headless CLI can drive the full task lifecycle without duplicating orchestration code.

**Architecture:** The `orchestrator` package exposes an `Engine` struct that owns the FSM, task store, task state, wave orchestrators, and agent spawn logic. The TUI's `home` struct delegates to `Engine` methods instead of inlining the logic. The Engine communicates results back via a channel of `Event` values (agent spawned, toast, FSM transition, etc.) that the TUI maps to `tea.Cmd`s and the CLI can handle synchronously. The Engine does NOT depend on bubbletea — it is a pure Go library with no TUI imports.

**Tech Stack:** Go 1.24, `config/taskfsm`, `config/taskstate`, `config/taskstore`, `config/taskparser`, `session`, `session/git`

**Size:** Large (estimated ~6 hours, 6 tasks, 3 waves)

---

## Wave 1: Define Engine Interface and Core Types

### Task 1: Create orchestrator package with Engine struct and Event types

**Files:**
- Create: `orchestrator/engine.go`
- Create: `orchestrator/event.go`
- Test: `orchestrator/engine_test.go`

**Step 1: write the failing test**

```go
// orchestrator/engine_test.go
package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine(t *testing.T) {
	eng, err := NewEngine(EngineConfig{
		RepoPath: "/tmp/test-repo",
		Project:  "test-project",
	})
	require.NoError(t, err)
	assert.NotNil(t, eng)
	assert.Equal(t, "/tmp/test-repo", eng.RepoPath())
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestrator/... -run TestNewEngine -v
```

expected: FAIL — `NewEngine undefined`

**Step 3: write minimal implementation**

Create `orchestrator/event.go` with the Event sum type:

```go
package orchestrator

// EventKind identifies the type of orchestration event.
type EventKind int

const (
	EventAgentSpawn      EventKind = iota // request to spawn an agent instance
	EventAgentKill                        // request to kill an agent instance
	EventToastInfo                        // informational toast message
	EventToastSuccess                     // success toast message
	EventToastError                       // error toast message
	EventFSMTransition                    // FSM state changed
	EventRefreshState                     // task state should be reloaded
	EventConfirmDialog                    // show a confirmation dialog to the user
	EventWaveAdvance                      // wave orchestrator advanced to next wave
)

// Event is emitted by the Engine to communicate side effects to the caller.
// The TUI maps these to tea.Cmd/tea.Msg; the CLI handles them synchronously.
type Event struct {
	Kind    EventKind
	Message string // human-readable description (toast text, dialog prompt, etc.)

	// Fields populated for specific event kinds:
	SpawnRequest *SpawnRequest // EventAgentSpawn
	KillRequest  *KillRequest  // EventAgentKill
	TaskFile     string        // EventFSMTransition, EventRefreshState
	ConfirmFunc  func() error  // EventConfirmDialog — called if user confirms
}

// SpawnRequest describes an agent that should be created.
type SpawnRequest struct {
	TaskFile    string
	AgentType   string // "planner", "coder", "reviewer", "fixer"
	Title       string
	Prompt      string
	Branch      string
	TaskNumber  int
	WaveNumber  int
	PeerCount   int
	ReviewCycle int
	UseSharedWorktree bool
}

// KillRequest describes an agent that should be terminated.
type KillRequest struct {
	TaskFile  string
	AgentType string
}
```

Create `orchestrator/engine.go` with the Engine struct:

```go
package orchestrator

import (
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
)

// EngineConfig holds the configuration needed to create an Engine.
type EngineConfig struct {
	RepoPath string
	Project  string
	Store    taskstore.Store
	StateDir string // path to plan-state directory (for legacy compat)
}

// Engine is the headless orchestration core. It processes signals, manages
// FSM transitions, and emits Events that the caller (TUI or CLI) acts on.
type Engine struct {
	repoPath string
	project  string
	store    taskstore.Store
	stateDir string
	fsm      *taskfsm.TaskStateMachine
	state    *taskstate.TaskState

	waveOrchestrators    map[string]*WaveOrchestrator
	pendingReviewFeedback map[string]string
	events               []Event // buffered events from the current processing cycle
}

// NewEngine creates a new orchestration engine.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	e := &Engine{
		repoPath:              cfg.RepoPath,
		project:               cfg.Project,
		store:                 cfg.Store,
		stateDir:              cfg.StateDir,
		waveOrchestrators:     make(map[string]*WaveOrchestrator),
		pendingReviewFeedback: make(map[string]string),
	}
	if cfg.Store != nil {
		e.fsm = taskfsm.New(cfg.Store, cfg.Project, cfg.StateDir)
	}
	return e, nil
}

// RepoPath returns the repository path this engine operates on.
func (e *Engine) RepoPath() string {
	return e.repoPath
}

// DrainEvents returns all buffered events and clears the buffer.
func (e *Engine) DrainEvents() []Event {
	events := e.events
	e.events = nil
	return events
}

func (e *Engine) emit(ev Event) {
	e.events = append(e.events, ev)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./orchestrator/... -run TestNewEngine -v
```

expected: PASS

**Step 5: commit**

```bash
git add orchestrator/
git commit -m "feat: create orchestrator package with Engine struct and Event types"
```

### Task 2: Move WaveOrchestrator into orchestrator package

**Files:**
- Create: `orchestrator/wave.go` (moved from `app/wave_orchestrator.go`)
- Create: `orchestrator/wave_test.go` (moved from `app/wave_orchestrator_test.go`)
- Modify: `app/wave_orchestrator.go` — delete (replaced by orchestrator/wave.go)
- Modify: `app/wave_orchestrator_test.go` — delete (replaced by orchestrator/wave_test.go)
- Modify: `app/app.go` — import `orchestrator` and use `orchestrator.WaveOrchestrator`
- Modify: `app/app_state.go` — update references
- Modify: `app/app_actions.go` — update references

**Step 1: write the failing test**

No new tests — the existing `wave_orchestrator_test.go` tests move to the new package. Verify they pass in the new location.

**Step 2: run test to verify baseline passes**

```bash
go test ./app/... -run TestWaveOrchestrator -v -count=1
```

expected: PASS (baseline)

**Step 3: move WaveOrchestrator to orchestrator package**

```bash
# 1. Copy the files
cp app/wave_orchestrator.go orchestrator/wave.go
cp app/wave_orchestrator_test.go orchestrator/wave_test.go

# 2. Update package declaration
sd 'package app' 'package orchestrator' orchestrator/wave.go orchestrator/wave_test.go

# 3. Export the previously-unexported types (taskStatus, WaveState are already exported)
sd 'type taskStatus int' 'type TaskStatus int' orchestrator/wave.go
sd 'taskPending taskStatus' 'TaskPending TaskStatus' orchestrator/wave.go
sd 'taskRunning' 'TaskRunning' orchestrator/wave.go
sd 'taskComplete' 'TaskComplete' orchestrator/wave.go
sd 'taskFailed' 'TaskFailed' orchestrator/wave.go
sd 'taskStatus' 'TaskStatus' orchestrator/wave.go orchestrator/wave_test.go

# 4. Update import path from planparser to taskparser (post-rename)
sd 'config/planparser' 'config/taskparser' orchestrator/wave.go orchestrator/wave_test.go

# 5. Update app/ to import from orchestrator and use qualified names
# Add import to app files that reference WaveOrchestrator
# Then update all references: WaveOrchestrator → orchestrator.WaveOrchestrator, etc.
sd 'NewWaveOrchestrator' 'orchestrator.NewWaveOrchestrator' $(fd -e go -p 'app/')
sd 'WaveOrchestrator' 'orchestrator.WaveOrchestrator' $(fd -e go -p 'app/')
sd 'WaveStateIdle' 'orchestrator.WaveStateIdle' $(fd -e go -p 'app/')
sd 'WaveStateRunning' 'orchestrator.WaveStateRunning' $(fd -e go -p 'app/')
sd 'WaveStateWaveComplete' 'orchestrator.WaveStateWaveComplete' $(fd -e go -p 'app/')
sd 'WaveStateAllComplete' 'orchestrator.WaveStateAllComplete' $(fd -e go -p 'app/')
sd 'taskComplete' 'orchestrator.TaskComplete' $(fd -e go -p 'app/')
sd 'taskFailed' 'orchestrator.TaskFailed' $(fd -e go -p 'app/')
sd 'taskRunning' 'orchestrator.TaskRunning' $(fd -e go -p 'app/')
sd 'taskPending' 'orchestrator.TaskPending' $(fd -e go -p 'app/')

# 6. Add orchestrator import to app files (manual — add to import blocks)
# 7. Remove old files
rm app/wave_orchestrator.go app/wave_orchestrator_test.go
```

Note: The import additions to `app/app.go`, `app/app_state.go`, `app/app_actions.go` must be done manually since `sd` cannot reliably insert into import blocks. Use `comby` or manual Edit:

```bash
comby 'import (:[imports])' 'import (:[imports]
	"github.com/kastheco/kasmos/orchestrator"
)' app/app.go -in-place
```

Repeat for `app/app_state.go` and `app/app_actions.go`.

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./orchestrator/... -v -count=1
go test ./app/... -count=1 2>&1 | tail -20
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: move WaveOrchestrator from app/ to orchestrator/ package"
```

## Wave 2: Extract Signal Processing and Agent Spawn Logic

> **depends on wave 1:** Engine struct and WaveOrchestrator must exist in orchestrator/ before signal processing can be moved there.

### Task 3: Extract signal and wave signal processing into Engine

**Files:**
- Modify: `orchestrator/engine.go` — add `ProcessSignals` and `ProcessWaveSignals` methods
- Create: `orchestrator/signals.go` — signal processing logic extracted from `app/app.go` lines 835-982
- Create: `orchestrator/wave_signals.go` — wave signal processing extracted from `app/app.go` lines 1012-1061
- Test: `orchestrator/signals_test.go`
- Modify: `app/app.go` — replace inline signal and wave signal processing with Engine calls

**Step 1: write the failing test**

```go
// orchestrator/signals_test.go
package orchestrator

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessSignals_ImplementFinished(t *testing.T) {
	store := newTestStore(t) // helper that creates in-memory SQLite store
	eng, err := NewEngine(EngineConfig{
		RepoPath: "/tmp/test",
		Project:  "test",
		Store:    store,
	})
	require.NoError(t, err)

	// Register a task and set it to implementing
	store.Create("test", taskstore.TaskEntry{Filename: "test.md", Status: "implementing"})

	signals := []taskfsm.Signal{
		{TaskFile: "test.md", Event: taskfsm.ImplementFinished},
	}
	eng.ProcessSignals(signals)

	events := eng.DrainEvents()
	// Should emit: agent spawn (reviewer), FSM transition
	var hasSpawn bool
	for _, ev := range events {
		if ev.Kind == EventAgentSpawn && ev.SpawnRequest.AgentType == "reviewer" {
			hasSpawn = true
		}
	}
	assert.True(t, hasSpawn, "expected reviewer spawn event after implement-finished")
}

func TestProcessWaveSignals_StartsWave(t *testing.T) {
	store := newTestStore(t)
	eng, err := NewEngine(EngineConfig{
		RepoPath: "/tmp/test",
		Project:  "test",
		Store:    store,
	})
	require.NoError(t, err)

	// Register a task with wave content
	store.Create("test", taskstore.TaskEntry{Filename: "test.md", Status: "implementing"})
	store.SetContent("test", "test.md", "## Wave 1\n### Task 1: Do thing\nContent")

	waveSignals := []taskfsm.WaveSignal{
		{TaskFile: "test.md", WaveNumber: 1},
	}
	eng.ProcessWaveSignals(waveSignals)

	events := eng.DrainEvents()
	var hasSpawn bool
	for _, ev := range events {
		if ev.Kind == EventAgentSpawn {
			hasSpawn = true
		}
	}
	assert.True(t, hasSpawn, "expected agent spawn for wave 1 task")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestrator/... -run "TestProcessSignals|TestProcessWaveSignals" -v
```

expected: FAIL — `ProcessSignals undefined`

**Step 3: write minimal implementation**

Extract the signal processing loop from `app/app.go` (the `case metadataResultMsg:` handler, lines 835-982) into `orchestrator/signals.go`:

```go
// ProcessSignals feeds FSM signals to the engine and emits Events for
// side effects (agent spawns, kills, toasts, dialogs).
// The caller is responsible for consuming sentinel files before calling this.
func (e *Engine) ProcessSignals(signals []taskfsm.Signal) {
    // ... extracted logic, emitting Events instead of tea.Cmds
}
```

Extract the wave signal processing from `app/app.go` lines 1012-1061 into `orchestrator/wave_signals.go`:

```go
// ProcessWaveSignals handles wave implementation signals — creates
// WaveOrchestrators and emits SpawnRequest events for each task in the wave.
func (e *Engine) ProcessWaveSignals(waveSignals []taskfsm.WaveSignal) {
    // ... extracted logic
}
```

Key differences from the TUI version:
- Instead of `m.spawnReviewer(planFile)` → emit `Event{Kind: EventAgentSpawn, SpawnRequest: &SpawnRequest{...}}`
- Instead of `m.toastManager.Success(...)` → emit `Event{Kind: EventToastSuccess, Message: ...}`
- Instead of `m.confirmAction(...)` → emit `Event{Kind: EventConfirmDialog, ...}`
- Instead of `m.loadPlanState()` → emit `Event{Kind: EventRefreshState}`

The TUI's `Update()` handler becomes a thin adapter: call `Engine.ProcessSignals` and `Engine.ProcessWaveSignals`, drain events, map each Event to the corresponding `tea.Cmd`.

**Step 4: run test to verify it passes**

```bash
go test ./orchestrator/... -run "TestProcessSignals|TestProcessWaveSignals" -v
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "feat: extract signal and wave signal processing into orchestrator.Engine"
```

### Task 4: Extract agent spawn helpers (programForAgent, buildPrompts)

**Files:**
- Create: `orchestrator/spawn.go` — `ProgramResolver`, prompt builders
- Test: `orchestrator/spawn_test.go`
- Modify: `app/app_state.go` — delegate to `orchestrator.ProgramResolver`

**Step 1: write the failing test**

```go
// orchestrator/spawn_test.go
package orchestrator

import (
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/stretchr/testify/assert"
)

func TestProgramResolver_DefaultProgram(t *testing.T) {
	r := NewProgramResolver("opencode", nil)
	assert.Equal(t, "opencode", r.ForAgent("coder"))
}

func TestProgramResolver_WithProfile(t *testing.T) {
	profiles := map[string]config.AgentProfile{
		"implementing": {Program: "claude", Enabled: true},
	}
	cfg := &config.Config{Profiles: profiles}
	r := NewProgramResolver("opencode", cfg)
	assert.Equal(t, "claude", r.ForAgent("coder"))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestrator/... -run TestProgramResolver -v
```

expected: FAIL — `NewProgramResolver undefined`

**Step 3: write minimal implementation**

Extract `programForAgent`, `withOpenCodeModelFlag`, `normalizeOpenCodeModelID` from `app/app_state.go` into `orchestrator/spawn.go`. Create a `ProgramResolver` struct that holds the default program and config, with a `ForAgent(agentType string) string` method.

Also extract the prompt builders (`buildImplementPrompt`, `buildPlanPrompt`, `buildModifyPlanPrompt`, `buildChatAboutPlanPrompt`) from `app/app_state.go` into `orchestrator/prompts.go`.

**Step 4: run test to verify it passes**

```bash
go test ./orchestrator/... -run TestProgramResolver -v
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "feat: extract ProgramResolver and prompt builders into orchestrator/"
```

## Wave 3: Wire TUI to Engine and Integration Tests

> **depends on wave 2:** Engine methods must exist before the TUI can delegate to them.

### Task 5: Wire TUI to Engine (signal processing + program resolution)

**Files:**
- Modify: `app/app.go` — add `engine *orchestrator.Engine` field, delegate `metadataResultMsg` handling, initialize Engine in `newHome()`
- Modify: `app/app_state.go` — replace `programForAgent` with `engine.Programs.ForAgent`, remove dead code
- Test: `app/app_planner_signal_test.go` — verify existing tests still pass with Engine delegation

**Step 1: write the failing test**

No new tests — existing signal processing tests in `app/` validate the behavior. The refactor must not change observable behavior.

**Step 2: run test to verify baseline passes**

```bash
go test ./app/... -count=1 2>&1 | tail -10
```

expected: PASS (baseline)

**Step 3: wire Engine into TUI**

Add `engine *orchestrator.Engine` to the `home` struct. In `newHome()`, create the Engine with the same store/project/stateDir.

Replace the inline signal processing in `case metadataResultMsg:` with:

```go
m.engine.ProcessSignals(msg.Signals)
m.engine.ProcessWaveSignals(msg.WaveSignals)
for _, ev := range m.engine.DrainEvents() {
    switch ev.Kind {
    case orchestrator.EventAgentSpawn:
        signalCmds = append(signalCmds, m.handleSpawnEvent(ev))
    case orchestrator.EventAgentKill:
        m.handleKillEvent(ev)
    case orchestrator.EventToastSuccess:
        m.toastManager.Success(ev.Message)
    // ... etc
    }
}
```

The `handleSpawnEvent` method creates the `session.Instance`, adds it to the nav panel, and returns the `tea.Cmd` that starts it. This keeps all TUI/bubbletea concerns in `app/`.

Replace `m.programForAgent(agentType)` calls in `app/app_state.go` with `m.engine.Programs.ForAgent(agentType)`. Remove the now-dead `programForAgent`, `withOpenCodeModelFlag`, and `normalizeOpenCodeModelID` functions from `app/app_state.go`.

**Step 4: run test to verify it passes**

```bash
go build ./...
go test ./app/... -count=1 2>&1 | tail -20
go test ./orchestrator/... -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "refactor: wire TUI signal processing and program resolution through orchestrator.Engine"
```

### Task 6: Integration test — headless Engine drives full lifecycle

**Files:**
- Create: `orchestrator/integration_test.go`

**Step 1: write the failing test**

```go
// orchestrator/integration_test.go
package orchestrator

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_FullLifecycle(t *testing.T) {
	store := newTestStore(t)
	eng, err := NewEngine(EngineConfig{
		RepoPath: "/tmp/test",
		Project:  "test",
		Store:    store,
	})
	require.NoError(t, err)

	// 1. Register a task
	store.Create("test", taskstore.TaskEntry{Filename: "feature.md", Status: "ready"})

	// 2. Transition to planning
	err = eng.fsm.Transition("feature.md", taskfsm.PlanStart)
	require.NoError(t, err)

	// 3. Planner finishes
	eng.ProcessSignals([]taskfsm.Signal{
		{TaskFile: "feature.md", Event: taskfsm.PlannerFinished},
	})
	events := eng.DrainEvents()
	assert.NotEmpty(t, events, "planner-finished should emit events")

	// 4. Start implementation
	err = eng.fsm.Transition("feature.md", taskfsm.ImplementStart)
	require.NoError(t, err)

	// 5. Implementation finishes → reviewer spawn
	eng.ProcessSignals([]taskfsm.Signal{
		{TaskFile: "feature.md", Event: taskfsm.ImplementFinished},
	})
	events = eng.DrainEvents()
	var reviewerSpawned bool
	for _, ev := range events {
		if ev.Kind == EventAgentSpawn && ev.SpawnRequest.AgentType == "reviewer" {
			reviewerSpawned = true
		}
	}
	assert.True(t, reviewerSpawned, "implement-finished should spawn reviewer")

	// 6. Review approved → done
	eng.ProcessSignals([]taskfsm.Signal{
		{TaskFile: "feature.md", Event: taskfsm.ReviewApproved},
	})
	events = eng.DrainEvents()
	var hasDoneTransition bool
	for _, ev := range events {
		if ev.Kind == EventFSMTransition {
			hasDoneTransition = true
		}
	}
	assert.True(t, hasDoneTransition, "review-approved should emit FSM transition to done")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestrator/... -run TestEngine_FullLifecycle -v
```

expected: FAIL (until ProcessSignals handles all events correctly)

**Step 3: write minimal implementation**

Fix any gaps in ProcessSignals to handle the full lifecycle. Add test helper `newTestStore` that creates an in-memory SQLite store.

**Step 4: run test to verify it passes**

```bash
go test ./orchestrator/... -v -count=1
go test ./... -count=1 2>&1 | tail -30
```

expected: PASS

**Step 5: commit**

```bash
git add -A
git commit -m "test: add integration test for headless Engine lifecycle"
```
