package daemon

import (
	"context"
	"testing"

	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/stretchr/testify/assert"
)

func TestTmuxSpawner_ImplementsInterface(t *testing.T) {
	var _ loop.AgentSpawner = (*TmuxSpawner)(nil)
}

func TestSpawnOpts_InstanceTitle(t *testing.T) {
	opts := loop.SpawnOpts{
		PlanFile:  "my-feature.md",
		AgentType: "reviewer",
		RepoPath:  "/tmp/repo",
		Branch:    "plan/my-feature",
		Prompt:    "review this",
		Program:   "opencode",
	}
	assert.Equal(t, "reviewer", opts.AgentType)
	assert.Equal(t, "my-feature.md", opts.PlanFile)
}

func TestTmuxSpawner_KillAgent_NoOp(t *testing.T) {
	s := NewTmuxSpawner()
	// KillAgent on a non-existent key should return nil (no error).
	err := s.KillAgent("missing.md", "coder")
	assert.NoError(t, err)
}

func TestTmuxSpawner_instanceKey(t *testing.T) {
	assert.Equal(t, "plan.md:coder", instanceKey("plan.md", "coder"))
	assert.Equal(t, "plan.md:reviewer", instanceKey("plan.md", "reviewer"))
}

func TestTmuxSpawner_SpawnReviewer_MissingRepoPath(t *testing.T) {
	s := NewTmuxSpawner()
	err := s.SpawnReviewer(context.Background(), loop.SpawnOpts{
		PlanFile: "plan.md",
		Branch:   "plan/plan",
		Program:  "opencode",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RepoPath")
}

func TestTmuxSpawner_SpawnCoder_MissingBranch(t *testing.T) {
	s := NewTmuxSpawner()
	err := s.SpawnCoder(context.Background(), loop.SpawnOpts{
		PlanFile: "plan.md",
		RepoPath: "/tmp/repo",
		Program:  "opencode",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Branch")
}

func TestTmuxSpawner_SpawnElaborator_MissingRepoPath(t *testing.T) {
	s := NewTmuxSpawner()
	err := s.SpawnElaborator(context.Background(), loop.SpawnOpts{
		PlanFile: "plan.md",
		Program:  "opencode",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RepoPath")
}
