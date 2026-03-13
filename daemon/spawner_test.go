package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
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
	err := s.KillAgent("/tmp/repo", "missing.md", "coder")
	assert.NoError(t, err)
}

func TestTmuxSpawner_KillAgent_PreservesTrackingWhenClientAttached(t *testing.T) {
	s := NewTmuxSpawner()

	// Simulate a client always attached — gracefulKill should skip the kill.
	s.hasAttachedClients = func(_ cmd.Executor, _ string) bool { return true }
	s.sleep = func(_ time.Duration) {}
	s.kill = func(_ *session.Instance) error { return nil }
	s.cleanupGracePeriod = 0

	// Manually register an instance so KillAgent can find it.
	const repoPath = "/tmp/repo"
	const planFile = "my-plan.md"
	const agentType = "coder"
	key := instanceKey(repoPath, planFile, agentType)
	inst := &session.Instance{Title: "my-plan-coder"}
	s.mu.Lock()
	s.instances[key] = inst
	s.planFileByKey[key] = planFile
	s.agentTypeByKey[key] = agentType
	s.projectByKey[key] = "my-project"
	s.mu.Unlock()

	err := s.KillAgent(repoPath, planFile, agentType)
	assert.NoError(t, err)

	// Instance must still be tracked because the kill was deferred.
	s.mu.Lock()
	_, stillTracked := s.instances[key]
	s.mu.Unlock()
	assert.True(t, stillTracked, "instance must remain in tracking maps when kill is deferred due to attached client")
}

func TestTmuxSpawner_instanceKey(t *testing.T) {
	assert.Equal(t, "/repo:plan.md:coder", instanceKey("/repo", "plan.md", "coder"))
	assert.Equal(t, "/repo:plan.md:reviewer", instanceKey("/repo", "plan.md", "reviewer"))
	// Two repos with the same plan filename must produce distinct keys.
	assert.NotEqual(t, instanceKey("/repo-a", "task.md", "coder"), instanceKey("/repo-b", "task.md", "coder"))
	assert.Equal(t, "/repo:plan.md:coder:w2:t3", instanceKeyForTask("/repo", "plan.md", "coder", 2, 3))
}

func TestTmuxSpawner_KillWaveAgents(t *testing.T) {
	s := NewTmuxSpawner()
	s.hasAttachedClients = func(_ cmd.Executor, _ string) bool { return false }
	s.sleep = func(_ time.Duration) {}
	killCalls := []string{}
	s.kill = func(inst *session.Instance) error {
		killCalls = append(killCalls, inst.Title)
		return nil
	}
	s.cleanupGracePeriod = 0

	const repoPath = "/tmp/repo"
	const planFile = "wave-plan.md"

	register := func(inst *session.Instance) {
		key := instanceKeyForTask(repoPath, planFile, inst.AgentType, inst.WaveNumber, inst.TaskNumber)
		s.mu.Lock()
		s.instances[key] = inst
		s.planFileByKey[key] = planFile
		s.agentTypeByKey[key] = inst.AgentType
		s.projectByKey[key] = "my-project"
		s.mu.Unlock()
	}

	register(&session.Instance{Title: "wave-plan-W1-T1", Path: repoPath, TaskFile: planFile, AgentType: session.AgentTypeCoder, TaskNumber: 1, WaveNumber: 1})
	register(&session.Instance{Title: "wave-plan-W1-T2", Path: repoPath, TaskFile: planFile, AgentType: session.AgentTypeCoder, TaskNumber: 2, WaveNumber: 1})
	register(&session.Instance{Title: "wave-plan-W2-T3", Path: repoPath, TaskFile: planFile, AgentType: session.AgentTypeCoder, TaskNumber: 3, WaveNumber: 2})

	err := s.KillWaveAgents(repoPath, planFile, 1)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"wave-plan-W1-T1", "wave-plan-W1-T2"}, killCalls)

	s.mu.Lock()
	remaining := make([]string, 0, len(s.instances))
	for _, inst := range s.instances {
		remaining = append(remaining, inst.Title)
	}
	s.mu.Unlock()
	assert.Equal(t, []string{"wave-plan-W2-T3"}, remaining)
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

