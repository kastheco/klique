package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentTypeElaborator_Constant(t *testing.T) {
	// AgentTypeElaborator was renamed from "elaborator" to "architect" to match
	// the opencode config block name after the elaborator→architect role rename.
	assert.Equal(t, "architect", AgentTypeElaborator)
}

func TestNewInstance_SetsPlanFile(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:    "plan-worker",
		Path:     ".",
		Program:  "claude",
		TaskFile: "plan-orchestration",
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	if inst.TaskFile != "plan-orchestration" {
		t.Fatalf("PlanFile = %q, want %q", inst.TaskFile, "plan-orchestration")
	}
}

func TestInstanceData_RoundTripPlanFile(t *testing.T) {
	data := InstanceData{
		Title:    "persisted",
		Path:     "/tmp/repo",
		Branch:   "feature/test",
		Status:   Paused,
		Program:  "claude",
		TaskFile: "plan",
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
	if inst.TaskFile != "plan" {
		t.Fatalf("instance PlanFile = %q, want %q", inst.TaskFile, "plan")
	}

	roundTrip := inst.ToInstanceData()
	if roundTrip.TaskFile != "plan" {
		t.Fatalf("ToInstanceData PlanFile = %q, want %q", roundTrip.TaskFile, "plan")
	}
}

func TestInstanceData_RoundTripImplementationComplete(t *testing.T) {
	data := InstanceData{
		Title:                  "coder-done",
		Path:                   "/tmp/repo",
		Branch:                 "feature/impl",
		Status:                 Paused,
		Program:                "opencode",
		TaskFile:               "plan",
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
		TaskFile:  "auth-refactor",
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
		TaskFile:  "auth-refactor",
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

func TestInstanceData_RoundTripSoloAgent(t *testing.T) {
	inst, err := NewInstance(InstanceOptions{
		Title:   "solo-worker",
		Path:    "/tmp/repo",
		Program: "opencode",
	})
	if err != nil {
		t.Fatalf("NewInstance() error = %v", err)
	}
	inst.SoloAgent = true

	data := inst.ToInstanceData()
	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData() error = %v", err)
	}
	if !restored.SoloAgent {
		t.Fatal("expected SoloAgent = true after InstanceData round-trip")
	}
}

// TestInstanceData_RoundTripExecutionMode verifies that ExecutionMode survives a
// full InstanceData round-trip, and that the empty string normalises to tmux.
func TestInstanceData_RoundTripExecutionMode(t *testing.T) {
	tests := []struct {
		name     string
		input    ExecutionMode
		expected ExecutionMode
	}{
		{"headless preserved", ExecutionModeHeadless, ExecutionModeHeadless},
		{"tmux preserved", ExecutionModeTmux, ExecutionModeTmux},
		{"empty defaults to tmux", "", ExecutionModeTmux},
		{"unknown defaults to tmux", ExecutionMode("unknown"), ExecutionModeTmux},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := InstanceData{
				Title:         "mode-test",
				Path:          "/tmp/repo",
				Branch:        "feature/test",
				Status:        Paused,
				Program:       "claude",
				ExecutionMode: tt.input,
				Worktree: GitWorktreeData{
					RepoPath:      "/tmp/repo",
					WorktreePath:  "/tmp/repo/.worktrees/mode-test",
					SessionName:   "mode-test",
					BranchName:    "feature/test",
					BaseCommitSHA: "abc123",
				},
			}

			inst, err := FromInstanceData(data)
			if err != nil {
				t.Fatalf("FromInstanceData() error = %v", err)
			}
			if inst.ExecutionMode != tt.expected {
				t.Fatalf("ExecutionMode = %q, want %q", inst.ExecutionMode, tt.expected)
			}

			roundTrip := inst.ToInstanceData()
			if roundTrip.ExecutionMode != tt.expected {
				t.Fatalf("ToInstanceData ExecutionMode = %q, want %q", roundTrip.ExecutionMode, tt.expected)
			}
		})
	}
}
