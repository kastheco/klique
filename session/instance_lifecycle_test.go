package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "command failed: %s %v\n%s", name, args, string(output))
}

func setupGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runCommand(t, repo, "git", "init")
	runCommand(t, repo, "git", "config", "user.email", "test@example.com")
	runCommand(t, repo, "git", "config", "user.name", "test")

	filePath := filepath.Join(repo, "README.md")
	require.NoError(t, os.WriteFile(filePath, []byte("test\n"), 0644))
	runCommand(t, repo, "git", "add", "README.md")
	runCommand(t, repo, "git", "commit", "-m", "initial")

	return repo
}

func TestStartTransfersQueuedPromptForOpenCode(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Ask anything"), nil
		},
	}

	inst := &Instance{
		Title:        "test-transfer",
		Path:         t.TempDir(),
		Program:      "opencode",
		QueuedPrompt: "Plan auth.",
		tmuxSession:  tmux.NewTmuxSessionWithDeps("test-transfer", "opencode", false, &testPtyFactory{}, cmdExec),
	}

	// Simulate StartOnMainBranch which is the simplest path.
	err := inst.StartOnMainBranch()
	require.NoError(t, err)

	// QueuedPrompt should be cleared (transferred to initialPrompt).
	assert.Empty(t, inst.QueuedPrompt)
}

func TestStartKeepsQueuedPromptForAider(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Open documentation url for more info"), nil
		},
	}

	inst := &Instance{
		Title:        "test-aider",
		Path:         t.TempDir(),
		Program:      "aider --model ollama_chat/gemma3:1b",
		QueuedPrompt: "Fix the bug.",
		tmuxSession:  tmux.NewTmuxSessionWithDeps("test-aider", "aider --model ollama_chat/gemma3:1b", false, &testPtyFactory{}, cmdExec),
	}

	err := inst.StartOnMainBranch()
	require.NoError(t, err)

	// QueuedPrompt should remain — aider doesn't support CLI prompts.
	assert.Equal(t, "Fix the bug.", inst.QueuedPrompt)
}

func TestRestartTmux_FailsWhenNotStarted(t *testing.T) {
	inst := &Instance{
		Title:   "test-restart-not-started",
		Path:    t.TempDir(),
		Program: "opencode",
	}
	err := inst.RestartTmux()
	assert.Error(t, err, "RestartTmux should return an error when instance has not been started")
}

func TestRestartTmux_ResetsStateAndRestartsSession(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("Ask anything"), nil },
	}

	inst := &Instance{
		Title:                  "test-restart",
		Path:                   t.TempDir(),
		Program:                "opencode",
		started:                true,
		Exited:                 true,
		ImplementationComplete: true,
		HasWorked:              true,
		AwaitingWork:           true,
		PromptDetected:         true,
	}
	inst.Status = Ready
	inst.tmuxSession = tmux.NewTmuxSessionWithDeps("test-restart", "opencode", false, &testPtyFactory{}, cmdExec)

	err := inst.RestartTmux()
	require.NoError(t, err)

	assert.Equal(t, Running, inst.Status, "status should be Running after restart")
	assert.False(t, inst.Exited, "Exited flag should be reset")
	assert.False(t, inst.ImplementationComplete, "ImplementationComplete flag should be reset")
	assert.False(t, inst.HasWorked, "HasWorked flag should be reset")
	assert.False(t, inst.AwaitingWork, "AwaitingWork flag should be reset")
	assert.False(t, inst.PromptDetected, "PromptDetected flag should be reset")
}

func TestRestartTmux_TransfersQueuedPrompt(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("Ask anything"), nil },
	}

	inst := &Instance{
		Title:        "test-restart-prompt",
		Path:         t.TempDir(),
		Program:      "opencode",
		started:      true,
		QueuedPrompt: "Implement the feature.",
	}
	inst.tmuxSession = tmux.NewTmuxSessionWithDeps("test-restart-prompt", "opencode", false, &testPtyFactory{}, cmdExec)

	err := inst.RestartTmux()
	require.NoError(t, err)

	// QueuedPrompt should be cleared (transferred to initialPrompt via transferPromptToCli).
	assert.Empty(t, inst.QueuedPrompt, "QueuedPrompt should be cleared after restart for opencode")
}

func TestStartOnBranch_SetsFields(t *testing.T) {
	repoPath := setupGitRepo(t)

	inst, err := NewInstance(InstanceOptions{
		Title:   "test-branch",
		Path:    repoPath,
		Program: "opencode",
	})
	require.NoError(t, err)
	assert.Equal(t, "", inst.Branch)

	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("Ask anything"), nil
		},
	}
	inst.tmuxSession = tmux.NewTmuxSessionWithDeps("test-branch", "opencode", false, &testPtyFactory{}, cmdExec)

	err = inst.StartOnBranch("feature/task-5")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, inst.Kill())
	})

	assert.Equal(t, "feature/task-5", inst.Branch)
	assert.Equal(t, Running, inst.Status)
	assert.True(t, inst.Started())
	assert.NotEqual(t, "", inst.GetWorktreePath(), fmt.Sprintf("worktree path should be set for %s", inst.Title))
}
