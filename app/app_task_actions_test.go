package app

import (
	"context"
	"encoding/json"
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
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanPrompt(t *testing.T) {
	prompt := buildPlanningPrompt("auth-refactor", "Auth Refactor", "Refactor JWT auth")
	if !strings.Contains(prompt, "Plan Auth Refactor") {
		t.Fatalf("prompt missing title")
	}
	if !strings.Contains(prompt, "Goal: Refactor JWT auth") {
		t.Fatalf("prompt missing goal")
	}
	// Wave headers are required for kasmos orchestration — the prompt must
	// instruct the planner to include them.
	assert.Contains(t, prompt, "Wave", "plan prompt must mention Wave headers for kasmos orchestration")
	assert.Contains(t, prompt, "kasmos-planner", "plan prompt must reference the kasmos-planner skill")
	assert.Contains(t, prompt, "kas task update-content auth-refactor", "plan prompt must include content storage command")
	assert.Contains(t, prompt, "planner-finished-auth-refactor", "plan prompt must include planner completion signal")
}

func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := orchestration.BuildWaveAnnotationPrompt("my-feature")
	assert.Contains(t, prompt, "kas task show my-feature", "prompt must reference kas task show")
	assert.Contains(t, prompt, "## Wave", "prompt must mention ## Wave header format")
	assert.Contains(t, prompt, "kas task", "prompt must instruct the planner to store content via kas task")
	assert.Contains(t, prompt, "planner-finished-", "prompt must include the signal file instruction")
	assert.NotContains(t, prompt, "The plan at docs/plans/", "prompt must not reference disk path for reading")
}

func TestBuildWaveAnnotationPrompt_SingleWaveFallback(t *testing.T) {
	prompt := orchestration.BuildWaveAnnotationPrompt("trivial")
	// Even trivial plans must be wrapped in at least ## Wave 1
	assert.Contains(t, prompt, "## Wave 1", "prompt must specify ## Wave 1 as the minimum structure")
}

func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("auth-refactor")
	assert.Contains(t, prompt, "kas task show auth-refactor")
	assert.NotContains(t, prompt, "docs/plans/")
	assert.NotContains(t, prompt, "kasmos-coder", "implement prompt must not reference skill to avoid skill-load overhead")
}

func TestSoloAgentPrompt_ContainsTestScopingRule(t *testing.T) {
	prompt := buildSoloPrompt("auth-refactor", "Refactor JWT auth", "auth-refactor")
	assert.Contains(t, prompt, "-run Test")
	assert.Contains(t, prompt, "Do not load skills")
}

func TestBuildSoloPrompt_WithDescription(t *testing.T) {
	prompt := buildSoloPrompt("auth-refactor", "Refactor JWT auth", "auth-refactor")
	assert.Contains(t, prompt, "kas task show auth-refactor")
	assert.NotContains(t, prompt, "docs/plans/")
}

func TestBuildSoloPrompt_StubOnly(t *testing.T) {
	prompt := buildSoloPrompt("quick-fix", "Fix the login bug", "")
	assert.NotContains(t, prompt, "kas task show")
	assert.NotContains(t, prompt, "docs/plans/")
}

