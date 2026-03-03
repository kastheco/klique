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

func TestRestart_KillsTmuxAndRestartsSession(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	inst := &Instance{
		Title:   "test-restart",
		Path:    t.TempDir(),
		Program: "opencode",
		started: true,
	}
	inst.tmuxSession = tmux.NewTmuxSessionWithDeps(inst.Title, inst.Program, false, &testPtyFactory{}, cmdExec)

	err := inst.Restart()
	assert.NoError(t, err)
	assert.Equal(t, Running, inst.Status)
	assert.True(t, inst.started)
}

func TestRestart_WorksWhenTmuxAlreadyDead(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	inst := &Instance{
		Title:   "test-restart-dead",
		Path:    t.TempDir(),
		Program: "opencode",
		started: true,
		Exited:  true,
	}
	inst.tmuxSession = tmux.NewTmuxSessionWithDeps(inst.Title, inst.Program, false, &testPtyFactory{}, cmdExec)

	err := inst.Restart()
	assert.NoError(t, err)
	assert.False(t, inst.Exited, "Exited flag should be cleared after restart")
	assert.Equal(t, Running, inst.Status)
}

func TestRestart_NotStarted_ReturnsError(t *testing.T) {
	inst := &Instance{Title: "never-started", started: false}
	err := inst.Restart()
	assert.Error(t, err)
}

func TestRestart_PausedInstance_ReturnsError(t *testing.T) {
	inst := &Instance{
		Title:   "paused-restart",
		started: true,
		Status:  Paused,
	}
	err := inst.Restart()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "paused")
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
