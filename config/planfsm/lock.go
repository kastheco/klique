//go:build !windows

package planfsm

import (
	"os"
	"path/filepath"
	"syscall"
)

const lockFile = ".plan-state.lock"

// withLock acquires an exclusive file lock, runs fn, then releases the lock.
func (m *PlanStateMachine) withLock(fn func() error) error {
	lockPath := filepath.Join(m.dir, lockFile)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fn() // fallback: run without lock if we can't create lock file
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fn() // fallback: run without lock
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	return fn()
}
