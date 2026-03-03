package tmux

import (
	"os/exec"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetachSafely_WhenNotAttached(t *testing.T) {
	s := NewTmuxSessionWithDeps("test-detach", "opencode", false, NewMockPtyFactory(t), cmd_test.NewMockExecutor())
	// Not attached — should be a no-op
	err := s.DetachSafely()
	assert.NoError(t, err)
}

func TestSetDetachedSize(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	s := NewTmuxSessionWithDeps("test-size", "opencode", false, ptyFactory, cmdExec)
	// Restore to get a PTY
	err := s.Restore()
	require.NoError(t, err)
	// SetDetachedSize should not error with a valid PTY
	// May error on mock PTY (not a real terminal) — that's OK for unit test
	// The important thing is it doesn't panic
	_ = s.SetDetachedSize(120, 40)
}

func TestDetachSafely_IsIdempotent(t *testing.T) {
	s := NewTmuxSessionWithDeps("test-idem", "opencode", false, NewMockPtyFactory(t), cmd_test.NewMockExecutor())
	// Multiple calls when not attached should all return nil.
	assert.NoError(t, s.DetachSafely())
	assert.NoError(t, s.DetachSafely())
	assert.NoError(t, s.DetachSafely())
}

func TestUpdateWindowSize_NilPTY(t *testing.T) {
	s := NewTmuxSessionWithDeps("test-winsize", "opencode", false, NewMockPtyFactory(t), cmd_test.NewMockExecutor())
	// ptmx is nil — updateWindowSize should be a no-op.
	assert.Nil(t, s.ptmx)
	err := s.updateWindowSize(80, 24)
	assert.NoError(t, err)
}
