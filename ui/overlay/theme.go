package overlay

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Rosé Pine Moon palette — mirrors ui/theme.go.
// https://rosepinetheme.com/palette/
var (
	// Base tones
	colorBase    = lipgloss.Color("#232136")
	colorOverlay = lipgloss.Color("#393552")
	colorMuted   = lipgloss.Color("#6e6a86")
	colorSubtle  = lipgloss.Color("#908caa")
	colorText    = lipgloss.Color("#e0def4")

	// Semantic colors
	colorLove = lipgloss.Color("#eb6f92") // error, danger
	colorGold = lipgloss.Color("#f6c177") // warning
	colorFoam = lipgloss.Color("#9ccfd8") // info, running
	colorIris = lipgloss.Color("#c4a7e7") // highlight, primary
)

// ThemeRosePine returns a huh theme matching the app's Rose Pine Moon palette.
func ThemeRosePine() *huh.Theme {
	t := huh.ThemeBase()

	t.Focused.Base = t.Focused.Base.BorderForeground(colorIris)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(colorIris).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(colorIris).Bold(true).MarginBottom(1)
	t.Focused.Description = t.Focused.Description.Foreground(colorMuted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(colorLove)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(colorLove)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorIris)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(colorIris)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(colorIris)
	t.Focused.Option = t.Focused.Option.Foreground(colorText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(colorIris)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorFoam)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(colorFoam).SetString("✓ ")
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(colorMuted).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(colorText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(colorBase).Background(colorIris)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(colorSubtle).Background(colorOverlay)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorFoam)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(colorMuted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorIris)
	t.Focused.TextInput.Text = t.Focused.TextInput.Text.Foreground(colorText)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	return t
}
