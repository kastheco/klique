package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/opencodesession"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"

	"github.com/atotto/clipboard"
)

// prepareExecutionSession returns the existing execution session if already wired, otherwise
// allocates a fresh one from the instance configuration.
func (i *Instance) prepareExecutionSession() ExecutionSession {
	if i.executionSession != nil {
		return i.executionSession
	}
	return NewExecutionSession(i.ExecutionMode, i.Title, i.Program, i.SkipPermissions)
}

// transferPromptToCli moves QueuedPrompt into the execution session's initialPrompt
// when the program supports CLI prompt injection. Programs that do not support it
// leave QueuedPrompt intact so a send-keys fallback can deliver it later.
func (i *Instance) transferPromptToCli() {
	if i.QueuedPrompt != "" && programSupportsCliPrompt(i.Program) {
		i.executionSession.SetInitialPrompt(i.QueuedPrompt)
		i.QueuedPrompt = ""
	}
}

// setExecutionTaskEnv pushes wave/task/peer identity into the execution session environment
// so that agents spawned inside the session inherit the orchestration context.
func (i *Instance) setExecutionTaskEnv() {
	if i.TaskNumber > 0 && i.executionSession != nil {
		i.executionSession.SetTaskEnv(i.TaskNumber, i.WaveNumber, i.PeerCount)
	}
}

// buildTitleOpts converts an Instance's metadata fields into the TitleOpts
// structure consumed by the opencodesession title builder.
func buildTitleOpts(inst *Instance) opencodesession.TitleOpts {
	displayName := ""
	if inst.TaskFile != "" {
		displayName = taskstate.DisplayName(inst.TaskFile)
	}
	return opencodesession.TitleOpts{
		PlanName:      displayName,
		AgentType:     inst.AgentType,
		WaveNumber:    inst.WaveNumber,
		TaskNumber:    inst.TaskNumber,
		InstanceTitle: inst.Title,
		ReviewCycle:   inst.ReviewCycle,
	}
}

// configureSessionTitle derives a session title from the instance metadata and
// registers a callback that writes it to the opencode database when the session
// becomes ready. It is a no-op for non-opencode programs.
func (i *Instance) configureSessionTitle() {
	if i.executionSession == nil || !strings.HasSuffix(i.Program, "opencode") {
		return
	}
	opts := buildTitleOpts(i)
	title := opencodesession.BuildTitle(opts)
	i.executionSession.SetSessionTitle(title)
	i.executionSession.SetTitleFunc(func(workDir string, beforeStart time.Time, t string) {
		if err := opencodesession.SetTitleDirect(workDir, beforeStart, t); err != nil {
			log.ErrorLog.Printf("opencodesession: set title: %v", err)
		}
	})
}

func dirtyWorktreeContext(worktreePath string) string {
	if strings.TrimSpace(worktreePath) == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", worktreePath, "status", "--short").CombinedOutput()
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	preview := lines
	if len(preview) > 5 {
		preview = preview[:5]
	}
	if len(lines) > len(preview) {
		return fmt.Sprintf(" (%s (+%d more))", strings.Join(preview, ", "), len(lines)-len(preview))
	}
	return fmt.Sprintf(" (%s)", strings.Join(preview, ", "))
}

// setProgressFunc injects a progress hook into the execution session if it
// implements progressReporter (i.e. tmuxExecutionSession). No-op otherwise.
func (i *Instance) setProgressFunc(fn func(int, string)) {
	if pr, ok := i.executionSession.(progressReporter); ok {
		pr.SetProgressFunc(fn)
	}
}