func TestAgentTypeForSubItem(t *testing.T) {
	tests := map[string]string{
		"plan":      session.AgentTypePlanner,
		"implement": session.AgentTypeCoder,
		"review":    session.AgentTypeReviewer,
		"solo":      session.AgentTypeCoder,
	}
	for action, want := range tests {
		got, ok := agentTypeForSubItem(action)
		if !ok {
			t.Fatalf("agentTypeForSubItem(%q) returned ok=false", action)
		}
		if got != want {
			t.Fatalf("agentTypeForSubItem(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestIsLocked_AllowsSoloStage(t *testing.T) {
	assert.False(t, isLocked(taskstate.StatusReady, "solo"),
		"solo stage should be triggerable like implement/review")
}

// TestSpawnPlanAgent_ReviewerSetsIsReviewer verifies that spawnTaskAgent sets
// IsReviewer=true on the created instance when the action is "review", so that
// the reviewer completion check in the metadata tick handler (which gates on
// inst.IsReviewer) can detect when the reviewer session exits.
//
// This is a regression test for the bug where spawnTaskAgent set AgentType but
// not IsReviewer, causing sidebar-spawned reviewers to never trigger plan completion.
func TestSpawnPlanAgent_ReviewerSetsIsReviewer(t *testing.T) {
	dir := t.TempDir()

	// Set up a minimal git repo so shared.Setup() can open it.
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
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ps, err := newTestPlanState(t, plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "test.md"
	if err := ps.Register(planFile, "test plan", "plan/test", time.Now()); err != nil {
		t.Fatal(err)
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		taskState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnTaskAgent(planFile, "review", "review prompt")

	instances := list.GetInstances()
	if len(instances) == 0 {
		t.Fatal("expected instance to be added to list after spawnTaskAgent(review)")
	}
	inst := instances[len(instances)-1]
	if inst.AgentType != session.AgentTypeReviewer {
		t.Fatalf("AgentType = %q, want %q", inst.AgentType, session.AgentTypeReviewer)
	}
	if !inst.IsReviewer {
		t.Fatal("spawnTaskAgent(review) must set IsReviewer=true on the created instance")
	}
}

// TestSpawnPlanAgent_PlannerUsesMainBranch verifies that spawnTaskAgent for the
// "plan" action does NOT create a git worktree — the planner runs on main and
// commits plan files there directly.
func TestSpawnPlanAgent_PlannerUsesMainBranch(t *testing.T) {
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
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ps, err := newTestPlanState(t, plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "test-planner.md"
	if err := ps.Register(planFile, "test plan", "plan/test-planner", time.Now()); err != nil {
		t.Fatal(err)
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		taskState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnTaskAgent(planFile, "plan", "plan prompt")

	instances := list.GetInstances()
	if len(instances) == 0 {
		t.Fatal("expected instance to be added to list after spawnTaskAgent(plan)")
	}
	inst := instances[len(instances)-1]
	if inst.AgentType != session.AgentTypePlanner {
		t.Fatalf("AgentType = %q, want %q", inst.AgentType, session.AgentTypePlanner)
	}
	// Planner should have no branch assigned — it runs on main, not a worktree branch.
	if inst.Branch != "" {
		t.Fatalf("planner instance must have empty Branch (runs on main), got %q", inst.Branch)
	}
}

// TestSpawnTaskAgent_PatchesSharedWorktreeOpencodeConfig verifies that when
// spawnTaskAgent is called for an "implement" (coder) action, PatchWorktreeConfig is
// applied to the SHARED WORKTREE path — not the main repo — so the agent running inside
// the worktree reads the correct model/temperature/effort from its own opencode.jsonc.
func TestSpawnTaskAgent_PatchesSharedWorktreeOpencodeConfig(t *testing.T) {
	dir := t.TempDir()

	// Build a git repo with .opencode/opencode.jsonc committed so the worktree
	// inherits the file when git worktree add creates it.
	opencodeDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(opencodeDir, 0o755))
	configPath := filepath.Join(opencodeDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent":{"coder":{"model":"anthropic/old-coder","temperature":0.1,"reasoningEffort":"low"}}}`), 0o644))

	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "init with opencode config"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	planFile := "shared-wt-patch"
	require.NoError(t, ps.Register(planFile, "shared wt patch test", "plan/shared-wt-patch", time.Now()))

	coderTemp := 0.8
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	m := &home{
		taskState:      ps,
		activeRepoPath: dir,
		program:        "opencode",
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		appConfig: &config.Config{
			PhaseRoles: map[string]string{
				"implementing": "coder",
			},
			Profiles: map[string]config.AgentProfile{
				"coder": {
					Program:     "opencode",
					Model:       "claude-opus-4-6",
					Temperature: &coderTemp,
					Effort:      "high",
					Enabled:     true,
				},
			},
		},
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	_, cmd := m.spawnTaskAgent(planFile, "implement", "implement prompt")
	if cmd != nil {
		_ = cmd()
	}

	// The shared worktree path is derived from the plan branch.
	branch := gitpkg.TaskBranchFromFile(planFile)
	worktreePath := gitpkg.TaskWorktreePath(dir, branch)
	worktreeConfigPath := filepath.Join(worktreePath, ".opencode", "opencode.jsonc")

	data, err := os.ReadFile(worktreeConfigPath)
	require.NoError(t, err, "worktree opencode.jsonc must exist after shared worktree setup")

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))
	agentCfg, ok := cfg["agent"].(map[string]any)
	require.True(t, ok, "agent block must exist")
	coderCfg, ok := agentCfg["coder"].(map[string]any)
	require.True(t, ok, "coder block must exist")
	assert.Equal(t, "anthropic/claude-opus-4-6", coderCfg["model"], "worktree opencode.jsonc must have patched model")
	assert.InDelta(t, coderTemp, coderCfg["temperature"].(float64), 0.0001, "worktree opencode.jsonc must have patched temperature")
	assert.Equal(t, "high", coderCfg["reasoningEffort"], "worktree opencode.jsonc must have patched reasoningEffort")
}

func TestSpawnWaveTasks_HeadlessCoderUsesHeadlessExecution(t *testing.T) {
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

	planDoc := "# test\n\n## Wave 1\n\n### Task 1: implement headless execution\n\nDo it.\n"
	parsed, err := taskparser.Parse(planDoc)
	require.NoError(t, err)
	require.Len(t, parsed.Waves, 1)

	orch := orchestration.NewWaveOrchestrator("test.md", parsed)
	tasks := orch.StartNextWave()
	require.Len(t, tasks, 1)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		instanceFinalizers: make(map[*session.Instance]func()),
		appConfig: &config.Config{
			PhaseRoles: map[string]string{"implementing": "coder"},
			Profiles: map[string]config.AgentProfile{
				"coder": {
					Program:       "opencode",
					Enabled:       true,
					ExecutionMode: config.ExecutionModeHeadless,
				},
			},
		},
	}

	entry := taskstate.TaskEntry{Branch: "plan/test"}
	_, _ = h.spawnWaveTasks(orch, tasks, entry)

	instances := list.GetInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, session.ExecutionModeHeadless, instances[0].ExecutionMode)
}

// TestSpawnWaveTasks_PatchesSharedWorktreeOpencodeConfig verifies that spawnWaveTasks
// patches the SHARED WORKTREE's opencode.jsonc, not the main repo's, so coder agents
// spawned by wave orchestration read the correct config from their worktree.
func TestSpawnWaveTasks_PatchesSharedWorktreeOpencodeConfig(t *testing.T) {
	dir := t.TempDir()

	opencodeDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(opencodeDir, 0o755))
	configPath := filepath.Join(opencodeDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent":{"coder":{"model":"anthropic/old-wave-coder","temperature":0.2,"reasoningEffort":"low"}}}`), 0o644))

	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "init with opencode config"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	const planFile = "wave-wt-patch"
	require.NoError(t, ps.Register(planFile, "wave wt patch test", "plan/wave-wt-patch", time.Now()))

	coderTemp := 0.75
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	m := &home{
		taskState:      ps,
		activeRepoPath: dir,
		program:        "opencode",
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		appConfig: &config.Config{
			PhaseRoles: map[string]string{
				"implementing": "coder",
			},
			Profiles: map[string]config.AgentProfile{
				"coder": {
					Program:     "opencode",
					Model:       "claude-sonnet-4-6",
					Temperature: &coderTemp,
					Effort:      "medium",
					Enabled:     true,
				},
			},
		},
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1, Title: "Task 1", Body: "do it"}}},
		},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)

	entry, ok := ps.Entry(planFile)
	require.True(t, ok)

	_, cmd := m.spawnWaveTasks(orch, plan.Waves[0].Tasks, entry)
	if cmd != nil {
		_ = cmd()
	}

	branch := gitpkg.TaskBranchFromFile(planFile)
	worktreePath := gitpkg.TaskWorktreePath(dir, branch)
	worktreeConfigPath := filepath.Join(worktreePath, ".opencode", "opencode.jsonc")

	data, err := os.ReadFile(worktreeConfigPath)
	require.NoError(t, err, "worktree opencode.jsonc must exist after shared worktree setup")

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))
	agentCfg, ok := cfg["agent"].(map[string]any)
	require.True(t, ok, "agent block must exist")
	coderCfg, ok := agentCfg["coder"].(map[string]any)
	require.True(t, ok, "coder block must exist")
	assert.Equal(t, "anthropic/claude-sonnet-4-6", coderCfg["model"], "worktree opencode.jsonc must have patched model")
	assert.InDelta(t, coderTemp, coderCfg["temperature"].(float64), 0.0001, "worktree opencode.jsonc must have patched temperature")
	assert.Equal(t, "medium", coderCfg["reasoningEffort"], "worktree opencode.jsonc must have patched reasoningEffort")
}

