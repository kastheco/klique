package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ghExecutor abstracts `gh` command execution for testability.
// It intentionally mirrors the cmd.Executor interface so that cmd_test.MockCmdExec
// satisfies it without creating an import cycle (cmd → session/git → cmd).
type ghExecutor interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

// realGHExec is the default executor that delegates to os/exec.
type realGHExec struct{}

func (realGHExec) Run(c *exec.Cmd) error              { return c.Run() }
func (realGHExec) Output(c *exec.Cmd) ([]byte, error) { return c.Output() }

// ghExec is the package-level executor used for all gh CLI invocations.
// Tests replace this with a mock to avoid real subprocess calls.
var ghExec ghExecutor = realGHExec{}

// PRReview holds the details of a single pull request review as returned by
// the GitHub API.
type PRReview struct {
	ID          int
	State       string
	Body        string
	User        string
	SubmittedAt time.Time
}

// PRReviewComment holds a pull request review comment as returned by the
// GitHub API. Only the comment ID is populated since that is the only field
// needed to post reactions.
type PRReviewComment struct {
	ID int
}

// ghOutput runs `gh <args...>` in repoPath and returns the stdout bytes.
// Stderr is captured into a buffer so any error message includes the CLI output.
func ghOutput(repoPath string, args ...string) ([]byte, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath must not be empty")
	}
	var stderr bytes.Buffer
	c := exec.Command("gh", args...)
	c.Dir = repoPath
	c.Stderr = &stderr
	out, err := ghExec.Output(c)
	if err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// ghRun runs `gh <args...>` in repoPath and discards stdout.
// Stderr is captured into a buffer so any error message includes the CLI output.
func ghRun(repoPath string, args ...string) error {
	if repoPath == "" {
		return fmt.Errorf("repoPath must not be empty")
	}
	var stderr bytes.Buffer
	c := exec.Command("gh", args...)
	c.Dir = repoPath
	c.Stderr = &stderr
	if err := ghExec.Run(c); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// ExtractPRNumber parses a GitHub pull request URL and returns the PR number.
// Returns an error for non-positive numbers or malformed URLs.
func ExtractPRNumber(prURL string) (int, error) {
	if prURL == "" {
		return 0, fmt.Errorf("pr url must not be empty")
	}
	u, err := url.Parse(prURL)
	if err != nil {
		return 0, fmt.Errorf("parse pr url %q: %w", prURL, err)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	// Expected path segments: owner / repo / pull / <number>
	if len(parts) < 4 || parts[len(parts)-2] != "pull" {
		return 0, fmt.Errorf("pr url %q has unexpected format (want .../pull/<number>)", prURL)
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("pr url %q: pr number is not an integer: %w", prURL, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("pr url %q: pr number must be positive, got %d", prURL, n)
	}
	return n, nil
}

// ExtractOwnerRepo runs `gh repo view --json nameWithOwner` inside repoPath
// and returns the repository owner and name.
// Returns an error if the output cannot be parsed or the owner/repo format is
// unexpected.
func ExtractOwnerRepo(repoPath string) (owner, repo string, err error) {
	if repoPath == "" {
		return "", "", fmt.Errorf("repoPath must not be empty")
	}
	out, err := ghOutput(repoPath, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return "", "", fmt.Errorf("extract owner/repo: %w", err)
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", "", fmt.Errorf("extract owner/repo: unmarshal: %w", err)
	}
	parts := strings.SplitN(v.NameWithOwner, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("extract owner/repo: unexpected nameWithOwner format %q", v.NameWithOwner)
	}
	return parts[0], parts[1], nil
}

// ListPRReviews returns all reviews for pull request prNumber in the repository
// at repoPath. The list may be empty when no reviews exist yet.
func ListPRReviews(repoPath string, prNumber int) ([]PRReview, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath must not be empty")
	}
	if prNumber <= 0 {
		return nil, fmt.Errorf("invalid pr number: %d", prNumber)
	}
	owner, repo, err := ExtractOwnerRepo(repoPath)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews?per_page=100", owner, repo, prNumber)
	out, err := ghOutput(repoPath, "api", endpoint)
	if err != nil {
		return nil, fmt.Errorf("list pr reviews: %w", err)
	}
	var raw []struct {
		ID    int    `json:"id"`
		State string `json:"state"`
		Body  string `json:"body"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
		SubmittedAt string `json:"submitted_at"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("list pr reviews: unmarshal: %w", err)
	}
	reviews := make([]PRReview, 0, len(raw))
	for _, r := range raw {
		rev := PRReview{
			ID:    r.ID,
			State: r.State,
			Body:  r.Body,
			User:  r.User.Login,
		}
		if r.SubmittedAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, r.SubmittedAt); parseErr == nil {
				rev.SubmittedAt = t
			}
		}
		reviews = append(reviews, rev)
	}
	return reviews, nil
}

// ListReviewComments returns the inline comments belonging to a specific
// review on a pull request. The list may be empty if the review has no inline
// comments (e.g. it is a body-only APPROVED review).
func ListReviewComments(repoPath string, prNumber, reviewID int) ([]PRReviewComment, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath must not be empty")
	}
	if prNumber <= 0 {
		return nil, fmt.Errorf("invalid pr number: %d", prNumber)
	}
	owner, repo, err := ExtractOwnerRepo(repoPath)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf(
		"repos/%s/%s/pulls/%d/reviews/%d/comments?per_page=100",
		owner, repo, prNumber, reviewID,
	)
	out, err := ghOutput(repoPath, "api", endpoint)
	if err != nil {
		return nil, fmt.Errorf("list review comments: %w", err)
	}
	var raw []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("list review comments: unmarshal: %w", err)
	}
	comments := make([]PRReviewComment, 0, len(raw))
	for _, r := range raw {
		comments = append(comments, PRReviewComment{ID: r.ID})
	}
	return comments, nil
}

// AddReviewReaction posts a reaction to a pull request review comment.
// The reaction must be one of GitHub's supported types
// ("+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", "eyes").
func AddReviewReaction(repoPath string, commentID int, reaction string) error {
	if repoPath == "" {
		return fmt.Errorf("repoPath must not be empty")
	}
	if commentID <= 0 {
		return fmt.Errorf("invalid comment id: %d", commentID)
	}
	if reaction == "" {
		return fmt.Errorf("reaction must not be empty")
	}
	owner, repo, err := ExtractOwnerRepo(repoPath)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/comments/%d/reactions", owner, repo, commentID)
	return ghRun(repoPath,
		"api",
		"--method", "POST",
		"-H", "Accept:application/vnd.github+json",
		endpoint,
		"-f", "content="+reaction,
	)
}

// PROpen reports whether the given pull request is open (neither closed nor merged).
// A 404 response is treated as a deleted/missing PR and returns false, nil so
// that callers can skip the poll cycle without crashing.
func PROpen(repoPath string, prNumber int) (bool, error) {
	if repoPath == "" {
		return false, fmt.Errorf("repoPath must not be empty")
	}
	if prNumber <= 0 {
		return false, fmt.Errorf("invalid pr number: %d", prNumber)
	}
	owner, repo, err := ExtractOwnerRepo(repoPath)
	if err != nil {
		return false, err
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, prNumber)
	out, err := ghOutput(repoPath, "api", endpoint)
	if err != nil {
		// Treat a 404 as "not found / deleted" — callers can safely skip the PR.
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("pr open check: %w", err)
	}
	var v struct {
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return false, fmt.Errorf("pr open check: unmarshal: %w", err)
	}
	if v.MergedAt != nil && *v.MergedAt != "" {
		return false, nil
	}
	return v.State == "open", nil
}
