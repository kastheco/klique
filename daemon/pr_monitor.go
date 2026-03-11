package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	gitpkg "github.com/kastheco/kasmos/session/git"
)

// dispatchFunc is the function signature used by PRMonitor to execute actions
// through the daemon's existing executeAction path.
type dispatchFunc func(ctx context.Context, e RepoEntry, action loop.Action) error

// PRMonitor polls open pull requests linked to tasks and automatically spawns
// fixer agents to address reviewer feedback. It runs as a goroutine alongside
// the daemon's existing tick loop.
//
// For each repo, it queries the task store for tasks with a non-empty PRURL,
// shells out to the `gh` CLI to fetch PR reviews, records unprocessed reviews,
// posts an "eyes" reaction to their inline comments, and emits a
// SpawnFixerAction via the daemon's executeAction path. Review IDs are tracked
// in the pr_reviews SQLite table to prevent re-processing.
type PRMonitor struct {
	cfg                PRMonitorConfig
	maxReviewFixCycles int
	repos              *RepoManager
	broadcaster        *api.EventBroadcaster
	logger             *slog.Logger
	dispatch           dispatchFunc
}

// NewPRMonitor creates a new PRMonitor. The monitor is inactive until Run is
// called.
//
//   - cfg: PR monitor configuration (poll interval, reactions, etc.)
//   - maxReviewFixCycles: maximum number of review-fix cycles (0 = unlimited)
//   - repos: shared repo manager used to find repos and their task stores
//   - broadcaster: SSE event broadcaster for emitting monitor events
//   - logger: structured logger
//   - dispatch: callback to execute loop actions (typically d.executeAction)
func NewPRMonitor(
	cfg PRMonitorConfig,
	maxReviewFixCycles int,
	repos *RepoManager,
	broadcaster *api.EventBroadcaster,
	logger *slog.Logger,
	dispatch dispatchFunc,
) *PRMonitor {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultDaemonConfig().PRMonitor.PollInterval
	}
	if len(cfg.Reactions) == 0 {
		cfg.Reactions = defaultDaemonConfig().PRMonitor.Reactions
	}
	return &PRMonitor{
		cfg:                cfg,
		maxReviewFixCycles: maxReviewFixCycles,
		repos:              repos,
		broadcaster:        broadcaster,
		logger:             logger,
		dispatch:           dispatch,
	}
}

// Run starts the PR monitor poll loop. It blocks until ctx is cancelled.
// Returns nil on clean shutdown (context cancelled).
func (m *PRMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

// poll iterates over all registered repos and checks for new PR review
// comments on tasks that have an open pull request.
func (m *PRMonitor) poll(ctx context.Context) {
	for _, e := range m.repos.List() {
		if ctx.Err() != nil {
			return
		}
		if e.Store == nil {
			continue
		}
		m.pollRepo(ctx, e)
	}
}

// pollRepo processes all tasks with a PRURL in the given repo.
func (m *PRMonitor) pollRepo(ctx context.Context, e RepoEntry) {
	tasks, err := e.Store.List(e.Project)
	if err != nil {
		m.logger.Error("pr monitor: list tasks failed", "repo", e.Path, "err", err)
		return
	}

	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if task.PRURL == "" {
			continue
		}
		// Skip tasks that have exceeded the review-fix cycle limit.
		if m.maxReviewFixCycles > 0 && task.ReviewCycle >= m.maxReviewFixCycles {
			m.logger.Debug("pr monitor: skipping task at cycle limit",
				"task", task.Filename, "cycle", task.ReviewCycle, "limit", m.maxReviewFixCycles)
			continue
		}
		m.pollTask(ctx, e, task.Filename, task.PRURL)
	}
}

