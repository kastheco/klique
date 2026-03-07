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
	cfg       *DaemonConfig
	repos     *RepoManager
	spawner   *TmuxSpawner
	logger    *slog.Logger
	pidLock   *PIDLock
	mu        sync.RWMutex
	startedAt time.Time
}

// daemonStateAdapter adapts the Daemon to the api.StateProvider interface.
type daemonStateAdapter struct {
	d *Daemon
}

func (a *daemonStateAdapter) Status() api.StatusResponse {
	a.d.mu.RLock()
	defer a.d.mu.RUnlock()
	uptime := ""
	if !a.d.startedAt.IsZero() {
		uptime = time.Since(a.d.startedAt).Round(time.Second).String()
	}
	repos := a.d.repos.List()
	repoStatuses := make([]api.RepoStatus, len(repos))
	for i, r := range repos {
		repoStatuses[i] = api.RepoStatus{Path: r.Path, Project: r.Project}
	}
	return api.StatusResponse{
		Running:   true,
		Repos:     repoStatuses,
		RepoCount: len(repoStatuses),
		Uptime:    uptime,
	}
}

func (a *daemonStateAdapter) ListRepos() []api.RepoStatus {
	repos := a.d.repos.List()
	out := make([]api.RepoStatus, len(repos))
	for i, r := range repos {
		out[i] = api.RepoStatus{Path: r.Path, Project: r.Project}
	}
	return out
}

func (a *daemonStateAdapter) AddRepo(path string) error {
	return a.d.repos.Add(path)
}

func (a *daemonStateAdapter) RemoveRepo(project string) error {
	return a.d.repos.RemoveByProject(project)
}

func (a *daemonStateAdapter) ListPlans(_ string) ([]taskstore.TaskEntry, error) {
	return nil, nil
}

func (a *daemonStateAdapter) ListInstances(_ string) []api.InstanceStatus {
	return nil
}

func (a *daemonStateAdapter) EventStream() <-chan api.Event {
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

	d := &Daemon{
		cfg:     cfg,
		repos:   NewRepoManager(),
		spawner: newTmuxSpawner(logger),
		logger:  logger,
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

	// Remove any stale socket file before listening.
	_ = os.Remove(d.cfg.SocketPath)

	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("daemon: listen unix %s: %w", d.cfg.SocketPath, err)
	}
	defer func() {
		ln.Close()
		_ = os.Remove(d.cfg.SocketPath)
	}()

	// Build and start the HTTP server on the control socket.
	state := &daemonStateAdapter{d: d}
	handler := api.NewHandler(state)
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
			return nil

		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

// tick executes one poll cycle across all registered repos.
func (d *Daemon) tick(ctx context.Context) {
	entries := d.repos.List()
	for _, e := range entries {
		if e.Store == nil || e.Processor == nil {
			// Processor requires a store; skip repos whose store is unavailable.
			continue
		}

		scan := loop.ScanAllSignals(e.Path, nil)

		// Use the persistent per-repo processor so wave orchestrator state
		// survives between ticks (prevents duplicate AdvanceWave emission).
		actions := e.Processor.Tick(scan)

		// Consume all signals now that the processor has validated them.
		// This prevents re-processing the same signals on the next tick.
		for _, sig := range scan.FSMSignals {
			taskfsm.ConsumeSignal(sig)
		}
		for _, ts := range scan.TaskSignals {
			taskfsm.ConsumeTaskSignal(ts)
		}
		for _, ws := range scan.WaveSignals {
			taskfsm.ConsumeWaveSignal(ws)
		}
		for _, es := range scan.ElaborationSignals {
			taskfsm.ConsumeElaborationSignal(es)
		}

		for _, action := range actions {
			d.executeAction(ctx, e, action)
		}
	}
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
			Branch:   branchFor(a.PlanFile),
		}
		if err := d.spawner.SpawnReviewer(ctx, opts); err != nil {
			d.logger.Error("spawn reviewer failed", "plan", a.PlanFile, "err", err)
		}
	case loop.SpawnCoderAction:
		opts := loop.SpawnOpts{
			PlanFile: a.PlanFile,
			RepoPath: e.Path,
			Branch:   branchFor(a.PlanFile),
			Feedback: a.Feedback,
		}
		if err := d.spawner.SpawnCoder(ctx, opts); err != nil {
			d.logger.Error("spawn coder failed", "plan", a.PlanFile, "err", err)
		}
	case loop.SpawnElaboratorAction:
		opts := loop.SpawnOpts{
			PlanFile: a.PlanFile,
			RepoPath: e.Path,
		}
		if err := d.spawner.SpawnElaborator(ctx, opts); err != nil {
			d.logger.Error("spawn elaborator failed", "plan", a.PlanFile, "err", err)
		}
	case loop.PausePlanAgentAction:
		if err := d.spawner.KillAgent(a.PlanFile, a.AgentType); err != nil {
			d.logger.Error("kill agent failed", "plan", a.PlanFile, "type", a.AgentType, "err", err)
		}
	case loop.ReviewApprovedAction:
		d.logger.Info("review approved", "plan", a.PlanFile, "repo", e.Path)
	case loop.CreatePRAction:
		d.logger.Info("create pr requested", "plan", a.PlanFile, "repo", e.Path)
	default:
		d.logger.Debug("unhandled action", "kind", action.Kind(), "repo", e.Path)
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
