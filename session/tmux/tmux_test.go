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

func TestStartClaudeWithLongPromptUsesFile(t *testing.T) {
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
	s := newTmuxSession("claude-long", "claude", false, ptyFactory, cmdExec)
	longPrompt := strings.Repeat("x", MaxInlinePromptLen+1)
	s.SetInitialPrompt(longPrompt)

	err := s.Start(workdir)
	require.NoError(t, err)

	// The command should reference a @.kasmos/prompt-*.md instead of inlining.
	cmdStr := cmd2.ToString(ptyFactory.cmds[0])
	require.Contains(t, cmdStr, "@.kasmos/prompt-")
	require.NotContains(t, cmdStr, longPrompt, "long prompt must not be inlined")

	// The prompt file should live under workdir/.kasmos/ and contain the prompt.
	require.NotEmpty(t, s.promptFile)
	assert.True(t, strings.HasPrefix(s.promptFile, filepath.Join(workdir, ".kasmos")),
		"prompt file should be under workdir/.kasmos/")
	content, err := os.ReadFile(s.promptFile)
	require.NoError(t, err)
	assert.Equal(t, longPrompt, string(content))

	// Close should clean up the temp file.
	s.Close()
	_, err = os.Stat(s.promptFile)
	assert.True(t, os.IsNotExist(err), "prompt file should be removed after Close")
}

func TestDiscoverOrphans(t *testing.T) {
	tests := []struct {
		name       string
		tmuxOutput string
		tmuxErr    error
		knownNames []string
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "no sessions running",
			tmuxErr:    &exec.ExitError{},
			knownNames: nil,
			wantCount:  0,
		},
		{
			name:       "all sessions tracked",
			tmuxOutput: "kas_foo|1740000000|1|0|80|24\nkas_bar|1740000000|1|0|120|40\n",
			knownNames: []string{"kas_foo", "kas_bar"},
			wantCount:  0,
		},
		{
			name:       "one orphan among tracked",
			tmuxOutput: "kas_foo|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames: []string{"kas_foo"},
			wantCount:  1,
		},
		{
			name:       "non-kas sessions ignored",
			tmuxOutput: "myshell|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames: nil,
			wantCount:  1,
		},
		{
			name:       "attached session detected",
			tmuxOutput: "kas_orphan|1740000000|1|1|80|24\n",
			knownNames: nil,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec := cmd_test.NewMockExecutor()
			if tt.tmuxErr != nil {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return nil, tt.tmuxErr
				}
			} else {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tt.tmuxOutput), nil
				}
			}

			orphans, err := DiscoverOrphans(cmdExec, tt.knownNames)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, orphans, tt.wantCount)
		})
	}
}

func TestDiscoverAll(t *testing.T) {
	tests := []struct {
		name        string
		tmuxOutput  string
		tmuxErr     error
		knownNames  []string
		wantTotal   int
		wantManaged int
		wantErr     bool
	}{
		{
			name:        "no sessions running",
			tmuxErr:     &exec.ExitError{},
			knownNames:  nil,
			wantTotal:   0,
			wantManaged: 0,
		},
		{
			name:        "all sessions managed",
			tmuxOutput:  "kas_foo|1740000000|1|0|80|24\nkas_bar|1740000000|1|0|120|40\n",
			knownNames:  []string{"kas_foo", "kas_bar"},
			wantTotal:   2,
			wantManaged: 2,
		},
		{
			name:        "mix of managed and orphaned",
			tmuxOutput:  "kas_foo|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames:  []string{"kas_foo"},
			wantTotal:   2,
			wantManaged: 1,
		},
		{
			name:        "non-kas sessions ignored",
			tmuxOutput:  "myshell|1740000000|1|0|80|24\nkas_orphan|1740000000|1|0|80|24\n",
			knownNames:  nil,
			wantTotal:   1,
			wantManaged: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec := cmd_test.NewMockExecutor()
			if tt.tmuxErr != nil {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return nil, tt.tmuxErr
				}
			} else {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tt.tmuxOutput), nil
				}
			}

			sessions, err := DiscoverAll(cmdExec, tt.knownNames)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, sessions, tt.wantTotal)

			managedCount := 0
			for _, s := range sessions {
				if s.Managed {
					managedCount++
				}
			}
			assert.Equal(t, tt.wantManaged, managedCount)
		})
	}
}

func TestCountKasSessions(t *testing.T) {
	tests := []struct {
		name       string
		tmuxOutput string
		tmuxErr    error
		want       int
	}{
		{
			name:    "no tmux server",
			tmuxErr: &exec.ExitError{},
			want:    0,
		},
		{
			name:       "two kas sessions one foreign",
			tmuxOutput: "kas_foo:1 windows\nkas_bar:1 windows\nmyshell:2 windows\n",
			want:       2,
		},
		{
			name:       "no kas sessions",
			tmuxOutput: "myshell:1 windows\n",
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec := cmd_test.NewMockExecutor()
			if tt.tmuxErr != nil {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return nil, tt.tmuxErr
				}
			} else {
				cmdExec.OutputFunc = func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tt.tmuxOutput), nil
				}
			}

			count := CountKasSessions(cmdExec)
			assert.Equal(t, tt.want, count)
		})
	}
}

func TestStartOpenCodeWithLongPromptUsesCommandSubstitution(t *testing.T) {
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
	s := newTmuxSession("oc-long", "opencode", false, ptyFactory, cmdExec)
	longPrompt := strings.Repeat("x", MaxInlinePromptLen+1)
	s.SetInitialPrompt(longPrompt)

	err := s.Start(workdir)
	require.NoError(t, err)

	// opencode should use $(cat ...) not @file syntax.
	cmdStr := cmd2.ToString(ptyFactory.cmds[0])
	require.Contains(t, cmdStr, "$(cat ")
	require.Contains(t, cmdStr, ".kasmos/prompt-")
	require.NotContains(t, cmdStr, "@.kasmos/prompt-", "opencode must not use Claude's @file syntax")
	require.NotContains(t, cmdStr, longPrompt, "long prompt must not be inlined")

	// The prompt file should contain the full prompt.
	require.NotEmpty(t, s.promptFile)
	content, err := os.ReadFile(s.promptFile)
	require.NoError(t, err)
	assert.Equal(t, longPrompt, string(content))

	// Close should clean up the temp file.
	s.Close()
	_, err = os.Stat(s.promptFile)
	assert.True(t, os.IsNotExist(err), "prompt file should be removed after Close")
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
