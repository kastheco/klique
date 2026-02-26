package tmux

import (
	"fmt"
	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"

	"github.com/stretchr/testify/assert"
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
			// Return the trust-screen string so the startup wait exits fast.
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", false, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s kas_test-session -c %s KASMOS_MANAGED=1 claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
	require.Equal(t, "tmux attach-session -t kas_test-session",
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
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Do you trust the files in this folder?"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", true, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s kas_test-session -c %s KASMOS_MANAGED=1 claude --dangerously-skip-permissions", workdir),
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
	t.Run("kills kas and legacy klique/hivemind sessions", func(t *testing.T) {
		var killedSessions []string
		cmdExec := cmd_test.MockCmdExec{
			RunFunc: recordKilledSessions(&killedSessions),
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				output := "kas_session1: 1 windows (created Thu Feb 20 10:00:00 2026)\n" +
					"kas_lazygit_session1: 1 windows (created Thu Feb 20 10:00:01 2026)\n" +
					"klique_legacy_session: 1 windows (created Thu Feb 20 09:30:00 2026)\n" +
					"hivemind_legacy: 1 windows (created Thu Feb 20 09:00:00 2026)\n" +
					"unrelated_session: 1 windows (created Thu Feb 20 08:00:00 2026)\n"
				return []byte(output), nil
			},
		}

		err := CleanupSessions(cmdExec)
		require.NoError(t, err)
		require.Len(t, killedSessions, 4)
		require.Contains(t, killedSessions, "kas_session1")
		require.Contains(t, killedSessions, "kas_lazygit_session1")
		require.Contains(t, killedSessions, "klique_legacy_session")
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
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Open documentation url for more info"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "aider --model gpt-4", true, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s kas_test-session -c %s KASMOS_MANAGED=1 aider --model gpt-4", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
}

func TestStartTmuxSessionOpenCode(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, cmd2.ToString(cmd))
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Return "Ask anything" immediately so the startup wait exits fast.
			if strings.Contains(cmd.String(), "capture-pane") {
				return []byte("Ask anything"), nil
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("oc-session", "opencode", false, ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)

	// Verify new-session used the right program.
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s kas_oc-session -c %s KASMOS_MANAGED=1 opencode", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))

	// Verify no send-keys tap was issued (opencode needs no trust-screen tap).
	for _, c := range ranCmds {
		require.NotContains(t, c, "send-keys", "opencode startup should not send any keys")
	}
}

func TestSendKeys(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, cmd2.ToString(cmd))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test-session", "opencode", false, ptyFactory, cmdExec)
	// Manually set sanitizedName by creating via the constructor (already done).

	err := session.SendKeys("hello world")
	require.NoError(t, err)
	require.Len(t, ranCmds, 1)
	require.Equal(t, "tmux send-keys -l -t kas_test-session hello world", ranCmds[0])
}

func TestTapEnter(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, cmd2.ToString(cmd))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test-session", "opencode", false, ptyFactory, cmdExec)

	err := session.TapEnter()
	require.NoError(t, err)
	require.Len(t, ranCmds, 1)
	require.Equal(t, "tmux send-keys -t kas_test-session Enter", ranCmds[0])
}

func TestStartTmuxSessionInjectsAgentFlag(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
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
	s := newTmuxSession("agent-test", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_agent-test -c %s KASMOS_MANAGED=1 opencode --agent planner", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}

func TestTapDAndEnter(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, cmd2.ToString(cmd))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test-session", "aider", false, ptyFactory, cmdExec)

	err := session.TapDAndEnter()
	require.NoError(t, err)
	require.Len(t, ranCmds, 1)
	require.Equal(t, "tmux send-keys -t kas_test-session D Enter", ranCmds[0])
}

func TestSetInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("output"), nil },
	}

	s := newTmuxSession("prompt-test", "opencode", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("hello world")

	// Verify the field is set (accessed via the Start command construction).
	assert.Equal(t, "hello world", s.initialPrompt)
}

func TestStartOpenCodeWithInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
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
	s := newTmuxSession("oc-prompt", "opencode", false, ptyFactory, cmdExec)
	s.SetAgentType("planner")
	s.SetInitialPrompt("Plan auth. Goal: JWT tokens.")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_oc-prompt -c %s KASMOS_MANAGED=1 opencode --agent planner --prompt 'Plan auth. Goal: JWT tokens.'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}

func TestStartClaudeWithInitialPrompt(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
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
	s := newTmuxSession("claude-prompt", "claude", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("Implement the auth module.")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_claude-prompt -c %s KASMOS_MANAGED=1 claude 'Implement the auth module.'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}

func TestStartOpenCodeWithPromptContainingSingleQuotes(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session does not exist yet")
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
	s := newTmuxSession("oc-quote", "opencode", false, ptyFactory, cmdExec)
	s.SetInitialPrompt("it's a test")

	err := s.Start(workdir)
	require.NoError(t, err)
	require.Equal(
		t,
		fmt.Sprintf("tmux new-session -d -s kas_oc-quote -c %s KASMOS_MANAGED=1 opencode --prompt 'it'\\''s a test'", workdir),
		cmd2.ToString(ptyFactory.cmds[0]),
	)
}
