package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// TaskGlyph represents the completion state of a single task in wave progress.
type TaskGlyph int

const (
	TaskGlyphComplete TaskGlyph = iota // task finished successfully
	TaskGlyphRunning                   // task currently executing
	TaskGlyphFailed                    // task ended with error
	TaskGlyphPending                   // task not yet started
)

// StatusBarData holds the contextual information displayed in the status bar.
type StatusBarData struct {
	Branch           string
	PlanName         string      // empty = no plan context
	PlanStatus       string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel        string      // "wave 2/4" or empty
	TaskGlyphs       []TaskGlyph // per-task status for wave progress
	FocusMode        bool        // true when in interactive/focus mode
	TmuxSessionCount int         // total kas_ tmux sessions (0 = hide)
	ProjectDir       string      // project directory name, shown right-aligned
	PRState          string      // approved, changes_requested, pending (empty = no PR)
	PRChecks         string      // passing, failing, pending (empty = unknown)
}

// StatusBar renders the top status bar row of the TUI.
type StatusBar struct {
	width int
	data  StatusBarData
}

// NewStatusBar constructs a zero-value StatusBar ready for use.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetSize records the terminal width so String() can lay out content correctly.
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetData replaces the status bar's content data.
func (s *StatusBar) SetData(data StatusBarData) {
	s.data = data
}

// Package-level styles — defined once to avoid repeated allocations.
var statusBarStyle = lipgloss.NewStyle().
	Background(ColorSurface).
	Foreground(ColorText).
	Padding(0, 1)

var statusBarAppNameStyle = lipgloss.NewStyle().
	Background(ColorSurface).
	Bold(true)

var statusBarSepStyle = lipgloss.NewStyle().
	Foreground(ColorOverlay).
	Background(ColorSurface)

var statusBarBranchStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Background(ColorSurface)

var statusBarWaveLabelStyle = lipgloss.NewStyle().
	Foreground(ColorSubtle).
	Background(ColorSurface)

var statusBarTmuxCountStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Background(ColorSurface)

var statusBarProjectDirStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Background(ColorSurface)

// planStatusStyle returns a styled version of status using semantic colors.
func planStatusStyle(status string) string {
	var fg color.Color
	switch status {
	case "implementing", "planning":
		fg = ColorFoam
	case "reviewing", "done":
		fg = ColorRose
	default:
		fg = ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg).Background(ColorSurface).Render(status)
}

// taskGlyphStr renders a single TaskGlyph symbol with the appropriate color.
func taskGlyphStr(g TaskGlyph) string {
	switch g {
	case TaskGlyphComplete:
		return lipgloss.NewStyle().Foreground(ColorFoam).Background(ColorSurface).Render("✓")
	case TaskGlyphRunning:
		return lipgloss.NewStyle().Foreground(ColorIris).Background(ColorSurface).Render("●")
	case TaskGlyphFailed:
		return lipgloss.NewStyle().Foreground(ColorLove).Background(ColorSurface).Render("✕")
	case TaskGlyphPending:
		return lipgloss.NewStyle().Foreground(ColorMuted).Background(ColorSurface).Render("○")
	default:
		return ""
	}
}

// rightPRGroup builds a compact PR review/check indicator for the right side.
// Returns "" when no PR state is set. Priority: failing checks > changes_requested > approved > pending.
func (s *StatusBar) rightPRGroup() string {
	if s.data.PRState == "" {
		return ""
	}

	// Failing checks is the strongest signal regardless of review decision.
	if s.data.PRChecks == "failing" {
		return lipgloss.NewStyle().Foreground(ColorLove).Background(ColorSurface).Render("✕ pr")
	}
	switch s.data.PRState {
	case "approved":
		return lipgloss.NewStyle().Foreground(ColorFoam).Background(ColorSurface).Render("✓ pr")
	case "changes_requested":
		return lipgloss.NewStyle().Foreground(ColorRose).Background(ColorSurface).Render("● pr")
	default:
		return lipgloss.NewStyle().Foreground(ColorMuted).Background(ColorSurface).Render("○ pr")
	}
}

