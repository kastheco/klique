# Pre-Implementation Plan Elaboration

**Goal:** Add an automatic elaboration phase between plan-ready and coder-start that expands terse task descriptions into detailed implementation instructions, reducing coder decision-making and improving output quality.

**Architecture:** When the user triggers "implement", kasmos spawns an elaborator agent before starting wave 1. The elaborator reads the plan from the task store, deeply reads the codebase files referenced by each task, and rewrites each task body with detailed implementation guidance (exact signatures, patterns from existing code, edge cases, error handling). It writes the enriched plan back to the store and signals completion via `elaborator-finished-<planfile>`. kasmos detects this signal, re-parses the updated plan into the wave orchestrator, and starts wave 1 normally. A context menu bypass ("implement directly") skips elaboration. The elaborator uses a configurable agent profile (`"elaborating"` phase) so a strong model can be assigned.

**Tech Stack:** Go 1.24+, bubbletea v1.3.x, existing kasmos packages (config/taskfsm, config/taskparser, orchestration, session, app)

**Size:** Medium (estimated ~4 hours, 6 tasks, 2 waves)

---

## Wave 1: Foundation Components

All tasks in this wave are independent — they create new files or add isolated code to existing files without cross-dependencies.

### Task 1: Elaboration Signal Type

**Files:**
- Create: `config/taskfsm/elaboration_signal.go`
- Test: `config/taskfsm/elaboration_signal_test.go`

**Step 1: write the failing test**

```go
// elaboration_signal_test.go
package taskfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseElaborationSignal(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOk   bool
		wantFile string
	}{
		{
			name:     "valid elaboration signal",
			filename: "elaborator-finished-my-feature.md",
			wantOk:   true,
			wantFile: "my-feature.md",
		},
		{
			name:     "not an elaboration signal",
			filename: "planner-finished-test.md",
			wantOk:   false,
		},
		{
			name:     "empty plan file",
			filename: "elaborator-finished-",
			wantOk:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, ok := ParseElaborationSignal(tt.filename)
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantFile, sig.TaskFile)
			}
		})
	}
}

func TestScanElaborationSignals(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write an elaboration signal and a non-matching file
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "elaborator-finished-test.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-test.md"), nil, 0o644))

	signals := ScanElaborationSignals(signalsDir)
	require.Len(t, signals, 1)
	assert.Equal(t, "test.md", signals[0].TaskFile)
}

func TestConsumeElaborationSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "elaborator-finished-test.md")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	sig := ElaborationSignal{TaskFile: "test.md", filePath: path}
	ConsumeElaborationSignal(sig)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}
```

**Step 2: run test to verify it fails**

```bash
go test ./config/taskfsm/... -run TestParseElaborationSignal -v
```

expected: FAIL — `ParseElaborationSignal` undefined

**Step 3: write minimal implementation**

Create `config/taskfsm/elaboration_signal.go` following the exact pattern from `wave_signal.go`:

```go
package taskfsm

import (
	"os"
	"path/filepath"
	"strings"
)

// ElaborationSignal represents a parsed elaborator-finished signal file.
type ElaborationSignal struct {
	TaskFile string
	filePath string // full path for deletion
}

const elaborationPrefix = "elaborator-finished-"

// ParseElaborationSignal attempts to parse a filename as an elaboration signal.
func ParseElaborationSignal(filename string) (ElaborationSignal, bool) {
	if !strings.HasPrefix(filename, elaborationPrefix) {
		return ElaborationSignal{}, false
	}
	planFile := strings.TrimPrefix(filename, elaborationPrefix)
	if planFile == "" {
		return ElaborationSignal{}, false
	}
	planFile = filepath.Base(planFile)
	return ElaborationSignal{TaskFile: planFile}, true
}

// ScanElaborationSignals reads the given signals directory and returns parsed
// elaboration signals. Like wave signals, these are handled separately from FSM
// signals — they don't map to state transitions but trigger orchestration actions.
func ScanElaborationSignals(signalsDir string) []ElaborationSignal {
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}
	var signals []ElaborationSignal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		es, ok := ParseElaborationSignal(entry.Name())
		if !ok {
			continue
		}
		es.filePath = filepath.Join(signalsDir, entry.Name())
		signals = append(signals, es)
	}
	return signals
}

// ConsumeElaborationSignal deletes the signal file after processing.
func ConsumeElaborationSignal(es ElaborationSignal) {
	_ = os.Remove(es.filePath)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./config/taskfsm/... -run "TestParseElaborationSignal|TestScanElaborationSignals|TestConsumeElaborationSignal" -v
```

