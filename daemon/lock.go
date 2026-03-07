package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// PIDLock represents an acquired PID lock file. It must be released via
// Release() when the daemon shuts down.
type PIDLock struct {
	path string
}

// AcquirePIDLock atomically creates a PID lock file at path. If the file
// already exists and the recorded PID is still live, an error is returned.
// Stale PID files (process no longer running) are silently removed and
// re-acquired.
func AcquirePIDLock(path string) (*PIDLock, error) {
	pid := os.Getpid()

	// Try atomic exclusive create first.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		// Success: write our PID and return.
		if _, werr := fmt.Fprintf(f, "%d\n", pid); werr != nil {
			f.Close()
			os.Remove(path)
			return nil, fmt.Errorf("pidlock: write pid: %w", werr)
		}
		f.Close()
		return &PIDLock{path: path}, nil
	}

	if !os.IsExist(err) {
		return nil, fmt.Errorf("pidlock: open: %w", err)
	}

	// File exists — check if the recorded PID is still alive.
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		return nil, fmt.Errorf("pidlock: read existing pid file: %w", rerr)
	}

	var existingPID int
	if _, serr := fmt.Sscanf(string(raw), "%d", &existingPID); serr != nil {
		// Malformed PID file — treat as stale.
		existingPID = 0
	}

	if existingPID > 0 && isProcessAlive(existingPID) {
		return nil, fmt.Errorf("pidlock: daemon already running (PID %d)", existingPID)
	}

	// Stale lock — remove and retry.
	if rerr := os.Remove(path); rerr != nil && !os.IsNotExist(rerr) {
		return nil, fmt.Errorf("pidlock: remove stale lock: %w", rerr)
	}

	f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("pidlock: acquire after stale removal: %w", err)
	}
	if _, werr := fmt.Fprintf(f, "%d\n", pid); werr != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("pidlock: write pid after stale removal: %w", werr)
	}
	f.Close()
	return &PIDLock{path: path}, nil
}

// Release removes the PID lock file. It is safe to call Release multiple times.
func (l *PIDLock) Release() error {
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("pidlock: release: %w", err)
	}
	return nil
}

// isProcessAlive returns true if the process with the given PID is alive by
// sending signal 0 (no-op probe).
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
