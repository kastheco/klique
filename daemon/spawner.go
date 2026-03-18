package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	tmuxpkg "github.com/kastheco/kasmos/session/tmux"
)

// TmuxSpawnerConfig holds optional configuration for TmuxSpawner.
type TmuxSpawnerConfig struct {
	// Logger is the structured logger to use. Defaults to slog.Default().
	Logger *slog.Logger
	// DrainTimeout is the maximum time to wait for agents to exit gracefully
	// during DrainAll. Defaults to 30 seconds.
	DrainTimeout time.Duration
}

// InstanceInfo carries lightweight metadata about a running agent instance.
type InstanceInfo struct {
	// Key is the internal tracking key: "planFile:agentType".
	Key string
	// PlanFile is the path to the plan markdown file.
	PlanFile string
	// AgentType is the type of agent (e.g. "coder", "reviewer").
	AgentType string
	// Project is the project name (basename of repo root) this instance belongs to.
	Project string
}

// TmuxSpawner implements loop.AgentSpawner using tmux-backed sessions managed
// by the session package. It tracks running instances in a map keyed by
// "repoPath:planFile:agentType" and provides a KillAgent method to stop them.
//
// TmuxSpawner does not handle UI concerns (toasts, navigation, overlay focus).
// It only manages process lifecycle.
type TmuxSpawner struct {
	logger       *slog.Logger
	drainTimeout time.Duration
	mu           sync.Mutex
	instances    map[string]*session.Instance
	// planFileByKey stores the planFile portion of the key for RunningInstances.
	planFileByKey  map[string]string
	agentTypeByKey map[string]string
	// projectByKey stores the project name for each running instance so that
	// ListInstances can filter by project.
	projectByKey map[string]string

	// injectable seams for testability and grace-period checks.
	hasAttachedClients func(cmd.Executor, string) bool
	sleep              func(time.Duration)
	kill               func(*session.Instance) error
	cmdExec            cmd.Executor
	discoverOrphans    func([]string) ([]tmuxpkg.SessionInfo, error)
	restoreInstance    func(session.InstanceData) (*session.Instance, error)
	cleanupGracePeriod time.Duration
}

// NewTmuxSpawner returns a TmuxSpawner. An optional TmuxSpawnerConfig may be
// provided; if omitted or zero-valued, sensible defaults are used.
func NewTmuxSpawner(cfgs ...TmuxSpawnerConfig) *TmuxSpawner {
	var cfg TmuxSpawnerConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return newTmuxSpawnerWithConfig(logger, cfg.DrainTimeout)
}

// newTmuxSpawner returns a TmuxSpawner using the provided logger. Used
// internally by the Daemon so all components share the same logger.
func newTmuxSpawner(logger *slog.Logger) *TmuxSpawner {
	return newTmuxSpawnerWithConfig(logger, 0)
}

// newTmuxSpawnerWithConfig constructs a TmuxSpawner with explicit logger and drain timeout.
func newTmuxSpawnerWithConfig(logger *slog.Logger, drainTimeout time.Duration) *TmuxSpawner {
	if drainTimeout <= 0 {
		drainTimeout = 30 * time.Second
	}
	return &TmuxSpawner{
		logger:         logger,
		drainTimeout:   drainTimeout,
		instances:      make(map[string]*session.Instance),
		planFileByKey:  make(map[string]string),
		agentTypeByKey: make(map[string]string),
		projectByKey:   make(map[string]string),

		hasAttachedClients: tmuxpkg.HasAttachedClients,
		sleep:              time.Sleep,
		kill:               func(inst *session.Instance) error { return inst.Kill() },
		cmdExec:            cmd.MakeExecutor(),
		discoverOrphans: func(known []string) ([]tmuxpkg.SessionInfo, error) {
			return tmuxpkg.DiscoverAll(cmd.MakeExecutor(), known)
		},
		restoreInstance:    session.FromInstanceData,
		cleanupGracePeriod: 30 * time.Second,
	}
}

