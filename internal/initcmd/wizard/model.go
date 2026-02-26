package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

type stepDoneMsg struct{}

type stepBackMsg struct{}

type stepModel interface {
	Init() tea.Cmd
	Update(tea.Msg) (stepModel, tea.Cmd)
	View(width, height int) string
	Apply(state *State)
}

type rootModel struct {
	registry *harness.Registry
	existing *config.TOMLConfigResult

	state      *State
	steps      []stepModel
	totalSteps int
	step       int

	width     int
	height    int
	cancelled bool
}

type stepFactory func(state *State, registry *harness.Registry, existing *config.TOMLConfigResult) stepModel

var rootStepFactories = []stepFactory{
	newPlaceholderHarnessStep,
	newPlaceholderAgentsStep,
	newPlaceholderReviewStep,
}

func newRootModel(registry *harness.Registry, existing *config.TOMLConfigResult) rootModel {
	state := &State{Registry: registry}
	if registry != nil {
		state.DetectResults = registry.DetectAll()
	}

	return rootModel{
		registry: registry,
		existing: existing,
		state:    state,
		step:     0,
	}
}

func (m rootModel) Init() tea.Cmd {
	m.steps = nil
	for _, factory := range rootStepFactories {
		m.steps = append(m.steps, factory(m.state, m.registry, m.existing))
	}
	m.totalSteps = len(m.steps)
	if m.totalSteps == 0 {
		return tea.Quit
	}
	return m.steps[m.step].Init()
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case stepDoneMsg:
		if m.step >= 0 && m.step < len(m.steps) {
			m.steps[m.step].Apply(m.state)
		}
		if m.step == 0 && len(m.state.Agents) == 0 {
			m.state.Agents = initAgentsFromExisting(m.state.SelectedHarness, m.existing)
		}
		if m.step >= m.totalSteps-1 {
			return m, tea.Quit
		}
		m.nextStep()
		return m, m.steps[m.step].Init()
	case stepBackMsg:
		m.prevStep()
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			return m, tea.Quit
		}
	}

	if m.step < 0 || m.step >= len(m.steps) {
		return m, nil
	}

	nextStep, cmd := m.steps[m.step].Update(msg)
	m.steps[m.step] = nextStep
	return m, cmd
}

func (m rootModel) View() string {
	if m.step < 0 || m.step >= len(m.steps) {
		return ""
	}

	header := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(gradientText("klique init wizard", gradientStart, gradientEnd)),
		subtitleStyle.Render("guided setup for harnesses and agents"),
		renderStepIndicator(m.step, m.totalSteps),
	)

	contentHeight := m.height - lipgloss.Height(header) - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	content := m.steps[m.step].View(m.width, contentHeight)

	return lipgloss.JoinVertical(lipgloss.Left, header, "", content)
}

func (m *rootModel) nextStep() {
	if m.totalSteps <= 0 {
		m.step = 0
		return
	}
	maxStep := m.totalSteps - 1
	if m.step < maxStep {
		m.step++
	}
}

func (m *rootModel) prevStep() {
	if m.step > 0 {
		m.step--
	}
}

func renderStepIndicator(current, total int) string {
	if total <= 0 {
		return ""
	}

	labels := []string{"harness", "agents", "done"}
	dots := make([]string, 0, total*2-1)
	for i := 0; i < total; i++ {
		switch {
		case i < current:
			dots = append(dots, stepDoneStyle.Render("●"))
		case i == current:
			dots = append(dots, stepActiveStyle.Render("●"))
		default:
			dots = append(dots, stepPendingStyle.Render("○"))
		}
		if i < total-1 {
			dots = append(dots, stepPendingStyle.Render("──"))
		}
	}

	var labelParts []string
	for i := 0; i < total; i++ {
		label := fmt.Sprintf("step %d", i+1)
		if i < len(labels) {
			label = labels[i]
		}
		if i == current {
			labelParts = append(labelParts, stepActiveStyle.Render(label))
		} else {
			labelParts = append(labelParts, stepPendingStyle.Render(label))
		}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		strings.Join(dots, " "),
		strings.Join(labelParts, "   "),
	)
}

func parseHex(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func gradientText(text, startHex, endHex string) string {
	if text == "" {
		return ""
	}
	r1, g1, b1 := parseHex(startHex)
	r2, g2, b2 := parseHex(endHex)
	runes := []rune(text)
	var sb strings.Builder
	for i, r := range runes {
		t := 0.0
		if len(runes) > 1 {
			t = float64(i) / float64(len(runes)-1)
		}
		cr, cg, cb := lerpByte(r1, r2, t), lerpByte(g1, g2, t), lerpByte(b1, b2, t)
		sb.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm%c", cr, cg, cb, r))
	}
	sb.WriteString("\033[0m")
	return sb.String()
}

type placeholderStepModel struct {
	title string
}

func newPlaceholderHarnessStep(_ *State, _ *harness.Registry, _ *config.TOMLConfigResult) stepModel {
	return placeholderStepModel{title: "harness"}
}

func newPlaceholderAgentsStep(_ *State, _ *harness.Registry, _ *config.TOMLConfigResult) stepModel {
	return placeholderStepModel{title: "agents"}
}

func newPlaceholderReviewStep(_ *State, _ *harness.Registry, _ *config.TOMLConfigResult) stepModel {
	return placeholderStepModel{title: "done"}
}

func (m placeholderStepModel) Init() tea.Cmd {
	return nil
}

func (m placeholderStepModel) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			return m, func() tea.Msg { return stepDoneMsg{} }
		case "esc":
			return m, func() tea.Msg { return stepBackMsg{} }
		}
	}
	return m, nil
}

func (m placeholderStepModel) View(width, _ int) string {
	body := subtitleStyle.Render(fmt.Sprintf("%s step placeholder", m.title))
	hint := hintDescStyle.Render("enter: next  esc: back  ctrl+c: cancel")
	return lipgloss.NewStyle().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, body, "", hint))
}

func (m placeholderStepModel) Apply(_ *State) {}