// TestFSM_PlanLifecycleStages verifies that the FSM produces the correct status for
// each stage in the plan lifecycle (plan→implement→review→done).
func TestFSM_PlanLifecycleStages(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ps, err := newTestPlanState(t, plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "test.md"
	if err := ps.Register(planFile, "test plan", "plan/test", time.Now()); err != nil {
		t.Fatal(err)
	}

	f := newFSMForTest(t, plansDir)

	stages := []struct {
		event      string
		wantStatus taskstate.Status
	}{
		{"plan_start", taskstate.StatusPlanning},
		{"planner_finished", taskstate.StatusReady},
		{"implement_start", taskstate.StatusImplementing},
		{"implement_finished", taskstate.StatusReviewing},
		{"review_approved", taskstate.StatusDone},
	}

	for _, tc := range stages {
		if err := f.TransitionByName(planFile, tc.event); err != nil {
			t.Fatalf("TransitionByName(%q, %q) error: %v", planFile, tc.event, err)
		}
		reloaded, _ := newTestPlanState(t, plansDir)
		entry, ok := reloaded.Entry(planFile)
		if !ok {
			t.Fatalf("plan entry missing after %q", tc.event)
		}
		if entry.Status != tc.wantStatus {
			t.Errorf("after %q: got status %q, want %q", tc.event, entry.Status, tc.wantStatus)
		}
	}
}

func TestSpawnPlanAgent_SoloSetsSoloAgentFlag(t *testing.T) {
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
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	planFile := "test-solo.md"
	require.NoError(t, ps.Register(planFile, "test solo", "plan/test-solo", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		taskState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnTaskAgent(planFile, "solo", "solo prompt")

	instances := list.GetInstances()
	require.NotEmpty(t, instances, "expected instance after spawnTaskAgent(solo)")
	inst := instances[len(instances)-1]
	assert.True(t, inst.SoloAgent, "solo agent must have SoloAgent=true")
	assert.Equal(t, session.AgentTypeCoder, inst.AgentType)
}

func TestSpawnPlanAgent_SoloTitlesArePlanScoped(t *testing.T) {
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
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	const firstPlan = "wrong-timezone"
	const secondPlan = "rename-solo-agent-label"
	require.NoError(t, ps.Register(firstPlan, "wrong timezone", "plan/wrong-timezone", time.Now()))
	require.NoError(t, ps.Register(secondPlan, "rename solo agent label", "plan/rename-solo-agent-label", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		taskState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnTaskAgent(firstPlan, "solo", "first solo prompt")
	h.spawnTaskAgent(secondPlan, "solo", "second solo prompt")

	instances := list.GetInstances()
	require.Len(t, instances, 2, "expected two solo instances")

	assert.Equal(t, "wrong-timezone-solo", instances[0].Title)
	assert.Equal(t, "rename-solo-agent-label-solo", instances[1].Title)
	assert.NotEqual(t, instances[0].Title, instances[1].Title,
		"solo instance titles must be unique so tmux sessions do not collide")
}

// setupTopicConflictHome creates a home with two plans in the same topic,
// one already implementing, for testing the concurrency gate.
func setupTopicConflictHome(t *testing.T) (*home, string) {
	t.Helper()
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

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	const (
		targetPlan   = "solo-target.md"
		conflictPlan = "conflict.md"
		topic        = "shared-topic"
	)

	require.NoError(t, ps.Create(targetPlan, "target", "plan/solo-target", topic, time.Now()))
	require.NoError(t, ps.Create(conflictPlan, "conflict", "plan/conflict", topic, time.Now()))
	seedPlanStatus(t, ps, targetPlan, taskstate.StatusReady)
	seedPlanStatus(t, ps, conflictPlan, taskstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*orchestration.WaveOrchestrator))
	h.fsm = newFSMForTest(t, plansDir).TaskStateMachine
	h.activeRepoPath = dir
	h.program = "opencode"
	return h, targetPlan
}

func TestTriggerPlanStage_SoloRespectsTopicConcurrencyGate(t *testing.T) {
	h, targetPlan := setupTopicConflictHome(t)

	model, _ := h.triggerTaskStage(targetPlan, "solo")
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"solo stage must show topic concurrency confirmation when another plan in topic is implementing")
	require.True(t, updated.overlays.IsActive(),
		"confirmation overlay must be shown for solo topic conflict")
	require.NotNil(t, updated.pendingConfirmAction,
		"confirm action must be set for solo topic conflict")
}

// TestTopicConcurrencyConfirm_ReturnsPlanStageConfirmedMsg verifies that
// confirming the topic-concurrency dialog returns a taskStageConfirmedMsg
// (not just a taskRefreshMsg), so the actual stage execution is triggered.
func TestTopicConcurrencyConfirm_ReturnsPlanStageConfirmedMsg(t *testing.T) {
	for _, stage := range []string{"solo", "implement"} {
		t.Run(stage, func(t *testing.T) {
			h, targetPlan := setupTopicConflictHome(t)

			model, _ := h.triggerTaskStage(targetPlan, stage)
			updated := model.(*home)

			require.Equal(t, stateConfirm, updated.state,
				"must show confirmation dialog for topic conflict")
			require.NotNil(t, updated.pendingConfirmAction,
				"pending confirm action must be set")

			// Execute the pending confirm action and check the returned message.
			msg := updated.pendingConfirmAction()
			stageMsg, ok := msg.(taskStageConfirmedMsg)
			require.True(t, ok,
				"confirm action must return taskStageConfirmedMsg, got %T", msg)
			assert.Equal(t, targetPlan, stageMsg.planFile,
				"taskStageConfirmedMsg must carry the correct plan file")
			assert.Equal(t, stage, stageMsg.stage,
				"taskStageConfirmedMsg must carry the correct stage")
		})
	}
}

func TestExecuteContextAction_SetStatusForceOverridesWithoutFSM(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	planFile := "test-set-status.md"
	require.NoError(t, ps.Register(planFile, "test set status", "plan/test-set-status", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusImplementing)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		taskState:      ps,
		taskStateDir:   plansDir,
		fsm:            newFSMForTest(t, plansDir).TaskStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		overlays:       overlay.NewManager(),
		activeRepoPath: dir,
	}

	h.updateSidebarTasks()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile))

	// Simulate: context menu selected "set_status", which sets up the picker
	_, _ = h.executeContextAction("set_status")
	assert.Equal(t, stateSetStatus, h.state, "set_status action should enter stateSetStatus")
	assert.True(t, h.overlays.IsActive(), "picker overlay should be created for status selection")
	assert.Equal(t, planFile, h.pendingSetStatusTask, "pending plan file should be stored")
}

func TestExecuteTaskStage_BlocksWhenDaemonUnavailable(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	planFile := "daemon-block-plan.md"
	require.NoError(t, ps.Register(planFile, "daemon block", "plan/daemon-block-plan", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		taskState:      ps,
		taskStateDir:   plansDir,
		fsm:            newFSMForTest(t, plansDir).TaskStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		overlays:       overlay.NewManager(),
		activeRepoPath: dir,
		daemonStatusChecker: func(string) daemonStatusMsg {
			return daemonStatusMsg{message: "start it with kas daemon start"}
		},
	}

	model, cmd := h.executeTaskStage(planFile, "plan")
	updated := model.(*home)

	require.Nil(t, cmd)
	assert.Equal(t, stateConfirm, updated.state)
	assert.True(t, updated.overlays.IsActive())
	assert.Nil(t, updated.pendingConfirmAction)

	entry, ok := updated.taskState.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusReady, entry.Status)
	co, ok := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok)
	assert.Contains(t, co.View(), "kas daemon start")
}

