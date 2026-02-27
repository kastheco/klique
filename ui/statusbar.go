package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TaskGlyph represents the status of a single task in wave progress.
type TaskGlyph int

const (
	TaskGlyphComplete TaskGlyph = iota
	TaskGlyphRunning
	TaskGlyphFailed
	TaskGlyphPending
)

// StatusBarData holds the contextual information displayed in the status bar.
type StatusBarData struct {
	RepoName         string
	Branch           string
	PlanName         string      // empty = no plan context
	PlanStatus       string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel        string      // "wave 2/4" or empty
	TaskGlyphs       []TaskGlyph // per-task status for wave progress
	FocusMode        bool        // true when in interactive/focus mode
	TmuxSessionCount int         // total kas_ tmux sessions (0 = hide)
}

// StatusBar is the top status bar component.
type StatusBar struct {
	width int
	data  StatusBarData
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetSize sets the terminal width for the status bar.
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetData updates the status bar content.
func (s *StatusBar) SetData(data StatusBarData) {
	s.data = data
}

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

var statusBarPlanNameStyle = lipgloss.NewStyle().
	Foreground(ColorText).
	Background(ColorSurface)

var statusBarWaveLabelStyle = lipgloss.NewStyle().
	Foreground(ColorSubtle).
	Background(ColorSurface)

var statusBarTmuxCountStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Background(ColorSurface)

func planStatusStyle(status string) string {
	var fg lipgloss.TerminalColor
	switch status {
	case "implementing":
		fg = ColorFoam
	case "reviewing":
		fg = ColorRose
	case "done":
		fg = ColorFoam
	default:
		fg = ColorMuted
	}

	return lipgloss.NewStyle().Foreground(fg).Background(ColorSurface).Render(status)
}

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

func (s *StatusBar) centerBranchGroup() string {
	if s.data.Branch == "" {
		return ""
	}

	return statusBarBranchStyle.Render("\ue725 " + s.data.Branch)
}

func (s *StatusBar) leftStatusGroup() string {
	var parts []string

	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		glyphParts := make([]string, 0, len(s.data.TaskGlyphs))
		for _, g := range s.data.TaskGlyphs {
			glyphParts = append(glyphParts, taskGlyphStr(g))
		}
		glyphs := strings.Join(glyphParts, " ")
		parts = append(parts, glyphs+" "+statusBarWaveLabelStyle.Render(s.data.WaveLabel))
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, statusBarSepStyle.Render(" · "))
}

func (s *StatusBar) String() string {
	if s.width < 10 {
		return ""
	}

	contentWidth := s.width - 2 // statusBarStyle has horizontal padding of 1 on each side
	if contentWidth < 1 {
		contentWidth = s.width
	}

	left := statusBarAppNameStyle.Render(GradientText("kasmos", GradientStart, GradientEnd))
	if leftStatus := s.leftStatusGroup(); leftStatus != "" {
		left = strings.Join([]string{left, statusBarSepStyle.Render(" · "), leftStatus}, "")
	}
	right := ""
	if s.data.RepoName != "" {
		right = statusBarPlanNameStyle.Render(s.data.RepoName)
	}
	center := s.centerBranchGroup()

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	centerWidth := lipgloss.Width(center)

	rightStart := contentWidth - rightWidth
	if rightStart < leftWidth+1 {
		rightStart = leftWidth + 1
	}

	centerStart := (contentWidth - centerWidth) / 2
	if centerStart < leftWidth+1 {
		centerStart = leftWidth + 1
	}

	// Keep branch/status grouped. If it cannot fit between left+right, drop the
	// whole center group instead of splitting branch and status apart.
	if center != "" && centerStart+centerWidth > rightStart-1 {
		center = ""
		centerWidth = 0
	}

	var b strings.Builder
	cursor := 0
	writeAt := func(start, visualWidth int, text string) {
		if text == "" {
			return
		}
		if start < cursor {
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

	if cursor < contentWidth {
		b.WriteString(strings.Repeat(" ", contentWidth-cursor))
	}

	return statusBarStyle.Width(s.width).Render(b.String())
}
