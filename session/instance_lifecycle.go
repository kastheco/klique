package session

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"

	"github.com/atotto/clipboard"
)

// firstTimeSetup is true if this is a new instance. Otherwise, it's one loaded from storage.
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
	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	i.tmuxSession = tmuxSession
	tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	// Wire up tmux progress to instance loading progress
	tmuxStageOffset := 3 // tmux stages start at 4 for first-time, 2 for reload
	if !firstTimeSetup {
		tmuxStageOffset = 1
	}
	tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(tmuxStageOffset+stage, desc)
	}
	i.transferPromptToCli()

	if firstTimeSetup {
		i.setLoadingProgress(2, "Creating git worktree...")
		gitWorktree, branchName, err := git.NewGitWorktree(i.Path, i.Title)
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitWorktree = gitWorktree
		i.Branch = branchName
	}

	// Setup error handler to cleanup resources on any error
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	if !firstTimeSetup {
		i.setLoadingProgress(2, "Restoring session...")
		// Reuse existing session
		if err := tmuxSession.Restore(); err != nil {
			setupErr = fmt.Errorf("failed to restore existing session: %w", err)
			return setupErr
		}
	} else {
		i.setLoadingProgress(3, "Setting up git worktree...")
		// Setup git worktree first
		if err := i.gitWorktree.Setup(); err != nil {
			setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
			return setupErr
		}

		i.setLoadingProgress(4, "Starting tmux session...")
		// Create new session
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			// Cleanup git worktree if tmux session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}
	}

	i.SetStatus(Running)

	return nil
}

// StartOnMainBranch starts the instance in the repo root without creating a git worktree.
// Used for planner agents that commit directly to main.
func (i *Instance) StartOnMainBranch() error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 5
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	i.tmuxSession = tmuxSession
	tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	}
	i.transferPromptToCli()

	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	if err := i.tmuxSession.Start(i.Path); err != nil {
		setupErr = fmt.Errorf("failed to start session on main branch: %w", err)
		return setupErr
	}

	i.SetStatus(Running)
	return nil
}

// StartOnBranch starts the instance in a worktree checked out to the specified branch.
// If the branch exists, it reuses it. If not, it creates a new branch from HEAD.
// Used for ad-hoc agent sessions with a branch override.
func (i *Instance) StartOnBranch(branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 8
	i.LoadingStage = 0
	i.LoadingMessage = "Initializing..."

	i.setLoadingProgress(1, "Preparing session...")
	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	i.tmuxSession = tmuxSession
	tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	tmuxSession.ProgressFunc = func(stage int, desc string) {
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

	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	i.setLoadingProgress(3, "Setting up git worktree...")
	if err := i.gitWorktree.Setup(); err != nil {
		setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
		return setupErr
	}

	i.setLoadingProgress(4, "Starting tmux session...")
	if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
		if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
		}
		setupErr = fmt.Errorf("failed to start new session: %w", err)
		return setupErr
	}

	i.SetStatus(Running)
	return nil
}

// StartInSharedWorktree starts the instance using a topic's shared worktree.
// Unlike Start(), this does NOT create a new git worktree — it uses the one provided.
func (i *Instance) StartInSharedWorktree(worktree *git.GitWorktree, branch string) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.LoadingTotal = 6
	i.setLoadingProgress(1, "Connecting to shared worktree...")

	i.gitWorktree = worktree
	i.Branch = branch
	i.sharedWorktree = true

	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	i.tmuxSession = tmuxSession
	tmuxSession.SetAgentType(i.AgentType)
	i.setTmuxTaskEnv()
	tmuxSession.ProgressFunc = func(stage int, desc string) {
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

// transferPromptToCli moves QueuedPrompt into the tmux session's initialPrompt
// for programs that support CLI prompt injection. For unsupported programs,
// QueuedPrompt stays set so the send-keys fallback fires.
func (i *Instance) transferPromptToCli() {
	if i.QueuedPrompt != "" && programSupportsCliPrompt(i.Program) {
		i.tmuxSession.SetInitialPrompt(i.QueuedPrompt)
		i.QueuedPrompt = ""
	}
}

// setTmuxTaskEnv wires task/wave/peer identity to the tmux session for env var injection.
func (i *Instance) setTmuxTaskEnv() {
	if i.TaskNumber > 0 && i.tmuxSession != nil {
		i.tmuxSession.SetTaskEnv(i.TaskNumber, i.WaveNumber, i.PeerCount)
	}
}

// Kill terminates the instance and cleans up all resources
func (i *Instance) Kill() error {
	if !i.started {
		// If instance was never started, just return success
		return nil
	}

	var errs []error

	// Always try to cleanup both resources, even if one fails
	// Clean up tmux session first since it's using the git worktree
	if i.tmuxSession != nil {
		if err := i.tmuxSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close tmux session: %w", err))
		}
	}

	// Then clean up git worktree (skip if shared — topic owns the worktree)
	if i.gitWorktree != nil && !i.sharedWorktree {
		if err := i.gitWorktree.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
		}
	}

	return errors.Join(errs...)
}

// StopTmux terminates the tmux session only. Does not touch the worktree,
// persistence, or plan state. The instance stays in the list as stopped.
func (i *Instance) StopTmux() {
	if i.tmuxSession != nil {
		_ = i.tmuxSession.Close()
	}
}

// Pause stops the tmux session and removes the worktree, preserving the branch
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	var errs []error

	if !i.sharedWorktree && i.gitWorktree != nil {
		// Check if there are any changes to commit
		if dirty, err := i.gitWorktree.IsDirty(); err != nil {
			errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
			log.ErrorLog.Print(err)
		} else if dirty {
			// Commit changes locally (without pushing to GitHub)
			commitMsg := fmt.Sprintf("[kas] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
			if err := i.gitWorktree.CommitChanges(commitMsg); err != nil {
				errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
				log.ErrorLog.Print(err)
				// Return early if we can't commit changes to avoid corrupted state
				return errors.Join(errs...)
			}
		}
	}

	// Detach from tmux session instead of closing to preserve session output
	if err := i.tmuxSession.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", err))
		log.ErrorLog.Print(err)
		// Continue with pause process even if detach fails
	}

	if !i.sharedWorktree && i.gitWorktree != nil {
		// Check if worktree exists before trying to remove it
		if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
			// Remove worktree but keep branch
			if err := i.gitWorktree.Remove(); err != nil {
				errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
				log.ErrorLog.Print(err)
				return errors.Join(errs...)
			}

			// Only prune if remove was successful
			if err := i.gitWorktree.Prune(); err != nil {
				errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
				log.ErrorLog.Print(err)
				return errors.Join(errs...)
			}
		}
	}

	if err := errors.Join(errs...); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.SetStatus(Paused)
	if i.gitWorktree != nil {
		_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	}
	return nil
}

// AdoptOrphanTmuxSession connects this instance to an existing orphaned tmux session
// identified by its raw tmux name. No worktree is created — the session keeps its
// existing working directory.
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

// Resume recreates the worktree and restarts the tmux session
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	// Check if branch is checked out
	if checked, err := i.gitWorktree.IsBranchCheckedOut(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	} else if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	// Setup git worktree
	if err := i.gitWorktree.Setup(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to setup git worktree: %w", err)
	}

	// Check if tmux session still exists from pause, otherwise create new one
	if i.tmuxSession.DoesSessionExist() {
		// Session exists, just restore PTY connection to it
		if err := i.tmuxSession.Restore(); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if tmux session creation fails
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Create new tmux session
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if tmux session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				log.ErrorLog.Print(err)
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.SetStatus(Running)
	return nil
}
