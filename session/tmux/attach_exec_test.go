package tmux

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion: *AttachExecCommand must satisfy tea.ExecCommand.
func TestAttachExecCommand_ImplementsExecCommand(t *testing.T) {
	var _ tea.ExecCommand = (*AttachExecCommand)(nil)
}

type stubAttacher struct {
	attach func() (chan struct{}, error)
}

func (s stubAttacher) Attach() (chan struct{}, error) {
	return s.attach()
}

func TestAttachExecCommand_RunWaitsForDetach(t *testing.T) {
	detachCh := make(chan struct{})
	called := false

	cmd := NewAttachExecCommand(stubAttacher{attach: func() (chan struct{}, error) {
		called = true
		return detachCh, nil
	}})

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	require.Eventually(t, func() bool { return called }, 200*time.Millisecond, 10*time.Millisecond)

	select {
	case err := <-done:
		t.Fatalf("Run returned before detach: %v", err)
	default:
	}

	close(detachCh)
	require.NoError(t, <-done)
}

func TestAttachExecCommand_RunReturnsAttachError(t *testing.T) {
	want := errors.New("attach failed")
	cmd := NewAttachExecCommand(stubAttacher{attach: func() (chan struct{}, error) {
		return nil, want
	}})

	err := cmd.Run()
	require.Error(t, err)
	require.ErrorIs(t, err, want)
}

func TestAttachExecCommand_SetStdinStdoutStderr_NoPanic(t *testing.T) {
	cmd := NewAttachExecCommand(stubAttacher{attach: func() (chan struct{}, error) {
		return make(chan struct{}), nil
	}})
	cmd.SetStdin(nil)
	cmd.SetStdout(nil)
	cmd.SetStderr(nil)
}