// Start launches the instance. When firstTimeSetup is true a fresh git worktree is
// created and the execution session starts inside it. When false the instance was loaded
// from storage and the existing session is restored instead.
func (i *Instance) Start(firstTimeSetup bool) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	if firstTimeSetup {
		i.LoadingTotal = 8
	} else {
		i.LoadingTotal = 6
	}
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	i.executionSession = i.prepareExecutionSession()
	i.executionSession.SetAgentType(i.AgentType)
	i.setExecutionTaskEnv()
	i.configureSessionTitle()

	// Offset internal progress stages so they map to the overall loading bar.
	stageBase := 3
	if !firstTimeSetup {
		stageBase = 1
	}
	i.setProgressFunc(func(stage int, desc string) {
		i.setLoadingProgress(stageBase+stage, desc)
	})
	i.transferPromptToCli()

	if firstTimeSetup {
		i.setLoadingProgress(2, "Creating git worktree...")
		worktree, branch, err := git.NewGitWorktree(i.Path, i.Title)
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitWorktree = worktree
		i.Branch = branch
	}

	// Clean up on any failure after this point.
	var startErr error
	defer func() {
		if startErr != nil {
			if killErr := i.Kill(); killErr != nil {
				startErr = fmt.Errorf("%v (cleanup: %v)", startErr, killErr)
			}
		} else {
			i.started = true
		}
	}()

	if firstTimeSetup {
		i.setLoadingProgress(3, "Setting up git worktree...")
		if err := i.gitWorktree.Setup(); err != nil {
			startErr = fmt.Errorf("failed to setup git worktree: %w", err)
			return startErr
		}
		i.setLoadingProgress(4, "Starting session...")
		if err := i.executionSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup: %v)", err, cleanupErr)
			}
			startErr = fmt.Errorf("failed to start session: %w", err)
			return startErr
		}
	} else {
		i.setLoadingProgress(2, "Restoring session...")
		if err := i.executionSession.Restore(); err != nil {
			startErr = fmt.Errorf("failed to restore existing session: %w", err)
			return startErr
		}
	}

	i.SetStatus(Running)
	return nil
}

// StartOnMainBranch launches the instance directly in the repository root without
// creating a git worktree. Intended for planner agents that operate on main.
func (i *Instance) StartOnMainBranch() error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 5
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	i.executionSession = i.prepareExecutionSession()
	i.executionSession.SetAgentType(i.AgentType)
	i.setExecutionTaskEnv()
	i.configureSessionTitle()
	i.setProgressFunc(func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	})
	i.transferPromptToCli()

	var startErr error
	defer func() {
		if startErr != nil {
			if killErr := i.Kill(); killErr != nil {
				startErr = fmt.Errorf("%v (cleanup: %v)", startErr, killErr)
			}
		} else {
			i.started = true
		}
	}()

	if err := i.executionSession.Start(i.Path); err != nil {
		startErr = fmt.Errorf("failed to start session on main branch: %w", err)
		return startErr
	}

	i.SetStatus(Running)
	return nil
}

// StartOnBranch creates a worktree on the specified branch (reusing an existing
// branch when it already exists) and starts the execution session inside it.
func (i *Instance) StartOnBranch(branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 8
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	i.executionSession = i.prepareExecutionSession()
	i.executionSession.SetAgentType(i.AgentType)
	i.setExecutionTaskEnv()
	i.configureSessionTitle()
	i.setProgressFunc(func(stage int, desc string) {
		i.setLoadingProgress(3+stage, desc)
	})
	i.transferPromptToCli()

	i.setLoadingProgress(2, "Creating git worktree...")
	worktree, branchName, err := git.NewGitWorktreeOnBranch(i.Path, i.Title, branch)
	if err != nil {
		return fmt.Errorf("failed to create git worktree on branch %s: %w", branch, err)
	}
	i.gitWorktree = worktree
	i.Branch = branchName

	var startErr error
	defer func() {
		if startErr != nil {
			if killErr := i.Kill(); killErr != nil {
				startErr = fmt.Errorf("%v (cleanup: %v)", startErr, killErr)
			}
		} else {
			i.started = true
		}
	}()

	i.setLoadingProgress(3, "Setting up git worktree...")
	if err := i.gitWorktree.Setup(); err != nil {
		startErr = fmt.Errorf("failed to setup git worktree: %w", err)
		return startErr
	}

	i.setLoadingProgress(4, "Starting session...")
	if err := i.executionSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
		if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup: %v)", err, cleanupErr)
		}
		startErr = fmt.Errorf("failed to start session: %w", err)
		return startErr
	}

	i.SetStatus(Running)
	return nil
}

