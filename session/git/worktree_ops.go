package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/kastheco/kasmos/log"
)

// Setup creates a new worktree for the session. It creates the worktrees
// directory and determines whether the target branch already exists,
// dispatching to the appropriate setup path.
func (g *GitWorktree) Setup() error {
	wtDir, err := getWorktreeDirectory(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	type result struct{ err error }
	mkdirDone := make(chan result, 1)
	branchCheckDone := make(chan result, 1)
	var branchFound bool

	go func() {
		mkdirDone <- result{os.MkdirAll(wtDir, 0755)}
	}()

	go func() {
		repo, openErr := git.PlainOpen(g.repoPath)
		if openErr != nil {
			branchCheckDone <- result{fmt.Errorf("failed to open repository: %w", openErr)}
			return
		}
		ref := plumbing.NewBranchReferenceName(g.branchName)
		if _, refErr := repo.Reference(ref, false); refErr == nil {
			branchFound = true
		}
		branchCheckDone <- result{nil}
	}()

	if r := <-mkdirDone; r.err != nil {
		return r.err
	}
	if r := <-branchCheckDone; r.err != nil {
		return r.err
	}

	if branchFound {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from a branch that already exists
// in the repository. It first removes any stale worktree at the target path,
// syncs the local branch with the remote, then creates the fresh worktree and
// resolves the base commit SHA for diff computation.
func (g *GitWorktree) setupFromExistingBranch() error {
	// Remove any stale worktree; ignore errors (it may not exist).
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath)

	// Best-effort sync with remote before creating the worktree.
	g.syncBranchWithRemote()

	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	// Resolve the base commit SHA for diff computation.
	if g.baseCommitSHA == "" {
		if out, err := g.runGitCommand(g.repoPath, "merge-base", "HEAD", g.branchName); err == nil {
			g.baseCommitSHA = strings.TrimSpace(out)
		} else if out, err := g.runGitCommand(g.worktreePath, "rev-parse", "HEAD"); err == nil {
			g.baseCommitSHA = strings.TrimSpace(out)
		}
	}

	return nil
}

// syncBranchWithRemote reconciles the local branch with its remote counterpart.
// It fetches origin/<branch>, and if the SHAs differ it either fast-forwards
// (when local is an ancestor of remote) or rebases local commits onto remote
// (when the histories have diverged). All errors are logged but not propagated —
// this is a best-effort operation.
func (g *GitWorktree) syncBranchWithRemote() {
	remote := "origin/" + g.branchName

	_, _ = g.runGitCommand(g.repoPath, "fetch", "origin", g.branchName)

	// Bail early if the remote tracking branch does not exist.
	if _, err := g.runGitCommand(g.repoPath, "rev-parse", "--verify", remote); err != nil {
		return
	}

	localOut, err := g.runGitCommand(g.repoPath, "rev-parse", g.branchName)
	if err != nil {
		return
	}
	remoteOut, err := g.runGitCommand(g.repoPath, "rev-parse", remote)
	if err != nil {
		return
	}

	if strings.TrimSpace(localOut) == strings.TrimSpace(remoteOut) {
		return // already in sync
	}

	// Fast-forward if local is an ancestor of remote.
	if _, err := g.runGitCommand(g.repoPath, "merge-base", "--is-ancestor", g.branchName, remote); err == nil {
		if _, ffErr := g.runGitCommand(g.repoPath, "branch", "-f", g.branchName, remote); ffErr != nil {
			log.WarningLog.Printf("syncBranchWithRemote: fast-forward %s to %s failed: %v", g.branchName, remote, ffErr)
		}
		return
	}

	// Histories have diverged — do NOT rebase from the main worktree.
	// Running `git rebase ... <branch>` checks out <branch> in the current
	// worktree as a side effect, which would poison the main worktree by
	// switching it away from its own branch. Instead, leave the branch as-is
	// and let the caller handle the divergence after the worktree is created.
	log.WarningLog.Printf(
		"syncBranchWithRemote: %s has diverged from %s — leaving as-is to avoid poisoning the main worktree",
		g.branchName, remote,
	)
}

// setupNewWorktree creates a brand-new branch from the current HEAD and adds
// a worktree for it. It errors early if the repository has no commits yet.
func (g *GitWorktree) setupNewWorktree() error {
	// Remove any stale worktree; ignore errors.
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath)

	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	if err := g.cleanupExistingBranch(repo); err != nil {
		return fmt.Errorf("failed to cleanup existing branch: %w", err)
	}

	headOut, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(msg, "fatal: not a valid object name") ||
			strings.Contains(msg, "fatal: HEAD: not a valid object name") {
			return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
		}
		return fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(headOut)
	g.baseCommitSHA = headCommit

	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	return nil
}

// Cleanup removes the worktree and its associated branch, then prunes. All
// sub-errors are collected and returned together via errors.Join.
func (g *GitWorktree) Cleanup() error {
	var errs []error

	if _, statErr := os.Stat(g.worktreePath); statErr == nil {
		if _, rmErr := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); rmErr != nil {
			errs = append(errs, rmErr)
		}
	} else if !os.IsNotExist(statErr) {
		errs = append(errs, fmt.Errorf("failed to stat worktree path: %w", statErr))
	}

	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to open repository for cleanup: %w", err))
		return errors.Join(errs...)
	}

	branchRef := plumbing.NewBranchReferenceName(g.branchName)
	if _, err := repo.Reference(branchRef, false); err == nil {
		if removeErr := repo.Storer.RemoveReference(branchRef); removeErr != nil {
			errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", g.branchName, removeErr))
		}
	} else if err != plumbing.ErrReferenceNotFound {
		errs = append(errs, fmt.Errorf("error checking branch %s: %w", g.branchName, err))
	}

	if pruneErr := g.Prune(); pruneErr != nil {
		errs = append(errs, pruneErr)
	}

	return errors.Join(errs...)
}

