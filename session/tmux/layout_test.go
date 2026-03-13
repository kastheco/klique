package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainSessionName(t *testing.T) {
	tests := []struct {
		name     string
		repoRoot string
		want     string
	}{
		{
			name:     "simple name",
			repoRoot: "/home/user/myrepo",
			want:     "kas_main_myrepo",
		},
		{
			name:     "name with dots",
			repoRoot: "/home/user/my.project",
			want:     "kas_main_my_project",
		},
		{
			name:     "name with spaces",
			repoRoot: "/home/user/my repo",
			want:     "kas_main_myrepo",
		},
		{
			name:     "nested path uses basename",
			repoRoot: "/home/user/work/kasmos",
			want:     "kas_main_kasmos",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MainSessionName(tc.repoRoot)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEnsureMainLayout_ExistingSession(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	repoRoot := "/home/user/myrepo"
	sessionName := MainSessionName(repoRoot)

	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			// has-session succeeds → session exists
			if strings.Contains(cmd.String(), "has-session") {
				return nil
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "show-environment") {
				args := cmd.Args
				varName := args[len(args)-1]
				switch varName {
				case "KASMOS_NAV_PANE":
					return []byte("KASMOS_NAV_PANE=%0\n"), nil
				case "KASMOS_WORKSPACE_PANE":
					return []byte("KASMOS_WORKSPACE_PANE=%1\n"), nil
				}
			}
			return []byte(""), nil
		},
	}

	layout, existed, err := EnsureMainLayout(ex, repoRoot, "kas tui --nav-only", 120, 40)
	require.NoError(t, err)
	assert.True(t, existed, "expected existing=true when session already exists")
	assert.Equal(t, sessionName, layout.SessionName)
	assert.Equal(t, "%0", layout.NavPaneID)
	assert.Equal(t, "%1", layout.WorkspacePaneID)
}

func TestEnsureMainLayout_NewSession(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	repoRoot := "/tmp/testrepo"
	sessionName := MainSessionName(repoRoot)

	var ranNewSession, ranSplitWindow bool
	var setEnvCalls []string
	var setOptCalls []string

	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			s := cmd.String()
			switch {
			case strings.Contains(s, "has-session"):
				// Session does not exist
				return fmt.Errorf("no server running")
			case strings.Contains(s, "set-environment"):
				// Record the env var name being set
				args := cmd.Args
				if len(args) >= 5 {
					setEnvCalls = append(setEnvCalls, args[len(args)-2])
				}
			case strings.Contains(s, "set-option"):
				args := cmd.Args
				if len(args) >= 5 {
					setOptCalls = append(setOptCalls, args[len(args)-2])
				}
			case strings.Contains(s, "kill-session"):
				// cleanup call — ok
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			s := cmd.String()
			if strings.Contains(s, "new-session") {
				ranNewSession = true
				// Return "sessionName|@0|%0"
				return []byte(sessionName + "|@0|%0\n"), nil
			}
			if strings.Contains(s, "split-window") {
				ranSplitWindow = true
				return []byte("%1\n"), nil
			}
			return []byte(""), nil
		},
	}

	layout, existed, err := EnsureMainLayout(ex, repoRoot, "kas tui --nav-only", 160, 50)
	require.NoError(t, err)
	assert.False(t, existed, "expected existed=false for new session")
	assert.True(t, ranNewSession, "expected new-session to be called")
	assert.True(t, ranSplitWindow, "expected split-window to be called")

	assert.Equal(t, sessionName, layout.SessionName)
	assert.Equal(t, "%0", layout.NavPaneID)
	assert.Equal(t, "%1", layout.WorkspacePaneID)
	assert.Contains(t, layout.WindowTarget, "@0")

	// Verify all required env vars were set
	assert.Contains(t, setEnvCalls, "KASMOS_LAYOUT")
	assert.Contains(t, setEnvCalls, "KASMOS_NAV_PANE")
	assert.Contains(t, setEnvCalls, "KASMOS_WORKSPACE_PANE")
	assert.Contains(t, setEnvCalls, "KASMOS_REPO_ROOT")

	// Verify tmux options were configured
	assert.Contains(t, setOptCalls, "mouse")
	assert.Contains(t, setOptCalls, "escape-time")
	assert.Contains(t, setOptCalls, "status")
	assert.Contains(t, setOptCalls, "status-position")
}