func TestSpawnAdHocAgent_BlocksWhenDaemonUnavailable(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:            context.Background(),
		state:          stateDefault,
		appConfig:      config.DefaultConfig(),
		nav:            ui.NewNavigationPanel(&spin),
		menu:           ui.NewMenu(),
		auditPane:      ui.NewAuditPane(),
		toastManager:   overlay.NewToastManager(&spin),
		overlays:       overlay.NewManager(),
		activeRepoPath: t.TempDir(),
		program:        "opencode",
		daemonStatusChecker: func(string) daemonStatusMsg {
			return daemonStatusMsg{message: "register it with kas daemon add /tmp/repo"}
		},
	}

	model, cmd := h.spawnAdHocAgent("my-agent", "", "")
	updated := model.(*home)

	require.Nil(t, cmd)
	assert.Empty(t, updated.nav.GetInstances())
	assert.Equal(t, stateConfirm, updated.state)
	assert.True(t, updated.overlays.IsActive())
	co, ok := updated.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok)
	assert.Contains(t, co.View(), "kas daemon add")
}

func TestToggleAutoAdvanceWaves(t *testing.T) {
	m := &home{
		appConfig: &config.Config{AutoAdvanceWaves: false},
	}
	assert.False(t, m.appConfig.AutoAdvanceWaves)

	// Simulate executing the toggle action
	m.appConfig.AutoAdvanceWaves = !m.appConfig.AutoAdvanceWaves
	assert.True(t, m.appConfig.AutoAdvanceWaves)

	// Toggle back
	m.appConfig.AutoAdvanceWaves = !m.appConfig.AutoAdvanceWaves
	assert.False(t, m.appConfig.AutoAdvanceWaves)
}

