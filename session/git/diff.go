package git

import (
	"fmt"
	"os"
	"strings"
)

// DiffStats holds the result of a diff computation between the worktree and its base commit.
type DiffStats struct {
	// Content is the full raw diff output.
	Content string
	// Added is the count of lines added (lines starting with "+" but not "+++").
	Added int
	// Removed is the count of lines removed (lines starting with "-" but not "---").
	Removed int
	// Error captures any error encountered during diff computation so callers
	// can inspect it without the method panicking or returning a nil pointer.
	Error error
}

// IsEmpty reports whether the diff has no changes and no content.
func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// Diff computes the diff between the worktree's current state and its base commit.
// It returns a populated DiffStats in all cases; callers should check DiffStats.Error.
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	if _, err := os.Stat(g.worktreePath); err != nil {
		stats.Error = fmt.Errorf("worktree path unavailable: %w", err)
		return stats
	}

	base := g.GetBaseCommitSHA()
	if base == "" {
		stats.Error = fmt.Errorf("base commit SHA not available")
		return stats
	}

	output, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", base)
	if err != nil {
		stats.Error = err
		return stats
	}

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			stats.Added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			stats.Removed++
		}
	}
	stats.Content = output

	return stats
}
