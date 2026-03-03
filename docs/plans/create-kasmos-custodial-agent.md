# Custodial Agent Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a hybrid custodial agent system with CLI subcommands (`kq plan`), a scaffolded agent persona, and slash commands for operational/janitorial tasks in the kasmos workflow.

**Architecture:** New `kq plan` cobra command group in `main.go` wrapping `config/planfsm` and `config/planstate` for safe state mutations. New `implement-wave-N` signal type in `config/planfsm/signals.go` consumed by the TUI's metadata tick. Custodial agent template added to the scaffold system (`internal/initcmd/scaffold/templates/`) with a static block in `opencode.jsonc`. Slash commands in `.opencode/commands/` use `kq plan` for state ops and raw git/gh for everything else.

**Tech Stack:** Go 1.24+, cobra, planfsm/planstate packages, opencode agent/command markdown format

**Size:** Medium (estimated ~4 hours, 6 tasks, 2 waves)

---

## Wave 1: CLI Infrastructure + Signal System

> **Justification:** The CLI subcommands and signal scanner are prerequisites for the slash commands and agent in Wave 2 — those commands call `kq plan` and rely on the TUI consuming wave signals.

### Task 1: `kq plan` CLI Command Group

**Files:**
- Create: `cmd/plan.go`
- Modify: `main.go:168-208` (init function — register `planCmd`)
- Modify: `config/planstate/planstate.go` (export `SetStatus` for force-override)
- Test: `cmd/plan_test.go`

**Step 1: Write the failing tests**

Create `cmd/plan_test.go` with table-driven tests covering:
- `kq plan list` — outputs all plans with status, filters by `--status`
- `kq plan set-status` — requires `--force`, validates status values, writes through planstate
- `kq plan transition` — applies valid FSM events, rejects invalid ones with helpful error
- `kq plan implement` — transitions to implementing, writes signal file

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestPlanState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ps := &planstate.PlanState{
		Dir:          dir,
		Plans:        make(map[string]planstate.PlanEntry),
		TopicEntries: make(map[string]planstate.TopicEntry),
	}
	ps.Plans["2026-02-20-test-plan.md"] = planstate.PlanEntry{
		Status:      "ready",
		Description: "test plan",
		Branch:      "plan/test-plan",
	}
	ps.Plans["2026-02-20-implementing-plan.md"] = planstate.PlanEntry{
		Status:      "implementing",
		Description: "implementing plan",
		Branch:      "plan/implementing-plan",
	}
	require.NoError(t, ps.Save())
	return dir
}