// Remove removes the worktree filesystem entry and git metadata but leaves the
// branch intact so it can be re-attached later.
func (g *GitWorktree) Remove() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}
	return nil
}

// Prune runs `git worktree prune` to remove stale administrative files.
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes every worktree found under repoPath/.worktrees/ and
// deletes the associated branch for each one, then prunes. Worktrees that
// cannot be removed via git fall back to os.RemoveAll.
func CleanupWorktrees(repoPath string) error {
	wtDir, err := getWorktreeDirectory(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(wtDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			return "", fmt.Errorf("git %v: %s (%w)", args, out, cmdErr)
		}
		return string(out), nil
	}

	// Build a path→branch map from the porcelain worktree list.
	listOut, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	pathToBranch := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(listOut, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentPath = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			branch := strings.TrimPrefix(line, "branch ")
			branch = strings.TrimPrefix(branch, "refs/heads/")
			if currentPath != "" {
				pathToBranch[currentPath] = branch
			}
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wtPath := filepath.Join(wtDir, entry.Name())

		if _, rmErr := run("worktree", "remove", "-f", wtPath); rmErr != nil {
			log.WarningLog.Printf("git worktree remove failed for %s, falling back to os.RemoveAll: %v", wtPath, rmErr)
			if fsErr := os.RemoveAll(wtPath); fsErr != nil {
				log.ErrorLog.Printf("failed to remove worktree path %s: %v", wtPath, fsErr)
			}
		}

		// Delete the branch that was associated with this worktree path.
		for p, branch := range pathToBranch {
			if strings.Contains(p, entry.Name()) {
				if _, delErr := run("branch", "-D", branch); delErr != nil {
					log.ErrorLog.Printf("failed to delete branch %s: %v", branch, delErr)
				}
				break
			}
		}
	}

	if _, pruneErr := run("worktree", "prune"); pruneErr != nil {
		return fmt.Errorf("failed to prune worktrees: %w", pruneErr)
	}

	return nil
}
