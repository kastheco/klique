package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
)

// ---------------------------------------------------------------------------
// Daemon
// ---------------------------------------------------------------------------

// Daemon is the multi-repo background orchestrator. It polls registered
// repositories for signal files and executes the resulting actions via the
// configured AgentSpawner.
type Daemon struct {
	cfg         *DaemonConfig
	repos       *RepoManager
	spawner     *TmuxSpawner
	logger      *slog.Logger
	pidLock     *PIDLock
	broadcaster *api.EventBroadcaster
	mu          sync.RWMutex
	startedAt   time.Time
}

// daemonStateAdapter adapts the Daemon to the api.StateProvider interface.
type daemonStateAdapter struct {
	d *Daemon
}

// activePlansByProject counts distinct plan files currently running per project.
func (a *daemonStateAdapter) activePlansByProject() map[string]int {
	counts := map[string]int{}
	for _, inst := range a.d.spawner.RunningInstances() {
		if inst.Project == "" {
			continue
		}
		key := inst.Project + "\x00" + inst.PlanFile
		counts[key]++
	}
	// Collapse to unique-plan count per project.
	perProject := map[string]int{}
	seen := map[string]struct{}{}
	for k := range counts {
		parts := strings.SplitN(k, "\x00", 2)
		proj := parts[0]
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			perProject[proj]++
		}
	}
	return perProject
}

func (a *daemonStateAdapter) Status() api.StatusResponse {
	a.d.mu.RLock()
	defer a.d.mu.RUnlock()
	uptime := ""
	if !a.d.startedAt.IsZero() {
		uptime = time.Since(a.d.startedAt).Round(time.Second).String()
	}
	repos := a.d.repos.List()
	active := a.activePlansByProject()
	repoStatuses := make([]api.RepoStatus, len(repos))
	for i, r := range repos {
		repoStatuses[i] = api.RepoStatus{Path: r.Path, Project: r.Project, ActivePlans: active[r.Project]}
	}
	return api.StatusResponse{
		Running:   true,
		Repos:     repoStatuses,
		RepoCount: len(repoStatuses),
		Uptime:    uptime,
	}
}

func (a *daemonStateAdapter) ListRepos() []api.RepoStatus {
	active := a.activePlansByProject()
	repos := a.d.repos.List()
	out := make([]api.RepoStatus, len(repos))
	for i, r := range repos {
		out[i] = api.RepoStatus{Path: r.Path, Project: r.Project, ActivePlans: active[r.Project]}
	}
	return out
}

func (a *daemonStateAdapter) AddRepo(path string) error {
	return a.d.repos.Add(path)
}

func (a *daemonStateAdapter) RemoveRepo(project string) error {
	return a.d.repos.RemoveByProject(project)
}

func (a *daemonStateAdapter) ListPlans(project string) ([]taskstore.TaskEntry, error) {
	entries := a.d.repos.List()
	for _, e := range entries {
		if e.Project == project && e.Store != nil {
			return e.Store.List(project)
		}
	}
	return nil, fmt.Errorf("project not found: %s", project)
}

func (a *daemonStateAdapter) ListInstances(project string) []api.InstanceStatus {
	all := a.d.spawner.RunningInstances()
	var out []api.InstanceStatus
	for _, info := range all {
		if info.Project != project {
			continue
		}
		out = append(out, api.InstanceStatus{
			ID:      info.Key,
			Project: info.Project,
			Plan:    info.PlanFile,
			Role:    info.AgentType,
			Active:  true,
		})
	}
	return out
}

func (a *daemonStateAdapter) EventStream() <-chan api.Event {
	if a.d.broadcaster != nil {
		return a.d.broadcaster.Subscribe()
	}
	return make(chan api.Event)
}