// shouldSkipCleanup returns true when a tmux client is attached, indicating
// that cleanup should be deferred to avoid interrupting a live user session.
func shouldSkipCleanup(hasClients bool) bool {
	return hasClients
}

// gracefulKill stops the instance only when no tmux clients are attached.
// It checks for attached clients before the grace period and once more after
// sleeping. If a client is present at either check the instance is left
// running (the orphan-discovery path will handle it later). If both checks
// show no client, kill is called.
// Returns (true, nil) when the instance was killed, (false, nil) when cleanup
// was skipped because a client is still attached, and (true, err) when the
// kill was attempted but failed.
func (s *TmuxSpawner) gracefulKill(inst *session.Instance, sessionName string) (bool, error) {
	if shouldSkipCleanup(s.hasAttachedClients(s.cmdExec, sessionName)) {
		s.logger.Info("grace period: client attached, skipping cleanup",
			"session", sessionName, "title", inst.Title)
		return false, nil
	}
	s.sleep(s.cleanupGracePeriod)
	if shouldSkipCleanup(s.hasAttachedClients(s.cmdExec, sessionName)) {
		s.logger.Info("grace period: client attached after sleep, skipping cleanup",
			"session", sessionName, "title", inst.Title)
		return false, nil
	}
	return true, s.kill(inst)
}

// RunningInstances returns a snapshot of all currently tracked agent instances.
func (s *TmuxSpawner) RunningInstances() []InstanceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]InstanceInfo, 0, len(s.instances))
	for key := range s.instances {
		out = append(out, InstanceInfo{
			Key:       key,
			PlanFile:  s.planFileByKey[key],
			AgentType: s.agentTypeByKey[key],
			Project:   s.projectByKey[key],
		})
	}
	return out
}

// DrainAll signals all tracked instances to stop, waits up to the configured
// drain timeout for graceful exit, then force-kills any remaining instances.
// It is safe to call concurrently but is intended to be called once during
// daemon shutdown.
func (s *TmuxSpawner) DrainAll(ctx context.Context) {
	s.mu.Lock()
	keys := make([]string, 0, len(s.instances))
	for k := range s.instances {
		keys = append(keys, k)
	}
	s.mu.Unlock()

	if len(keys) == 0 {
		return
	}

	s.logger.Info("draining agents", "count", len(keys))

	deadline := time.Now().Add(s.drainTimeout)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for _, key := range keys {
			s.mu.Lock()
			inst, ok := s.instances[key]
			s.mu.Unlock()
			if !ok {
				continue
			}
			// Attempt graceful kill (closes tmux session and removes worktree if clean).
			if err := inst.Kill(); err != nil {
				s.logger.Warn("graceful kill failed, instance may need manual cleanup",
					"key", key, "err", err)
			}
			s.mu.Lock()
			delete(s.instances, key)
			delete(s.planFileByKey, key)
			delete(s.agentTypeByKey, key)
			delete(s.projectByKey, key)
			s.mu.Unlock()
		}
	}()

	select {
	case <-done:
		s.logger.Info("all agents drained gracefully")
	case <-time.After(time.Until(deadline)):
		s.logger.Warn("drain timeout exceeded; force-killing remaining instances")
		s.mu.Lock()
		remaining := make([]string, 0, len(s.instances))
		for k := range s.instances {
			remaining = append(remaining, k)
		}
		s.mu.Unlock()
		for _, key := range remaining {
			s.mu.Lock()
			inst, ok := s.instances[key]
			if ok {
				delete(s.instances, key)
				delete(s.planFileByKey, key)
				delete(s.agentTypeByKey, key)
				delete(s.projectByKey, key)
			}
			s.mu.Unlock()
			if ok {
				if err := inst.Kill(); err != nil {
					s.logger.Error("force kill failed", "key", key, "err", err)
				}
			}
		}
	case <-ctx.Done():
		s.logger.Warn("drain interrupted by context cancellation")
	}
}

