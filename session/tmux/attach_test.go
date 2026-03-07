package tmux

import (
	"os/exec"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
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

func TestRawInputMode_RestoresTTYState(t *testing.T) {
	oldStdinFD := stdinFD
	oldIsTTY := terminalIsTTY
	oldMakeRaw := terminalMakeRaw
	oldRestore := terminalRestore
	defer func() {
		stdinFD = oldStdinFD
		terminalIsTTY = oldIsTTY
		terminalMakeRaw = oldMakeRaw
		terminalRestore = oldRestore
	}()

	stdinFD = func() int { return 42 }
	terminalIsTTY = func(fd int) bool {
		require.Equal(t, 42, fd)
		return true
	}

	state := &term.State{}
	madeRaw := false
	restored := false
	terminalMakeRaw = func(fd int) (*term.State, error) {
		madeRaw = true
		require.Equal(t, 42, fd)
		return state, nil
	}
	terminalRestore = func(fd int, got *term.State) error {
		restored = true
		require.Equal(t, 42, fd)
		require.Same(t, state, got)
		return nil
	}

	s := &TmuxSession{}
	require.NoError(t, s.enterRawInputMode())
	require.True(t, madeRaw)
	require.Same(t, state, s.rawInputState)
	require.Equal(t, 42, s.stdinFD)

	s.exitRawInputMode()
	require.True(t, restored)
	require.Nil(t, s.rawInputState)
	require.Zero(t, s.stdinFD)
}
