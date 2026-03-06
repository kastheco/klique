package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kastheco/kasmos/log"
)

// PRState holds the current state of a GitHub pull request as returned by
// `gh pr view --json url,reviewDecision,statusCheckRollup,isDraft,number`.
type PRState struct {
	URL            string
	ReviewDecision string
	CheckStatus    string
	IsDraft        bool
	Number         int
}

// ParsePRViewJSON parses the JSON output of `gh pr view --json ...` into a PRState.
// A missing statusCheckRollup is not an error — CheckStatus will be empty.
func ParsePRViewJSON(data []byte) (PRState, error) {
	var raw struct {
		URL               string `json:"url"`
		ReviewDecision    string `json:"reviewDecision"`
		StatusCheckRollup *struct {
			State string `json:"state"`
		} `json:"statusCheckRollup"`
		IsDraft bool `json:"isDraft"`
		Number  int  `json:"number"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return PRState{}, fmt.Errorf("parse pr view json: %w", err)
	}
	state := PRState{
		URL:            raw.URL,
		ReviewDecision: raw.ReviewDecision,
		IsDraft:        raw.IsDraft,
		Number:         raw.Number,
	}
	if raw.StatusCheckRollup != nil {
		state.CheckStatus = raw.StatusCheckRollup.State
	}
	return state, nil
}

// QueryPRState queries GitHub for the current state of the pull request
// associated with the worktree's branch. It returns a zero PRState (and nil
// error) when no pull request exists for the branch.
func (g *GitWorktree) QueryPRState() (PRState, error) {
	cmd := exec.Command("gh", "pr", "view", g.branchName,
		"--json", "url,reviewDecision,statusCheckRollup,isDraft,number")
	cmd.Dir = g.worktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "no pull requests found") {
			return PRState{}, nil
		}
		return PRState{}, fmt.Errorf("failed to query PR state: %s (%w)", out, err)
	}
	return ParsePRViewJSON(out)
}

// PostGitHubReview posts a review comment (or approval) to the pull request
// identified by prNumber. When approve is true the review is submitted as an
// approval; otherwise it is posted as a plain comment.
func (g *GitWorktree) PostGitHubReview(prNumber int, body string, approve bool) error {
	if prNumber <= 0 {
		return fmt.Errorf("invalid pr number: %d", prNumber)
	}
	numStr := fmt.Sprintf("%d", prNumber)
	var cmd *exec.Cmd
	if approve {
		cmd = exec.Command("gh", "pr", "review", numStr, "--approve", "-b", body)
	} else {
		cmd = exec.Command("gh", "pr", "review", numStr, "--comment", "-b", body)
	}
	cmd.Dir = g.worktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to post github review: %s (%w)", out, err)
	}
	return nil
}

// runGitCommand executes a git command with the given path as the working directory.
// It builds the invocation as: git -C <path> <args...>, captures combined stdout+stderr,
// and wraps any error with the captured output for context.
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", path}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", out, err)
	}
	return string(out), nil
}

// PushChanges stages, commits, and pushes the worktree's current state to the remote.
// It requires the GitHub CLI (gh) to be present and authenticated.
// When open is true the branch URL is opened in the browser after a successful push.
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}
	if err := g.CommitChanges(commitMessage); err != nil {
		return err
	}
	return g.Push(open)
}

// Push pushes the current branch to origin without committing first.
// If open is true it attempts to open the remote branch URL; any error from
// that step is logged but not returned.
func (g *GitWorktree) Push(open bool) error {
	if _, err := g.runGitCommand(g.worktreePath, "push", "-u", "origin", g.branchName); err != nil {
		return fmt.Errorf("failed to push branch %s: %w", g.branchName, err)
	}
	if open {
		if err := g.OpenBranchURL(); err != nil {
			log.ErrorLog.Printf("failed to open branch URL: %v", err)
		}
	}
	return nil
}

// GeneratePRBody builds a markdown pull-request description that summarises
// the files changed, the commit history, and diff statistics since baseCommitSHA.
// It returns an error if no base commit SHA is available.
func (g *GitWorktree) GeneratePRBody() (string, error) {
	base := g.GetBaseCommitSHA()
	if base == "" {
		return "", fmt.Errorf("no base commit SHA available")
	}

	var sections []string

	// List of files that changed relative to the base commit.
	if files, err := g.runGitCommand(g.worktreePath, "diff", "--name-only", base); err == nil {
		if trimmed := strings.TrimSpace(files); trimmed != "" {
			sections = append(sections, "## Changes\n\n"+trimmed)
		}
	}

	// One-line commit log from base to HEAD.
	if commits, err := g.runGitCommand(g.worktreePath, "log", "--oneline", base+"..HEAD"); err == nil {
		if trimmed := strings.TrimSpace(commits); trimmed != "" {
			sections = append(sections, "## Commits\n\n"+trimmed)
		}
	}

	// Summary statistics of insertions/deletions.
	if stats, err := g.runGitCommand(g.worktreePath, "diff", "--stat", base); err == nil {
		if trimmed := strings.TrimSpace(stats); trimmed != "" {
			sections = append(sections, "## Stats\n\n"+trimmed)
		}
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n"), nil
}

// CreatePR pushes the current branch and opens a pull request on GitHub.
// If the PR already exists it opens the existing one in the browser instead.
func (g *GitWorktree) CreatePR(title, body, commitMsg string) error {
	if err := g.PushChanges(commitMsg, false); err != nil {
		return fmt.Errorf("failed to push changes: %w", err)
	}

	prCmd := exec.Command("gh", "pr", "create", "--title", title, "--body", body, "--head", g.branchName)
	prCmd.Dir = g.worktreePath
	out, err := prCmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "already exists") {
			viewCmd := exec.Command("gh", "pr", "view", "--web", g.branchName)
			viewCmd.Dir = g.worktreePath
			_ = viewCmd.Run()
			return nil
		}
		return fmt.Errorf("failed to create PR: %s (%w)", out, err)
	}

	viewCmd := exec.Command("gh", "pr", "view", "--web", g.branchName)
	viewCmd.Dir = g.worktreePath
	_ = viewCmd.Run()
	return nil
}

// CommitChanges stages all changes and creates a commit with the given message.
// It is a no-op when the worktree is clean.
func (g *GitWorktree) CommitChanges(commitMessage string) error {
	dirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}
	if !dirty {
		return nil
	}
	if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to stage changes: %w", err)
	}
	if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	return nil
}

// IsDirty reports whether the worktree contains uncommitted changes.
func (g *GitWorktree) IsDirty() (bool, error) {
	out, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return len(out) > 0, nil
}

// IsBranchCheckedOut reports whether the configured branch is the currently
// checked-out branch in the repository (not in the worktree).
func (g *GitWorktree) IsBranchCheckedOut() (bool, error) {
	out, err := g.runGitCommand(g.repoPath, "branch", "--show-current")
	if err != nil {
		return false, fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(out) == g.branchName, nil
}

// OpenBranchURL opens the remote branch page in the default browser using the
// GitHub CLI. It requires gh to be present and authenticated.
func (g *GitWorktree) OpenBranchURL() error {
	if err := checkGHCLI(); err != nil {
		return err
	}
	cmd := exec.Command("gh", "browse", "--branch", g.branchName)
	cmd.Dir = g.worktreePath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open branch URL: %w", err)
	}
	return nil
}
