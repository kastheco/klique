package tmux

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionExists_Present verifies that a zero exit code means the session exists.
func TestSessionExists_Present(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return nil // has-session succeeds
		},
	}
	ok, err := SessionExists(ex, "kas_test")
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestSessionExists_Missing verifies that *exec.ExitError is treated as (false, nil).
func TestSessionExists_Missing(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return &exec.ExitError{} // has-session exits non-zero
		},
	}
	ok, err := SessionExists(ex, "kas_nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestLayoutPaneID_Happy verifies that "VAR=value\n" output is parsed correctly.
func TestLayoutPaneID_Happy(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("KASMOS_WORKSPACE_PANE=%42\n"), nil
		},
	}
	paneID, err := LayoutPaneID(ex, "kas_outer", "KASMOS_WORKSPACE_PANE")
	require.NoError(t, err)
	assert.Equal(t, "%42", paneID)
}

// TestLayoutPaneID_NoValue verifies that an empty value after "=" returns an error.
func TestLayoutPaneID_NoValue(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("KASMOS_WORKSPACE_PANE=\n"), nil
		},
	}
	_, err := LayoutPaneID(ex, "kas_outer", "KASMOS_WORKSPACE_PANE")
	assert.Error(t, err)
}

// TestSwapRightPaneToSession_Happy verifies the correct tmux commands are issued.
func TestSwapRightPaneToSession_Happy(t *testing.T) {
	var ranCmds []string
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, strings.Join(cmd.Args, " "))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			arg := strings.Join(cmd.Args, " ")
			switch {
			case strings.Contains(arg, "show-environment"):
				return []byte("KASMOS_WORKSPACE_PANE=%1\n"), nil
			case strings.Contains(arg, "display-message"):
				return []byte("%99\n"), nil
			}
			return nil, nil
		},
	}

	err := SwapRightPaneToSession(ex, "kas_outer", "kas_agent")
	require.NoError(t, err)
	require.Len(t, ranCmds, 1)
	assert.Contains(t, ranCmds[0], "swap-pane")
	assert.Contains(t, ranCmds[0], "%1")
	assert.Contains(t, ranCmds[0], "%99")
}

// TestSwapRightPaneToSession_MissingEnv verifies that a missing KASMOS_WORKSPACE_PANE
// env var causes SwapRightPaneToSession to return an error.
func TestSwapRightPaneToSession_MissingEnv(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, &exec.ExitError{}
		},
	}
	err := SwapRightPaneToSession(ex, "kas_outer", "kas_agent")
	assert.Error(t, err)
}

// TestSwapRightPaneToWorkspace_AlreadyThere verifies that no swap-pane command
// is issued when the workspace pane is already at position :0.1.
func TestSwapRightPaneToWorkspace_AlreadyThere(t *testing.T) {
	var swapCalled bool
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(strings.Join(cmd.Args, " "), "swap-pane") {
				swapCalled = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Both show-environment and display-message return the same pane ID.
			return []byte("KASMOS_WORKSPACE_PANE=%1\n"), nil
		},
	}
	// Override display-message response to match the workspace pane ID.
	ex.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		arg := strings.Join(cmd.Args, " ")
		if strings.Contains(arg, "show-environment") {
			return []byte("KASMOS_WORKSPACE_PANE=%1\n"), nil
		}
		// display-message for :0.1 returns same ID → already in place
		return []byte("%1\n"), nil
	}

	err := SwapRightPaneToWorkspace(ex, "kas_outer")
	require.NoError(t, err)
	assert.False(t, swapCalled, "swap-pane should not be called when pane is already at :0.1")
}

// TestSwapRightPaneToWorkspace_NeedsSwap verifies that swap-pane is called
// when the workspace pane is not at position :0.1.
func TestSwapRightPaneToWorkspace_NeedsSwap(t *testing.T) {
	var swapCalled bool
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(strings.Join(cmd.Args, " "), "swap-pane") {
				swapCalled = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			arg := strings.Join(cmd.Args, " ")
			if strings.Contains(arg, "show-environment") {
				return []byte("KASMOS_WORKSPACE_PANE=%1\n"), nil
			}
			// display-message for :0.1 returns a different pane ID
			return []byte("%99\n"), nil
		},
	}

	err := SwapRightPaneToWorkspace(ex, "kas_outer")
	require.NoError(t, err)
	assert.True(t, swapCalled, "swap-pane should be called when workspace pane is not at :0.1")
}

// TestSwapRightPaneToWorkspace_MissingEnv verifies idempotent fallback when
// KASMOS_WORKSPACE_PANE is not set in the session environment.
func TestSwapRightPaneToWorkspace_MissingEnv(t *testing.T) {
	var swapCalled bool
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(strings.Join(cmd.Args, " "), "swap-pane") {
				swapCalled = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, &exec.ExitError{} // show-environment fails
		},
	}

	err := SwapRightPaneToWorkspace(ex, "kas_outer")
	require.NoError(t, err, "missing env should return nil, not an error")
	assert.False(t, swapCalled, "swap-pane should not be called when env is missing")
}