// DiscoverOrphanSessions returns all kas_-prefixed tmux sessions that are not
// tracked by this spawner (i.e. Managed == false). When tmux is unavailable or
// returns no sessions, a non-nil empty slice is returned.
func (s *TmuxSpawner) DiscoverOrphanSessions() []tmuxpkg.SessionInfo {
	s.mu.Lock()
	known := make([]string, 0, len(s.instances))
	for _, inst := range s.instances {
		// Instance.Title is the tmux session identifier (the kas_ name without prefix).
		known = append(known, inst.Title)
	}
	s.mu.Unlock()

	discover := s.discoverOrphans
	if discover == nil {
		discover = func(known []string) ([]tmuxpkg.SessionInfo, error) {
			return tmuxpkg.DiscoverAll(cmd.MakeExecutor(), known)
		}
	}

	all, err := discover(known)
	if err != nil {
		s.logger.Warn("discover tmux sessions failed", "err", err)
		return []tmuxpkg.SessionInfo{}
	}
	if all == nil {
		return []tmuxpkg.SessionInfo{}
	}

	orphans := make([]tmuxpkg.SessionInfo, 0)
	for _, si := range all {
		if !si.Managed {
			orphans = append(orphans, si)
		}
	}
	return orphans
}

// RestoreTrackedInstance re-adopts a previously running agent instance into the
// spawner's tracking maps.
func (s *TmuxSpawner) RestoreTrackedInstance(repoPath, project, planFile, agentType string, data session.InstanceData) error {
	restore := s.restoreInstance
	if restore == nil {
		restore = session.FromInstanceData
	}

	inst, err := restore(data)
	if err != nil {
		return err
	}

	key := instanceKey(repoPath, planFile, agentType)
	s.mu.Lock()
	s.instances[key] = inst
	s.planFileByKey[key] = planFile
	s.agentTypeByKey[key] = agentType
	s.projectByKey[key] = project
	s.mu.Unlock()
	return nil
}

// InstancesForRepo returns a snapshot of tracked instances for the given repo.
func (s *TmuxSpawner) InstancesForRepo(repoPath string) []*session.Instance {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*session.Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		if inst == nil || inst.Path != repoPath {
			continue
		}
		out = append(out, inst)
	}
	return out
}

// instanceKey returns the map key for the given repo path, plan file, and agent
// type. Including repoPath prevents key collisions when two registered repos
// share the same plan filename.
func instanceKey(repoPath, planFile, agentType string) string {
	return instanceKeyForTask(repoPath, planFile, agentType, 0, 0)
}

func instanceKeyForTask(repoPath, planFile, agentType string, waveNumber, taskNumber int) string {
	if waveNumber > 0 || taskNumber > 0 {
		return fmt.Sprintf("%s:%s:%s:w%d:t%d", repoPath, planFile, agentType, waveNumber, taskNumber)
	}
	return repoPath + ":" + planFile + ":" + agentType
}

// SpawnReviewer launches a reviewer agent in the plan's shared worktree.
func (s *TmuxSpawner) SpawnReviewer(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn reviewer", "plan", opts.PlanFile, "wave", opts.Wave)
	return s.spawnInSharedWorktree(ctx, opts, session.AgentTypeReviewer)
}

// SpawnPlanner launches a planner agent on the main branch (no worktree).
func (s *TmuxSpawner) SpawnPlanner(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn planner", "plan", opts.PlanFile)
	return s.spawnOnMainBranch(ctx, opts, session.AgentTypePlanner, "plan")
}

// SpawnCoder launches a coder agent in the plan's shared worktree.
func (s *TmuxSpawner) SpawnCoder(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn coder", "plan", opts.PlanFile, "wave", opts.Wave)
	return s.spawnInSharedWorktree(ctx, opts, session.AgentTypeCoder)
}