// NewDaemon creates a new Daemon from the given configuration. The daemon is
// not started until Run is called.
func NewDaemon(cfg *DaemonConfig) (*Daemon, error) {
	if cfg == nil {
		cfg = defaultDaemonConfig()
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.SocketPath == "" {
		cfg.SocketPath = defaultSocketPath()
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	repos := NewRepoManager()
	repos.autoReviewFix = cfg.AutoReviewFix
	repos.maxReviewFixCycles = cfg.MaxReviewFixCycles

	d := &Daemon{
		cfg:         cfg,
		repos:       repos,
		spawner:     newTmuxSpawner(logger),
		logger:      logger,
		broadcaster: api.NewEventBroadcaster(),
	}

	// Pre-register repos from config.
	for _, r := range cfg.Repos {
		if err := d.repos.Add(r); err != nil {
			return nil, fmt.Errorf("daemon: add repo %s: %w", r, err)
		}
	}

	return d, nil
}

// defaultSocketPath returns the default Unix domain socket path for the daemon.
// It prefers $XDG_RUNTIME_DIR/kasmos/kas.sock, then falls back to
// /tmp/kasmos-<uid>/kas.sock.
func defaultSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "kasmos", "kas.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("kasmos-%d", os.Getuid()), "kas.sock")
}

// DefaultSocketPath returns the default Unix domain socket path for the daemon.
func DefaultSocketPath() string {
	return defaultSocketPath()
}

// AddRepo registers a repository root with the daemon. The repo will be
// polled on the next tick. Safe to call concurrently.
func (d *Daemon) AddRepo(root string) error {
	return d.repos.Add(root)
}

// ListRepos returns the current list of registered repo root paths.
func (d *Daemon) ListRepos() []string {
	entries := d.repos.List()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}

// Run starts the daemon event loop. It blocks until ctx is cancelled, then
// performs a clean shutdown.
//
// The event loop:
//  1. Creates a ticker at cfg.PollInterval.
//  2. Listens on the Unix domain socket (cfg.SocketPath) and serves the
//     control API via api.Handler.
//  3. On each tick: scans all registered repos for signal files, feeds results
//     to per-repo Processor.Tick(), and executes the returned actions.
//  4. On context cancellation: releases the PID lock and closes the socket.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("daemon starting", "poll_interval", d.cfg.PollInterval, "socket", d.cfg.SocketPath)

	d.mu.Lock()
	d.startedAt = time.Now()
	d.mu.Unlock()

	// Ensure the socket directory exists.
	if err := os.MkdirAll(filepath.Dir(d.cfg.SocketPath), 0o700); err != nil {
		return fmt.Errorf("daemon: create socket dir: %w", err)
	}

	lock, err := AcquirePIDLock(d.pidLockPath())
	if err != nil {
		return fmt.Errorf("daemon: acquire pid lock: %w", err)
	}
	d.pidLock = lock

	// Ensure signal directories exist and recover any in-flight signals that
	// were interrupted by a previous crash before beginning the poll loop.
	for _, e := range d.repos.List() {
		allSignalDirs := []string{filepath.Join(e.Path, ".kasmos", "signals")}
		for _, wt := range sharedWorktreePaths(e.Path) {
			allSignalDirs = append(allSignalDirs, filepath.Join(wt, ".kasmos", "signals"))
		}
		for _, sd := range allSignalDirs {
			if ensErr := taskfsm.EnsureSignalDirs(sd); ensErr != nil {
				d.logger.Warn("ensure signal dirs failed on startup", "dir", sd, "err", ensErr)
				continue
			}
			if n := taskfsm.RecoverInFlight(sd); n > 0 {
				d.logger.Info("recovered in-flight signals", "dir", sd, "count", n)
			}
		}
	}

	// Remove any stale socket file before listening.
	_ = os.Remove(d.cfg.SocketPath)

	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		_ = d.pidLock.Release()
		d.pidLock = nil
		return fmt.Errorf("daemon: listen unix %s: %w", d.cfg.SocketPath, err)
	}
	defer func() {
		ln.Close()
		_ = os.Remove(d.cfg.SocketPath)
	}()

	// Build and start the HTTP server on the control socket.
	// Use NewHandlerWithBroadcaster so each connecting client gets its own
	// subscription to the live event stream rather than a dead channel.
	state := &daemonStateAdapter{d: d}
	handler := api.NewHandlerWithBroadcaster(state, d.broadcaster)
	srv := &http.Server{Handler: handler}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			d.logger.Error("control socket server error", "err", serveErr)
		}
	}()

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	// Reaper goroutine: reset signals stuck in "processing" for >60s.
	reaperTicker := time.NewTicker(30 * time.Second)
	defer reaperTicker.Stop()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-reaperTicker.C:
				reapStuckSignals(d.repos.List(), 60*time.Second, d.logger)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon shutting down")

			// Drain all running agent instances before closing the control socket.
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 35*time.Second)
			d.spawner.DrainAll(drainCtx)
			drainCancel()

			if shutErr := srv.Shutdown(context.Background()); shutErr != nil {
				d.logger.Warn("control socket shutdown error", "err", shutErr)
			}
			wg.Wait()
			if d.pidLock != nil {
				_ = d.pidLock.Release()
				d.pidLock = nil
			}
			// Close broadcaster after HTTP server shuts down so no new SSE
			// connections are started after we signal EOF.
			d.broadcaster.Close()
			return nil

		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

