package cmd

import (
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// Executor abstracts command execution for testability.
type Executor interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

// Exec is the real executor that delegates to the underlying os/exec.Cmd methods.
type Exec struct{}

// Run executes the command and waits for it to complete.
func (e Exec) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

// Output runs the command and returns its standard output.
func (e Exec) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

// MakeExecutor returns the default real executor.
func MakeExecutor() Executor {
	return Exec{}
}

// ToString returns a human-readable representation of a command's arguments.
// Returns "<nil>" when cmd is nil.
func ToString(cmd *exec.Cmd) string {
	if cmd == nil {
		return "<nil>"
	}
	return strings.Join(cmd.Args, " ")
}

// NewRootCmd builds the root cobra command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kas",
		Short: "kas - Manage multiple AI agents",
	}
	root.AddCommand(NewTaskCmd())
	root.AddCommand(NewServeCmd())
	return root
}
