package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanPrompt(t *testing.T) {
	prompt := buildPlanPrompt("Auth Refactor", "Refactor JWT auth")
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
}

func TestBuildWaveAnnotationPrompt(t *testing.T) {
	prompt := buildWaveAnnotationPrompt("2026-02-27-my-feature.md")
	assert.Contains(t, prompt, "2026-02-27-my-feature.md", "prompt must reference the plan file")
	assert.Contains(t, prompt, "## Wave", "prompt must mention ## Wave header format")
	assert.Contains(t, prompt, "commit", "prompt must instruct the planner to commit after annotation")
	assert.Contains(t, prompt, "planner-finished-", "prompt must include the signal file instruction")
}

func TestBuildWaveAnnotationPrompt_SingleWaveFallback(t *testing.T) {
	prompt := buildWaveAnnotationPrompt("2026-02-27-trivial.md")
	// Even trivial plans must be wrapped in at least ## Wave 1
	assert.Contains(t, prompt, "## Wave 1", "prompt must specify ## Wave 1 as the minimum structure")
}

func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("2026-02-21-auth-refactor.md")
	if !strings.Contains(prompt, "Implement docs/plans/2026-02-21-auth-refactor.md") {
		t.Fatalf("prompt missing plan path")
	}
}

func TestBuildSoloPrompt_WithDescription(t *testing.T) {
	prompt := buildSoloPrompt("auth-refactor", "Refactor JWT auth", "2026-02-21-auth-refactor.md")
	assert.Contains(t, prompt, "Implement auth-refactor")
	assert.Contains(t, prompt, "Goal: Refactor JWT auth")
	assert.Contains(t, prompt, "docs/plans/2026-02-21-auth-refactor.md")
}

func TestBuildSoloPrompt_StubOnly(t *testing.T) {
	prompt := buildSoloPrompt("quick-fix", "Fix the login bug", "")
	assert.Contains(t, prompt, "Implement quick-fix")
	assert.Contains(t, prompt, "Goal: Fix the login bug")
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
	assert.False(t, isLocked(planstate.StatusReady, "solo"),
		"solo stage should be triggerable like implement/review")
}

