package session

import (
	"testing"
)

func TestNewInstance_SetsPlanFile(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:    "plan-worker",
		Path:     ".",
		Program:  "claude",
		PlanFile: "2026-02-20-plan-orchestration.md",
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	if inst.PlanFile != "2026-02-20-plan-orchestration.md" {
		t.Fatalf("PlanFile = %q, want %q", inst.PlanFile, "2026-02-20-plan-orchestration.md")
	}
}

func TestInstanceData_RoundTripPlanFile(t *testing.T) {
	data := InstanceData{
		Title:    "persisted",
		Path:     "/tmp/repo",
		Branch:   "feature/test",
		Status:   Paused,
		Program:  "claude",
		PlanFile: "plan.md",
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/repo/.worktrees/persisted",
			SessionName:   "persisted",
			BranchName:    "feature/test",
			BaseCommitSHA: "abc123",
		},
	}

	inst, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if inst.PlanFile != "plan.md" {
		t.Fatalf("instance PlanFile = %q, want %q", inst.PlanFile, "plan.md")
	}

	roundTrip := inst.ToInstanceData()
	if roundTrip.PlanFile != "plan.md" {
		t.Fatalf("ToInstanceData PlanFile = %q, want %q", roundTrip.PlanFile, "plan.md")
	}
}

func TestInstanceData_RoundTripImplementationComplete(t *testing.T) {
	data := InstanceData{
		Title:                  "coder-done",
		Path:                   "/tmp/repo",
		Branch:                 "feature/impl",
		Status:                 Paused,
		Program:                "opencode",
		PlanFile:               "plan.md",
		ImplementationComplete: true,
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/repo/.worktrees/coder-done",
			SessionName:   "coder-done",
			BranchName:    "feature/impl",
			BaseCommitSHA: "def456",
		},
	}

	inst, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if !inst.ImplementationComplete {
		t.Fatal("expected ImplementationComplete = true after FromInstanceData")
	}

	roundTrip := inst.ToInstanceData()
	if !roundTrip.ImplementationComplete {
		t.Fatal("expected ImplementationComplete = true after ToInstanceData round-trip")
	}
}

func TestNewInstance_SetsAgentType(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:     "planner-worker",
		Path:      ".",
		Program:   "opencode",
		PlanFile:  "2026-02-21-auth-refactor.md",
		AgentType: AgentTypePlanner,
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	if inst.AgentType != AgentTypePlanner {
		t.Fatalf("AgentType = %q, want %q", inst.AgentType, AgentTypePlanner)
	}
}

func TestInstanceData_RoundTripAgentType(t *testing.T) {
	data := InstanceData{
		Title:     "persisted",
		Path:      "/tmp/repo",
		Branch:    "plan/auth-refactor",
		Status:    Paused,
		Program:   "opencode",
		PlanFile:  "2026-02-21-auth-refactor.md",
		AgentType: AgentTypeReviewer,
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/repo/.worktrees/plan-auth-refactor",
			SessionName:   "persisted",
			BranchName:    "plan/auth-refactor",
			BaseCommitSHA: "abc123",
		},
	}

	inst, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if inst.AgentType != AgentTypeReviewer {
		t.Fatalf("instance AgentType = %q, want %q", inst.AgentType, AgentTypeReviewer)
	}

	roundTrip := inst.ToInstanceData()
	if roundTrip.AgentType != AgentTypeReviewer {
		t.Fatalf("ToInstanceData AgentType = %q, want %q", roundTrip.AgentType, AgentTypeReviewer)
	}
}

func TestInstanceData_ImplementationCompleteFalseByDefault(t *testing.T) {
	data := InstanceData{
		Title:   "normal-session",
		Path:    "/tmp/repo",
		Branch:  "feature/x",
		Status:  Paused,
		Program: "claude",
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/repo/.worktrees/normal-session",
			SessionName:   "normal-session",
			BranchName:    "feature/x",
			BaseCommitSHA: "aaa111",
		},
	}

	inst, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if inst.ImplementationComplete {
		t.Fatal("expected ImplementationComplete = false for a normal instance")
	}
}
