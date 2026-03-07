package daemon

import (
	"io"
	"log/slog"
)

// NewDaemonLogger creates a structured logger for the daemon.
//
// When humanReadable is false (the default for production/systemd), JSON output
// is produced — suitable for consumption by journalctl with -o json.
// When humanReadable is true (foreground debugging), a human-friendly text
// format is used instead.
func NewDaemonLogger(w io.Writer, humanReadable bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	var handler slog.Handler
	if humanReadable {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}

// WithRepo returns a child logger with the repo path pre-attached as a field.
// Use this when entering the per-repo processing loop so every log line is
// automatically tagged with the repository it belongs to.
func WithRepo(logger *slog.Logger, repoPath string) *slog.Logger {
	return logger.With("repo", repoPath)
}

// WithPlan returns a child logger with the plan file pre-attached as a field.
// Combine with WithRepo for fully-scoped log lines inside plan execution.
func WithPlan(logger *slog.Logger, planFile string) *slog.Logger {
	return logger.With("plan", planFile)
}