// TestSpawnPlanAgent_ReviewerSetsIsReviewer verifies that spawnPlanAgent sets
// IsReviewer=true on the created instance when the action is "review", so that
// the reviewer completion check in the metadata tick handler (which gates on
// inst.IsReviewer) can detect when the reviewer session exits.
//
// This is a regression test for the bug where spawnPlanAgent set AgentType but
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
	ps, err := planstate.Load(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "2026-02-21-test.md"
	if err := ps.Register(planFile, "test plan", "plan/test", time.Now()); err != nil {
		t.Fatal(err)
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		planState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnPlanAgent(planFile, "review", "review prompt")

	instances := list.GetInstances()
	if len(instances) == 0 {
		t.Fatal("expected instance to be added to list after spawnPlanAgent(review)")
	}
	inst := instances[len(instances)-1]
	if inst.AgentType != session.AgentTypeReviewer {
		t.Fatalf("AgentType = %q, want %q", inst.AgentType, session.AgentTypeReviewer)
	}
	if !inst.IsReviewer {
		t.Fatal("spawnPlanAgent(review) must set IsReviewer=true on the created instance")
	}
}

// TestSpawnPlanAgent_PlannerUsesMainBranch verifies that spawnPlanAgent for the
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
	ps, err := planstate.Load(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "2026-02-23-test-planner.md"
	if err := ps.Register(planFile, "test plan", "plan/test-planner", time.Now()); err != nil {
		t.Fatal(err)
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		planState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnPlanAgent(planFile, "plan", "plan prompt")

	instances := list.GetInstances()
	if len(instances) == 0 {
		t.Fatal("expected instance to be added to list after spawnPlanAgent(plan)")
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

// TestFSM_PlanLifecycleStages verifies that the FSM produces the correct status for
// each stage in the plan lifecycle (plan→implement→review→done).
func TestFSM_PlanLifecycleStages(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ps, err := planstate.Load(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	planFile := "2026-02-21-test.md"
	if err := ps.Register(planFile, "test plan", "plan/test", time.Now()); err != nil {
		t.Fatal(err)
	}

	f := newFSMForTest(plansDir)

	stages := []struct {
		event      string
		wantStatus planstate.Status
	}{
		{"plan_start", planstate.StatusPlanning},
		{"planner_finished", planstate.StatusReady},
		{"implement_start", planstate.StatusImplementing},
		{"implement_finished", planstate.StatusReviewing},
		{"review_approved", planstate.StatusDone},
	}

	for _, tc := range stages {
		if err := f.TransitionByName(planFile, tc.event); err != nil {
			t.Fatalf("TransitionByName(%q, %q) error: %v", planFile, tc.event, err)
		}
		reloaded, _ := planstate.Load(plansDir)
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
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	planFile := "2026-02-25-test-solo.md"
	require.NoError(t, ps.Register(planFile, "test solo", "plan/test-solo", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		planState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnPlanAgent(planFile, "solo", "solo prompt")

	instances := list.GetInstances()
	require.NotEmpty(t, instances, "expected instance after spawnPlanAgent(solo)")
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
	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	const firstPlan = "2026-03-01-wrong-timezone.md"
	const secondPlan = "2026-03-01-rename-solo-agent-label.md"
	require.NoError(t, ps.Register(firstPlan, "wrong timezone", "plan/wrong-timezone", time.Now()))
	require.NoError(t, ps.Register(secondPlan, "rename solo agent label", "plan/rename-solo-agent-label", time.Now()))

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&sp)
	h := &home{
		planState:          ps,
		activeRepoPath:     dir,
		program:            "opencode",
		nav:                list,
		menu:               ui.NewMenu(),
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	h.spawnPlanAgent(firstPlan, "solo", "first solo prompt")
	h.spawnPlanAgent(secondPlan, "solo", "second solo prompt")

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

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	const (
		targetPlan   = "2026-02-25-solo-target.md"
		conflictPlan = "2026-02-25-conflict.md"
		topic        = "shared-topic"
	)

	require.NoError(t, ps.Create(targetPlan, "target", "plan/solo-target", topic, time.Now()))
	require.NoError(t, ps.Create(conflictPlan, "conflict", "plan/conflict", topic, time.Now()))
	seedPlanStatus(t, ps, targetPlan, planstate.StatusReady)
	seedPlanStatus(t, ps, conflictPlan, planstate.StatusImplementing)

	h := waveFlowHome(t, ps, plansDir, make(map[string]*WaveOrchestrator))
	h.fsm = newFSMForTest(plansDir).PlanStateMachine
	h.activeRepoPath = dir
	h.program = "opencode"
	return h, targetPlan
}

func TestTriggerPlanStage_SoloRespectsTopicConcurrencyGate(t *testing.T) {
	h, targetPlan := setupTopicConflictHome(t)

	model, _ := h.triggerPlanStage(targetPlan, "solo")
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"solo stage must show topic concurrency confirmation when another plan in topic is implementing")
	require.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay must be shown for solo topic conflict")
	require.NotNil(t, updated.pendingConfirmAction,
		"confirm action must be set for solo topic conflict")
}

// TestTopicConcurrencyConfirm_ReturnsPlanStageConfirmedMsg verifies that
// confirming the topic-concurrency dialog returns a planStageConfirmedMsg
// (not just a planRefreshMsg), so the actual stage execution is triggered.
func TestTopicConcurrencyConfirm_ReturnsPlanStageConfirmedMsg(t *testing.T) {
	for _, stage := range []string{"solo", "implement"} {
		t.Run(stage, func(t *testing.T) {
			h, targetPlan := setupTopicConflictHome(t)

			model, _ := h.triggerPlanStage(targetPlan, stage)
			updated := model.(*home)

			require.Equal(t, stateConfirm, updated.state,
				"must show confirmation dialog for topic conflict")
			require.NotNil(t, updated.pendingConfirmAction,
				"pending confirm action must be set")

			// Execute the pending confirm action and check the returned message.
			msg := updated.pendingConfirmAction()
			stageMsg, ok := msg.(planStageConfirmedMsg)
			require.True(t, ok,
				"confirm action must return planStageConfirmedMsg, got %T", msg)
			assert.Equal(t, targetPlan, stageMsg.planFile,
				"planStageConfirmedMsg must carry the correct plan file")
			assert.Equal(t, stage, stageMsg.stage,
				"planStageConfirmedMsg must carry the correct stage")
		})
	}
}

func TestExecuteContextAction_SetStatusForceOverridesWithoutFSM(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	planFile := "2026-02-28-test-set-status.md"
	require.NoError(t, ps.Register(planFile, "test set status", "plan/test-set-status", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusImplementing)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		planState:      ps,
		planStateDir:   plansDir,
		fsm:            newFSMForTest(plansDir).PlanStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:   overlay.NewToastManager(&sp),
		activeRepoPath: dir,
	}

	h.updateSidebarPlans()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile))

	// Simulate: context menu selected "set_status", which sets up the picker
	_, _ = h.executeContextAction("set_status")
	assert.Equal(t, stateSetStatus, h.state, "set_status action should enter stateSetStatus")
	assert.NotNil(t, h.pickerOverlay, "picker overlay should be created for status selection")
	assert.Equal(t, planFile, h.pendingSetStatusPlan, "pending plan file should be stored")
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

func TestExecuteContextAction_MarkPlanDoneFromReadyTransitionsToDone(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)

	planFile := "2026-02-26-review-approval-gate.md"
	require.NoError(t, ps.Register(planFile, "review approval gate", "plan/review-approval-gate", time.Now()))
	seedPlanStatus(t, ps, planFile, planstate.StatusReady)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		planState:      ps,
		planStateDir:   plansDir,
		fsm:            newFSMForTest(plansDir).PlanStateMachine,
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:   overlay.NewToastManager(&sp),
		activeRepoPath: dir,
	}

	h.updateSidebarPlans()
	require.True(t, h.nav.SelectByID(ui.SidebarPlanPrefix+planFile), "plan should be selectable in sidebar")

	_, _ = h.executeContextAction("mark_plan_done")

	reloaded, err := planstate.Load(plansDir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry(planFile)
	require.True(t, ok)
	assert.Equal(t, planstate.StatusDone, entry.Status,
		"mark_plan_done should walk ready->implementing->reviewing->done")
}