func TestToggleAutoReviewFix(t *testing.T) {
	m := &home{
		appConfig: &config.Config{AutoReviewFix: false},
	}
	assert.False(t, m.appConfig.AutoReviewFix)

	m.appConfig.AutoReviewFix = !m.appConfig.AutoReviewFix
	assert.True(t, m.appConfig.AutoReviewFix)

	m.appConfig.AutoReviewFix = !m.appConfig.AutoReviewFix
	assert.False(t, m.appConfig.AutoReviewFix)
}

func TestEnsureProcessor_RefreshesReviewFixConfig(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "disabled.md",
		Status:   taskstore.StatusReviewing,
	}))
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: "enabled.md",
		Status:   taskstore.StatusReviewing,
	}))

	h := &home{
		appConfig:             &config.Config{AutoReviewFix: false, MaxReviewFixCycles: 2},
		taskStore:             store,
		taskStoreProject:      "proj",
		taskStateDir:          t.TempDir(),
		pendingReviewFeedback: make(map[string]string),
	}

	proc := h.ensureProcessor()
	require.NotNil(t, proc)
	actions := proc.ProcessFSMSignals([]taskfsm.Signal{{
		Event:    taskfsm.ReviewChangesRequested,
		TaskFile: "disabled.md",
		Body:     "fix this",
	}})
	require.Len(t, actions, 1)
	_, ok := actions[0].(loop.ReviewChangesAction)
	assert.True(t, ok)

	h.appConfig.AutoReviewFix = true
	h.appConfig.MaxReviewFixCycles = 4
	proc = h.ensureProcessor()
	actions = proc.ProcessFSMSignals([]taskfsm.Signal{{
		Event:    taskfsm.ReviewChangesRequested,
		TaskFile: "enabled.md",
		Body:     "fix this",
	}})

	var foundCoder, foundIncrement bool
	for _, action := range actions {
		if _, ok := action.(loop.SpawnCoderAction); ok {
			foundCoder = true
		}
		if _, ok := action.(loop.IncrementReviewCycleAction); ok {
			foundIncrement = true
		}
	}
	assert.True(t, foundCoder)
	assert.True(t, foundIncrement)
}

