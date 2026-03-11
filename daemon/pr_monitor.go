package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	gitpkg "github.com/kastheco/kasmos/session/git"
)

// dispatchFunc is the function signature used by PRMonitor to execute actions
// through the daemon's existing executeAction path.
type dispatchFunc func(ctx context.Context, e RepoEntry, action loop.Action) error

// PRMonitor polls open pull requests linked to tasks, acknowledges new review
// comments with a reaction, and spawns a fixer agent to address reviewer feedback.
// It runs as a goroutine alongside the daemon's existing tick loop.
//
// Architecture summary:
//   - Run creates a ticker at cfg.PollInterval and calls pollOnce on each tick.
//   - pollOnce iterates repos (mirroring daemon.go:345) and calls pollRepo.
//   - pollRepo fetches tasks with PRURL+Branch, verifies the PR is open, lists
//     reviews, and calls handleReview for each.
//   - handleReview records the review (INSERT OR IGNORE), conditionally reacts
//     to its inline comments, enforces the review-fix cycle cap, and dispatches
//     a SpawnFixerAction through the dispatch callback.
//   - Review IDs are tracked in the pr_reviews SQLite table to prevent re-processing.
type PRMonitor struct {
	cfg                 PRMonitorConfig
	maxReviewFixCycles  int
	repos               *RepoManager
	broadcaster         *api.EventBroadcaster
	logger              *slog.Logger
	dispatch            dispatchFunc
	ghUnavailableLogged atomic.Bool

	// Injectable seams for tests; initialised to real gitpkg functions by
	// NewPRMonitor so production code never sets them explicitly.
	ghPROpen       func(string, int) (bool, error)
	ghListReviews  func(string, int) ([]gitpkg.PRReview, error)
	ghListComments func(string, int, int) ([]gitpkg.PRReviewComment, error)
	ghAddReaction  func(string, int, string) error
}

// NewPRMonitor creates a new PRMonitor wired to the real gh CLI. The monitor
// is inactive until Run is called.
//
//   - cfg: PR monitor configuration (poll interval, reactions, enabled flag, …)
//   - maxReviewFixCycles: maximum review-fix loop iterations (0 = unlimited)
//   - repos: shared repo manager used to discover repos and their task stores
//   - broadcaster: SSE event broadcaster for emitting monitor events
//   - logger: structured logger
//   - dispatch: callback to execute loop actions (typically d.executeAction wrapped)
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
		ghPROpen:           gitpkg.PROpen,
		ghListReviews:      gitpkg.ListPRReviews,
		ghListComments:     gitpkg.ListReviewComments,
		ghAddReaction:      gitpkg.AddReviewReaction,
	}
}

// Run starts the PR monitor poll loop. It blocks until ctx is cancelled and
// returns nil on clean shutdown. Goroutine/cancellation style mirrors daemon.go:285.
func (m *PRMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.pollOnce(ctx)
		}
	}
}

// pollOnce executes one poll cycle across all registered repos.
// Repo iteration mirrors daemon.go:345.
func (m *PRMonitor) pollOnce(ctx context.Context) {
	for _, repo := range m.repos.List() {
		if ctx.Err() != nil {
			return
		}
		if repo.Store == nil {
			continue
		}
		m.pollRepo(ctx, repo)
	}
}

// pollRepo polls one repository for open PRs with unprocessed reviews.
func (m *PRMonitor) pollRepo(ctx context.Context, repo RepoEntry) {
	entries, err := repo.Store.ListByStatus(repo.Project,
		taskstore.StatusDone, taskstore.StatusReviewing)
	if err != nil {
		m.logger.Warn("pr_monitor: list tasks failed", "repo", repo.Path, "err", err)
		return
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		// Only process tasks that have both a PR URL and a branch.
		if entry.PRURL == "" || entry.Branch == "" {
			continue
		}

		prNumber, err := gitpkg.ExtractPRNumber(entry.PRURL)
		if err != nil {
			m.logger.Warn("pr_monitor: invalid pr url",
				"plan", entry.Filename, "url", entry.PRURL, "err", err)
			continue
		}

		open, err := m.ghPROpen(repo.Path, prNumber)
		if err != nil {
			if m.isGHUnavailableErr(err) {
				return // skip rest of this cycle without spamming the log
			}
			m.logger.Warn("pr_monitor: pr open check failed",
				"plan", entry.Filename, "pr", prNumber, "err", err)
			continue
		}
		if !open {
			continue
		}

		reviews, err := m.ghListReviews(repo.Path, prNumber)
		if err != nil {
			if m.isGHUnavailableErr(err) {
				return // skip rest of this cycle
			}
			m.logger.Warn("pr_monitor: list reviews failed",
				"plan", entry.Filename, "pr", prNumber, "err", err)
			continue
		}

		for _, review := range reviews {
			if ctx.Err() != nil {
				return
			}
			if err := m.handleReview(ctx, repo, entry, review); err != nil {
				m.logger.Warn("pr_monitor: handle review failed",
					"plan", entry.Filename, "review_id", review.ID, "err", err)
			}
		}
	}
}