// centerBranchGroup builds the centered git branch indicator.
// Returns an empty string when no branch is set.
func (s *StatusBar) centerBranchGroup() string {
	if s.data.Branch == "" {
		return ""
	}
	return statusBarBranchStyle.Render("\ue725 " + s.data.Branch)
}

// leftStatusGroup assembles the status segment placed immediately after the logo.
// Priority: wave-progress glyphs + label > plan status string.
func (s *StatusBar) leftStatusGroup() string {
	var parts []string

	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		rendered := make([]string, 0, len(s.data.TaskGlyphs))
		for _, g := range s.data.TaskGlyphs {
			rendered = append(rendered, taskGlyphStr(g))
		}
		glyphs := strings.Join(rendered, " ")
		parts = append(parts, glyphs+" "+statusBarWaveLabelStyle.Render(s.data.WaveLabel))
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, statusBarSepStyle.Render(" · "))
}

// String renders the status bar as a single styled line exactly s.width cells wide.
// Returns an empty string when the width is too narrow to be useful.
func (s *StatusBar) String() string {
	if s.width < 10 {
		return ""
	}

	// The statusBarStyle has horizontal padding of 1 on each side.
	contentWidth := s.width - 2
	if contentWidth < 1 {
		contentWidth = s.width
	}

	// Build left section: logo + optional status group.
	left := statusBarAppNameStyle.Render(GradientText("kasmos", GradientStart, GradientEnd))
	if ls := s.leftStatusGroup(); ls != "" {
		left = left + statusBarSepStyle.Render(" · ") + ls
	}

	// Build center section: branch indicator.
	center := s.centerBranchGroup()

	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)

	// Ideal center start keeps the group visually centered.
	centerStart := (contentWidth - centerWidth) / 2
	if centerStart < leftWidth+1 {
		centerStart = leftWidth + 1
	}

	// Drop center entirely if it cannot fit without running off the right edge.
	if center != "" && centerStart+centerWidth > contentWidth {
		center = ""
		centerWidth = 0
	}

	// Build right section: [prGroup · projectDir] or just one of them.
	prGroup := s.rightPRGroup()
	right := ""
	rightWidth := 0

	if prGroup != "" && s.data.ProjectDir != "" {
		// Compose both together.
		composed := prGroup + statusBarSepStyle.Render(" · ") + statusBarProjectDirStyle.Render(s.data.ProjectDir)
		composedWidth := lipgloss.Width(composed)
		rightStart := contentWidth - composedWidth
		if rightStart >= centerStart+centerWidth+1 {
			right = composed
			rightWidth = composedWidth
		} else {
			// Can't fit both — try just prGroup.
			right = prGroup
			rightWidth = lipgloss.Width(prGroup)
			if contentWidth-rightWidth < centerStart+centerWidth+1 {
				right = ""
				rightWidth = 0
			}
		}
	} else if prGroup != "" {
		right = prGroup
		rightWidth = lipgloss.Width(prGroup)
	} else if s.data.ProjectDir != "" {
		right = statusBarProjectDirStyle.Render(s.data.ProjectDir)
		rightWidth = lipgloss.Width(right)
	}

	rightStart := contentWidth - rightWidth

	// Drop right section if it would collide with center.
	if right != "" && rightStart < centerStart+centerWidth+1 {
		right = ""
		rightWidth = 0
		rightStart = contentWidth
	}

	// Compose the content string using cursor-based positioning.
	var b strings.Builder
	cursor := 0

	writeAt := func(start, visualWidth int, text string) {
		if text == "" || start < cursor {
			return
		}
		if start > cursor {
			b.WriteString(strings.Repeat(" ", start-cursor))
		}
		b.WriteString(text)
		cursor = start + visualWidth
	}

	writeAt(0, leftWidth, left)
	writeAt(centerStart, centerWidth, center)
	writeAt(rightStart, rightWidth, right)

	// Pad remaining space so the background spans the full content width.
	if cursor < contentWidth {
		b.WriteString(strings.Repeat(" ", contentWidth-cursor))
	}

	return statusBarStyle.Width(s.width).Render(b.String())
}
