package cmd_test

import (
	"os/exec"
)

type MockCmdExec struct {
	RunFunc    func(cmd *exec.Cmd) error
	OutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (e MockCmdExec) Run(cmd *exec.Cmd) error {
	return e.RunFunc(cmd)
}

func (e MockCmdExec) Output(cmd *exec.Cmd) ([]byte, error) {
	return e.OutputFunc(cmd)
}

// NewMockExecutor returns a *MockCmdExec with no-op defaults.
// Callers may override RunFunc and OutputFunc before use.
func NewMockExecutor() *MockCmdExec {
	return &MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, nil
		},
	}
}
