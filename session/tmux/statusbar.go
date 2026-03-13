package tmux

import (
	"strings"
)

// NOTE: StatusBarData and TaskGlyph are intentionally mirrored from
// ui.StatusBarData and ui.TaskGlyph. A direct import of
// github.com/kastheco/kasmos/ui from this package creates an import cycle:
//
//	session/tmux → ui → session → session/headless → session/tmux
//
// Until the shared types are extracted to a cycle-free package, callers that
// hold a ui.StatusBarData value should construct a tmux.StatusBarData manually
// or via a thin adapter in app/ where both packages are importable.
//
// Keep field names, types, and TaskGlyph constant values in sync with
// ui/statusbar.go any time those types change.

// TaskGlyph mirrors ui.TaskGlyph — the completion state of a single wave task.
type TaskGlyph int

const (
	TaskGlyphComplete TaskGlyph = iota // task finished successfully
	TaskGlyphRunning                   // task currently executing
	TaskGlyphFailed                    // task ended with error
	TaskGlyphPending                   // task not yet started
)

// StatusBarData mirrors ui.StatusBarData — contextual information for the
// tmux status bar. Field semantics are identical to the ui counterpart.
type StatusBarData struct {
	Branch           string
	Version          string
	PlanName         string      // empty = no plan context
	PlanStatus       string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel        string      // "wave 2/4" or empty
	TaskGlyphs       []TaskGlyph // per-task status for wave progress
	FocusMode        bool
	TmuxSessionCount int
	ProjectDir       string // project directory name
	PRState          string // approved, changes_requested, pending (empty = no PR)
	PRChecks         string // passing, failing, pending (empty = unknown)
}

// Tmux 256-colour constants for the status bar — centralised so tests can assert
// exact output strings without chasing scattered literals.
const (
	// tmuxColorFoam is a cyan-teal colour used for running/complete/info states.
	tmuxColorFoam = "colour45"
	// tmuxColorIris is a light purple used for running task glyphs.
	tmuxColorIris = "colour183"
	// tmuxColorLove is a red-pink used for failures and errors.
	tmuxColorLove = "colour204"
	// tmuxColorRose is a coral-pink used for reviewing and changes-requested states.
	tmuxColorRose = "colour210"
	// tmuxColorMuted is a medium grey used for muted text (version, pending, project dir).
	tmuxColorMuted = "colour102"
	// tmuxColorSubtle is a light periwinkle used for wave labels.
	tmuxColorSubtle = "colour146"
)

// StatusBarRender holds the two string segments that feed tmux status-left and
// status-right (or a combined status-format string).
//
// Each segment is a valid tmux format string that can be passed directly to
// `tmux set-option status-left` / `tmux set-option status-right`.
// All colour segments are terminated with #[default] so styles do not leak.
type StatusBarRender struct {
	Left  string
	Right string
}

// RenderStatusBar translates StatusBarData into tmux format-string segments.
//
// Left segment shape:
//
//	kasmos [version] · [wave-glyphs wave-label | plan-status]
//
// Right segment shape:
//
//	[branch] · [pr-indicator] · [project-dir]
//
// Fields collapse cleanly: if a field is empty its separator is omitted so
// two consecutive " · " are never emitted. Semantics and precedence rules mirror
// ui/statusbar.go exactly — wave glyphs + label override plain plan status; PR
// display prefers failing checks over changes_requested over approved over pending.
func RenderStatusBar(data StatusBarData) StatusBarRender {
	return StatusBarRender{
		Left:  tmuxLeftSegment(data),
		Right: tmuxRightSegment(data),
	}
}

// tmuxLeftSegment builds the status-left string.
//
// The app name is always bold. Version (if set) is appended with a plain space
// matching the TUI layout. The status group (wave or plan) is preceded by " · ".
func tmuxLeftSegment(data StatusBarData) string {
	left := "#[bold]kasmos#[default]"

	if data.Version != "" {
		left += " " + tmuxFG(tmuxColorMuted, data.Version)
	}

	if sg := tmuxLeftStatusGroup(data); sg != "" {
		left += " · " + sg
	}

	return left
}

// tmuxRightSegment builds the status-right string.
// Empty fields are omitted; non-empty fields are joined with " · ".
func tmuxRightSegment(data StatusBarData) string {
	var parts []string

	if data.Branch != "" {
		parts = append(parts, data.Branch)
	}

	if pr := tmuxPRGroup(data); pr != "" {
		parts = append(parts, pr)
	}

	if data.ProjectDir != "" {
		parts = append(parts, tmuxFG(tmuxColorMuted, data.ProjectDir))
	}

	return strings.Join(parts, " · ")
}

// tmuxLeftStatusGroup returns the status portion of the left segment.
// Priority: wave glyphs + label override plan status string, mirroring
// leftStatusGroup in ui/statusbar.go.
func tmuxLeftStatusGroup(data StatusBarData) string {
	if data.WaveLabel != "" && len(data.TaskGlyphs) > 0 {
		glyphs := make([]string, 0, len(data.TaskGlyphs))
		for _, g := range data.TaskGlyphs {
			if s := tmuxTaskGlyph(g); s != "" {
				glyphs = append(glyphs, s)
			}
		}
		return strings.Join(glyphs, " ") + " " + tmuxFG(tmuxColorSubtle, data.WaveLabel)
	}
	if data.PlanStatus != "" {
		return tmuxStatusColor(data.PlanStatus)
	}
	return ""
}

// tmuxStatusColor returns a plan status string wrapped in the appropriate tmux
// colour markup. Semantic mapping mirrors planStatusStyle in ui/statusbar.go.
func tmuxStatusColor(status string) string {
	var color string
	switch status {
	case "implementing", "planning":
		color = tmuxColorFoam
	case "reviewing", "done":
		color = tmuxColorRose
	default:
		color = tmuxColorMuted
	}
	return tmuxFG(color, status)
}

// tmuxTaskGlyph renders a TaskGlyph symbol with its associated tmux colour markup.
// Uses the same glyph characters as taskGlyphStr in ui/statusbar.go.
func tmuxTaskGlyph(g TaskGlyph) string {
	switch g {
	case TaskGlyphComplete:
		return tmuxFG(tmuxColorFoam, "✓")
	case TaskGlyphRunning:
		return tmuxFG(tmuxColorIris, "●")
	case TaskGlyphFailed:
		return tmuxFG(tmuxColorLove, "✕")
	case TaskGlyphPending:
		return tmuxFG(tmuxColorMuted, "○")
	default:
		return ""
	}
}

// tmuxPRGroup builds the PR review/check indicator for the right segment.
// Returns "" when PRState is empty.
// Priority: failing checks > changes_requested > approved > pending.
// Mirrors rightPRGroup in ui/statusbar.go.
func tmuxPRGroup(data StatusBarData) string {
	if data.PRState == "" {
		return ""
	}
	// Failing checks is the strongest signal regardless of review decision.
	if data.PRChecks == "failing" {
		return tmuxFG(tmuxColorLove, "✕ pr")
	}
	switch data.PRState {
	case "approved":
		return tmuxFG(tmuxColorFoam, "✓ pr")
	case "changes_requested":
		return tmuxFG(tmuxColorRose, "● pr")
	default:
		return tmuxFG(tmuxColorMuted, "○ pr")
	}
}

// tmuxFG wraps text with a tmux #[fg=color] segment and always appends
// #[default] so that styles do not leak into adjacent content.
func tmuxFG(color, text string) string {
	return "#[fg=" + color + "]" + text + "#[default]"
}
