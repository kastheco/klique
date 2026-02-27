//go:build windows

package planfsm

// withLock on Windows runs fn directly without file locking (syscall.Flock is unavailable).
func (m *PlanStateMachine) withLock(fn func() error) error {
	return fn()
}