// handleReview processes a single PR review: records it (idempotent), optionally
// reacts to its inline comments, enforces the review-fix cycle cap, and dispatches
// a fixer agent.
func (m *PRMonitor) handleReview(ctx context.Context, repo RepoEntry, entry taskstore.TaskEntry, review gitpkg.PRReview) error {
	// RecordPRReview uses INSERT OR IGNORE, so repeated polls are idempotent.
	if err := repo.Store.RecordPRReview(
		repo.Project, entry.Filename,
		review.ID, review.State, review.Body, review.User,
	); err != nil {
		return fmt.Errorf("record pr review %d: %w", review.ID, err)
	}

	if !shouldTriggerFixer(review) {
		return nil
	}

	// Check whether fixer has already been dispatched for this review by
	// looking it up in the pending set (fixer_dispatched = 0 rows).
	pending, err := repo.Store.ListPendingReviews(repo.Project, entry.Filename)
	if err != nil {
		return fmt.Errorf("list pending reviews: %w", err)
	}
	var pendingEntry *taskstore.PRReviewEntry
	for i := range pending {
		if pending[i].ReviewID == review.ID {
			pendingEntry = &pending[i]
			break
		}
	}
	if pendingEntry == nil {
		// Already dispatched on a previous cycle; nothing more to do.
		return nil
	}

	// Emit pr_review_detected before any side effect.
	m.broadcaster.Emit(api.Event{
		Kind:     "pr_review_detected",
		Message:  "PR review detected: " + review.State,
		Repo:     repo.Path,
		PlanFile: entry.Filename,
	})

	// Reaction step — best-effort, only when there are inline comments and the
	// reaction has not already been posted (supports retry after partial runs).
	if !pendingEntry.ReactionPosted {
		prNumber, _ := gitpkg.ExtractPRNumber(entry.PRURL)
		comments, commErr := m.ghListComments(repo.Path, prNumber, review.ID)
		if commErr != nil {
			// list-comments failure is non-fatal; log and continue without reaction.
			m.logger.Warn("pr_monitor: list review comments failed",
				"plan", entry.Filename, "review_id", review.ID, "err", commErr)
		} else if len(comments) > 0 {
			firstID := comments[0].ID
			reactionOK := true
			for _, reaction := range m.cfg.Reactions {
				if rErr := m.ghAddReaction(repo.Path, firstID, reaction); rErr != nil {
					m.logger.Warn("pr_monitor: add reaction failed",
						"plan", entry.Filename,
						"comment_id", firstID,
						"reaction", reaction,
						"err", rErr)
					reactionOK = false
					break
				}
			}
			if !reactionOK {
				// Leave fixer_dispatched = 0 so the next poll retries from
				// persisted state (matches task specification edge case).
				return nil
			}
			// All reactions posted — persist and announce.
			if markErr := repo.Store.MarkReviewReacted(
				repo.Project, entry.Filename, review.ID,
			); markErr != nil {
				m.logger.Warn("pr_monitor: mark review reacted failed",
					"plan", entry.Filename, "review_id", review.ID, "err", markErr)
			}
			m.broadcaster.Emit(api.Event{
				Kind:     "pr_reaction_posted",
				Message:  "reaction posted for review",
				Repo:     repo.Path,
				PlanFile: entry.Filename,
			})
		}
		// If no comments, fall through and still dispatch the fixer
		// (reaction support is best-effort per the task spec).
	}

	// Enforce max_review_fix_cycles exactly like processor.go:160.
	if m.maxReviewFixCycles > 0 {
		currentEntry, getErr := repo.Store.Get(repo.Project, entry.Filename)
		if getErr != nil {
			return fmt.Errorf("get task for cycle check: %w", getErr)
		}
		if currentEntry.ReviewCycle+1 > m.maxReviewFixCycles {
			return m.dispatch(ctx, repo, loop.ReviewCycleLimitAction{
				PlanFile: entry.Filename,
				Cycle:    currentEntry.ReviewCycle + 1,
				Limit:    m.maxReviewFixCycles,
			})
		}
	}

	// Dispatch the fixer agent via the daemon's executeAction path.
	if err := m.dispatch(ctx, repo, loop.SpawnFixerAction{
		PlanFile: entry.Filename,
		Feedback: review.Body,
	}); err != nil {
		// Leave fixer_dispatched = 0 so the next poll retries.
		return fmt.Errorf("dispatch spawn_fixer: %w", err)
	}

	// Mark the review so subsequent polls skip it.
	if err := repo.Store.MarkReviewFixerDispatched(repo.Project, entry.Filename, review.ID); err != nil {
		m.logger.Warn("pr_monitor: mark fixer dispatched failed",
			"plan", entry.Filename, "review_id", review.ID, "err", err)
	}
	return nil
}

// isGHUnavailableErr returns true when err indicates that the gh CLI is not
// installed or not authenticated. On the first match it logs a single structured
// warning; subsequent calls are silent (warn-once behaviour via ghUnavailableLogged).
func (m *PRMonitor) isGHUnavailableErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	unavailable := strings.Contains(s, "executable file not found") ||
		strings.Contains(s, "not found in $PATH") ||
		strings.Contains(s, "exit status 127") ||
		strings.Contains(s, "not logged in") ||
		strings.Contains(s, "not authenticated") ||
		strings.Contains(s, "authentication token") ||
		strings.Contains(s, "GITHUB_TOKEN")
	if unavailable {
		if m.ghUnavailableLogged.CompareAndSwap(false, true) {
			m.logger.Warn("pr_monitor: gh unavailable, skipping poll cycle", "err", err)
		}
	}
	return unavailable
}

// shouldTriggerFixer reports whether a review warrants spawning a fixer agent.
// Only reviews that request changes or leave substantive comments from real
// (non-bot) humans qualify.
func shouldTriggerFixer(review gitpkg.PRReview) bool {
	state := review.State
	return (state == "CHANGES_REQUESTED" || state == "COMMENTED") &&
		strings.TrimSpace(review.Body) != "" &&
		!isBotLogin(review.User)
}

// isBotLogin returns true when the GitHub login identifies an automated account.
// The "[bot]" suffix is the canonical GitHub convention for bot accounts.
func isBotLogin(login string) bool {
	lower := strings.ToLower(login)
	return strings.HasSuffix(lower, "[bot]") ||
		strings.HasSuffix(lower, "-bot") ||
		lower == "dependabot"
}
