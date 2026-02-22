package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/ui"
)

func TestBuildPlanPrompt(t *testing.T) {
	prompt := buildPlanPrompt("Auth Refactor", "Refactor JWT auth")
	if !strings.Contains(prompt, "Plan Auth Refactor") {
		t.Fatalf("prompt missing title")
	}
	if !strings.Contains(prompt, "Goal: Refactor JWT auth") {
		t.Fatalf("prompt missing goal")
	}
}

func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("2026-02-21-auth-refactor.md")
	if !strings.Contains(prompt, "Implement docs/plans/2026-02-21-auth-refactor.md") {
		t.Fatalf("prompt missing plan path")
	}
}

func TestAgentTypeForSubItem(t *testing.T) {
	tests := map[string]string{
		"plan":      session.AgentTypePlanner,
		"implement": session.AgentTypeCoder,
		"review":    session.AgentTypeReviewer,
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
	list := ui.NewList(&sp, false)
	h := &home{
		planState:      ps,
		activeRepoPath: dir,
		program:        "opencode",
		list:           list,
		menu:           ui.NewMenu(),
		sidebar:        ui.NewSidebar(),
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

// TestPlanStageStatus_UsesCorrectLifecycleStatuses verifies that planStageStatus
// writes the new lifecycle-stage status constants (StatusPlanning, StatusImplementing)
// rather than the generic StatusInProgress for plan and implement stages.
func TestPlanStageStatus_UsesCorrectLifecycleStatuses(t *testing.T) {
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

	tests := []struct {
		stage      string
		wantStatus planstate.Status
	}{
		{"plan", planstate.StatusPlanning},
		{"implement", planstate.StatusImplementing},
		{"review", planstate.StatusReviewing},
		{"finished", planstate.StatusCompleted},
	}

	for _, tc := range tests {
		if err := planStageStatus(planFile, tc.stage, ps); err != nil {
			t.Fatalf("planStageStatus(%q) error: %v", tc.stage, err)
		}
		entry, ok := ps.Entry(planFile)
		if !ok {
			t.Fatalf("plan entry missing after planStageStatus(%q)", tc.stage)
		}
		if entry.Status != tc.wantStatus {
			t.Errorf("planStageStatus(%q): got status %q, want %q", tc.stage, entry.Status, tc.wantStatus)
		}
	}
}