expected: PASS

**Step 5: commit**

```bash
git add config/taskfsm/elaboration_signal.go config/taskfsm/elaboration_signal_test.go
git commit -m "feat(task-1): add elaboration signal type for pre-implementation plan enrichment"
```

### Task 2: WaveOrchestrator Elaboration State

**Files:**
- Modify: `orchestration/engine.go`
- Test: `orchestration/engine_test.go`

**Step 1: write the failing test**

Add to `engine_test.go`:

```go
func TestWaveOrchestrator_ElaboratingState(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	orch.SetElaborating()
	assert.Equal(t, WaveStateElaborating, orch.State())

	// StartNextWave should be blocked while elaborating
	tasks := orch.StartNextWave()
	assert.Nil(t, tasks, "must not start waves while elaborating")
	assert.Equal(t, WaveStateElaborating, orch.State())
}

func TestWaveOrchestrator_UpdatePlan(t *testing.T) {
	plan := &taskparser.Plan{
		Goal: "original",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "terse body"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan.md", plan)
	orch.SetElaborating()

	updated := &taskparser.Plan{
		Goal: "original",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "detailed body with signatures and patterns"},
			}},
		},
	}
	orch.UpdatePlan(updated)

	// Should transition back to Idle so StartNextWave works
	assert.Equal(t, WaveStateIdle, orch.State())

	// Verify the plan was replaced
	tasks := orch.StartNextWave()
	require.Len(t, tasks, 1)
	assert.Contains(t, tasks[0].Body, "detailed body")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestration/... -run "TestWaveOrchestrator_ElaboratingState|TestWaveOrchestrator_UpdatePlan" -v
```

expected: FAIL — `WaveStateElaborating` undefined

**Step 3: write minimal implementation**

Add to `orchestration/engine.go`:

1. Add `WaveStateElaborating` to the `WaveState` const block (after `WaveStateIdle`):

```go
WaveStateElaborating              // Waiting for elaborator to enrich task descriptions
```

2. Add `SetElaborating()` method:

```go
// SetElaborating puts the orchestrator into the elaborating state.
// StartNextWave is blocked until UpdatePlan is called.
func (o *WaveOrchestrator) SetElaborating() {
	o.state = WaveStateElaborating
}
```

3. Add `UpdatePlan()` method:

```go
// UpdatePlan replaces the plan with an elaborated version and resets the
// orchestrator to Idle so waves can begin. Task states are cleared since
// no tasks have started yet.
func (o *WaveOrchestrator) UpdatePlan(plan *taskparser.Plan) {
	o.plan = plan
	o.state = WaveStateIdle
	o.currentWave = 0
	o.taskStates = make(map[int]taskStatus)
}
```

4. Guard `StartNextWave()` — add at the top of the method:

```go
if o.state == WaveStateElaborating {
	return nil
}
```

**Step 4: run test to verify it passes**

```bash
go test ./orchestration/... -run "TestWaveOrchestrator_ElaboratingState|TestWaveOrchestrator_UpdatePlan" -v
```

expected: PASS

Also verify no regressions:

```bash
go test ./orchestration/... -v
```

**Step 5: commit**

```bash
git add orchestration/engine.go orchestration/engine_test.go
git commit -m "feat(task-2): add WaveStateElaborating and UpdatePlan to orchestrator"
```

### Task 3: Elaboration Prompt Builder

**Files:**
- Modify: `orchestration/prompt.go`
- Test: `orchestration/prompt_test.go`

**Step 1: write the failing test**

Add to `prompt_test.go`:

```go
func TestBuildElaborationPrompt(t *testing.T) {
	prompt := BuildElaborationPrompt("my-feature.md")

	// Must reference the plan file for retrieval
	assert.Contains(t, prompt, "kas task show my-feature.md")
	// Must reference updating the plan
	assert.Contains(t, prompt, "kas task update-content my-feature.md")
	// Must reference the signal
	assert.Contains(t, prompt, "elaborator-finished-my-feature.md")
	// Must instruct to expand task bodies
	assert.Contains(t, prompt, "implementation detail")
	// Must instruct to preserve structure
	assert.Contains(t, prompt, "preserve")
	// Must reference reading the codebase
	assert.Contains(t, prompt, "codebase")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./orchestration/... -run TestBuildElaborationPrompt -v
```