func TestEnsureMainLayout_NewSession_NavCmdHasLayoutEnv(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	var newSessionCmd string
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "new-session") {
				newSessionCmd = cmd.String()
				return []byte("kas_main_testrepo|@0|%0\n"), nil
			}
			if strings.Contains(cmd.String(), "split-window") {
				return []byte("%1\n"), nil
			}
			return []byte(""), nil
		},
	}

	_, _, err := EnsureMainLayout(ex, "/tmp/testrepo", "kas tui --nav-only", 120, 40)
	require.NoError(t, err)

	// The nav pane command must include KASMOS_LAYOUT=1 so the root kas command
	// recognises it is already inside the layout.
	assert.Contains(t, newSessionCmd, "KASMOS_LAYOUT=1")
	assert.Contains(t, newSessionCmd, "kas tui --nav-only")
}

func TestEnsureMainLayout_MissingTmux(t *testing.T) {
	// When has-session fails with a non-ExitError (e.g., exec: not found),
	// and LookPath would fail, EnsureMainLayout should return an error.
	// We can't easily mock LookPath, so this test verifies the error wrapping
	// by checking that new-session failures are propagated.
	log.Initialize(false)
	defer log.Close()

	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return fmt.Errorf("no session")
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "new-session") {
				return nil, fmt.Errorf("tmux: command not found")
			}
			return []byte(""), nil
		},
	}

	_, _, err := EnsureMainLayout(ex, "/tmp/repo", "kas tui --nav-only", 120, 40)
	// The error should be wrapped under "ensure main layout"
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ensure main layout")
}

func TestFocusPane(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	var selectPaneTarget string
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "select-pane") {
				args := cmd.Args
				selectPaneTarget = args[len(args)-1]
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		},
	}

	err := FocusPane(ex, "%42")
	require.NoError(t, err)
	assert.Equal(t, "%42", selectPaneTarget)
}

// --- ApplyStatusBar tests ---

func TestApplyStatusBar_SetsLeftRightAndStyleOptions(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	// tmux set-option command shape: tmux set-option -t <session> <opt> <val>
	// args indices:                  [0]  [1]         [2] [3]      [4]   [5]
	var optNames []string
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			args := cmd.Args
			if strings.Contains(cmd.String(), "set-option") && len(args) >= 5 {
				optNames = append(optNames, args[4])
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		},
	}

	render := StatusBarRender{Left: "kasmos v1.0.0", Right: "main · myproj"}
	err := ApplyStatusBar(ex, "kas_main_testrepo", render)
	require.NoError(t, err)

	assert.Contains(t, optNames, "status-style")
	assert.Contains(t, optNames, "status-left-length")
	assert.Contains(t, optNames, "status-right-length")
	assert.Contains(t, optNames, "status-left")
	assert.Contains(t, optNames, "status-right")
}

func TestApplyStatusBar_SetsCorrectLeftAndRightValues(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	optValues := make(map[string]string)
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			args := cmd.Args
			if strings.Contains(cmd.String(), "set-option") && len(args) >= 6 {
				optName := args[4]
				optVal := args[5]
				optValues[optName] = optVal
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		},
	}

	render := StatusBarRender{Left: "#[bold]kasmos#[default]", Right: "main"}
	err := ApplyStatusBar(ex, "kas_main_testrepo", render)
	require.NoError(t, err)

	assert.Equal(t, "#[bold]kasmos#[default]", optValues["status-left"])
	assert.Equal(t, "main", optValues["status-right"])
}

func TestApplyStatusBar_ReturnsFirstErrorFromSetOption(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	callCount := 0
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			callCount++
			if callCount == 1 && strings.Contains(cmd.String(), "status-style") {
				return fmt.Errorf("tmux: unknown option: status-style")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		},
	}

	render := StatusBarRender{Left: "kasmos", Right: ""}
	err := ApplyStatusBar(ex, "kas_main_testrepo", render)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status-style")
	// Remaining options are still attempted (loop continues after first error).
	assert.Greater(t, callCount, 1)
}

func TestApplyStatusBar_EmptyRender_SetsEmptyStrings(t *testing.T) {
	log.Initialize(false)
	defer log.Close()

	optValues := make(map[string]string)
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			args := cmd.Args
			if strings.Contains(cmd.String(), "set-option") && len(args) >= 6 {
				optValues[args[4]] = args[5]
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil
		},
	}

	err := ApplyStatusBar(ex, "kas_main_testrepo", StatusBarRender{})
	require.NoError(t, err)
	assert.Equal(t, "", optValues["status-left"])
	assert.Equal(t, "", optValues["status-right"])
}

func TestShowSessionEnvVar_ParsesValue(t *testing.T) {
	ex := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("KASMOS_NAV_PANE=%0\n"), nil
		},
	}
	val, err := showSessionEnvVar(ex, "kas_main_test", "KASMOS_NAV_PANE")
	require.NoError(t, err)
	assert.Equal(t, "%0", val)
}
