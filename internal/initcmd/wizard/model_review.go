package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const reviewConfigPath = "~/.config/kasmos/config.toml"

type reviewStep struct {
	agents    []AgentState
	harnesses []string
}

func newReviewStep(agents []AgentState, harnesses []string) *reviewStep {
	return &reviewStep{
		agents:    append([]AgentState(nil), agents...),
		harnesses: append([]string(nil), harnesses...),
	}
}

func (r *reviewStep) Init() tea.Cmd {
	return nil
}

func (r *reviewStep) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return r, nil
	}

	switch keyMsg.String() {
	case "enter":
		return r, func() tea.Msg { return stepDoneMsg{} }
	case "esc":
		return r, func() tea.Msg { return stepBackMsg{} }
	case "q", "ctrl+c":
		return r, func() tea.Msg { return stepCancelMsg{} }
	}

	return r, nil
}

func (r *reviewStep) View(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	rows := make([]string, 0, len(r.agents)+12)
	rows = append(rows,
		titleStyle.Render("review configuration"),
		separatorStyle.Render(strings.Repeat("─", max(width-2, 1))),
		lipgloss.NewStyle().Foreground(colorFoam).Render("harnesses: "+strings.Join(r.harnesses, " ")),
		"",
	)

	lines := make([]string, 0, len(r.agents))
	for _, a := range r.agents {
		lines = append(lines, formatReviewLine(a))
	}
	rows = append(rows, cardStyle.Render(strings.Join(lines, "\n")), "")

	rows = append(rows,
		fmt.Sprintf("%s %s", labelStyle.Render("config:"), pathStyle.Render(reviewConfigPath)),
		fmt.Sprintf("%s %s", labelStyle.Render("scaffold:"), pathStyle.Render(strings.Join(reviewScaffoldPaths(r.harnesses), " "))),
		"",
		hintDescStyle.Render("enter apply · esc go back · q quit"),
	)

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, body)
}

func (r *reviewStep) Apply(_ *State) {}

func formatReviewLine(a AgentState) string {
	if !a.Enabled {
		return fmt.Sprintf("%s %-8s (disabled)", dotDisabledStyle.Render("○"), a.Role)
	}

	effort := a.Effort
	if effort == "" {
		effort = "default"
	}
	temp := a.Temperature
	if temp == "" {
		temp = "default"
	}

	return fmt.Sprintf("%s %-8s %s / %s / %s / temp %s", dotEnabledStyle.Render("●"), a.Role, a.Harness, a.Model, effort, temp)
}

func reviewScaffoldPaths(harnesses []string) []string {
	if len(harnesses) == 0 {
		return []string{"./<harness>/agents/*.md"}
	}

	paths := make([]string, 0, len(harnesses))
	for _, h := range harnesses {
		paths = append(paths, fmt.Sprintf("./.%s/agents/*.md", h))
	}
	return paths
}
