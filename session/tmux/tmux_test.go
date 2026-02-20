package tmux

import (
	"fmt"
	cmd2 "github.com/kastheco/klique/cmd"
	"github.com/kastheco/klique/log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kastheco/klique/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.Initialize(false)
	code := m.Run()
	log.Close()
	os.Exit(code)
}

type MockPtyFactory struct {
	t *testing.T

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", pt.t.Name(), rand.Int31()))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)
	}
	return f, err
}

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
	session := NewTmuxSession("asdf", "program", false)
	require.Equal(t, TmuxPrefix+"asdf", session.sanitizedName)

	session = NewTmuxSession("a sd f . . asdf", "program", false)
	require.Equal(t, TmuxPrefix+"asdf__asdf", session.sanitizedName)
}

func TestStartTmuxSession(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", false, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s klique_test-session -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
	require.Equal(t, "tmux attach-session -t klique_test-session",
		cmd2.ToString(ptyFactory.cmds[1]))

	require.Equal(t, 2, len(ptyFactory.files))

	// File should be closed.
	_, err = ptyFactory.files[0].Stat()
	require.Error(t, err)
	// File should be open
	_, err = ptyFactory.files[1].Stat()
	require.NoError(t, err)
}

func TestStartTmuxSessionWithSkipPermissions(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", true, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s klique_test-session -c %s claude --dangerously-skip-permissions", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}

func recordKilledSessions(killedSessions *[]string) func(cmd *exec.Cmd) error {
	return func(cmd *exec.Cmd) error {
		args := cmd.Args
		// Only record sessions killed by kill-session, not other tmux subcommands
		if len(args) >= 2 && args[1] == "kill-session" {
			for i, arg := range args {
				if arg == "-t" && i+1 < len(args) {
					*killedSessions = append(*killedSessions, args[i+1])
				}
			}
		}
		return nil
	}
}

func TestCleanupSessions(t *testing.T) {
	t.Run("kills klique, legacy hivemind, and lazygit sessions", func(t *testing.T) {
		var killedSessions []string
		cmdExec := cmd_test.MockCmdExec{
			RunFunc: recordKilledSessions(&killedSessions),
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				output := "klique_session1: 1 windows (created Thu Feb 20 10:00:00 2026)\n" +
					"klique_lazygit_session1: 1 windows (created Thu Feb 20 10:00:01 2026)\n" +
					"hivemind_legacy: 1 windows (created Thu Feb 20 09:00:00 2026)\n" +
					"unrelated_session: 1 windows (created Thu Feb 20 08:00:00 2026)\n"
				return []byte(output), nil
			},
		}

		err := CleanupSessions(cmdExec)
		require.NoError(t, err)
		require.Len(t, killedSessions, 3)
		require.Contains(t, killedSessions, "klique_session1")
		require.Contains(t, killedSessions, "klique_lazygit_session1")
		require.Contains(t, killedSessions, "hivemind_legacy")
	})

	t.Run("leaves unrelated sessions alone", func(t *testing.T) {
		var killedSessions []string
		cmdExec := cmd_test.MockCmdExec{
			RunFunc: recordKilledSessions(&killedSessions),
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				output := "unrelated_session: 1 windows (created Thu Feb 20 08:00:00 2026)\n"
				return []byte(output), nil
			},
		}

		err := CleanupSessions(cmdExec)
		require.NoError(t, err)
		require.Len(t, killedSessions, 0)
	})
}

func TestStartTmuxSessionSkipPermissionsNotAppliedToAider(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "aider --model gpt-4", true, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s klique_test-session -c %s aider --model gpt-4", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}
