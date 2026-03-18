package fstools

import (
	"context"
	"os/exec"
)

// CmdRunner abstracts external command execution for testability.
type CmdRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner is the real CmdRunner that delegates to os/exec.
type ExecRunner struct{}

// Output runs name with args under the given context and returns its standard output.
func (r *ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
