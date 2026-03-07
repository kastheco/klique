package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToKasTmuxName(t *testing.T) {
	assert.Equal(t, "kas_asdf", toKasTmuxName("asdf"))
	assert.Equal(t, "kas_asdf__asdf", toKasTmuxName("a sd f . . asdf"))
}

func TestNewTmuxSession_SanitizesName(t *testing.T) {
	s := NewTmuxSession("my.session", "claude", false)
	assert.Equal(t, "kas_my_session", s.GetSanitizedName())
}

func TestSetAgentType(t *testing.T) {
	s := NewTmuxSession("test", "opencode", false)
	s.SetAgentType("  planner  ")
	// agentType should be trimmed — verified indirectly via Start command construction
	assert.Equal(t, "planner", s.agentType)
}

func TestSetTaskEnv(t *testing.T) {
	s := NewTmuxSession("test", "claude", false)
	s.SetTaskEnv(3, 2, 4)
	assert.Equal(t, 3, s.taskNumber)
	assert.Equal(t, 2, s.waveNumber)
	assert.Equal(t, 4, s.peerCount)
}

func TestNewReset_PreservesDeps(t *testing.T) {
	pty := NewMockPtyFactory(t)
	exec := cmd_test.NewMockExecutor()
	s := NewTmuxSessionWithDeps("orig", "claude", false, pty, exec)
	reset := s.NewReset("new", "opencode", true)
	assert.Equal(t, "kas_new", reset.GetSanitizedName())
}

func TestNewTmuxSessionFromExisting(t *testing.T) {
	s := NewTmuxSessionFromExisting("kas_orphan", "claude", false)
	assert.Equal(t, "kas_orphan", s.GetSanitizedName())
}

func TestToKasTmuxNamePublic(t *testing.T) {
	assert.Equal(t, "kas_foo", ToKasTmuxNamePublic("foo"))
}

func TestCleanupSessions_KillsKasAndLegacy(t *testing.T) {
	var killed []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			args := cmd.Args
			if len(args) >= 2 && args[1] == "kill-session" {
				for i, arg := range args {
					if arg == "-t" && i+1 < len(args) {
						killed = append(killed, args[i+1])
					}
				}
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(
				"kas_session1: 1 windows\n" +
					"klique_legacy: 1 windows\n" +
					"hivemind_old: 1 windows\n" +
					"unrelated: 1 windows\n",
			), nil
		},
	}
	err := CleanupSessions(cmdExec)
	require.NoError(t, err)
	assert.Len(t, killed, 3)
	assert.NotContains(t, killed, "unrelated")
}

func TestDiscoverOrphans_FindsUntracked(t *testing.T) {
	cmdExec := cmd_test.NewMockExecutor()
	cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("kas_foo|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n"), nil
	}
	orphans, err := DiscoverOrphans(cmdExec, []string{"kas_foo"})
	require.NoError(t, err)
	assert.Len(t, orphans, 1)
	assert.Equal(t, "kas_orphan", orphans[0].Name)
}

func TestDiscoverAll_MarksManaged(t *testing.T) {
	cmdExec := cmd_test.NewMockExecutor()
	cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("kas_foo|1740000000|1|0|80|24\nkas_bar|1740000000|1|0|80|24\n"), nil
	}
	sessions, err := DiscoverAll(cmdExec, []string{"kas_foo"})
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	managed := 0
	for _, s := range sessions {
		if s.Managed {
			managed++
		}
	}
	assert.Equal(t, 1, managed)
}

func TestCountKasSessions_Simple(t *testing.T) {
	cmdExec := cmd_test.NewMockExecutor()
	cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("kas_foo:1 windows\nkas_bar:1 windows\nother:1 windows\n"), nil
	}
	assert.Equal(t, 2, CountKasSessions(cmdExec))
}

func TestStart_CreatesAndRestoresSession(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-start", "claude", false, ptyFactory, cmdExec)
	err := s.Start(workdir)
	require.NoError(t, err)
	require.Len(t, ptyFactory.cmds, 2) // new-session + attach-session
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "new-session")
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "KASMOS_MANAGED=1")
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[1]), "attach-session")
}

func TestStart_WithSkipPermissions(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-skip", "claude", true, ptyFactory, cmdExec)
	err := s.Start(workdir)
	require.NoError(t, err)
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "--dangerously-skip-permissions")
}

func TestStart_OpenCode_NoTrustTap(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, cmd2.ToString(cmd))
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-oc", "opencode", false, ptyFactory, cmdExec)
	err := s.Start(workdir)
	require.NoError(t, err)
	for _, c := range ranCmds {
		assert.NotContains(t, c, "send-keys")
	}
}

func TestStart_InjectsAgentFlag(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-agent", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")
	err := s.Start(workdir)
	require.NoError(t, err)
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "--agent planner")
}

func TestStart_InjectsTaskEnvVars(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-env", "claude", false, ptyFactory, cmdExec)
	s.SetTaskEnv(3, 2, 4)
	err := s.Start(workdir)
	require.NoError(t, err)
	cmdStr := cmd2.ToString(ptyFactory.cmds[0])
	assert.Contains(t, cmdStr, "KASMOS_TASK=3")
	assert.Contains(t, cmdStr, "KASMOS_WAVE=2")
	assert.Contains(t, cmdStr, "KASMOS_PEERS=4")
}

func TestStart_WithInitialPrompt_OpenCode(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-prompt", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")
	s.SetInitialPrompt("Plan auth. Goal: JWT tokens.")
	err := s.Start(workdir)
	require.NoError(t, err)
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "--prompt 'Plan auth. Goal: JWT tokens.'")
}

func TestHasAttachedClients(t *testing.T) {
	t.Run("attached", func(t *testing.T) {
		var capturedCmd *exec.Cmd
		cmdExec := cmd_test.NewMockExecutor()
		cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
			capturedCmd = cmd
			return []byte("2\n"), nil
		}
		result := HasAttachedClients(cmdExec, "kas_mysession")
		assert.True(t, result)
		require.NotNil(t, capturedCmd)
		cmdStr := strings.Join(capturedCmd.Args, " ")
		assert.Contains(t, cmdStr, "display-message")
		assert.Contains(t, cmdStr, "-p")
		assert.Contains(t, cmdStr, "#{session_attached}")
	})

	t.Run("detached", func(t *testing.T) {
		cmdExec := cmd_test.NewMockExecutor()
		cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("0\n"), nil
		}
		result := HasAttachedClients(cmdExec, "kas_mysession")
		assert.False(t, result)
	})

	t.Run("tmux error returns false", func(t *testing.T) {
		cmdExec := cmd_test.NewMockExecutor()
		cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
			return nil, fmt.Errorf("tmux unavailable")
		}
		result := HasAttachedClients(cmdExec, "kas_mysession")
		assert.False(t, result)
	})
}

func TestParseClientCount(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"2", 2},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"-1", 0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseClientCount(tc.input))
		})
	}
}

func TestStart_WithInitialPrompt_Claude(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("no session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	s := NewTmuxSessionWithDeps("test-claude-prompt", "claude", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("Implement the auth module.")
	err := s.Start(workdir)
	require.NoError(t, err)
	assert.Contains(t, cmd2.ToString(ptyFactory.cmds[0]), "'Implement the auth module.'")
}