// tick executes one poll cycle across all registered repos using atomic
// per-signal processing: each signal is moved to processing/ before handling,
// then either completed (deleted) or dead-lettered into failed/.
func (d *Daemon) tick(ctx context.Context) {
	for _, e := range d.repos.List() {
		d.tickRepo(ctx, e)
	}
}

// tickRepo executes one poll cycle for a single repo entry.
// If the entry has a SignalGateway, it uses the DB-backed pipeline:
// bridge filesystem sentinels → claim via gateway → Processor.Tick → executeAction → MarkProcessed.
// If SignalGateway is nil, it falls back to the legacy filesystem-only path.
func (d *Daemon) tickRepo(ctx context.Context, e RepoEntry) {
	if e.Store == nil || e.Processor == nil {
		// Processor requires a store; skip repos whose store is unavailable.
		return
	}

	if e.SignalGateway == nil {
		// Legacy filesystem path — unchanged behavior.
		scan := loop.ScanAllSignals(e.Path, sharedWorktreePaths(e.Path))

		var actions []loop.Action

		// --- FSM signals ---
		for _, sig := range scan.FSMSignals {
			sigDir := sig.Dir()
			sigFile := sig.Filename()

			if err := taskfsm.EnsureSignalDirs(sigDir); err != nil {
				d.logger.Warn("ensure signal dirs failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			procPath, err := taskfsm.BeginProcessing(sigDir, sigFile)
			if err != nil {
				d.logger.Warn("begin processing failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			d.logger.Info("processing fsm signal", "file", sigFile, "event", sig.Event, "repo", e.Path)

			acts := e.Processor.ProcessFSMSignals([]taskfsm.Signal{sig})
			if len(acts) > 0 {
				actions = append(actions, acts...)
				taskfsm.CompleteProcessing(procPath)
			} else if sig.Event == taskfsm.ImplementFinished {
				// Benign suppressed/duplicate implement-finished — wave orchestrator
				// owns this transition; silently complete without dead-lettering.
				d.logger.Info("suppressed implement-finished signal", "file", sigFile, "plan", sig.TaskFile, "repo", e.Path)
				taskfsm.CompleteProcessing(procPath)
			} else {
				d.logger.Warn("dead-lettering fsm signal", "file", sigFile, "event", sig.Event, "repo", e.Path)
				taskfsm.FailProcessing(sigDir, sigFile, "signal rejected by processor")
			}
		}

		// --- Task signals ---
		for _, ts := range scan.TaskSignals {
			sigDir := ts.Dir()
			sigFile := ts.Filename()

			if err := taskfsm.EnsureSignalDirs(sigDir); err != nil {
				d.logger.Warn("ensure signal dirs failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			procPath, err := taskfsm.BeginProcessing(sigDir, sigFile)
			if err != nil {
				d.logger.Warn("begin processing failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			d.logger.Info("processing task signal", "file", sigFile, "repo", e.Path)

			acts := e.Processor.ProcessTaskSignals([]taskfsm.TaskSignal{ts})
			if len(acts) > 0 {
				actions = append(actions, acts...)
				taskfsm.CompleteProcessing(procPath)
			} else {
				d.logger.Warn("dead-lettering task signal", "file", sigFile, "repo", e.Path)
				taskfsm.FailProcessing(sigDir, sigFile, "no active orchestrator / wrong wave / already-finished task")
			}
		}

		// --- Wave signals ---
		for _, ws := range scan.WaveSignals {
			sigDir := ws.Dir()
			sigFile := ws.Filename()

			if err := taskfsm.EnsureSignalDirs(sigDir); err != nil {
				d.logger.Warn("ensure signal dirs failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			procPath, err := taskfsm.BeginProcessing(sigDir, sigFile)
			if err != nil {
				d.logger.Warn("begin processing failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			d.logger.Info("processing wave signal", "file", sigFile, "repo", e.Path)

			acts := e.Processor.ProcessWaveSignals([]taskfsm.WaveSignal{ws})
			if len(acts) > 0 {
				actions = append(actions, acts...)
				taskfsm.CompleteProcessing(procPath)
			} else {
				d.logger.Warn("dead-lettering wave signal", "file", sigFile, "repo", e.Path)
				taskfsm.FailProcessing(sigDir, sigFile, "processor could not start the requested wave")
			}
		}

		// --- Elaboration signals ---
		for _, es := range scan.ElaborationSignals {
			sigDir := es.Dir()
			sigFile := es.Filename()

			if err := taskfsm.EnsureSignalDirs(sigDir); err != nil {
				d.logger.Warn("ensure signal dirs failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			procPath, err := taskfsm.BeginProcessing(sigDir, sigFile)
			if err != nil {
				d.logger.Warn("begin processing failed", "file", sigFile, "repo", e.Path, "err", err)
				continue
			}
			d.logger.Info("processing elaboration signal", "file", sigFile, "repo", e.Path)

			acts := e.Processor.ProcessElaborationSignals([]taskfsm.ElaborationSignal{es})
			if len(acts) > 0 {
				actions = append(actions, acts...)
				taskfsm.CompleteProcessing(procPath)
			} else {
				d.logger.Warn("dead-lettering elaboration signal", "file", sigFile, "repo", e.Path)
				taskfsm.FailProcessing(sigDir, sigFile, "no active elaboration state to resume")
			}
		}

		for _, action := range actions {
			d.logger.Info("executing action", "kind", action.Kind(), "repo", e.Path)
			d.executeAction(ctx, e, action)
		}
		return
	}

	// DB-backed gateway path.
	workerID := fmt.Sprintf("daemon:%s:%d", e.Project, os.Getpid())

	if _, err := loop.BridgeFilesystemSignals(e.SignalGateway, e.Project, e.Path, sharedWorktreePaths(e.Path)); err != nil {
		d.logger.Error("bridge filesystem signals failed", "repo", e.Path, "err", err)
		return
	}

	scan, ids, err := loop.ScanGateway(e.SignalGateway, e.Project, workerID)
	if err != nil {
		d.logger.Error("scan gateway failed", "repo", e.Path, "err", err)
		return
	}

	actions := e.Processor.Tick(scan)
	for _, action := range actions {
		d.executeAction(ctx, e, action)
	}

	for _, id := range ids {
		if err := e.SignalGateway.MarkProcessed(id, taskstore.SignalDone, ""); err != nil {
			d.logger.Error("mark processed failed", "repo", e.Path, "id", id, "err", err)
		}
	}
}

// reapStuckSignals resets signals that have been stuck in "processing" for
// longer than timeout across all repos with a SignalGateway. Returns the
// total count of signals reset.
func reapStuckSignals(repos []RepoEntry, timeout time.Duration, logger *slog.Logger) int {
	total := 0
	for _, e := range repos {
		if e.SignalGateway == nil {
			continue
		}
		n, err := e.SignalGateway.ResetStuck(timeout)
		if err != nil {
			logger.Error("reap stuck signals failed", "repo", e.Path, "project", e.Project, "err", err)
			continue
		}
		total += n
	}
	return total
}

// executeAction dispatches a single action to the configured spawner.
// It resolves RepoPath from e.Path and looks up Branch from the task store so
// that spawnInSharedWorktree has the required context.
func (d *Daemon) executeAction(ctx context.Context, e RepoEntry, action loop.Action) {
	// branchFor looks up the git branch for a plan from the task store.
	branchFor := func(planFile string) string {
		if e.Store == nil {
			return ""
		}
		entry, err := e.Store.Get(e.Project, planFile)
		if err != nil {
			return ""
		}
		return entry.Branch
	}

	switch a := action.(type) {
	case loop.SpawnReviewerAction:
		opts := loop.SpawnOpts{
			PlanFile: a.PlanFile,
			RepoPath: e.Path,
			Project:  e.Project,
			Branch:   branchFor(a.PlanFile),
		}
		if err := d.spawner.SpawnReviewer(ctx, opts); err != nil {
			d.logger.Error("spawn reviewer failed", "plan", a.PlanFile, "err", err)
		} else {
			d.broadcaster.Emit(api.Event{
				Kind:      "agent_spawned",
				Message:   "reviewer spawned for " + a.PlanFile,
				Repo:      e.Path,
				PlanFile:  a.PlanFile,
				AgentType: "reviewer",
			})
		}
	case loop.SpawnCoderAction:
		opts := coderSpawnOpts(e, a.PlanFile, branchFor(a.PlanFile), a.Feedback)
		if err := d.spawner.SpawnCoder(ctx, opts); err != nil {
			d.logger.Error("spawn coder failed", "plan", a.PlanFile, "err", err)
		} else {
			d.broadcaster.Emit(api.Event{
				Kind:      "agent_spawned",
				Message:   "coder spawned for " + a.PlanFile,
				Repo:      e.Path,
				PlanFile:  a.PlanFile,
				AgentType: "coder",
			})
		}
	case loop.SpawnElaboratorAction:
		opts := loop.SpawnOpts{
			PlanFile: a.PlanFile,
			RepoPath: e.Path,
			Project:  e.Project,
		}
		if err := d.spawner.SpawnElaborator(ctx, opts); err != nil {
			d.logger.Error("spawn elaborator failed", "plan", a.PlanFile, "err", err)
		} else {
			d.broadcaster.Emit(api.Event{
				Kind:      "agent_spawned",
				Message:   "elaborator spawned for " + a.PlanFile,
				Repo:      e.Path,
				PlanFile:  a.PlanFile,
				AgentType: "elaborator",
			})
		}
	case loop.SpawnFixerAction:
		opts := coderSpawnOpts(e, a.PlanFile, branchFor(a.PlanFile), a.Feedback)
		if err := d.spawner.SpawnFixer(ctx, opts); err != nil {
			d.logger.Error("spawn fixer failed", "plan", a.PlanFile, "err", err)
		} else {
			d.broadcaster.Emit(api.Event{
				Kind:      "agent_spawned",
				Message:   "fixer spawned for " + a.PlanFile,
				Repo:      e.Path,
				PlanFile:  a.PlanFile,
				AgentType: "fixer",
			})
		}
	case loop.PausePlanAgentAction:
		if err := d.spawner.KillAgent(e.Path, a.PlanFile, a.AgentType); err != nil {
			d.logger.Error("kill agent failed", "plan", a.PlanFile, "type", a.AgentType, "err", err)
		} else {
			d.broadcaster.Emit(api.Event{
				Kind:      "agent_killed",
				Message:   a.AgentType + " killed for " + a.PlanFile,
				Repo:      e.Path,
				PlanFile:  a.PlanFile,
				AgentType: a.AgentType,
			})
		}
	case loop.AdvanceWaveAction:
		d.logger.Info("wave advanced", "plan", a.PlanFile, "wave", a.Wave, "repo", e.Path)
		d.broadcaster.Emit(api.Event{
			Kind:     "wave_advanced",
			Message:  fmt.Sprintf("wave %d started for %s", a.Wave, a.PlanFile),
			Repo:     e.Path,
			PlanFile: a.PlanFile,
		})
	case loop.TransitionAction:
		d.logger.Info("fsm transition", "plan", a.PlanFile, "event", a.Event, "repo", e.Path)
		d.broadcaster.Emit(api.Event{
			Kind:     "transition_applied",
			Message:  fmt.Sprintf("fsm event %v for %s", a.Event, a.PlanFile),
			Repo:     e.Path,
			PlanFile: a.PlanFile,
		})
	case loop.ReviewApprovedAction:
		d.logger.Info("review approved", "plan", a.PlanFile, "repo", e.Path)
		d.broadcaster.Emit(api.Event{
			Kind:     "signal_processed",
			Message:  "review approved for " + a.PlanFile,
			Repo:     e.Path,
			PlanFile: a.PlanFile,
		})
	case loop.CreatePRAction:
		d.logger.Info("create pr requested", "plan", a.PlanFile, "repo", e.Path)
		d.broadcaster.Emit(api.Event{
			Kind:     "signal_processed",
			Message:  "create PR for " + a.PlanFile,
			Repo:     e.Path,
			PlanFile: a.PlanFile,
		})
	case loop.ReviewCycleLimitAction:
		d.logger.Warn("review-fix cycle limit reached",
			"plan", a.PlanFile, "cycle", a.Cycle, "limit", a.Limit, "repo", e.Path)
		d.broadcaster.Emit(api.Event{
			Kind:     "review_cycle_limit",
			Message:  fmt.Sprintf("review-fix cycle limit reached (%d/%d) for %s", a.Cycle, a.Limit, a.PlanFile),
			Repo:     e.Path,
			PlanFile: a.PlanFile,
		})
	default:
		d.logger.Debug("unhandled action", "kind", action.Kind(), "repo", e.Path)
	}
}

func (d *Daemon) pidLockPath() string {
	return d.cfg.SocketPath + ".pid"
}

func sharedWorktreePaths(repoPath string) []string {
	entries, err := os.ReadDir(filepath.Join(repoPath, ".worktrees"))
	if err != nil {
		return nil
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(repoPath, ".worktrees", entry.Name()))
	}
	return paths
}

func coderSpawnOpts(e RepoEntry, planFile, branch, feedback string) loop.SpawnOpts {
	return loop.SpawnOpts{
		PlanFile: planFile,
		RepoPath: e.Path,
		Project:  e.Project,
		Branch:   branch,
		Prompt:   feedback,
		Feedback: feedback,
	}
}

// RecoverSessions discovers orphaned kas_ tmux sessions and attempts to
// re-adopt them into the spawner's tracking map. This should be called once
// on daemon startup, before the first tick.
//
// The recovery process:
//  1. Calls spawner.DiscoverOrphanSessions() to list kas_ sessions not tracked
//     by the spawner.
//  2. Cross-references orphan session names with task filenames in each
//     registered repo's task store to identify sessions this daemon owns.
//  3. Logs and counts matched sessions. Full re-hydration of Instance objects
//     from stored task metadata is a future enhancement.
//
// Returns the number of sessions matched to known tasks and logged as recovered.
func (d *Daemon) RecoverSessions() (int, error) {
	orphans := d.spawner.DiscoverOrphanSessions()
	if len(orphans) == 0 {
		return 0, nil
	}

	d.logger.Info("discovered orphaned sessions", "count", len(orphans))

	// Build a set of orphan session titles (without the kas_ prefix) for lookup.
	orphanTitles := make(map[string]bool, len(orphans))
	for _, o := range orphans {
		orphanTitles[o.Title] = true
	}

	recovered := 0

	// Cross-reference orphan sessions with tasks in each registered repo.
	entries := d.repos.List()
	for _, e := range entries {
		if e.Store == nil {
			continue
		}
		tasks, err := e.Store.List(e.Project)
		if err != nil {
			d.logger.Warn("recover sessions: list tasks failed", "repo", e.Path, "err", err)
			continue
		}
		// Agent types to check for each task.
		agentSuffixes := []string{"coder", "reviewer", "elaborator"}
		for _, task := range tasks {
			planName := task.Filename
			for _, suffix := range agentSuffixes {
				title := planName + "-" + suffix
				if orphanTitles[title] {
					d.logger.Info("re-adopting orphan session",
						"session", title, "repo", e.Path, "plan", planName)
					recovered++
				}
			}
		}
	}

	return recovered, nil
}

// ---------------------------------------------------------------------------
// Legacy API (deprecated)
// ---------------------------------------------------------------------------

// RunDaemon is the legacy auto-accept daemon entry point. Kept for backward
// compatibility.
//
// Deprecated: use NewDaemon + Run instead.
func RunDaemon(cfg *config.Config) error {
	log.InfoLog.Printf("daemon starting")

	state := config.LoadState()

	storage, err := session.NewStorage(state)
	if err != nil {
		return fmt.Errorf("daemon: storage init failed: %w", err)
	}

	instances, err := storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("daemon: load instances failed: %w", err)
	}

	// Daemon always operates in auto-accept mode.
	for _, inst := range instances {
		inst.AutoYes = true
	}

	pollInterval := time.Duration(cfg.DaemonPollInterval) * time.Millisecond

	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		t := time.NewTimer(pollInterval)
		for {
			for _, inst := range instances {
				if inst.Started() && !inst.Paused() {
					if _, hasPrompt := inst.HasUpdated(); hasPrompt {
						inst.TapEnter()
					}
				}
			}

			// Check for stop before blocking on the timer.
			select {
			case <-stopCh:
				return
			default:
			}

			<-t.C
			t.Reset(pollInterval)
		}
	}()

	// Block until a termination signal arrives.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	received := <-sigCh
	log.InfoLog.Printf("daemon received signal: %s", received)

	// Signal the poll goroutine and wait for it to exit before persisting state.
	close(stopCh)
	wg.Wait()

	if saveErr := storage.SaveInstances(instances); saveErr != nil {
		log.ErrorLog.Printf("daemon: failed to save instances on shutdown: %v", saveErr)
	}

	return nil
}

// LaunchDaemon forks a detached daemon child process running
// `kas daemon start --foreground` and records its PID.
//
// The child is placed in a new session (Setsid=true on Unix) so it survives
// the parent terminal closing. Use `kas daemon start --foreground` directly
// when running under systemd.
func LaunchDaemon() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("daemon: could not resolve executable path: %w", err)
	}

	cmd := exec.Command(execPath, "daemon", "start", "--foreground")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("daemon: failed to start process: %w", err)
	}

	log.InfoLog.Printf("daemon child process started, PID=%d", cmd.Process.Pid)

	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("daemon: failed to locate config directory: %w", err)
	}

	pidPath := filepath.Join(cfgDir, "daemon.pid")
	pidContent := fmt.Sprintf("%d", cmd.Process.Pid)
	if err := os.WriteFile(pidPath, []byte(pidContent), 0644); err != nil {
		return fmt.Errorf("daemon: failed to write PID file: %w", err)
	}

	return nil
}

// StopDaemon terminates a running daemon process identified by its PID file.
// If no PID file exists the function returns without error (daemon is not running).
func StopDaemon() error {
	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("daemon: failed to locate config directory: %w", err)
	}

	pidPath := filepath.Join(cfgDir, "daemon.pid")
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("daemon: could not read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		return fmt.Errorf("daemon: malformed PID file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("daemon: could not find process %d: %w", pid, err)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("daemon: kill process %d failed: %w", pid, err)
	}

	if err := os.Remove(pidPath); err != nil {
		return fmt.Errorf("daemon: failed to remove PID file: %w", err)
	}

	log.InfoLog.Printf("daemon stopped (PID=%d)", pid)
	return nil
}
