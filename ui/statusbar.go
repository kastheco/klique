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
	RepoName   string
	Branch     string
	PlanName   string      // empty = no plan context
	PlanStatus string      // "ready", "planning", "implementing", "reviewing", "done"
	WaveLabel  string      // "wave 2/4" or empty
	TaskGlyphs []TaskGlyph // per-task status for wave progress
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
	Foreground(ColorIris).
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

const statusBarSep = " │ "

func (s *StatusBar) String() string {
	if s.width < 10 {
		return ""
	}

	parts := make([]string, 0, 6)
	parts = append(parts, statusBarAppNameStyle.Render("kasmos"))

	if s.data.RepoName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.RepoName))
	}

	if s.data.Branch != "" {
		parts = append(parts, statusBarBranchStyle.Render("\ue725 "+s.data.Branch))
	}

	if s.data.PlanName != "" {
		parts = append(parts, statusBarPlanNameStyle.Render(s.data.PlanName))
	}

	if s.data.WaveLabel != "" && len(s.data.TaskGlyphs) > 0 {
		var glyphs strings.Builder
		for _, g := range s.data.TaskGlyphs {
			glyphs.WriteString(taskGlyphStr(g))
		}
		parts = append(parts, statusBarWaveLabelStyle.Render(s.data.WaveLabel)+" "+glyphs.String())
	} else if s.data.PlanStatus != "" {
		parts = append(parts, planStatusStyle(s.data.PlanStatus))
	}

	sep := statusBarSepStyle.Render(statusBarSep)
	content := strings.Join(parts, sep)

	return statusBarStyle.Width(s.width).Render(content)
}