func TestTmuxSpawner_SpawnFixer_MissingBranch(t *testing.T) {
	s := NewTmuxSpawner()
	err := s.SpawnFixer(context.Background(), loop.SpawnOpts{
		PlanFile: "plan.md",
		RepoPath: "/tmp/repo",
		Program:  "opencode",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Branch")
}

func TestTmuxSpawner_SpawnFixer_MissingRepoPath(t *testing.T) {
	s := NewTmuxSpawner()
	err := s.SpawnFixer(context.Background(), loop.SpawnOpts{
		PlanFile: "plan.md",
		Branch:   "plan/plan",
		Program:  "opencode",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RepoPath")
}

func TestTmuxSpawner_SpawnFixer_KillsExistingAgents(t *testing.T) {
	s := NewTmuxSpawner()

	// Inject no-op kill so we don't need real tmux sessions.
	s.hasAttachedClients = func(_ cmd.Executor, _ string) bool { return false }
	s.sleep = func(_ time.Duration) {}
	killCalls := []string{}
	s.kill = func(inst *session.Instance) error {
		killCalls = append(killCalls, inst.Title)
		return nil
	}
	s.cleanupGracePeriod = 0

	const repoPath = "/tmp/repo"
	const planFile = "my-plan.md"

	// Pre-register a fixer and a coder instance.
	for _, agentType := range []string{session.AgentTypeFixer, session.AgentTypeCoder} {
		key := instanceKey(repoPath, planFile, agentType)
		inst := &session.Instance{Title: "my-plan-" + agentType}
		s.mu.Lock()
		s.instances[key] = inst
		s.planFileByKey[key] = planFile
		s.agentTypeByKey[key] = agentType
		s.projectByKey[key] = "my-project"
		s.mu.Unlock()
	}

	// SpawnFixer should kill both existing agents before failing on missing git.
	// The error from spawnInSharedWorktree is expected (no real git/tmux).
	_ = s.SpawnFixer(context.Background(), loop.SpawnOpts{
		PlanFile: planFile,
		RepoPath: repoPath,
		Branch:   "plan/my-plan",
	})

	// Both the fixer and coder must have been killed (kill called for each).
	assert.Len(t, killCalls, 2, "expected kill calls for fixer and coder")
}

func TestShouldSkipCleanup_AttachedClient(t *testing.T) {
	assert.True(t, shouldSkipCleanup(true), "should skip cleanup when a client is attached")
}

func TestShouldSkipCleanup_NoClient(t *testing.T) {
	assert.False(t, shouldSkipCleanup(false), "should not skip cleanup when no client is attached")
}

func TestTmuxSpawner_GracefulKill_SkipsWhenClientAttached(t *testing.T) {
	s := NewTmuxSpawner()

	killCalled := false
	s.hasAttachedClients = func(_ cmd.Executor, _ string) bool { return true }
	s.sleep = func(_ time.Duration) {}
	s.kill = func(_ *session.Instance) error {
		killCalled = true
		return nil
	}
	s.cleanupGracePeriod = 0

	inst := &session.Instance{Title: "plan-coder"}
	killed, err := s.gracefulKill(inst, "kas_plan-coder")
	assert.NoError(t, err)
	assert.False(t, killed, "should report not killed when a client is attached")
	assert.False(t, killCalled, "kill must not be called when a client is attached")
}

func TestTmuxSpawner_GracefulKill_KillsAfterSecondCheck(t *testing.T) {
	s := NewTmuxSpawner()

	killCalled := false
	s.hasAttachedClients = func(_ cmd.Executor, _ string) bool { return false }
	s.sleep = func(_ time.Duration) {}
	s.kill = func(_ *session.Instance) error {
		killCalled = true
		return nil
	}
	s.cleanupGracePeriod = 0

	inst := &session.Instance{Title: "plan-coder"}
	killed, err := s.gracefulKill(inst, "kas_plan-coder")
	assert.NoError(t, err)
	assert.True(t, killed, "should report killed when no client is attached")
	assert.True(t, killCalled, "kill must be called when no client is attached after grace period")
}
