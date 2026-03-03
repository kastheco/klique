package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
)

// RunDaemon is the entry point for daemon mode. It polls all active sessions
// and automatically accepts prompts on their behalf. The daemon shuts down
// cleanly when it receives SIGINT or SIGTERM.
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

	// Rate-limited logger: identical errors are suppressed for 60 s at a time.
	throttle := log.NewEvery(60 * time.Second)

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
						if diffErr := inst.UpdateDiffStats(); diffErr != nil {
							if throttle.ShouldLog() {
								log.WarningLog.Printf("diff stats update failed for %s: %v", inst.Title, diffErr)
							}
						}
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