func TestPlanList(t *testing.T) {
	dir := setupTestPlanState(t)

	tests := []struct {
		name           string
		statusFilter   string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "all plans",
			wantContains: []string{"2026-02-20-test-plan.md", "2026-02-20-implementing-plan.md"},
		},
		{
			name:           "filter by ready",
			statusFilter:   "ready",
			wantContains:   []string{"2026-02-20-test-plan.md"},
			wantNotContain: []string{"2026-02-20-implementing-plan.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := executePlanList(dir, tt.statusFilter)
			for _, want := range tt.wantContains {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}

func TestPlanSetStatus(t *testing.T) {
	dir := setupTestPlanState(t)

	// Requires --force
	err := executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", false)
	assert.Error(t, err, "should require --force flag")

	// Valid override
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "done", true)
	require.NoError(t, err)

	ps, err := planstate.Load(dir)
	require.NoError(t, err)
	entry, ok := ps.Entry("2026-02-20-test-plan.md")
	require.True(t, ok)
	assert.Equal(t, planstate.Status("done"), entry.Status)

	// Invalid status
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "bogus", true)
	assert.Error(t, err, "should reject invalid status")
}

func TestPlanTransition(t *testing.T) {
	dir := setupTestPlanState(t)

	// Valid transition: ready → planning via plan_start
	newStatus, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start")
	require.NoError(t, err)
	assert.Equal(t, "planning", newStatus)

	// Invalid transition
	_, err = executePlanTransition(dir, "2026-02-20-test-plan.md", "review_approved")
	assert.Error(t, err)
}

func TestPlanImplement(t *testing.T) {
	dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	err := executePlanImplement(dir, "2026-02-20-test-plan.md", 1)
	require.NoError(t, err)

	// Verify plan transitioned to implementing
	ps, err := planstate.Load(dir)
	require.NoError(t, err)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.Status("implementing"), entry.Status)

	// Verify signal file created
	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	var found bool
	for _, e := range entries {
		if e.Name() == "implement-wave-1-2026-02-20-test-plan.md" {
			found = true
		}
	}
	assert.True(t, found, "signal file should exist")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestPlan -v`
Expected: compilation errors (functions don't exist yet)

**Step 3: Export `SetStatus` in planstate and implement `cmd/plan.go`**

First, export the force-set capability in `config/planstate/planstate.go`. The existing `setStatus` is unexported and meant for tests. Add a new exported `ForceSetStatus` that takes a flock, validates the status string, and writes:

```go
// ForceSetStatus overrides a plan's status regardless of FSM rules.
// Caller is responsible for flock. Validates the status is a known value.
func (ps *PlanState) ForceSetStatus(filename string, status Status) error {
	if !isValidStatus(status) {
		return fmt.Errorf("invalid status %q: must be one of ready, planning, implementing, reviewing, done, cancelled", status)
	}
	if _, ok := ps.Plans[filename]; !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry := ps.Plans[filename]
	entry.Status = status
	ps.Plans[filename] = entry
	return ps.Save()
}

func isValidStatus(s Status) bool {
	switch s {
	case StatusReady, StatusPlanning, StatusImplementing, StatusReviewing, StatusDone, StatusCancelled:
		return true
	}
	return false
}
```

Then create `cmd/plan.go` with four subcommands. The core functions (`executePlanList`, `executePlanSetStatus`, `executePlanTransition`, `executePlanImplement`) are exported for testing:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/spf13/cobra"
)

var validStatuses = []string{"ready", "planning", "implementing", "reviewing", "done", "cancelled"}

func executePlanList(plansDir, statusFilter string) string {
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for _, info := range ps.List() {
		if statusFilter != "" && string(info.Status) != statusFilter {
			continue
		}
		line := fmt.Sprintf("%-14s %-50s %s", info.Status, info.Filename, info.Branch)
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return sb.String()
}

func executePlanSetStatus(plansDir, planFile, status string, force bool) error {
	if !force {
		return fmt.Errorf("--force required to override plan status (this bypasses the FSM)")
	}
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return err
	}
	return ps.ForceSetStatus(planFile, planstate.Status(status))
}

func executePlanTransition(plansDir, planFile, event string) (string, error) {
	eventMap := map[string]planfsm.Event{
		"plan_start":         planfsm.PlanStart,
		"planner_finished":   planfsm.PlannerFinished,
		"implement_start":    planfsm.ImplementStart,
		"implement_finished": planfsm.ImplementFinished,
		"review_approved":    planfsm.ReviewApproved,
		"review_changes":     planfsm.ReviewChangesRequested,
		"request_review":     planfsm.RequestReview,
		"start_over":         planfsm.StartOver,
		"reimplement":        planfsm.Reimplement,
		"cancel":             planfsm.Cancel,
		"reopen":             planfsm.Reopen,
	}
	fsmEvent, ok := eventMap[event]
	if !ok {
		names := make([]string, 0, len(eventMap))
		for k := range eventMap {
			names = append(names, k)
		}
		return "", fmt.Errorf("unknown event %q; valid events: %s", event, strings.Join(names, ", "))
	}
	fsm := planfsm.New(plansDir)
	if err := fsm.Transition(planFile, fsmEvent); err != nil {
		return "", err
	}
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return "", err
	}
	entry, _ := ps.Entry(planFile)
	return string(entry.Status), nil
}

func executePlanImplement(plansDir, planFile string, wave int) error {
	// Transition to implementing via FSM (handles planning→ready→implementing).
	fsm := planfsm.New(plansDir)
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return err
	}
	entry, ok := ps.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	current := planfsm.Status(entry.Status)
	if current == planfsm.StatusPlanning {
		if err := fsm.Transition(planFile, planfsm.PlannerFinished); err != nil {
			return err
		}
	}
	if current != planfsm.StatusImplementing {
		if err := fsm.Transition(planFile, planfsm.ImplementStart); err != nil {
			return err
		}
	}

	// Write the wave signal file.
	signalsDir := filepath.Join(plansDir, ".signals")
	if err := os.MkdirAll(signalsDir, 0o755); err != nil {
		return err
	}
	signalName := fmt.Sprintf("implement-wave-%d-%s", wave, planFile)
	return os.WriteFile(filepath.Join(signalsDir, signalName), nil, 0o644)
}

// NewPlanCmd builds the `kq plan` cobra command tree.
func NewPlanCmd() *cobra.Command {
	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Manage plan lifecycle (list, set-status, transition, implement)",
	}

	// kq plan list
	var statusFilter string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all plans with status",
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			fmt.Print(executePlanList(plansDir, statusFilter))
			return nil
		},
	}
	listCmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status (ready, planning, implementing, reviewing, done, cancelled)")
	planCmd.AddCommand(listCmd)

	// kq plan set-status
	var forceFlag bool
	setStatusCmd := &cobra.Command{
		Use:   "set-status <plan-file> <status>",
		Short: "Force-override a plan's status (bypasses FSM)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanSetStatus(plansDir, args[0], args[1], forceFlag); err != nil {
				return err
			}
			fmt.Printf("%s → %s\n", args[0], args[1])
			return nil
		},
	}
	setStatusCmd.Flags().BoolVar(&forceFlag, "force", false, "Confirm intent to bypass FSM transition rules")
	planCmd.AddCommand(setStatusCmd)

	// kq plan transition
	transitionCmd := &cobra.Command{
		Use:   "transition <plan-file> <event>",
		Short: "Apply an FSM event to a plan",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			newStatus, err := executePlanTransition(plansDir, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Printf("%s → %s\n", args[0], newStatus)
			return nil
		},
	}
	planCmd.AddCommand(transitionCmd)

	// kq plan implement
	var waveNum int
	implementCmd := &cobra.Command{
		Use:   "implement <plan-file>",
		Short: "Trigger implementation of a specific wave",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanImplement(plansDir, args[0], waveNum); err != nil {
				return err
			}
			fmt.Printf("implementation triggered: %s wave %d\n", args[0], waveNum)
			return nil
		},
	}
	implementCmd.Flags().IntVar(&waveNum, "wave", 1, "Wave number to trigger (default: 1)")
	planCmd.AddCommand(implementCmd)

	return planCmd
}

func resolvePlansDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	dir := filepath.Join(cwd, "docs", "plans")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("plans directory not found: %s", dir)
	}
	return dir, nil
}
```

Register in `main.go` init():

```go
rootCmd.AddCommand(NewPlanCmd())
```

Where `NewPlanCmd` is imported from `cmd` — but since `main.go` already imports `cmd2 "github.com/kastheco/kasmos/cmd"`, just add:

```go
rootCmd.AddCommand(cmd2.NewPlanCmd())
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestPlan -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add cmd/plan.go cmd/plan_test.go config/planstate/planstate.go main.go
git commit -m "feat: add kq plan CLI command group (list, set-status, transition, implement)"
```

### Task 2: `implement-wave-N` Signal Scanner

**Files:**
- Modify: `config/planfsm/signals.go:23-31` (add wave signal prefix + new `ImplementWave` event type)
- Modify: `config/planfsm/fsm.go` (add `ImplementWave` event constant)
- Create: `config/planfsm/wave_signal.go` (wave signal parsing with wave number extraction)
- Modify: `app/app.go:620-660` (handle wave signals in metadataResultMsg)
- Test: `config/planfsm/wave_signal_test.go`

**Step 1: Write the failing tests**

Create `config/planfsm/wave_signal_test.go`:

```go
package planfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWaveSignal(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOK   bool
		wantWave int
		wantPlan string
	}{
		{
			name:     "valid wave 1 signal",
			filename: "implement-wave-1-2026-02-20-test-plan.md",
			wantOK:   true,
			wantWave: 1,
			wantPlan: "2026-02-20-test-plan.md",
		},
		{
			name:     "valid wave 3 signal",
			filename: "implement-wave-3-2026-02-20-multi-wave.md",
			wantOK:   true,
			wantWave: 3,
			wantPlan: "2026-02-20-multi-wave.md",
		},
		{
			name:     "not a wave signal",
			filename: "planner-finished-2026-02-20-test-plan.md",
			wantOK:   false,
		},
		{
			name:     "malformed wave number",
			filename: "implement-wave-abc-2026-02-20-test-plan.md",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, ok := ParseWaveSignal(tt.filename)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantWave, ws.WaveNumber)
				assert.Equal(t, tt.wantPlan, ws.PlanFile)
			}
		})
	}
}

func TestScanSignals_IncludesWaveSignals(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write a regular signal and a wave signal
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-2026-02-20-test.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "implement-wave-2-2026-02-20-test.md"), nil, 0o644))

	signals := ScanSignals(dir)
	// Regular signal should be present
	assert.Len(t, signals, 1, "only regular signals returned by ScanSignals")

	// Wave signals have their own scanner
	waveSignals := ScanWaveSignals(dir)
	require.Len(t, waveSignals, 1)
	assert.Equal(t, 2, waveSignals[0].WaveNumber)
	assert.Equal(t, "2026-02-20-test.md", waveSignals[0].PlanFile)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./config/planfsm/ -run TestParseWaveSignal -v && go test ./config/planfsm/ -run TestScanSignals_IncludesWaveSignals -v`
Expected: compilation errors

**Step 3: Implement wave signal parsing**

Create `config/planfsm/wave_signal.go`:

```go
package planfsm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// WaveSignal represents a parsed implement-wave signal file.
type WaveSignal struct {
	WaveNumber int
	PlanFile   string
	filePath   string // full path for deletion
}

var waveSignalRe = regexp.MustCompile(`^implement-wave-(\d+)-(.+\.md)$`)

// ParseWaveSignal attempts to parse a filename as a wave signal.
func ParseWaveSignal(filename string) (WaveSignal, bool) {
	m := waveSignalRe.FindStringSubmatch(filename)
	if m == nil {
		return WaveSignal{}, false
	}
	wave, err := strconv.Atoi(m[1])
	if err != nil {
		return WaveSignal{}, false
	}
	return WaveSignal{
		WaveNumber: wave,
		PlanFile:   m[2],
	}, true
}

// ScanWaveSignals reads docs/plans/.signals/ and returns parsed wave signals.
// These are handled separately from FSM signals because they don't map to
// state transitions — they trigger wave orchestration in the TUI.
func ScanWaveSignals(plansDir string) []WaveSignal {
	signalsDir := filepath.Join(plansDir, ".signals")
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}

	var signals []WaveSignal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ws, ok := ParseWaveSignal(entry.Name())
		if !ok {
			continue
		}
		ws.filePath = filepath.Join(signalsDir, entry.Name())
		signals = append(signals, ws)
	}
	return signals
}

// ConsumeWaveSignal deletes the wave signal file after processing.
func ConsumeWaveSignal(ws WaveSignal) {
	_ = os.Remove(ws.filePath)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/planfsm/ -run "TestParseWaveSignal|TestScanSignals_IncludesWaveSignals" -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add config/planfsm/wave_signal.go config/planfsm/wave_signal_test.go
git commit -m "feat: add implement-wave signal parsing for CLI-triggered wave orchestration"
```

### Task 3: TUI Wave Signal Consumption

**Files:**
- Modify: `app/app.go:560-620` (add wave signal scanning to metadata tick)
- Modify: `app/app.go:620-660` (handle wave signals in metadataResultMsg)
- Modify: `app/app.go:1560-1570` (add WaveSignals to metadataResultMsg)
- Test: `app/app_wave_orchestration_flow_test.go` (add test for signal-triggered wave)

**Step 1: Write the failing test**

Add to `app/app_wave_orchestration_flow_test.go`:

```go
func TestWaveSignal_TriggersImplementation(t *testing.T) {
	plansDir := t.TempDir()
	signalsDir := filepath.Join(plansDir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Create a plan with wave headers
	planContent := "# Test\n\n**Goal:** test\n\n## Wave 1\n\n### Task 1: Do thing\n\nDo the thing.\n"
	planFile := "2026-02-20-wave-signal-test.md"
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0o644))

	// Register plan as implementing
	ps := &planstate.PlanState{Dir: plansDir, Plans: make(map[string]planstate.PlanEntry), TopicEntries: make(map[string]planstate.TopicEntry)}
	ps.Plans[planFile] = planstate.PlanEntry{
		Status: "implementing",
		Branch: "plan/wave-signal-test",
	}
	require.NoError(t, ps.Save())

	// Write a wave signal
	signalFile := fmt.Sprintf("implement-wave-1-%s", planFile)
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, signalFile), nil, 0o644))

	// Verify signal is scannable
	waveSignals := planfsm.ScanWaveSignals(plansDir)
	require.Len(t, waveSignals, 1)
	assert.Equal(t, 1, waveSignals[0].WaveNumber)
	assert.Equal(t, planFile, waveSignals[0].PlanFile)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestWaveSignal_TriggersImplementation -v`
Expected: FAIL (missing import or function)

**Step 3: Wire wave signals into the metadata tick**

In `app/app.go`, add `WaveSignals` field to `metadataResultMsg`:

```go
type metadataResultMsg struct {
	Results          []metadataResult
	PlanState        *planstate.PlanState
	Signals          []planfsm.Signal
	WaveSignals      []planfsm.WaveSignal  // NEW
	TmuxSessionCount int
}
```

In the metadata tick goroutine (around line 590), after scanning regular signals, add:

```go
var waveSignals []planfsm.WaveSignal
if planStateDir != "" {
	waveSignals = planfsm.ScanWaveSignals(planStateDir)
}
```

Pass `waveSignals` into the `metadataResultMsg`.

In the `metadataResultMsg` handler (around line 660), after processing regular signals, add wave signal handling:

```go
// Process wave signals — trigger implementation for specific waves.
for _, ws := range msg.WaveSignals {
	planfsm.ConsumeWaveSignal(ws)

	// Check if orchestrator already exists
	if _, exists := m.waveOrchestrators[ws.PlanFile]; exists {
		m.toastManager.Warn(fmt.Sprintf("wave already running for '%s'", planstate.DisplayName(ws.PlanFile)))
		continue
	}

	// Read and parse the plan
	plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
	content, err := os.ReadFile(filepath.Join(plansDir, ws.PlanFile))
	if err != nil {
		log.WarningLog.Printf("wave signal: could not read plan %s: %v", ws.PlanFile, err)
		continue
	}
	plan, err := planparser.Parse(string(content))
	if err != nil {
		m.toastManager.Warn(fmt.Sprintf("plan '%s' has no wave headers", planstate.DisplayName(ws.PlanFile)))
		continue
	}

	if ws.WaveNumber > len(plan.Waves) {
		m.toastManager.Warn(fmt.Sprintf("plan has %d waves, requested wave %d", len(plan.Waves), ws.WaveNumber))
		continue
	}

	entry, ok := m.planState.Entry(ws.PlanFile)
	if !ok {
		continue
	}

	orch := NewWaveOrchestrator(ws.PlanFile, plan)
	m.waveOrchestrators[ws.PlanFile] = orch

	// Fast-forward to the requested wave
	for i := 1; i < ws.WaveNumber; i++ {
		tasks := orch.StartNextWave()
		for _, t := range tasks {
			orch.MarkTaskComplete(t.Number)
		}
	}

	mdl, cmd := m.startNextWave(orch, entry)
	m = mdl.(*home)
	if cmd != nil {
		signalCmds = append(signalCmds, cmd)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./app/ -run TestWaveSignal -v`
Expected: PASS

Run: `go test ./... 2>&1 | tail -5`
Expected: all existing tests still pass

**Step 5: Commit**

```bash
git add app/app.go app/app_wave_orchestration_flow_test.go
git commit -m "feat: wire implement-wave signals into TUI metadata tick for CLI-triggered waves"
```

## Wave 2: Agent, Templates, Slash Commands
> **Depends on Wave 1:** Slash commands call `kq plan` subcommands and rely on the wave signal system.

### Task 4: Custodial Agent Template + Scaffold Integration

**Files:**
- Create: `internal/initcmd/scaffold/templates/opencode/agents/custodial.md`
- Create: `internal/initcmd/scaffold/templates/claude/agents/custodial.md`
- Modify: `internal/initcmd/scaffold/templates/opencode/opencode.jsonc` (add static custodial block)
- Modify: `internal/initcmd/scaffold/scaffold.go:115` (add "custodial" to static agent blocks in `renderOpenCodeConfig`)
- Modify: `.opencode/agents/custodial.md` (create local copy for immediate use)
- Modify: `.opencode/opencode.jsonc` (add custodial agent block to local config)
- Test: `internal/initcmd/scaffold/scaffold_test.go` (add test for custodial agent scaffolding)

**Step 1: Write the failing test**

Add to `internal/initcmd/scaffold/scaffold_test.go`:

```go
func TestScaffold_IncludesCustodialAgent(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Harness: "opencode", Role: "coder", Model: "anthropic/claude-sonnet-4-6"},
	}
	results, err := WriteOpenCodeProject(dir, agents, nil, true)
	require.NoError(t, err)

	// Custodial agent should be scaffolded even though it wasn't in agents list
	custodialPath := filepath.Join(dir, ".opencode", "agents", "custodial.md")
	assert.FileExists(t, custodialPath)

	content, err := os.ReadFile(custodialPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "custodial")

	// Check opencode.jsonc includes custodial block
	var foundConfig bool
	for _, r := range results {
		if strings.Contains(r.Path, "opencode.jsonc") {
			foundConfig = true
		}
	}
	assert.True(t, foundConfig)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/initcmd/scaffold/ -run TestScaffold_IncludesCustodialAgent -v`
Expected: FAIL (custodial.md template doesn't exist)

**Step 3: Create templates and wire scaffold**

Create `internal/initcmd/scaffold/templates/opencode/agents/custodial.md`:

```markdown
---
description: Custodial agent - operational fixes, state resets, cleanup, branch management
mode: primary
---

You are the custodial agent. Handle operational touchup tasks and quick fixes in the kasmos workflow.

## Role

You are an ops/janitor role. You fix workflow state, clean up debris, and execute well-defined
operational tasks. You do NOT plan features, write implementation code, or review PRs.

## Operations

Use `kq plan` CLI for all plan state mutations:
- `kq plan list [--status <status>]` — show plans and filter by status
- `kq plan set-status <plan> <status> --force` — force-override a plan's status
- `kq plan transition <plan> <event>` — apply a valid FSM event
- `kq plan implement <plan> [--wave N]` — trigger wave implementation

Use raw git/gh for branch and worktree operations:
- `git worktree list` / `git worktree remove <path>` — manage worktrees
- `git branch -d <branch>` — clean up branches
- `gh pr create` — create pull requests
- `git merge` — merge branches

## Slash Commands

These commands are available for one-shot operations:
- `/kas.reset-plan <plan-file> <status>` — force-reset a plan's status
- `/kas.finish-branch [plan-file]` — merge to main or create PR
- `/kas.cleanup [--dry-run]` — remove stale worktrees and orphan branches
- `/kas.implement <plan-file> [--wave N]` — trigger wave implementation
- `/kas.triage` — bulk scan and triage plans

## Behavior

- Always confirm what you're about to do before doing it (one-line summary)
- Report what changed after each operation
- Refuse feature work, code implementation, design, or review tasks
- Be terse — no walls of text, just action and result

## CLI Tools (MANDATORY)

You MUST read the `cli-tools` skill (SKILL.md) at the start of every session.
```

Create `internal/initcmd/scaffold/templates/claude/agents/custodial.md` with similar content (adjusted for claude's format — no frontmatter needed since claude uses CLAUDE.md convention, but keep the .md agent file for consistency with the per-role scaffold pattern).

Add the static `"custodial"` block to `opencode.jsonc` template. The custodial block uses hardcoded values (not wizard-templated), placed after the `"reviewer"` block:

```jsonc
    "custodial": {
      "model": "anthropic/claude-sonnet-4-6",
      "permission": {
        "bash": "allow",
        "edit": "allow",
        "external_directory": {
          "*": "ask",
          "{{PROJECT_DIR}}/*": "allow",
          "{{PROJECT_DIR}}/**": "allow",
          "/tmp/*": "allow",
          "/tmp/**": "allow",
          "{{HOME_DIR}}/.config/opencode/*": "allow",
          "{{HOME_DIR}}/.config/opencode/**": "allow"
        },
        "glob": "allow",
        "grep": "allow",
        "read": "allow",
        "write": "allow"
      },
      "reasoningEffort": "low",
      "temperature": 0.1,
      "textVerbosity": "low"
    }
```

Update `renderOpenCodeConfig` in `scaffold.go` to substitute `{{PROJECT_DIR}}` and `{{HOME_DIR}}` in the custodial block (these are already globally replaced so no code change needed for that).

Also update the local `.opencode/opencode.jsonc` and create `.opencode/agents/custodial.md` directly.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/initcmd/scaffold/ -run TestScaffold_IncludesCustodialAgent -v`
Expected: PASS

Run: `go test ./... 2>&1 | tail -5`
Expected: all pass

**Step 5: Commit**

```bash
git add internal/initcmd/scaffold/templates/opencode/agents/custodial.md \
        internal/initcmd/scaffold/templates/claude/agents/custodial.md \
        internal/initcmd/scaffold/templates/opencode/opencode.jsonc \
        internal/initcmd/scaffold/scaffold.go \
        .opencode/agents/custodial.md \
        .opencode/opencode.jsonc
git commit -m "feat: add custodial agent to scaffold system and local config"
```

### Task 5: Slash Commands

**Files:**
- Create: `.opencode/commands/kas.reset-plan.md`
- Create: `.opencode/commands/kas.finish-branch.md`
- Create: `.opencode/commands/kas.cleanup.md`
- Create: `.opencode/commands/kas.implement.md`
- Create: `.opencode/commands/kas.triage.md`

These are all markdown-only agent command files — no Go code needed.

**Step 1: Create `/kas.reset-plan`**

```markdown
---
description: Force-reset a plan's status (bypasses FSM)
agent: custodial
---

# /kas.reset-plan

Force-override a plan's status, bypassing normal FSM transition rules.

## Arguments

```
$ARGUMENTS
```

Expected format: `<plan-file> <status>`

Example: `/kas.reset-plan 2026-02-20-my-plan.md ready`

## Process

1. Parse arguments into plan filename and target status
2. If arguments are missing or malformed, show usage and list current plans:
   ```bash
   kq plan list
   ```
3. Show current status before changing:
   ```bash
   kq plan list --status "" | grep "<plan-file>"
   ```
4. Execute the override:
   ```bash
   kq plan set-status <plan-file> <status> --force
   ```
5. Confirm the change:
   ```bash
   kq plan list --status "" | grep "<plan-file>"
   ```

## Valid statuses
ready, planning, implementing, reviewing, done, cancelled
```

**Step 2: Create `/kas.finish-branch`**

```markdown
---
description: Merge a plan's branch to main or create a PR
agent: custodial
---

# /kas.finish-branch

Finish a development branch by merging to main or creating a pull request.

## Arguments

```
$ARGUMENTS
```

Optional: plan filename. If omitted, infer from current git branch.

## Process

1. Resolve the plan and its branch:
   - If argument provided, look up branch from `kq plan list`
   - If no argument, detect from `git branch --show-current` and match to a plan
2. Verify the branch has commits ahead of main:
   ```bash
   git log main..<branch> --oneline
   ```
   If no commits ahead, report "branch is up to date with main" and stop.
3. Run tests:
   ```bash
   go test ./...
   ```
   If tests fail, report failures and stop.
4. Present options:
   ```
   branch '<branch>' has N commits ahead of main.

   1. merge to main locally
   2. push and create a pull request
   3. keep as-is
   ```
5. Execute chosen option:
   - **Merge**: `git checkout main && git merge <branch> && git branch -d <branch>`
   - **PR**: `git push -u origin <branch> && gh pr create --title "<plan-name>" --body "..."`
   - **Keep**: do nothing
6. On merge or PR, update plan status:
   ```bash
   kq plan set-status <plan-file> done --force
   ```
7. If worktree exists for this branch, offer to clean it up:
   ```bash
   git worktree remove <path>
   ```
```

**Step 3: Create `/kas.cleanup`**

```markdown
---
description: Remove stale worktrees, orphan branches, and ghost plan entries
agent: custodial
---

# /kas.cleanup

Three-pass cleanup of worktrees, branches, and plan state.

## Arguments

```
$ARGUMENTS
```

Optional flags: `--execute` to actually delete (default is dry-run).

## Process

### Pass 1: Stale worktrees

Find worktrees whose plan is done or cancelled:

```bash
git worktree list
kq plan list
```

Cross-reference: any worktree on a `plan/*` branch where the plan status is `done` or `cancelled` is stale.

### Pass 2: Orphan branches

Find local `plan/*` branches with no matching entry in plan-state.json:

```bash
git branch --list 'plan/*'
kq plan list
```

### Pass 3: Ghost plan entries

Find plan entries in plan-state.json that have no corresponding .md file on disk:

```bash
kq plan list
ls docs/plans/*.md
```

### Output

Report findings grouped by pass. If `--execute` was specified, remove stale worktrees
(`git worktree remove`), delete orphan branches (`git branch -d`), and flag ghost entries.

If dry-run (default), just list what would be cleaned.
```

**Step 4: Create `/kas.implement`**

```markdown
---
description: Trigger implementation of a specific wave for a plan
agent: custodial
---

# /kas.implement

Trigger wave-based implementation for a plan via signal file.

## Arguments

```
$ARGUMENTS
```

Expected format: `<plan-file> [--wave N]`
Default wave: 1

## Process

1. Parse arguments for plan filename and optional wave number
2. If no arguments, show available plans:
   ```bash
   kq plan list --status ready
   kq plan list --status implementing
   ```
3. Verify plan exists and has wave headers:
   ```bash
   head -50 docs/plans/<plan-file>
   ```
4. Execute:
   ```bash
   kq plan implement <plan-file> --wave <N>
   ```
5. Confirm:
   ```
   implementation triggered: <plan-file> wave <N>
   the TUI will pick up the signal on the next tick (~2s).
   ```
```

**Step 5: Create `/kas.triage`**

```markdown
---
description: Bulk scan and triage non-terminal plans
agent: custodial
---

# /kas.triage

Scan all non-done/cancelled plans and present them for triage.

## Arguments

```
$ARGUMENTS
```

Optional: specific status to triage (e.g., `ready`, `implementing`).

## Process

1. List all active plans:
   ```bash
   kq plan list
   ```
2. For each non-terminal plan (not done/cancelled), gather context:
   - Branch existence: `git branch --list '<branch>'`
   - Worktree existence: `git worktree list | grep '<branch>'`
   - Last commit on branch: `git log <branch> -1 --format='%ar - %s' 2>/dev/null`
   - Plan file exists on disk: `ls docs/plans/<filename>`
3. Present grouped by status:
   ```
   ## ready (N plans)
   - plan-name.md — branch: plan/name, worktree: yes/no, last commit: 2d ago
   ...

   ## implementing (N plans)
   ...
   ```
4. For each group, ask what to do:
   - ready plans: "implement, cancel, or skip?"
   - implementing plans: "the branch may be stale. cancel, reset to ready, or skip?"
   - reviewing plans: "mark done, reset to implementing, or skip?"
5. Execute chosen actions via `kq plan set-status --force` or `kq plan transition`.
```

**Step 6: Commit**

```bash
git add .opencode/commands/kas.reset-plan.md \
        .opencode/commands/kas.finish-branch.md \
        .opencode/commands/kas.cleanup.md \
        .opencode/commands/kas.implement.md \
        .opencode/commands/kas.triage.md
git commit -m "feat: add custodial slash commands (reset-plan, finish-branch, cleanup, implement, triage)"
```

### Task 6: Integration Test + Build Verification

**Files:**
- Modify: `cmd/plan_test.go` (add end-to-end test exercising the full CLI flow)
- No new files

**Step 1: Write integration test**

Add to `cmd/plan_test.go`:

```go
func TestPlanCLI_EndToEnd(t *testing.T) {
	dir := setupTestPlanState(t)
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// List all
	output := executePlanList(dir, "")
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "implementing")

	// Transition ready → planning
	status, err := executePlanTransition(dir, "2026-02-20-test-plan.md", "plan_start")
	require.NoError(t, err)
	assert.Equal(t, "planning", status)

	// Force set back to ready
	err = executePlanSetStatus(dir, "2026-02-20-test-plan.md", "ready", true)
	require.NoError(t, err)

	// Implement with wave signal
	err = executePlanImplement(dir, "2026-02-20-test-plan.md", 2)
	require.NoError(t, err)

	// Verify signal file
	entries, _ := os.ReadDir(signalsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "implement-wave-2-2026-02-20-test-plan.md")

	// Verify final status
	ps, _ := planstate.Load(dir)
	entry, _ := ps.Entry("2026-02-20-test-plan.md")
	assert.Equal(t, planstate.Status("implementing"), entry.Status)
}
```

**Step 2: Run tests**

Run: `go test ./cmd/ -run TestPlanCLI_EndToEnd -v`
Expected: PASS

**Step 3: Full build and test verification**

```bash
go build ./...
go test ./...
go vet ./...
```

All must pass.

**Step 4: Commit**

```bash
git add cmd/plan_test.go
git commit -m "test: add end-to-end integration test for kq plan CLI"
```
