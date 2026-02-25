package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// FormOverlay is a multi-field form overlay backed by huh.Form.
type FormOverlay struct {
	form      *huh.Form
	nameVal   string
	descVal   string
	title     string
	submitted bool
	canceled  bool
	width     int
}

// NewFormOverlay creates a form overlay with name and description inputs.
func NewFormOverlay(title string, width int) *FormOverlay {
	f := &FormOverlay{
		title: title,
		width: width,
	}

	formWidth := width - 6
	if formWidth < 34 {
		formWidth = 34
	}

	f.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("name").
				Value(&f.nameVal),
			huh.NewInput().
				Key("desc").
				Title("description (optional)").
				Value(&f.descVal),
		),
	).
		WithTheme(ThemeRosePine()).
		WithWidth(formWidth).
		WithShowHelp(false).
		WithShowErrors(false)

	_ = f.form.Init()

	return f
}

func (f *FormOverlay) updateForm(msg tea.Msg) {
	updated, _ := f.form.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		f.form = form
	}
}

// HandleKeyPress processes a key and returns true when the overlay should close.
func (f *FormOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		f.canceled = true
		return true

	case tea.KeyEnter:
		if strings.TrimSpace(f.nameVal) == "" {
			return false
		}
		f.submitted = true
		return true

	case tea.KeyTab, tea.KeyDown:
		f.updateForm(huh.NextField())
		return false

	case tea.KeyShiftTab, tea.KeyUp:
		f.updateForm(huh.PrevField())
		return false

	default:
		f.updateForm(msg)
		return false
	}
}

// Render returns the styled overlay string.
func (f *FormOverlay) Render() string {
	w := f.width
	if w < 40 {
		w = 40
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(colorIris).
		Bold(true).
		MarginBottom(1)

	hintStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		MarginTop(1)

	content := titleStyle.Render(f.title) + "\n"
	content += f.form.View() + "\n"
	content += hintStyle.Render("tab/↑↓ navigate · enter create")

	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorIris).
		Padding(1, 2).
		Width(w)

	return style.Render(content)
}

// Name returns the name field value.
func (f *FormOverlay) Name() string {
	return strings.TrimSpace(f.nameVal)
}

// Description returns the description field value.
func (f *FormOverlay) Description() string {
	return strings.TrimSpace(f.descVal)
}

// IsSubmitted returns true when the form was submitted.
func (f *FormOverlay) IsSubmitted() bool {
	return f.submitted
}