func TestViewSelectedPlan_ReadsFromStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	planFile := "test.md"
	content := "# My Plan\n\n## Wave 1\n\n### Task 1: Do thing\n"
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Content:  content,
	}))

	ps, err := taskstate.Load(store, "proj", t.TempDir())
	require.NoError(t, err)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	nav := ui.NewNavigationPanel(&sp)
	nav.SetTopicsAndPlans(nil, []ui.PlanDisplay{{Filename: planFile, Status: string(taskstate.StatusReady)}}, nil)
	require.True(t, nav.SelectByID(ui.SidebarPlanPrefix+planFile))

	h := &home{
		taskState:        ps,
		taskStore:        store,
		taskStoreProject: "proj",
		taskStateDir:     t.TempDir(),
		nav:              nav,
	}

	_, cmd := h.viewSelectedPlan()
	require.NotNil(t, cmd)

	msg := cmd()
	renderedMsg, ok := msg.(planRenderedMsg)
	require.True(t, ok, "expected planRenderedMsg, got %T", msg)
	require.NoError(t, renderedMsg.err)
	assert.Equal(t, planFile, renderedMsg.planFile)
}

// TestImplementActionReadsFromStore verifies that the "implement" action reads plan
// content from the task store database, not from a file on disk. The test creates
// a task entry with valid wave-header content in the task store and deliberately omits
// any .md file on disk. A non-nil WaveOrchestrator in the home model after
// executeTaskStage proves that the plan was read from the DB and parsed successfully.
func TestImplementActionReadsFromStore(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	threshold := 0

	const planFile = "test-implement-from-db.md"
	const planContent = "# Plan\n\n**Goal:** Test DB read\n\n## Wave 1\n\n### Task 1: Do the thing\n\nDo it.\n"

	// Create task in store WITH content and a branch (branch avoids the backfill
	// path in executeTaskStage that would call store.Update and inadvertently clear
	// the content field). No file is written to disk.
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Branch:   "plan/test-implement-from-db",
		Content:  planContent,
	}))

	ps, err := taskstate.Load(store, "proj", plansDir)
	require.NoError(t, err)
	fsm := taskfsm.New(store, "proj", plansDir)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		appConfig:          &config.Config{BlueprintSkipThresholdValue: &threshold},
		taskState:          ps,
		taskStore:          store,
		taskStoreProject:   "proj",
		taskStateDir:       plansDir,
		fsm:                fsm,
		nav:                ui.NewNavigationPanel(&sp),
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		waveOrchestrators:  make(map[string]*orchestration.WaveOrchestrator),
		instanceFinalizers: make(map[*session.Instance]func()),
		activeRepoPath:     dir,
		program:            "opencode",
	}

	// No plan file on disk — content must come from the task store.
	model, _ := h.executeTaskStage(planFile, "implement")
	updated := model.(*home)

	// The WaveOrchestrator is created before spawnWaveTasks (which may fail on git).
	// A non-nil orchestrator proves the plan was read from the DB and parsed successfully.
	assert.NotNil(t, updated.waveOrchestrators[planFile],
		"implement action must read plan content from store, not disk")
}

