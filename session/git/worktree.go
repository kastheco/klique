package git

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/log"
)

// getWorktreeDirectory returns the directory used to store git worktrees for
// the given repository. Returns an error when repoPath is empty.
func getWorktreeDirectory(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repo path is required for worktree directory")
	}
	return filepath.Join(repoPath, ".worktrees"), nil
}

// GitWorktree manages git worktree operations for a session.
type GitWorktree struct {
	repoPath      string
	worktreePath  string
	sessionName   string
	branchName    string
	baseCommitSHA string
}

// NewGitWorktreeFromStorage constructs a GitWorktree directly from persisted
// state without any filesystem interaction. Used when restoring from storage.
func NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) *GitWorktree {
	return &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  worktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
	}
}

// NewGitWorktree creates a GitWorktree with an auto-generated branch name
// derived from the configured branch prefix and the session name.
func NewGitWorktree(repoPath, sessionName string) (*GitWorktree, string, error) {
	cfg := config.LoadConfig()
	branchName := sanitizeBranchName(fmt.Sprintf("%s%s", cfg.BranchPrefix, sessionName))

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.ErrorLog.Printf("git worktree path abs error, falling back to repoPath %s: %s", repoPath, err)
		absPath = repoPath
	}

	resolvedRoot, err := findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory(resolvedRoot)
	if err != nil {
		return nil, "", err
	}

	worktreePath := fmt.Sprintf("%s_%x", filepath.Join(worktreeDir, branchName), time.Now().UnixNano())

	return &GitWorktree{
		repoPath:     resolvedRoot,
		worktreePath: worktreePath,
		sessionName:  sessionName,
		branchName:   branchName,
	}, branchName, nil
}

// NewGitWorktreeOnBranch creates a GitWorktree targeting a specific branch
// rather than generating one from the config prefix. The branch name is
// sanitized before use. Setup handles whether the branch already exists.
func NewGitWorktreeOnBranch(repoPath, sessionName, branch string) (*GitWorktree, string, error) {
	branch = sanitizeBranchName(branch)

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	resolvedRoot, err := findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory(resolvedRoot)
	if err != nil {
		return nil, "", err
	}

	worktreePath := fmt.Sprintf("%s_%x", filepath.Join(worktreeDir, branch), time.Now().UnixNano())

	return &GitWorktree{
		repoPath:     resolvedRoot,
		worktreePath: worktreePath,
		sessionName:  sessionName,
		branchName:   branch,
	}, branch, nil
}

// GetWorktreePath returns the filesystem path of the worktree.
func (g *GitWorktree) GetWorktreePath() string {
	return g.worktreePath
}

// GetBranchName returns the git branch associated with this worktree.
func (g *GitWorktree) GetBranchName() string {
	return g.branchName
}

// GetRepoPath returns the root path of the git repository.
func (g *GitWorktree) GetRepoPath() string {
	return g.repoPath
}

// GetRepoName returns the base name of the repository directory.
func (g *GitWorktree) GetRepoName() string {
	return filepath.Base(g.repoPath)
}

// GetBaseCommitSHA returns the commit SHA recorded as the base when the
// worktree was set up.
func (g *GitWorktree) GetBaseCommitSHA() string {
	return g.baseCommitSHA
}
