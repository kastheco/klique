package git

import "os/exec"

// GHExecutor is the exported mirror of the package-internal ghExecutor interface.
// It abstracts `gh` command execution so external test packages can supply mock
// implementations without creating an import cycle
// (cmd → session/git → cmd is forbidden; this interface breaks that chain).
type GHExecutor interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

// SetGHExec replaces the package-level gh executor used for all CLI invocations
// and returns a function that restores the original. Intended for use in tests
// from external packages (e.g. package daemon) that need to mock gh CLI calls
// without spinning up real subprocesses.
//
// Typical usage:
//
//	t.Cleanup(gitpkg.SetGHExec(myMock))
func SetGHExec(e GHExecutor) func() {
	orig := ghExec
	ghExec = e
	return func() { ghExec = orig }
}
