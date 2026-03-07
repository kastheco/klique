package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
)

// TmuxSpawner implements loop.AgentSpawner using tmux-backed sessions managed
// by the session package. It tracks running instances in a map keyed by
// "planFile:agentType" and provides a KillAgent method to stop them.
//
// TmuxSpawner does not handle UI concerns (toasts, navigation, overlay focus).
// It only manages process lifecycle.
type TmuxSpawner struct {
	logger    *slog.Logger
	mu        sync.Mutex
	instances map[string]*session.Instance
}

// NewTmuxSpawner returns a TmuxSpawner using the default slog logger.
func NewTmuxSpawner() *TmuxSpawner {
	return newTmuxSpawner(slog.Default())
}

// newTmuxSpawner returns a TmuxSpawner using the provided logger. Used
// internally by the Daemon so all components share the same logger.
func newTmuxSpawner(logger *slog.Logger) *TmuxSpawner {
	return &TmuxSpawner{
		logger:    logger,
		instances: make(map[string]*session.Instance),
	}
}

// instanceKey returns the map key for the given plan file and agent type.
func instanceKey(planFile, agentType string) string {
	return planFile + ":" + agentType
}

// SpawnReviewer launches a reviewer agent in the plan's shared worktree.
func (s *TmuxSpawner) SpawnReviewer(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn reviewer", "plan", opts.PlanFile, "wave", opts.Wave)
	return s.spawnInSharedWorktree(ctx, opts, session.AgentTypeReviewer)
}

// SpawnCoder launches a coder agent in the plan's shared worktree.
func (s *TmuxSpawner) SpawnCoder(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn coder", "plan", opts.PlanFile, "wave", opts.Wave)
	return s.spawnInSharedWorktree(ctx, opts, session.AgentTypeCoder)
}

// SpawnElaborator launches an elaborator agent on the main branch (no worktree).
// The elaborator only reads the codebase and updates the task store, so it
// does not need an isolated worktree.
func (s *TmuxSpawner) SpawnElaborator(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn elaborator", "plan", opts.PlanFile)

	if opts.RepoPath == "" {
		return fmt.Errorf("TmuxSpawner.SpawnElaborator: RepoPath is required")
	}

	planName := taskstate.DisplayName(opts.PlanFile)
	title := fmt.Sprintf("%s-elaborator", planName)
	program := opts.Program
	if program == "" {
		program = "opencode"
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      opts.RepoPath,
		Program:   program,
		AgentType: session.AgentTypeElaborator,
		TaskFile:  opts.PlanFile,
	})
	if err != nil {
		return fmt.Errorf("TmuxSpawner.SpawnElaborator: create instance: %w", err)
	}
	inst.QueuedPrompt = opts.Prompt
	inst.SetStatus(session.Loading)

	if err := inst.StartOnMainBranch(); err != nil {
		return fmt.Errorf("TmuxSpawner.SpawnElaborator: start: %w", err)
	}

	s.mu.Lock()
	s.instances[instanceKey(opts.PlanFile, session.AgentTypeElaborator)] = inst
	s.mu.Unlock()
	return nil
}

// KillAgent stops the running agent of the given type for the plan and removes
// it from the tracked instances map. It is a no-op when no matching instance is
// found.
func (s *TmuxSpawner) KillAgent(planFile, agentType string) error {
	s.logger.Info("kill agent", "plan", planFile, "type", agentType)

	key := instanceKey(planFile, agentType)

	s.mu.Lock()
	inst, ok := s.instances[key]
	if ok {
		delete(s.instances, key)
	}
	s.mu.Unlock()

	if !ok {
		return nil
	}
	return inst.Kill()
}

// spawnInSharedWorktree creates an instance for the given agent type, sets up
// the plan's shared worktree, and starts the instance inside it.
func (s *TmuxSpawner) spawnInSharedWorktree(_ context.Context, opts loop.SpawnOpts, agentType string) error {
	if opts.RepoPath == "" {
		return fmt.Errorf("TmuxSpawner.%s: RepoPath is required", agentType)
	}
	if opts.Branch == "" {
		return fmt.Errorf("TmuxSpawner.%s: Branch is required", agentType)
	}

	planName := taskstate.DisplayName(opts.PlanFile)
	title := fmt.Sprintf("%s-%s", planName, agentType)
	program := opts.Program
	if program == "" {
		program = "opencode"
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:     title,
		Path:      opts.RepoPath,
		Program:   program,
		AgentType: agentType,
		TaskFile:  opts.PlanFile,
	})
	if err != nil {
		return fmt.Errorf("TmuxSpawner.%s: create instance: %w", agentType, err)
	}
	inst.QueuedPrompt = opts.Prompt
	inst.SetStatus(session.Loading)

	shared := gitpkg.NewSharedTaskWorktree(opts.RepoPath, opts.Branch)
	if err := shared.Setup(); err != nil {
		return fmt.Errorf("TmuxSpawner.%s: setup shared worktree: %w", agentType, err)
	}
	if err := inst.StartInSharedWorktree(shared, opts.Branch); err != nil {
		return fmt.Errorf("TmuxSpawner.%s: start in shared worktree: %w", agentType, err)
	}

	s.mu.Lock()
	s.instances[instanceKey(opts.PlanFile, agentType)] = inst
	s.mu.Unlock()
	return nil
}
