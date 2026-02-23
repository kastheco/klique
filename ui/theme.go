package ui

import "github.com/charmbracelet/lipgloss"

// Rosé Pine Moon palette
// https://rosepinetheme.com/palette/
var (
	// Base tones
	ColorBase    = lipgloss.Color("#232136")
	ColorSurface = lipgloss.Color("#2a273f")
	ColorOverlay = lipgloss.Color("#393552")
	ColorMuted   = lipgloss.Color("#6e6a86")
	ColorSubtle  = lipgloss.Color("#908caa")
	ColorText    = lipgloss.Color("#e0def4")

	// Semantic colors
	ColorLove = lipgloss.Color("#eb6f92") // error, danger
	ColorGold = lipgloss.Color("#f6c177") // warning
	ColorRose = lipgloss.Color("#ea9a97") // accent, secondary
	ColorPine = lipgloss.Color("#3e8fb0") // link
	ColorFoam = lipgloss.Color("#9ccfd8") // info, running
	ColorIris = lipgloss.Color("#c4a7e7") // highlight, primary

	// Gradient endpoints for the banner and focused tab label
	GradientStart = "#9ccfd8" // foam
	GradientEnd   = "#c4a7e7" // iris

	// Diff-specific (keep readable semantic greens/reds)
	ColorDiffAdd    = lipgloss.Color("#9ccfd8") // foam for additions
	ColorDiffDelete = lipgloss.Color("#eb6f92") // love for deletions
	ColorDiffHunk   = lipgloss.Color("#c4a7e7") // iris for hunk headers
)
