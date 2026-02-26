package wizard

import "github.com/charmbracelet/lipgloss"

// Rose Pine Moon - self-contained copy (no ui/ import).
var (
	colorBase    = lipgloss.Color("#232136")
	colorSurface = lipgloss.Color("#2a273f")
	colorOverlay = lipgloss.Color("#393552")
	colorMuted   = lipgloss.Color("#6e6a86")
	colorSubtle  = lipgloss.Color("#908caa")
	colorText    = lipgloss.Color("#e0def4")

	colorLove = lipgloss.Color("#eb6f92")
	colorGold = lipgloss.Color("#f6c177")
	colorRose = lipgloss.Color("#ea9a97")
	colorPine = lipgloss.Color("#3e8fb0")
	colorFoam = lipgloss.Color("#9ccfd8")
	colorIris = lipgloss.Color("#c4a7e7")

	gradientStart = "#9ccfd8"
	gradientEnd   = "#c4a7e7"
)

// Layout styles.
var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	subtitleStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	separatorStyle = lipgloss.NewStyle().Foreground(colorOverlay)
	hintKeyStyle   = lipgloss.NewStyle().Foreground(colorSubtle)
	hintDescStyle  = lipgloss.NewStyle().Foreground(colorMuted)

	// Harness list.
	harnessSelectedStyle = lipgloss.NewStyle().Foreground(colorIris)
	harnessNormalStyle   = lipgloss.NewStyle().Foreground(colorText)
	harnessDimStyle      = lipgloss.NewStyle().Foreground(colorSubtle)
	harnessDescStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	pathStyle            = lipgloss.NewStyle().Foreground(colorSubtle)

	// Agent list (left panel).
	roleActiveStyle  = lipgloss.NewStyle().Foreground(colorIris)
	roleNormalStyle  = lipgloss.NewStyle().Foreground(colorText)
	roleMutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	dotEnabledStyle  = lipgloss.NewStyle().Foreground(colorFoam)
	dotDisabledStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// Detail panel (right).
	labelStyle = lipgloss.NewStyle().Foreground(colorSubtle)
	valueStyle = lipgloss.NewStyle().Foreground(colorText)

	// Review card.
	cardStyle = lipgloss.NewStyle().
			Background(colorSurface).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOverlay)

	// Inline field styles.
	fieldActiveStyle = lipgloss.NewStyle().Foreground(colorIris)
	fieldNormalStyle = lipgloss.NewStyle().Foreground(colorText)
	defaultTagStyle  = lipgloss.NewStyle().Foreground(colorGold)

	// Step indicator.
	stepDoneStyle    = lipgloss.NewStyle().Foreground(colorFoam)
	stepActiveStyle  = lipgloss.NewStyle().Foreground(colorIris)
	stepPendingStyle = lipgloss.NewStyle().Foreground(colorOverlay)
)