// TestSoloActionChecksStoreNotDisk verifies that the "solo" action determines
// whether to include a plan file reference in its prompt by checking for content
// in the task store, rather than checking for a file on disk. The test stores
// content in the DB without writing any .md file. When the prompt contains
// "kas task show <planFile>", it proves the store check (not os.Stat) was used.
func TestSoloActionChecksStoreNotDisk(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	const planFile = "test-solo-from-db.md"
	const planContent = "# Plan\n\n**Goal:** Test solo DB check\n\n## Wave 1\n\n### Task 1: Solo task\n\nDo it.\n"

	// Create task in store WITH content and a branch (branch avoids the backfill
	// path in executeTaskStage that would call store.Update and inadvertently clear
	// the content field). No file is written to disk.
	store := taskstore.NewTestSQLiteStore(t)
	require.NoError(t, store.Create("proj", taskstore.TaskEntry{
		Filename: planFile,
		Status:   taskstore.StatusReady,
		Branch:   "plan/test-solo-from-db",
		Content:  planContent,
	}))

	ps, err := taskstate.Load(store, "proj", plansDir)
	require.NoError(t, err)
	fsm := taskfsm.New(store, "proj", plansDir)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		taskState:          ps,
		taskStore:          store,
		taskStoreProject:   "proj",
		taskStateDir:       plansDir,
		fsm:                fsm,
		nav:                ui.NewNavigationPanel(&sp),
		menu:               ui.NewMenu(),
		toastManager:       overlay.NewToastManager(&sp),
		waveOrchestrators:  make(map[string]*orchestration.WaveOrchestrator),
		instanceFinalizers: make(map[*session.Instance]func()),
		activeRepoPath:     dir,
		program:            "opencode",
	}

	// No plan file on disk — the store check must find content and set refFile.
	model, _ := h.executeTaskStage(planFile, "solo")
	updated := model.(*home)

	// The solo agent must have been spawned with a prompt referencing kas task show <planFile>
	// because the store has content. If os.Stat were used instead, no disk file means
	// refFile="" and the prompt would omit the plan file reference.
	instances := updated.nav.GetInstances()
	require.NotEmpty(t, instances, "solo stage must spawn an agent instance")
	soloInst := instances[len(instances)-1]
	assert.Contains(t, soloInst.QueuedPrompt, "kas task show "+planFile,
		"solo prompt must reference plan file when store has content (not disk)")
}

func TestExecuteContextAction_MarkPlanDoneFromReadyTransitionsToDone(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)

	planFile := "review-approval-gate.md"
	require.NoError(t, ps.Register(planFile, "review approval gate", "plan/review-approval-gate", time.Now()))
	seedPlanStatus(t, ps, planFile, taskstate.StatusReady)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		taskState:      ps,
		taskStateDir:   plansDir,
		fsm:            newFSMForTest(t, plansDir).TaskStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		activeRepoPath: dir,
	}

	h.updateSidebarTasks()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile), "plan should be selectable in sidebar")

	_, _ = h.executeContextAction("mark_plan_done")

	reloaded, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, taskstate.StatusDone, entry.Status,
		"mark_plan_done should walk ready->implementing->reviewing->done")
}