// StartInSharedWorktree connects the instance to a topic-owned worktree. No new
// worktree is created; the instance borrows the one passed by the caller.
func (i *Instance) StartInSharedWorktree(worktree *git.GitWorktree, branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 6
	i.setLoadingProgress(1, "Connecting to shared worktree...")

	i.gitWorktree = worktree
	i.Branch = branch
	i.sharedWorktree = true

	i.executionSession = i.prepareExecutionSession()
	i.executionSession.SetAgentType(i.AgentType)
	i.setExecutionTaskEnv()
	i.configureSessionTitle()
	i.setProgressFunc(func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	})
	i.transferPromptToCli()

	i.setLoadingProgress(2, "Starting session...")
	if err := i.executionSession.Start(worktree.GetWorktreePath()); err != nil {
		return fmt.Errorf("failed to start session in shared worktree: %w", err)
	}

	i.started = true
	i.SetStatus(Running)
	return nil
}

// Kill terminates the execution session and removes the git worktree.
// The git branch is preserved so the instance can be inspected or resumed later.
// Returns nil for instances that were never started.
func (i *Instance) Kill() error {
	if !i.started {
		return nil
	}
	if i.gitWorktree != nil && !i.sharedWorktree {
		dirty, err := i.gitWorktree.IsDirty()
		if err != nil {
			return fmt.Errorf("failed to check if worktree is dirty: %w", err)
		}
		if dirty {
			return fmt.Errorf("cannot kill instance with uncommitted changes%s; commit or stash first", dirtyWorktreeContext(i.gitWorktree.GetWorktreePath()))
		}
	}

	var errs []error

	// Close the execution session first — it may hold an open handle to the worktree directory.
	if i.executionSession != nil {
		if err := i.executionSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close session: %w", err))
		}
	}

	// Shared worktrees are owned by the topic, not the instance.
	if i.gitWorktree != nil && !i.sharedWorktree {
		if err := i.gitWorktree.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
		}
		if err := i.gitWorktree.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
		}
	}

	return errors.Join(errs...)
}

// StopTmux closes the underlying execution session without touching the worktree or
// any other instance state. The instance remains in the list as stopped.
func (i *Instance) StopTmux() {
	if i.executionSession != nil {
		_ = i.executionSession.Close()
	}
}

// Pause detaches from the session and removes the git worktree, preserving
// the branch for a later Resume.
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}
	if i.gitWorktree != nil && !i.sharedWorktree {
		dirty, err := i.gitWorktree.IsDirty()
		if err != nil {
			return fmt.Errorf("failed to check if worktree is dirty: %w", err)
		}
		if dirty {
			return fmt.Errorf("cannot pause instance with uncommitted changes%s; commit or stash first", dirtyWorktreeContext(i.gitWorktree.GetWorktreePath()))
		}
	}

	var errs []error

	if err := i.executionSession.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach session: %w", err))
		log.ErrorLog.Print(err)
	}

	if !i.sharedWorktree && i.gitWorktree != nil {
		worktreePath := i.gitWorktree.GetWorktreePath()
		if _, statErr := os.Stat(worktreePath); statErr == nil {
			if removeErr := i.gitWorktree.Remove(); removeErr != nil {
				errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", removeErr))
				log.ErrorLog.Print(removeErr)
				return errors.Join(errs...)
			}
			if pruneErr := i.gitWorktree.Prune(); pruneErr != nil {
				errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", pruneErr))
				log.ErrorLog.Print(pruneErr)
				return errors.Join(errs...)
			}
		}
	}

	if joined := errors.Join(errs...); joined != nil {
		log.ErrorLog.Print(joined)
		return joined
	}

	i.SetStatus(Paused)
	if i.gitWorktree != nil {
		_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	}
	return nil
}

