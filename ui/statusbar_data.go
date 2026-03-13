package ui

import "charm.land/lipgloss/v2"

// TaskGlyph represents the completion state of a single task in wave progress.
type TaskGlyph int

const (
	TaskGlyphComplete TaskGlyph = iota // task finished successfully
	TaskGlyphRunning                   // task currently executing
	TaskGlyphFailed                    // task ended with error
	TaskGlyphPending                   // task not yet started
)

// StatusBarData holds the contextual information fed to the tmux status-format
// helper and previously rendered by the (now deleted) TUI StatusBar component.
type StatusBarData struct {
	Branch           string
	Version          string
	PlanName         string      // empty = no plan context
	PlanStatus       string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel        string      // "wave 2/4" or empty
	TaskGlyphs       []TaskGlyph // per-task status for wave progress
	TmuxSessionCount int         // total kas_ tmux sessions (0 = hide)
	ProjectDir       string      // project directory name, shown right-aligned
	PRState          string      // approved, changes_requested, pending (empty = no PR)
	PRChecks         string      // passing, failing, pending (empty = unknown)
}

// statusBarTmuxCountStyle is the style for the tmux session count badge rendered
// in the bottom Menu bar. Originally defined in statusbar.go; kept here because
// menu.go references it and it belongs to the same ui package.
var statusBarTmuxCountStyle = lipgloss.NewStyle().Foreground(ColorMuted)