expected: FAIL — `BuildElaborationPrompt` undefined

**Step 3: write minimal implementation**

Add to `orchestration/prompt.go`:

```go
// BuildElaborationPrompt returns the prompt for an elaborator agent session.
// The elaborator reads the plan, deeply reads the codebase for each task's files,
// and expands task bodies with detailed implementation instructions.
func BuildElaborationPrompt(planFile string) string {
	return fmt.Sprintf(
		"You are the elaborator agent. Your job: enrich a plan's task descriptions with "+
			"detailed implementation instructions so coder agents make fewer decisions.\n\n"+
			"Load the `kasmos-elaborator` skill before starting. Also load `cli-tools`.\n\n"+
			"## Instructions\n\n"+
			"1. Retrieve the plan: `kas task show %[1]s`\n"+
			"2. For each task, read the codebase files listed in its **Files:** section. "+
			"Study existing patterns, interfaces, function signatures, error handling, "+
			"and data flow in those files and their neighbors.\n"+
			"3. Expand each task body with concrete implementation detail:\n"+
			"   - Exact function signatures to create or modify\n"+
			"   - Existing codebase patterns to follow (with file references)\n"+
			"   - Edge cases and error handling requirements\n"+
			"   - Import paths and dependencies\n"+
			"   - Concrete code snippets where helpful\n"+
			"4. Preserve the plan structure — do not change wave organization, "+
			"task numbering, file lists, or the header fields. Only expand task bodies.\n"+
			"5. Write the updated plan: pipe content to `kas task update-content %[1]s`\n"+
			"6. Signal completion: `touch .kasmos/signals/elaborator-finished-%[1]s`\n",
		planFile,
	)
}
```

**Step 4: run test to verify it passes**

```bash
go test ./orchestration/... -run TestBuildElaborationPrompt -v
```

expected: PASS

**Step 5: commit**

```bash
git add orchestration/prompt.go orchestration/prompt_test.go
git commit -m "feat(task-3): add BuildElaborationPrompt for pre-implementation plan enrichment"
```

### Task 4: Elaborator Agent Type and Profile Resolution

**Files:**
- Modify: `session/instance.go`
- Modify: `app/app_state.go`
- Test: `session/instance_taskfile_test.go`

**Step 1: write the failing test**

Add to `session/instance_taskfile_test.go`:

```go
func TestAgentTypeElaborator_Constant(t *testing.T) {
	assert.Equal(t, "elaborator", AgentTypeElaborator)
}
```

Also verify profile resolution works by checking `programForAgent` handles the new type. This is tested indirectly — the constant existence and the switch case are the key assertions.

**Step 2: run test to verify it fails**

```bash
go test ./session/... -run TestAgentTypeElaborator_Constant -v
```

expected: FAIL — `AgentTypeElaborator` undefined

**Step 3: write minimal implementation**

1. In `session/instance.go`, add to the `AgentType` constants block (after `AgentTypeFixer`):

```go
AgentTypeElaborator = "elaborator"
```

2. In `app/app_state.go`, add a case to the `programForAgent` switch (after the `AgentTypeFixer` case):

```go
case session.AgentTypeElaborator:
	profile = m.appConfig.ResolveProfile("elaborating", m.program)
```

**Step 4: run test to verify it passes**

```bash
go test ./session/... -run TestAgentTypeElaborator_Constant -v
```

expected: PASS

Verify no regressions:

```bash
go build ./...
```

**Step 5: commit**

```bash
git add session/instance.go app/app_state.go
git commit -m "feat(task-4): add AgentTypeElaborator constant and elaborating profile resolution"
```

### Task 5: Elaborator Skill

**Files:**
- Create: `.opencode/skills/kasmos-elaborator/SKILL.md`

This task has no testable Go logic — it creates a markdown skill file. TDD steps 1-2 are omitted because there is no code to test; the skill is a prompt document.

**Step 3: write the skill**

Create `.opencode/skills/kasmos-elaborator/SKILL.md` with the elaborator agent's instructions. The skill must instruct the agent to:

- Read the plan from the task store via `kas task show`
- For each task, deeply read all files in the **Files:** section plus neighboring files to understand patterns
- Expand each task body with: exact function signatures, existing patterns to follow (with snippets), error handling, edge cases, imports, concrete test code where the plan has placeholder tests
- Preserve plan structure: wave headers, task numbers, file lists, header metadata — only expand the body text below each `### Task N:` heading
- Write the updated plan back via `kas task update-content`
- Signal completion via sentinel file
- Include the CLI tools hard gate (banned tools table)