// AdoptOrphanTmuxSession wires the instance to an existing tmux session that was
// not created through the normal lifecycle. No worktree is involved.
func (i *Instance) AdoptOrphanTmuxSession(tmuxName string) error {
	ts := tmux.NewTmuxSessionFromExisting(tmuxName, i.Program, i.SkipPermissions)
	w := &tmuxExecutionSession{s: ts}
	i.executionSession = w
	if err := ts.Restore(); err != nil {
		return fmt.Errorf("failed to adopt orphan session %s: %w", tmuxName, err)
	}
	i.started = true
	i.SetStatus(Ready)
	return nil
}

// resetExecutionSession creates a fresh execution session for Restart().
// For tmux sessions, the underlying TmuxSession.NewReset preserves injected
// test dependencies (ptyFactory, cmdExec). For all other modes a new session
// is constructed via NewExecutionSession.
func (i *Instance) resetExecutionSession() ExecutionSession {
	if ts, ok := i.executionSession.(*tmuxExecutionSession); ok {
		return &tmuxExecutionSession{s: ts.s.NewReset(i.Title, i.Program, i.SkipPermissions)}
	}
	return NewExecutionSession(i.ExecutionMode, i.Title, i.Program, i.SkipPermissions)
}

// Restart closes the current execution session (best-effort) and launches a fresh one
// with the same configuration. The worktree and branch are preserved. Ephemeral
// per-run flags are reset so the instance appears freshly started.
func (i *Instance) Restart() error {
	if !i.started {
		return fmt.Errorf("cannot restart instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("cannot restart paused instance; resume it first")
	}

	// Best-effort: session may already be dead.
	if i.executionSession != nil {
		_ = i.executionSession.Close()
	}

	// Allocate a new session object, carrying over injected test dependencies.
	i.executionSession = i.resetExecutionSession()
	i.executionSession.SetAgentType(i.AgentType)
	i.setExecutionTaskEnv()
	i.configureSessionTitle()

	workDir := i.Path
	if i.gitWorktree != nil {
		workDir = i.gitWorktree.GetWorktreePath()
	}

	if err := i.executionSession.Start(workDir); err != nil {
		return fmt.Errorf("failed to restart session: %w", err)
	}

	// Reset ephemeral per-run state.
	i.Exited = false
	i.PromptDetected = false
	i.HasWorked = false
	i.AwaitingWork = false
	i.Notified = false
	i.CachedContentSet = false
	i.CachedContent = ""

	i.SetStatus(Running)
	return nil
}

// Resume recreates the worktree for a paused instance and reconnects or starts
// a fresh execution session inside it.
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	checked, err := i.gitWorktree.IsBranchCheckedOut()
	if err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	}
	if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	if err := i.gitWorktree.Setup(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to setup git worktree: %w", err)
	}

	worktreePath := i.gitWorktree.GetWorktreePath()

	if i.executionSession.DoesSessionExist() {
		if restoreErr := i.executionSession.Restore(); restoreErr != nil {
			log.ErrorLog.Print(restoreErr)
			// Fall back to a fresh session start.
			if startErr := i.executionSession.Start(worktreePath); startErr != nil {
				log.ErrorLog.Print(startErr)
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					startErr = fmt.Errorf("%v (cleanup: %v)", startErr, cleanupErr)
					log.ErrorLog.Print(startErr)
				}
				return fmt.Errorf("failed to start new session: %w", startErr)
			}
		}
	} else {
		if err := i.executionSession.Start(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup: %v)", err, cleanupErr)
				log.ErrorLog.Print(err)
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.SetStatus(Running)
	return nil
}
