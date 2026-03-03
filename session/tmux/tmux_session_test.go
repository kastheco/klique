package tmux

import (
	"os/exec"
	"testing"

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
