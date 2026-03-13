package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteTaskStage_BlueprintSkipSmallPlan(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	const planFile = "small-plan"
	require.NoError(t, ps.Register(planFile, "small plan", "plan/small-plan", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusReady)

	content := strings.Join([]string{
		"# Test Plan",
		"",
		"**Goal:** test",
		"**Architecture:** test",
		"**Tech Stack:** Go",
		"**Size:** Trivial",
		"",
		"## Wave 1",
		"",
		"### Task 1: First task",
		"",
		"Do the first thing.",
		"",
	}, "\n")
	require.NoError(t, store.SetContent("test", planFile, content))

	threshold := 2
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                context.Background(),
		state:              stateDefault,
		appConfig:          &config.Config{BlueprintSkipThresholdValue: &threshold},
		nav:                ui.NewNavigationPanel(&sp),
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		overlays:           overlay.NewManager(),
		taskState:          ps,
		taskStateDir:       plansDir,
		taskStore:          store,
		taskStoreProject:   "test",
		fsm:                fsm,
		waveOrchestrators:  make(map[string]*orchestration.WaveOrchestrator),
		activeRepoPath:     dir,
		program:            "opencode",
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	model, cmd := h.executeTaskStage(planFile, "implement")
	updated := model.(*home)

	require.NotNil(t, cmd)
	_, hasOrch := updated.waveOrchestrators[planFile]
	assert.False(t, hasOrch, "small plan should not create a WaveOrchestrator")

	entry, ok := updated.taskState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusImplementing, entry.Status)

	instances := updated.nav.GetInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, session.AgentTypeCoder, instances[0].AgentType)
	assert.Equal(t, planFile, instances[0].TaskFile)
	assert.Contains(t, instances[0].QueuedPrompt, "First task")
	assert.Contains(t, instances[0].QueuedPrompt, "kas signal emit implement_finished small-plan")
}

func TestExecuteTaskStage_BlueprintSkipDirectClearsStaleOrchestrator(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	const planFile = "small-plan-direct"
	require.NoError(t, ps.Register(planFile, "small plan direct", "plan/small-plan-direct", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	content := strings.Join([]string{
		"# Test Plan",
		"",
		"**Goal:** test",
		"**Architecture:** test",
		"**Tech Stack:** Go",
		"**Size:** Trivial",
		"",
		"## Wave 1",
		"",
		"### Task 1: First task",
		"",
		"Do the first thing.",
		"",
	}, "\n")
	require.NoError(t, store.SetContent("test", planFile, content))

	plan, err := taskparser.Parse(content)
	require.NoError(t, err)

	threshold := 2
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:              context.Background(),
		state:            stateDefault,
		appConfig:        &config.Config{BlueprintSkipThresholdValue: &threshold},
		nav:              ui.NewNavigationPanel(&sp),
		menu:             ui.NewMenu(),
		toastManager:     overlay.NewToastManager(&sp),
		overlays:         overlay.NewManager(),
		taskState:        ps,
		taskStateDir:     plansDir,
		taskStore:        store,
		taskStoreProject: "test",
		fsm:              fsm,
		waveOrchestrators: map[string]*orchestration.WaveOrchestrator{
			planFile: orchestration.NewWaveOrchestrator(planFile, plan),
		},
		activeRepoPath:     dir,
		program:            "opencode",
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	model, cmd := h.executeTaskStage(planFile, "implement_direct")
	updated := model.(*home)

	require.NotNil(t, cmd)
	_, hasOrch := updated.waveOrchestrators[planFile]
	assert.False(t, hasOrch, "blueprint skip direct mode should clear any stale WaveOrchestrator")

	instances := updated.nav.GetInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, session.AgentTypeCoder, instances[0].AgentType)
	assert.Equal(t, planFile, instances[0].TaskFile)
	assert.Contains(t, instances[0].QueuedPrompt, "kas signal emit implement_finished small-plan-direct")
}

func TestExecuteTaskStage_BlueprintSkipDirectClearsProcessorWaveState(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	const planFile = "small-plan-processor"
	require.NoError(t, ps.Register(planFile, "small plan processor", "plan/small-plan-processor", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	content := strings.Join([]string{
		"# Test Plan",
		"",
		"**Goal:** test",
		"**Architecture:** test",
		"**Tech Stack:** Go",
		"",
		"## Wave 1",
		"",
		"### Task 1: First task",
		"",
		"Do the first thing.",
		"",
	}, "\n")
	require.NoError(t, store.SetContent("test", planFile, content))

	threshold := 2
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                context.Background(),
		state:              stateDefault,
		appConfig:          &config.Config{BlueprintSkipThresholdValue: &threshold},
		nav:                ui.NewNavigationPanel(&sp),
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		overlays:           overlay.NewManager(),
		taskState:          ps,
		taskStateDir:       plansDir,
		taskStore:          store,
		taskStoreProject:   "test",
		fsm:                fsm,
		waveOrchestrators:  make(map[string]*orchestration.WaveOrchestrator),
		activeRepoPath:     dir,
		program:            "opencode",
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	proc := h.ensureProcessor()
	require.NotNil(t, proc)
	proc.SetWaveOrchestratorActive(planFile, true)

	_, cmd := h.executeTaskStage(planFile, "implement_direct")
	require.NotNil(t, cmd)

	actions := h.ensureProcessor().ProcessFSMSignals([]taskfsm.Signal{{
		TaskFile: planFile,
		Event:    taskfsm.ImplementFinished,
	}})
	require.Len(t, actions, 1)
	_, ok := actions[0].(loop.SpawnReviewerAction)
	assert.True(t, ok, "implement_finished should no longer be suppressed after direct blueprint skip clears wave state")
}

func TestExecuteTaskStage_BlueprintSkipImplementDoesNotDuplicateCoder(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	store, ps, fsm := newSharedStoreForTest(t, plansDir)
	const planFile = "small-plan-running"
	require.NoError(t, ps.Register(planFile, "small plan running", "plan/small-plan-running", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	content := strings.Join([]string{
		"# Test Plan",
		"",
		"**Goal:** test",
		"**Architecture:** test",
		"**Tech Stack:** Go",
		"",
		"## Wave 1",
		"",
		"### Task 1: First task",
		"",
		"Do the first thing.",
		"",
	}, "\n")
	require.NoError(t, store.SetContent("test", planFile, content))

	threshold := 2
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:                context.Background(),
		state:              stateDefault,
		appConfig:          &config.Config{BlueprintSkipThresholdValue: &threshold},
		nav:                ui.NewNavigationPanel(&sp),
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		overlays:           overlay.NewManager(),
		taskState:          ps,
		taskStateDir:       plansDir,
		taskStore:          store,
		taskStoreProject:   "test",
		fsm:                fsm,
		waveOrchestrators:  make(map[string]*orchestration.WaveOrchestrator),
		activeRepoPath:     dir,
		program:            "opencode",
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.nav.AddInstance(&session.Instance{
		Title:     "small-plan-running-implement",
		TaskFile:  planFile,
		AgentType: session.AgentTypeCoder,
		Path:      dir,
	})

	model, cmd := h.executeTaskStage(planFile, "implement")
	updated := model.(*home)

	require.NotNil(t, cmd)
	instances := updated.nav.GetInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, session.AgentTypeCoder, instances[0].AgentType)
	assert.Equal(t, planFile, instances[0].TaskFile)
}
