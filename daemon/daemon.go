package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/config"
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
	cfg     *DaemonConfig
	repos   *RepoManager
	spawner *TmuxSpawner
	logger  *slog.Logger
	pidLock *PIDLock
	mu      sync.RWMutex
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
//  2. On each tick: scans all registered repos for signal files, feeds results
//     to per-repo Processor.Tick(), and executes the returned actions.
//  3. On context cancellation: releases the PID lock and returns nil.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("daemon starting", "poll_interval", d.cfg.PollInterval)

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon shutting down")
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
		scan := loop.ScanAllSignals(e.Path, nil)

		if e.Store == nil {
			// Processor requires a store; skip repos whose store is unavailable.
			continue
		}

		// Build a processor for this entry using the per-repo store.
		proc := loop.NewProcessor(loop.ProcessorConfig{
			Store:   e.Store,
			Project: e.Project,
		})

		actions := proc.Tick(scan)
		for _, action := range actions {
			d.executeAction(ctx, e, action)
		}
	}
}

// executeAction dispatches a single action to the configured spawner.
func (d *Daemon) executeAction(ctx context.Context, e RepoEntry, action loop.Action) {
	switch a := action.(type) {
	case loop.SpawnReviewerAction:
		if err := d.spawner.SpawnReviewer(ctx, loop.SpawnOpts{PlanFile: a.PlanFile}); err != nil {
			d.logger.Error("spawn reviewer failed", "plan", a.PlanFile, "err", err)
		}
	case loop.SpawnCoderAction:
		if err := d.spawner.SpawnCoder(ctx, loop.SpawnOpts{PlanFile: a.PlanFile, Feedback: a.Feedback}); err != nil {
			d.logger.Error("spawn coder failed", "plan", a.PlanFile, "err", err)
		}
	case loop.SpawnElaboratorAction:
		if err := d.spawner.SpawnElaborator(ctx, loop.SpawnOpts{PlanFile: a.PlanFile}); err != nil {
			d.logger.Error("spawn elaborator failed", "plan", a.PlanFile, "err", err)
		}
	case loop.PausePlanAgentAction:
		if err := d.spawner.KillAgent(a.PlanFile, a.AgentType); err != nil {
			d.logger.Error("kill agent failed", "plan", a.PlanFile, "type", a.AgentType, "err", err)
		}
	default:
		d.logger.Debug("unhandled action", "kind", action.Kind(), "repo", e.Path)
	}
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

// LaunchDaemon forks a detached daemon child process and records its PID.
func LaunchDaemon() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("daemon: could not resolve executable path: %w", err)
	}

	cmd := exec.Command(execPath, "--daemon")
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