The skill should also include the `KASMOS_MANAGED` env var check and appropriate signaling instructions.

**Step 5: commit**

```bash
git add .opencode/skills/kasmos-elaborator/SKILL.md
git commit -m "feat(task-5): add kasmos-elaborator skill for pre-implementation plan enrichment"
```

## Wave 2: App Integration

> **depends on wave 1:** uses `ElaborationSignal` (task 1), `WaveStateElaborating`/`UpdatePlan` (task 2), `BuildElaborationPrompt` (task 3), `AgentTypeElaborator` (task 4), and the elaborator skill (task 5) to wire elaboration into the TUI orchestration flow.

### Task 6: Wire Elaboration into App Orchestration

**Files:**
- Modify: `app/app.go` (metadataResultMsg struct + signal processing loop + metadata tick goroutine)
- Modify: `app/app_actions.go` (executeTaskStage + context menu)
- Modify: `app/app_state.go` (add spawnElaborator + buildElaborationPrompt wrapper)
- Test: `app/app_wave_orchestration_flow_test.go`

**Step 1: write the failing test**

Add to `app/app_wave_orchestration_flow_test.go`:

```go
func TestImplementTriggersElaborationBeforeWave1(t *testing.T) {
	h := newTestHarness(t)

	const planFile = "elab-test.md"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\n**Files:**\n- Create: `foo.go`\n\nImplement foo."
	h.registerPlan(planFile, planContent, "plan/elab-test")

	model, _ := h.executeTaskStage(planFile, "implement")
	m := model.(*home)

	// Orchestrator should exist and be in elaborating state
	orch, exists := m.waveOrchestrators[planFile]
	require.True(t, exists, "orchestrator must be created")
	assert.Equal(t, WaveStateElaborating, orch.State(),
		"orchestrator must be in elaborating state, not running")

	// An elaborator instance should have been spawned
	var foundElaborator bool
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == planFile && inst.AgentType == session.AgentTypeElaborator {
			foundElaborator = true
			assert.Contains(t, inst.QueuedPrompt, "elaborator",
				"elaborator prompt must reference the elaborator role")
			break
		}
	}
	assert.True(t, foundElaborator, "elaborator instance must be spawned")
}

func TestImplementDirectlySkipsElaboration(t *testing.T) {
	h := newTestHarness(t)

	const planFile = "direct-test.md"
	planContent := "**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\nDo it."
	h.registerPlan(planFile, planContent, "plan/direct-test")

	model, _ := h.executeTaskStage(planFile, "implement_direct")
	m := model.(*home)

	// Orchestrator should exist and be running (not elaborating)
	orch, exists := m.waveOrchestrators[planFile]
	require.True(t, exists, "orchestrator must be created")
	assert.NotEqual(t, WaveStateElaborating, orch.State(),
		"direct implement must skip elaboration")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./app/... -run "TestImplementTriggersElaborationBeforeWave1|TestImplementDirectlySkipsElaboration" -v
```

expected: FAIL — `"implement_direct"` not handled, elaboration not wired

**Step 3: write minimal implementation**

This is the integration task. The changes span three files:

**A. `app/app.go` — Add elaboration signals to metadata tick and processing:**

1. Add `ElaborationSignals` field to `metadataResultMsg`:
```go
ElaborationSignals []taskfsm.ElaborationSignal // elaborator-finished signal files
```

2. In the metadata tick goroutine (the `tickUpdateMetadataMessage` handler around line 810), add scanning:
```go
elaborationSignals := taskfsm.ScanElaborationSignals(signalsDir)
```
Pass it through to `metadataResultMsg`.

