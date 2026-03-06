package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/opencodesession"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"

	"github.com/atotto/clipboard"
)

// prepareTmuxSession returns the existing tmux session if already wired, otherwise
// allocates a fresh one from the instance configuration.
func (i *Instance) prepareTmuxSession() *tmux.TmuxSession {
	if i.tmuxSession != nil {
		return i.tmuxSession
	}
	return tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
}

// transferPromptToCli moves QueuedPrompt into the tmux session's initialPrompt
// when the program supports CLI prompt injection. Programs that do not support it
// leave QueuedPrompt intact so a send-keys fallback can deliver it later.
func (i *Instance) transferPromptToCli() {
	if i.QueuedPrompt != "" && programSupportsCliPrompt(i.Program) {
		i.tmuxSession.SetInitialPrompt(i.QueuedPrompt)
		i.QueuedPrompt = ""
	}
}

// setTmuxTaskEnv pushes wave/task/peer identity into the tmux session environment
// so that agents spawned inside the session inherit the orchestration context.
func (i *Instance) setTmuxTaskEnv() {
	if i.TaskNumber > 0 && i.tmuxSession != nil {
		i.tmuxSession.SetTaskEnv(i.TaskNumber, i.WaveNumber, i.PeerCount)
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
	if i.tmuxSession == nil || !strings.HasSuffix(i.Program, "opencode") {
		return
	}
	opts := buildTitleOpts(i)
	title := opencodesession.BuildTitle(opts)
	i.tmuxSession.SetSessionTitle(title)
	i.tmuxSession.SetTitleFunc(func(workDir string, beforeStart time.Time, t string) {
		if err := opencodesession.SetTitleDirect(workDir, beforeStart, t); err != nil {
			log.ErrorLog.Printf("opencodesession: set title: %v", err)
		}
	})
}

// Start launches the instance. When firstTimeSetup is true a fresh git worktree is
// created and the tmux session starts inside it. When false the instance was loaded
// from storage and the existing tmux session is restored instead.
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
	i.tmuxSession = i.prepareTmuxSession()
	i.tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	i.configureSessionTitle()

	// Offset tmux-internal progress stages so they map to the overall loading bar.
	stageBase := 3
	if !firstTimeSetup {
		stageBase = 1
	}
	i.tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(stageBase+stage, desc)
	}
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
		i.setLoadingProgress(4, "Starting tmux session...")
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup: %v)", err, cleanupErr)
			}
			startErr = fmt.Errorf("failed to start tmux session: %w", err)
			return startErr
		}
	} else {
		i.setLoadingProgress(2, "Restoring session...")
		if err := i.tmuxSession.Restore(); err != nil {
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
	i.tmuxSession = i.prepareTmuxSession()
	i.tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	i.configureSessionTitle()
	i.tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	}
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

	if err := i.tmuxSession.Start(i.Path); err != nil {
		startErr = fmt.Errorf("failed to start session on main branch: %w", err)
		return startErr
	}

	i.SetStatus(Running)
	return nil
}

// StartOnBranch creates a worktree on the specified branch (reusing an existing
// branch when it already exists) and starts the tmux session inside it.
func (i *Instance) StartOnBranch(branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 8
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	i.tmuxSession = i.prepareTmuxSession()
	i.tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	i.configureSessionTitle()
	i.tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(3+stage, desc)
	}
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

	i.setLoadingProgress(4, "Starting tmux session...")
	if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
		if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup: %v)", err, cleanupErr)
		}
		startErr = fmt.Errorf("failed to start tmux session: %w", err)
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

	i.tmuxSession = i.prepareTmuxSession()
	i.tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	i.configureSessionTitle()
	i.tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	}
	i.transferPromptToCli()

	i.setLoadingProgress(2, "Starting tmux session...")
	if err := i.tmuxSession.Start(worktree.GetWorktreePath()); err != nil {
		return fmt.Errorf("failed to start session in shared worktree: %w", err)
	}

	i.started = true
	i.SetStatus(Running)
	return nil
}

// Kill terminates the tmux session and removes the git worktree.
// The git branch is preserved so the instance can be inspected or resumed later.
// Returns nil for instances that were never started.
func (i *Instance) Kill() error {
	if !i.started {
		return nil
	}

	var errs []error

	// Close tmux first — it holds an open handle to the worktree directory.
	if i.tmuxSession != nil {
		if err := i.tmuxSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close tmux session: %w", err))
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

// StopTmux closes the underlying tmux session without touching the worktree or
// any other instance state. The instance remains in the list as stopped.
func (i *Instance) StopTmux() {
	if i.tmuxSession != nil {
		_ = i.tmuxSession.Close()
	}
}

// Pause detaches from the tmux session and removes the git worktree, preserving
// the branch for a later Resume.
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	var errs []error

	if err := i.tmuxSession.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", err))
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
	i.tmuxSession = ts
	if err := ts.Restore(); err != nil {
		return fmt.Errorf("failed to adopt orphan session %s: %w", tmuxName, err)
	}
	i.started = true
	i.SetStatus(Ready)
	return nil
}

// Restart closes the current tmux session (best-effort) and launches a fresh one
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
	if i.tmuxSession != nil {
		_ = i.tmuxSession.Close()
	}

	// Allocate a new session object, carrying over injected test dependencies.
	ts := i.tmuxSession.NewReset(i.Title, i.Program, i.SkipPermissions)
	i.tmuxSession = ts
	ts.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	i.configureSessionTitle()

	workDir := i.Path
	if i.gitWorktree != nil {
		workDir = i.gitWorktree.GetWorktreePath()
	}

	if err := ts.Start(workDir); err != nil {
		return fmt.Errorf("failed to restart tmux session: %w", err)
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
// a fresh tmux session inside it.
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

	if i.tmuxSession.DoesSessionExist() {
		if restoreErr := i.tmuxSession.Restore(); restoreErr != nil {
			log.ErrorLog.Print(restoreErr)
			// Fall back to a fresh session start.
			if startErr := i.tmuxSession.Start(worktreePath); startErr != nil {
				log.ErrorLog.Print(startErr)
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					startErr = fmt.Errorf("%v (cleanup: %v)", startErr, cleanupErr)
					log.ErrorLog.Print(startErr)
				}
				return fmt.Errorf("failed to start new session: %w", startErr)
			}
		}
	} else {
		if err := i.tmuxSession.Start(worktreePath); err != nil {
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
