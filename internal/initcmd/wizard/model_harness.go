package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

type stepCancelMsg struct{}

type harnessStep struct {
	items    []harness.DetectResult
	selected map[string]bool
	cursor   int
}

func newHarnessStep(items []harness.DetectResult) *harnessStep {
	selected := make(map[string]bool, len(items))
	for _, item := range items {
		selected[item.Name] = item.Found
	}

	cloned := make([]harness.DetectResult, len(items))
	copy(cloned, items)

	return &harnessStep{
		items:    cloned,
		selected: selected,
	}
}

func (h *harnessStep) Init() tea.Cmd {
	return nil
}

func (h *harnessStep) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return h, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		h.cursorDown()
	case "k", "up":
		h.cursorUp()
	case " ", "space":
		h.toggle()
	case "enter":
		if h.canProceed() {
			return h, func() tea.Msg { return stepDoneMsg{} }
		}
	case "q", "ctrl+c":
		return h, func() tea.Msg { return stepCancelMsg{} }
	}

	return h, nil
}

func (h *harnessStep) View(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	lines := []string{
		titleStyle.Render(gradientText("klique init wizard", gradientStart, gradientEnd)),
		renderHarnessStepDots(),
		"",
		titleStyle.Render("select agent harnesses"),
		"",
	}

	for i, item := range h.items {
		cursor := " "
		if i == h.cursor {
			cursor = "›"
		}

		stateGlyph := "○"
		lineStyle := harnessNormalStyle
		if h.selected[item.Name] {
			stateGlyph = "◉"
			lineStyle = harnessSelectedStyle
		}

		location := "not found"
		if item.Path != "" {
			location = item.Path
		}

		row := fmt.Sprintf("%s %s %s %s", cursor, stateGlyph, item.Name, location)
		lines = append(lines, lineStyle.Render(row))

		desc := strings.TrimSpace(HarnessDescription(item.Name))
		if desc != "" {
			lines = append(lines, harnessDescStyle.Render("  "+desc))
		}
	}

	lines = append(lines, "")
	lines = append(lines, hintDescStyle.Render("space toggle · enter continue · q quit"))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, body)
}

func (h *harnessStep) Apply(state *State) {
	state.SelectedHarness = h.selectedNames()
	state.DetectResults = append([]harness.DetectResult(nil), h.items...)
}

func (h *harnessStep) cursorDown() {
	if h.cursor < len(h.items)-1 {
		h.cursor++
	}
}

func (h *harnessStep) cursorUp() {
	if h.cursor > 0 {
		h.cursor--
	}
}

func (h *harnessStep) toggle() {
	if h.cursor < 0 || h.cursor >= len(h.items) {
		return
	}
	name := h.items[h.cursor].Name
	h.selected[name] = !h.selected[name]
}

func (h *harnessStep) canProceed() bool {
	for _, selected := range h.selected {
		if selected {
			return true
		}
	}
	return false
}

func (h *harnessStep) selectedNames() []string {
	names := make([]string, 0, len(h.items))
	for _, item := range h.items {
		if h.selected[item.Name] {
			names = append(names, item.Name)
		}
	}
	return names
}

func renderHarnessStepDots() string {
	return strings.Join([]string{
		stepActiveStyle.Render("●"),
		stepPendingStyle.Render("──"),
		stepPendingStyle.Render("○"),
		stepPendingStyle.Render("──"),
		stepPendingStyle.Render("○"),
	}, " ")
}