// pollTask fetches reviews for a single PR and processes any unhandled ones.
func (m *PRMonitor) pollTask(ctx context.Context, e RepoEntry, filename, prURL string) {
	prNumber, err := gitpkg.ExtractPRNumber(prURL)
	if err != nil {
		m.logger.Warn("pr monitor: invalid pr url", "task", filename, "url", prURL, "err", err)
		return
	}

	open, err := gitpkg.PROpen(e.Path, prNumber)
	if err != nil {
		m.logger.Warn("pr monitor: pr open check failed", "task", filename, "pr", prNumber, "err", err)
		return
	}
	if !open {
		m.logger.Debug("pr monitor: pr is not open, skipping", "task", filename, "pr", prNumber)
		return
	}

	reviews, err := gitpkg.ListPRReviews(e.Path, prNumber)
	if err != nil {
		m.logger.Warn("pr monitor: list pr reviews failed", "task", filename, "pr", prNumber, "err", err)
		return
	}

	for _, review := range reviews {
		if ctx.Err() != nil {
			return
		}
		// Skip pure approvals with no body text — no actionable feedback.
		if review.State == "APPROVED" && review.Body == "" {
			continue
		}
		m.processReview(ctx, e, filename, prNumber, review)
	}
}

// processReview handles a single PR review: records it in the store,
// posts reactions to its inline comments, and dispatches a fixer if needed.
func (m *PRMonitor) processReview(ctx context.Context, e RepoEntry, filename string, prNumber int, review gitpkg.PRReview) {
	// Record the review in the store (INSERT OR IGNORE — idempotent).
	if err := e.Store.RecordPRReview(e.Project, filename, review.ID, review.State, review.Body, review.User); err != nil {
		m.logger.Error("pr monitor: record pr review failed",
			"task", filename, "review_id", review.ID, "err", err)
		return
	}

	// Fetch pending reviews (fixer_dispatched = 0).
	pending, err := e.Store.ListPendingReviews(e.Project, filename)
	if err != nil {
		m.logger.Error("pr monitor: list pending reviews failed", "task", filename, "err", err)
		return
	}

	// Check whether this review still needs a fixer dispatched.
	var needsDispatch bool
	for _, p := range pending {
		if p.ReviewID == review.ID {
			needsDispatch = true
			break
		}
	}
	if !needsDispatch {
		return
	}

	// Post reactions to all inline comments for this review.
	m.reactToReviewComments(e.Path, prNumber, review.ID)

	// Mark reaction as posted in the store.
	if err := e.Store.MarkReviewReacted(e.Project, filename, review.ID); err != nil {
		m.logger.Warn("pr monitor: mark review reacted failed",
			"task", filename, "review_id", review.ID, "err", err)
	}

	// Build feedback message from review body.
	feedback := review.Body
	if feedback == "" {
		feedback = "address reviewer feedback from PR review"
	}

	// Dispatch a fixer agent via the daemon's executeAction path.
	action := loop.SpawnFixerAction{
		PlanFile: filename,
		Feedback: feedback,
	}
	m.logger.Info("pr monitor: dispatching fixer",
		"task", filename, "review_id", review.ID, "reviewer", review.User)

	if err := m.dispatch(ctx, e, action); err != nil {
		m.logger.Error("pr monitor: dispatch fixer failed",
			"task", filename, "review_id", review.ID, "err", err)
		// Do NOT mark fixer dispatched when dispatch failed.
		return
	}

	// Only record dispatch success after the action actually succeeded.
	if err := e.Store.MarkReviewFixerDispatched(e.Project, filename, review.ID); err != nil {
		m.logger.Error("pr monitor: mark review fixer dispatched failed",
			"task", filename, "review_id", review.ID, "err", err)
	}

	m.broadcaster.Emit(api.Event{
		Kind:     "fixer_dispatched",
		Message:  "fixer dispatched for " + filename + " (review by " + review.User + ")",
		Repo:     e.Path,
		PlanFile: filename,
	})
}

// reactToReviewComments posts the configured reactions to all inline comments
// belonging to reviewID. Errors are logged but do not abort processing.
func (m *PRMonitor) reactToReviewComments(repoPath string, prNumber, reviewID int) {
	comments, err := gitpkg.ListReviewComments(repoPath, prNumber, reviewID)
	if err != nil {
		m.logger.Warn("pr monitor: list review comments failed",
			"pr", prNumber, "review_id", reviewID, "err", err)
		return
	}
	for _, comment := range comments {
		for _, reaction := range m.cfg.Reactions {
			if err := gitpkg.AddReviewReaction(repoPath, comment.ID, reaction); err != nil {
				m.logger.Warn("pr monitor: add reaction failed",
					"comment_id", comment.ID, "reaction", reaction, "err", err)
			}
		}
	}
}