// SpawnFixer launches a fixer agent in the plan's shared worktree to address
// reviewer feedback. Any running fixer or coder for the same plan is killed
// first so the fixer starts with a clean slate.
func (s *TmuxSpawner) SpawnFixer(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn fixer", "plan", opts.PlanFile, "wave", opts.Wave)
	if err := s.KillAgent(opts.RepoPath, opts.PlanFile, session.AgentTypeFixer); err != nil {
		return fmt.Errorf("spawn fixer: kill existing fixer: %w", err)
	}
	if err := s.KillAgent(opts.RepoPath, opts.PlanFile, session.AgentTypeCoder); err != nil {
		return fmt.Errorf("spawn fixer: kill existing coder: %w", err)
	}
	return s.spawnInSharedWorktree(ctx, opts, session.AgentTypeFixer)
}

// SpawnElaborator launches an elaborator agent on the main branch (no worktree).
// The elaborator only reads the codebase and updates the task store, so it
// does not need an isolated worktree.
func (s *TmuxSpawner) SpawnElaborator(ctx context.Context, opts loop.SpawnOpts) error {
	s.logger.Info("spawn elaborator", "plan", opts.PlanFile)
	return s.spawnOnMainBranch(ctx, opts, session.AgentTypeElaborator, "elaborator")
}

func (s *TmuxSpawner) spawnOnMainBranch(_ context.Context, opts loop.SpawnOpts, agentType, titleSuffix string) error {
	if opts.RepoPath == "" {
		return fmt.Errorf("TmuxSpawner.%s: RepoPath is required", agentType)
	}

	planName := taskstate.DisplayName(opts.PlanFile)
	title := fmt.Sprintf("%s-%s", planName, titleSuffix)
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

	if err := inst.StartOnMainBranch(); err != nil {
		return fmt.Errorf("TmuxSpawner.%s: start: %w", agentType, err)
	}

	key := instanceKey(opts.RepoPath, opts.PlanFile, agentType)
	s.mu.Lock()
	s.instances[key] = inst
	s.planFileByKey[key] = opts.PlanFile
	s.agentTypeByKey[key] = agentType
	s.projectByKey[key] = opts.Project
	s.mu.Unlock()
	return nil
}

// KillAgent stops the running agent of the given type for the plan and removes
// it from the tracked instances map. repoPath must match the value used when
// the agent was spawned so the correct instance is identified across repos.
// It is a no-op when no matching instance is found.
func (s *TmuxSpawner) KillAgent(repoPath, planFile, agentType string) error {
	s.logger.Info("kill agent", "repo", repoPath, "plan", planFile, "type", agentType)

	key := instanceKey(repoPath, planFile, agentType)

	// Look up the instance without removing it from the tracking maps yet.
	// We only remove it after gracefulKill confirms the session was actually
	// terminated. If a tmux client is attached gracefulKill returns (false,
	// nil) and the daemon must retain ownership so it can target the session
	// in future operations.
	s.mu.Lock()
	inst, ok := s.instances[key]
	s.mu.Unlock()

	if !ok {
		return nil
	}
	sessionName := tmuxpkg.ToKasTmuxNamePublic(inst.Title)
	killed, err := s.gracefulKill(inst, sessionName)
	if killed {
		s.mu.Lock()
		delete(s.instances, key)
		delete(s.planFileByKey, key)
		delete(s.agentTypeByKey, key)
		delete(s.projectByKey, key)
		s.mu.Unlock()
	}
	return err
}

// KillWaveAgents stops all tracked task agents for the given plan/wave. It is a
// no-op when there are no matching task instances.
func (s *TmuxSpawner) KillWaveAgents(repoPath, planFile string, waveNumber int) error {
	s.logger.Info("kill wave agents", "repo", repoPath, "plan", planFile, "wave", waveNumber)

	s.mu.Lock()
	targets := make([]struct {
		key  string
		inst *session.Instance
	}, 0)
	for key, inst := range s.instances {
		if inst == nil {
			continue
		}
		if inst.Path != repoPath || inst.TaskFile != planFile {
			continue
		}
		if inst.TaskNumber <= 0 || inst.WaveNumber != waveNumber {
			continue
		}
		targets = append(targets, struct {
			key  string
			inst *session.Instance
		}{key: key, inst: inst})
	}
	s.mu.Unlock()

	var firstErr error
	for _, target := range targets {
		sessionName := tmuxpkg.ToKasTmuxNamePublic(target.inst.Title)
		killed, err := s.gracefulKill(target.inst, sessionName)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if killed {
			s.mu.Lock()
			delete(s.instances, target.key)
			delete(s.planFileByKey, target.key)
			delete(s.agentTypeByKey, target.key)
			delete(s.projectByKey, target.key)
			s.mu.Unlock()
		}
	}

	return firstErr
}

