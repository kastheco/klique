package cmd

import (
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type Executor interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

type Exec struct{}

func (e Exec) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

func (e Exec) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func MakeExecutor() Executor {
	return Exec{}
}

func ToString(cmd *exec.Cmd) string {
	if cmd == nil {
		return "<nil>"
	}
	return strings.Join(cmd.Args, " ")
}

// NewRootCmd returns a minimal root cobra command with all subcommands registered.
// Used in tests and for command discovery.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kas",
		Short: "kas - Manage multiple AI agents",
	}
	root.AddCommand(NewTaskCmd())
	root.AddCommand(NewServeCmd())
	return root
}