3. In the `metadataResultMsg` handler (after wave signal processing around line 1060), add elaboration signal processing:
```go
for _, es := range msg.ElaborationSignals {
	taskfsm.ConsumeElaborationSignal(es)

	orch, exists := m.waveOrchestrators[es.TaskFile]
	if !exists || orch.State() != WaveStateElaborating {
		log.WarningLog.Printf("ignoring elaborator-finished signal for %q — no active elaboration", es.TaskFile)
		continue
	}

	// Re-read the enriched plan from the store
	content, err := m.taskStore.GetContent(m.taskStoreProject, es.TaskFile)
	if err != nil {
		log.WarningLog.Printf("elaboration signal: could not read plan %s: %v", es.TaskFile, err)
		continue
	}
	plan, err := taskparser.Parse(content)
	if err != nil {
		log.WarningLog.Printf("elaboration signal: could not parse enriched plan %s: %v", es.TaskFile, err)
		continue
	}

	// Replace the plan in the orchestrator with the enriched version
	orch.UpdatePlan(plan)

	// Kill the elaborator instance
	for _, inst := range m.nav.GetInstances() {
		if inst.TaskFile == es.TaskFile && inst.AgentType == session.AgentTypeElaborator {
			_ = inst.Kill()
			break
		}
	}

	entry, ok := m.taskState.Entry(es.TaskFile)
	if !ok {
		continue
	}

	m.toastManager.Info(fmt.Sprintf("plan elaborated — starting wave 1 for '%s'", taskstate.DisplayName(es.TaskFile)))
	mdl, cmd := m.startNextWave(orch, entry)
	m = mdl.(*home)
	if cmd != nil {
		signalCmds = append(signalCmds, cmd)
	}
}
```

**B. `app/app_actions.go` — Add implement_direct stage and elaboration to implement:**

1. In `executeTaskStage`, modify the `"implement"` case: after creating the orchestrator and calling `fsmSetImplementing`, instead of directly calling `startNextWave`, set elaborating state and spawn the elaborator:
```go
orch.SetElaborating()
return m.spawnElaborator(planFile, orch, entry)
```

2. Add a new `"implement_direct"` case that preserves the current behavior (no elaboration):
```go
case "implement_direct":
	// Same as implement but skips elaboration — goes straight to wave 1.
	// [same plan parsing + orchestrator creation as "implement"]
	return m.startNextWave(orch, entry)
```

3. In `openTaskContextMenu`, for `StatusReady` and `StatusPlanning`, add the bypass option:
```go
overlay.ContextMenuItem{Label: "implement directly", Action: "start_implement_direct"},
```

4. In the action dispatch (the switch on action string), add:
```go
case "start_implement_direct":
	return m.triggerTaskStage(planFile, "implement_direct")
```

**C. `app/app_state.go` — Add spawnElaborator helper:**

Add a `spawnElaborator` method that creates an elaborator instance on the main branch (not in a worktree — the elaborator only reads code and updates the store, it doesn't modify files):

```go
func (m *home) spawnElaborator(planFile string, orch *WaveOrchestrator, entry taskstate.TaskEntry) (tea.Model, tea.Cmd) {
	planName := taskstate.DisplayName(planFile)
	prompt := orchestration.BuildElaborationPrompt(planFile)

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:    fmt.Sprintf("%s-elaborator", planName),
		Path:     m.activeRepoPath,
		Program:  m.programForAgent(session.AgentTypeElaborator),
		TaskFile: planFile,
	})
	if err != nil {
		return m, m.handleError(err)
	}
	inst.AgentType = session.AgentTypeElaborator
	inst.QueuedPrompt = prompt
	inst.SetStatus(session.Loading)
	inst.LoadingTotal = 6
	inst.LoadingMessage = "elaborating plan..."

	m.addInstanceFinalizer(inst, m.nav.AddInstance(inst))

	startCmd := func() tea.Msg {
		return instanceStartedMsg{instance: inst, err: inst.StartOnMainBranch()}
	}

	m.audit(auditlog.EventAgentSpawned, fmt.Sprintf("spawned elaborator for %s", planName),
		auditlog.WithPlan(planFile),
		auditlog.WithAgent(session.AgentTypeElaborator))

	m.toastManager.Info(fmt.Sprintf("elaborating plan '%s' before implementation", planName))
	return m, tea.Batch(tea.RequestWindowSize, startCmd, m.toastTickCmd())
}
```

**Step 4: run test to verify it passes**

```bash
go test ./app/... -run "TestImplementTriggersElaborationBeforeWave1|TestImplementDirectlySkipsElaboration" -v
```

expected: PASS

Verify no regressions:

```bash
go build ./...
go test ./app/... -v -count=1
```

**Step 5: commit**

```bash
git add app/app.go app/app_actions.go app/app_state.go app/app_wave_orchestration_flow_test.go
git commit -m "feat(task-6): wire elaboration phase into implement flow with direct-implement bypass"
```
