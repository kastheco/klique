package tmux

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturePaneContent(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("pane output here"), nil
			}
			return []byte(""), nil
		},
	}
	s := NewTmuxSessionWithDeps("capture-test", "opencode", false, &MockPtyFactory{}, cmdExec)

	content, err := s.CapturePaneContent()
	require.NoError(t, err)
	assert.Equal(t, "pane output here", content)
}

func TestCapturePaneContent_UsesCorrectTmuxArgs(t *testing.T) {
	var capturedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			capturedCmds = append(capturedCmds, cmd.String())
			return []byte(""), nil
		},
	}
	s := NewTmuxSessionWithDeps("ct", "opencode", false, &MockPtyFactory{}, cmdExec)

	_, err := s.CapturePaneContent()
	require.NoError(t, err)
	require.Len(t, capturedCmds, 1)
	assert.Contains(t, capturedCmds[0], "capture-pane")
	assert.Contains(t, capturedCmds[0], "-p")
	assert.Contains(t, capturedCmds[0], "-e")
	assert.Contains(t, capturedCmds[0], "-J")
	assert.Contains(t, capturedCmds[0], "kas_ct")
}

func TestCapturePaneContentWithOptions(t *testing.T) {
	var capturedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			capturedCmds = append(capturedCmds, cmd.String())
			return []byte("scrollback content"), nil
		},
	}
	s := NewTmuxSessionWithDeps("opts-test", "opencode", false, &MockPtyFactory{}, cmdExec)

	content, err := s.CapturePaneContentWithOptions("-1000", "0")
	require.NoError(t, err)
	assert.Equal(t, "scrollback content", content)
	require.Len(t, capturedCmds, 1)
	assert.Contains(t, capturedCmds[0], "-S")
	assert.Contains(t, capturedCmds[0], "-1000")
	assert.Contains(t, capturedCmds[0], "-E")
	assert.Contains(t, capturedCmds[0], "0")
}

func TestGetPanePID(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "display-message") {
				return []byte("12345\n"), nil
			}
			return []byte(""), nil
		},
	}
	s := NewTmuxSessionWithDeps("pid-test", "opencode", false, &MockPtyFactory{}, cmdExec)

	pid, err := s.GetPanePID()
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)
}

func TestGetPanePID_UsesCorrectFormat(t *testing.T) {
	var capturedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			capturedCmds = append(capturedCmds, cmd.String())
			return []byte("999\n"), nil
		},
	}
	s := NewTmuxSessionWithDeps("pid-fmt", "opencode", false, &MockPtyFactory{}, cmdExec)

	_, err := s.GetPanePID()
	require.NoError(t, err)
	require.Len(t, capturedCmds, 1)
	assert.Contains(t, capturedCmds[0], "display-message")
	assert.Contains(t, capturedCmds[0], "#{pane_pid}")
	assert.Contains(t, capturedCmds[0], "kas_pid-fmt")
}

func TestGetSanitizedName(t *testing.T) {
	s := NewTmuxSessionWithDeps("my-session", "opencode", false, &MockPtyFactory{}, cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return nil, nil },
	})
	assert.Equal(t, "kas_my-session", s.GetSanitizedName())
}

func TestHasUpdated_ContentChange(t *testing.T) {
	callCount := 0
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				return []byte("content A"), nil
			default:
				return []byte("content B"), nil
			}
		},
	}
	s := NewTmuxSessionWithDeps("upd-test", "opencode", false, &MockPtyFactory{}, cmdExec)
	s.monitor = NewStatusMonitor()

	// First capture: always reports updated (new content).
	updated1, _ := s.HasUpdated()
	assert.True(t, updated1, "first capture should report updated")

	// Second capture with different content: should still report updated.
	updated2, _ := s.HasUpdated()
	assert.True(t, updated2, "changed content should report updated")
}

func TestHasUpdated_StableContentDebounces(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("stable content"), nil
		},
	}
	s := NewTmuxSessionWithDeps("stable-test", "opencode", false, &MockPtyFactory{}, cmdExec)
	s.monitor = NewStatusMonitor()

	// First call: content is new — reports updated.
	s.HasUpdated()

	// Calls within the debounce window (debounceThreshold-1 more unchanged ticks).
	for i := 0; i < debounceThreshold-1; i++ {
		updated, _ := s.HasUpdated()
		assert.True(t, updated, "should still report updated within debounce window (tick %d)", i)
	}

	// One more tick pushes past the debounce threshold — reports not updated.
	updated, _ := s.HasUpdated()
	assert.False(t, updated, "should report not-updated after debounce threshold")
}

func TestHasUpdatedWithContent_ReturnsContent(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Ask anything here"), nil
		},
	}
	s := NewTmuxSessionWithDeps("wc-test", "opencode", false, &MockPtyFactory{}, cmdExec)
	s.monitor = NewStatusMonitor()

	_, _, content, captured := s.HasUpdatedWithContent()
	assert.True(t, captured)
	assert.Equal(t, "Ask anything here", content)
}

func TestHasUpdatedWithContent_DetectsOpenCodePrompt(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// opencode idle: no "esc interrupt" → prompt detected
			return []byte("Ask anything\nsome output here"), nil
		},
	}
	s := NewTmuxSessionWithDeps("prompt-det", "opencode", false, &MockPtyFactory{}, cmdExec)
	s.monitor = NewStatusMonitor()

	_, hasPrompt, _, _ := s.HasUpdatedWithContent()
	assert.True(t, hasPrompt, "opencode idle pane should have prompt detected")
}

func TestHasUpdatedWithContent_NoPromptWhileRunning(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// opencode active: "esc interrupt" present → not at prompt
			return []byte("Working on it... esc interrupt"), nil
		},
	}
	s := NewTmuxSessionWithDeps("running-det", "opencode", false, &MockPtyFactory{}, cmdExec)
	s.monitor = NewStatusMonitor()

	_, hasPrompt, _, _ := s.HasUpdatedWithContent()
	assert.False(t, hasPrompt, "opencode running pane should not have prompt detected")
}