// SpawnWaveTask launches a coder instance for a specific task within a wave.
func (s *TmuxSpawner) SpawnWaveTask(_ context.Context, opts loop.SpawnOpts, task taskparser.Task, prompt string, peerCount int) error {
	if opts.RepoPath == "" {
		return fmt.Errorf("TmuxSpawner.wave-task: RepoPath is required")
	}
	if opts.Branch == "" {
		return fmt.Errorf("TmuxSpawner.wave-task: Branch is required")
	}
	if opts.Wave <= 0 {
		return fmt.Errorf("TmuxSpawner.wave-task: Wave is required")
	}

	planName := taskstate.DisplayName(opts.PlanFile)
	title := fmt.Sprintf("%s-W%d-T%d", planName, opts.Wave, task.Number)
	program := opts.Program
	if program == "" {
		program = "opencode"
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:      title,
		Path:       opts.RepoPath,
		Program:    program,
		AgentType:  session.AgentTypeCoder,
		TaskFile:   opts.PlanFile,
		TaskNumber: task.Number,
		WaveNumber: opts.Wave,
		PeerCount:  peerCount,
	})
	if err != nil {
		return fmt.Errorf("TmuxSpawner.wave-task: create instance: %w", err)
	}
	inst.QueuedPrompt = prompt
	inst.SetStatus(session.Loading)

	shared := gitpkg.NewSharedTaskWorktree(opts.RepoPath, opts.Branch)
	if err := shared.Setup(); err != nil {
		return fmt.Errorf("TmuxSpawner.wave-task: setup shared worktree: %w", err)
	}
	if err := ensureWorktreeScaffold(shared.GetWorktreePath(), program, session.AgentTypeCoder); err != nil {
		return fmt.Errorf("TmuxSpawner.wave-task: sync scaffold: %w", err)
	}
	if err := inst.StartInSharedWorktree(shared, opts.Branch); err != nil {
		return fmt.Errorf("TmuxSpawner.wave-task: start in shared worktree: %w", err)
	}

	key := instanceKeyForTask(opts.RepoPath, opts.PlanFile, session.AgentTypeCoder, opts.Wave, task.Number)
	s.mu.Lock()
	s.instances[key] = inst
	s.planFileByKey[key] = opts.PlanFile
	s.agentTypeByKey[key] = session.AgentTypeCoder
	s.projectByKey[key] = opts.Project
	s.mu.Unlock()
	return nil
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
	if err := ensureWorktreeScaffold(shared.GetWorktreePath(), program, agentType); err != nil {
		return fmt.Errorf("TmuxSpawner.%s: sync scaffold: %w", agentType, err)
	}
	if err := inst.StartInSharedWorktree(shared, opts.Branch); err != nil {
		return fmt.Errorf("TmuxSpawner.%s: start in shared worktree: %w", agentType, err)
	}

	key := instanceKey(opts.RepoPath, opts.PlanFile, agentType)
	s.mu.Lock()
	s.instances[key] = inst
	s.planFileByKey[key] = opts.PlanFile
	s.agentTypeByKey[key] = agentType
	s.projectByKey[key] = opts.Project
	s.mu.Unlock()
	return nil
}

func ensureWorktreeScaffold(worktreePath, program, role string) error {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return nil
	}

	harnessName := filepath.Base(fields[0])
	switch harnessName {
	case "opencode", "claude", "codex":
		_, err := scaffold.SyncScaffold(worktreePath, []harness.AgentConfig{{
			Role:    role,
			Harness: harnessName,
			Enabled: true,
		}})
		return err
	default:
		return nil
	}
}
