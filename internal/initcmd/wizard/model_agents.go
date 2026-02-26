package wizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/config"
)

type agentMode int

const (
	agentBrowseMode agentMode = iota
	agentEditMode
)

type agentStepModel struct {
	agents     []AgentState
	cursor     int
	mode       agentMode
	harnesses  []string
	modelCache map[string][]string

	editField int

	allModels      []string
	filteredModels []string
	filterText     string
	modelCursor    int
	modelOffset    int
	filtering      bool

	tempInput    textinput.Model
	effortLevels map[string][]string
}

func initAgentsFromExisting(harnesses []string, existing *config.TOMLConfigResult) []AgentState {
	roles := DefaultAgentRoles()
	defaults := RoleDefaults()
	agents := make([]AgentState, 0, len(roles))

	defaultHarness := ""
	if len(harnesses) > 0 {
		defaultHarness = harnesses[0]
	}

	for _, role := range roles {
		as := defaults[role]
		if as.Role == "" {
			as = AgentState{Role: role, Enabled: true}
		}
		if as.Harness == "" {
			as.Harness = defaultHarness
		}

		if existing != nil {
			if profile, ok := existing.Profiles[role]; ok {
				as.Harness = profile.Program
				as.Model = profile.Model
				as.Effort = profile.Effort
				as.Enabled = profile.Enabled
				as.Temperature = ""
				if profile.Temperature != nil {
					as.Temperature = fmt.Sprintf("%g", *profile.Temperature)
				}
			}
		}

		agents = append(agents, as)
	}

	return agents
}

func newAgentStep(agents []AgentState, harnesses []string, modelCache map[string][]string) *agentStepModel {
	agentCopy := append([]AgentState(nil), agents...)
	harnessCopy := append([]string(nil), harnesses...)
	cacheCopy := map[string][]string{}
	for name, models := range modelCache {
		cacheCopy[name] = append([]string(nil), models...)
	}

	tempInput := textinput.New()
	tempInput.Prompt = ""
	tempInput.Placeholder = "default"
	tempInput.CharLimit = 16
	tempInput.Blur()

	m := &agentStepModel{
		agents:     agentCopy,
		cursor:     0,
		mode:       agentBrowseMode,
		harnesses:  harnessCopy,
		modelCache: cacheCopy,
		tempInput:  tempInput,
		effortLevels: map[string][]string{
			"claude": {"", "low", "medium", "high", "max"},
			"codex":  {"", "low", "medium", "high", "xhigh"},
		},
	}
	m.syncModelChoices()
	m.syncTemperatureInput()
	return m
}

func (m *agentStepModel) Init() tea.Cmd {
	return nil
}

func (m *agentStepModel) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode == agentEditMode {
		return m.updateEditMode(keyMsg)
	}

	switch keyMsg.String() {
	case "up", "k":
		m.cursorUp()
	case "down", "j":
		m.cursorDown()
	case " ":
		m.toggleEnabled()
	case "enter":
		m.enterEditMode()
	case "tab":
		return m, func() tea.Msg { return stepDoneMsg{} }
	case "esc":
		return m, func() tea.Msg { return stepBackMsg{} }
	case "q":
		return m, func() tea.Msg { return stepCancelMsg{} }
	}

	return m, nil
}

func (m *agentStepModel) View(width, height int) string {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	leftWidth := width / 3
	if leftWidth > 32 {
		leftWidth = 32
	}
	if leftWidth < 1 {
		leftWidth = 1
	}

	rightWidth := width - leftWidth - 1
	if rightWidth < 1 {
		rightWidth = 1
	}

	left := m.renderRolePanel(leftWidth, height)
	right := m.renderDetailPanel(rightWidth, height)
	separator := renderVerticalSeparator(height)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, separator, right)
}

func renderVerticalSeparator(height int) string {
	if height < 1 {
		height = 1
	}
	rows := make([]string, height)
	for i := range rows {
		rows[i] = separatorStyle.Render("┊")
	}
	return strings.Join(rows, "\n")
}

func (m *agentStepModel) Apply(state *State) {
	state.Agents = append([]AgentState(nil), m.agents...)
}

func (m *agentStepModel) cursorUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *agentStepModel) cursorDown() {
	if m.cursor < m.maxNavigableIndex() {
		m.cursor++
	}
}

func (m *agentStepModel) maxNavigableIndex() int {
	max := len(m.agents) - 1
	if max > 2 {
		max = 2
	}
	if max < 0 {
		return 0
	}
	return max
}

func (m *agentStepModel) toggleEnabled() {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return
	}
	m.agents[m.cursor].Enabled = !m.agents[m.cursor].Enabled
}

func (m *agentStepModel) renderRolePanel(width, height int) string {
	if width < 1 {
		width = 1
	}

	rows := []string{titleStyle.Render("roles"), ""}
	for i := 0; i <= m.maxNavigableIndex() && i < len(m.agents); i++ {
		agent := m.agents[i]

		dot := dotDisabledStyle.Render("○")
		if agent.Enabled {
			dot = dotEnabledStyle.Render("●")
		}

		prefix := " "
		lineStyle := roleNormalStyle
		harnessStyle := roleMutedStyle
		if i == m.cursor {
			if m.mode == agentEditMode {
				prefix = roleActiveStyle.Render("◂")
			} else {
				prefix = roleActiveStyle.Render("›")
			}
			lineStyle = roleActiveStyle
			harnessStyle = roleActiveStyle
		}

		line := fmt.Sprintf("%s %s %-8s %s", prefix, dot, agent.Role, harnessStyle.Render(agent.Harness))
		rows = append(rows, lineStyle.Render(line))
	}

	hint := "j/k navigate · enter edit · space toggle · tab next step · q quit"
	if m.mode == agentEditMode {
		hint = "tab next field · h/l cycle · / filter models · esc done editing"
	}
	rows = append(rows, "", hintDescStyle.Render(hint))
	panel := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(panel)
}

func (m *agentStepModel) renderDetailPanel(width, height int) string {
	if width < 1 {
		width = 1
	}
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return lipgloss.NewStyle().Width(width).Height(height).Render("")
	}

	if m.mode == agentEditMode {
		return m.renderEditPanel(width, height)
	}

	a := m.agents[m.cursor]
	temp := a.Temperature
	if temp == "" {
		temp = "default"
	}
	effort := a.Effort
	if effort == "" {
		effort = "default"
	}
	state := "disabled"
	if a.Enabled {
		state = "enabled"
	}

	lines := []string{
		titleStyle.Render(a.Role),
		subtitleStyle.Render(RoleDescription(a.Role)),
		"",
		fmt.Sprintf("%s %s", labelStyle.Render("harness:"), valueStyle.Render(a.Harness)),
		fmt.Sprintf("%s %s", labelStyle.Render("model:"), valueStyle.Render(a.Model)),
		fmt.Sprintf("%s %s", labelStyle.Render("effort:"), valueStyle.Render(effort)),
		fmt.Sprintf("%s %s", labelStyle.Render("temperature:"), valueStyle.Render(temp)),
		fmt.Sprintf("%s %s", labelStyle.Render("status:"), valueStyle.Render(state)),
		"",
		subtitleStyle.Render(RolePhaseText(a.Role)),
	}

	panel := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(panel)
}

func (m *agentStepModel) enterEditMode() {
	m.mode = agentEditMode
	m.editField = 0
	m.filtering = false
	m.filterText = ""
	m.syncModelChoices()
	m.syncTemperatureInput()
}

func (m *agentStepModel) exitEditMode() {
	m.mode = agentBrowseMode
	m.filtering = false
	m.filterText = ""
	m.tempInput.Blur()
}

func (m *agentStepModel) nextField() {
	max := m.maxEditField()
	m.editField++
	if m.editField > max {
		m.editField = 0
	}
	m.filtering = false
	m.syncTemperatureInput()
}

func (m *agentStepModel) prevField() {
	max := m.maxEditField()
	m.editField--
	if m.editField < 0 {
		m.editField = max
	}
	m.filtering = false
	m.syncTemperatureInput()
}

func (m *agentStepModel) cycleFieldValue(direction int) {
	if direction == 0 || m.cursor < 0 || m.cursor >= len(m.agents) {
		return
	}

	a := &m.agents[m.cursor]
	switch m.editField {
	case 0:
		if len(m.harnesses) == 0 {
			return
		}
		index := 0
		for i, h := range m.harnesses {
			if h == a.Harness {
				index = i
				break
			}
		}
		next := index + direction
		for next < 0 {
			next += len(m.harnesses)
		}
		next = next % len(m.harnesses)
		a.Harness = m.harnesses[next]
		m.syncModelChoices()
		m.syncEffortForCurrentAgent()
		m.syncTemperatureInput()
	case 2:
		levels := m.effortOptions(a.Harness, a.Model)
		if len(levels) == 0 {
			return
		}
		index := 0
		for i, lvl := range levels {
			if lvl == a.Effort {
				index = i
				break
			}
		}
		next := index + direction
		for next < 0 {
			next += len(levels)
		}
		next = next % len(levels)
		a.Effort = levels[next]
	}
}

func (m *agentStepModel) updateEditMode(keyMsg tea.KeyMsg) (stepModel, tea.Cmd) {
	if keyMsg.String() == "q" {
		return m, func() tea.Msg { return stepCancelMsg{} }
	}

	switch keyMsg.String() {
	case "esc":
		m.exitEditMode()
		return m, nil
	case "tab":
		m.nextField()
		return m, nil
	case "shift+tab", "backtab":
		m.prevField()
		return m, nil
	case "h", "left":
		m.cycleFieldValue(-1)
		return m, nil
	case "l", "right":
		m.cycleFieldValue(1)
		return m, nil
	}

	if m.editField == 1 {
		switch keyMsg.String() {
		case "j", "down":
			m.moveModelCursor(1)
			return m, nil
		case "k", "up":
			m.moveModelCursor(-1)
			return m, nil
		case "/":
			m.filtering = true
			return m, nil
		case "enter":
			m.selectCurrentModel()
			m.filtering = false
			return m, nil
		case "backspace", "ctrl+h":
			if m.filtering && len(m.filterText) > 0 {
				runes := []rune(m.filterText)
				m.filterText = string(runes[:len(runes)-1])
				m.applyModelFilter()
			}
			return m, nil
		}
		if m.filtering && keyMsg.Type == tea.KeyRunes {
			m.filterText += string(keyMsg.Runes)
			m.applyModelFilter()
			return m, nil
		}
	}

	if keyMsg.String() == "enter" {
		if m.editField == m.maxEditField() {
			m.exitEditMode()
		} else {
			m.nextField()
		}
		return m, nil
	}

	if m.editField == 3 && m.currentHarnessSupportsTemperature() {
		var cmd tea.Cmd
		m.tempInput, cmd = m.tempInput.Update(keyMsg)
		m.agents[m.cursor].Temperature = m.tempInput.Value()
		return m, cmd
	}

	return m, nil
}

func (m *agentStepModel) renderEditPanel(width, height int) string {
	a := m.agents[m.cursor]

	lines := []string{
		titleStyle.Render(a.Role + " ── editing"),
		subtitleStyle.Render(RoleDescription(a.Role)),
		"",
		m.renderCycleField("harness", a.Harness, m.editField == 0),
		"",
		m.renderModelField(width-2, m.editField == 1),
		"",
		m.renderCycleField("effort", m.displayValue(a.Effort), m.editField == 2),
	}

	if m.currentHarnessSupportsTemperature() {
		lines = append(lines, "")
		lines = append(lines, m.renderTemperatureField(m.editField == 3))
	}

	panel := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(panel)
}

func (m *agentStepModel) renderCycleField(name, value string, active bool) string {
	style := fieldNormalStyle
	if active {
		style = fieldActiveStyle
	}
	return fmt.Sprintf("%s %s", labelStyle.Render(name+":"), style.Render("‹ "+value+" ›"))
}

func (m *agentStepModel) renderTemperatureField(active bool) string {
	style := fieldNormalStyle
	if active {
		style = fieldActiveStyle
	}
	view := m.tempInput.View()
	if strings.TrimSpace(view) == "" {
		view = defaultTagStyle.Render("default")
	}
	return fmt.Sprintf("%s %s", labelStyle.Render("temperature:"), style.Render(view))
}

func (m *agentStepModel) renderModelField(width int, active bool) string {
	if width < 12 {
		width = 12
	}
	header := labelStyle.Render("model:")
	if active {
		header = fieldActiveStyle.Render("model:")
	}

	rows := []string{}
	if m.filtering {
		rows = append(rows, fieldActiveStyle.Render("/ "+m.filterText+"█"))
	}

	if len(m.filteredModels) == 0 {
		rows = append(rows, roleMutedStyle.Render("no models"))
	} else {
		innerWidth := width - 4
		if innerWidth < 1 {
			innerWidth = 1
		}
		m.ensureModelCursorVisible()
		end := m.modelOffset + 6
		if end > len(m.filteredModels) {
			end = len(m.filteredModels)
		}
		for i := m.modelOffset; i < end; i++ {
			prefix := " "
			lineStyle := fieldNormalStyle
			if i == m.modelCursor {
				prefix = "›"
				if active {
					lineStyle = fieldActiveStyle
				}
			}
			rows = append(rows, lineStyle.Render(truncateForCell(prefix+" "+m.filteredModels[i], innerWidth)))
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(rows, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, header, box)
}

func truncateForCell(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	runes := []rune(value)
	kept := make([]rune, 0, len(runes))
	for _, r := range runes {
		candidate := string(append(append([]rune(nil), kept...), r))
		if lipgloss.Width(candidate)+3 > width {
			break
		}
		kept = append(kept, r)
	}
	return string(kept) + "..."
}

func (m *agentStepModel) moveModelCursor(delta int) {
	if len(m.filteredModels) == 0 {
		m.modelCursor = 0
		m.modelOffset = 0
		return
	}
	m.modelCursor += delta
	if m.modelCursor < 0 {
		m.modelCursor = 0
	}
	if m.modelCursor >= len(m.filteredModels) {
		m.modelCursor = len(m.filteredModels) - 1
	}
	m.ensureModelCursorVisible()
}

func (m *agentStepModel) ensureModelCursorVisible() {
	if m.modelCursor < m.modelOffset {
		m.modelOffset = m.modelCursor
	}
	if m.modelCursor >= m.modelOffset+6 {
		m.modelOffset = m.modelCursor - 5
	}
	if m.modelOffset < 0 {
		m.modelOffset = 0
	}
}

func (m *agentStepModel) selectCurrentModel() {
	if m.cursor < 0 || m.cursor >= len(m.agents) || len(m.filteredModels) == 0 {
		return
	}
	if m.modelCursor < 0 {
		m.modelCursor = 0
	}
	if m.modelCursor >= len(m.filteredModels) {
		m.modelCursor = len(m.filteredModels) - 1
	}
	m.agents[m.cursor].Model = m.filteredModels[m.modelCursor]
	m.syncEffortForCurrentAgent()
}

func (m *agentStepModel) syncModelChoices() {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		m.allModels = nil
		m.filteredModels = nil
		return
	}
	a := &m.agents[m.cursor]
	models := append([]string(nil), m.modelCache[a.Harness]...)
	if len(models) == 0 && a.Model != "" {
		models = append(models, a.Model)
	}
	m.allModels = models
	m.filterText = ""
	m.filtering = false
	m.applyModelFilter()
}

func (m *agentStepModel) applyModelFilter() {
	needle := strings.ToLower(strings.TrimSpace(m.filterText))
	m.filteredModels = m.filteredModels[:0]
	for _, model := range m.allModels {
		if needle == "" || strings.Contains(strings.ToLower(model), needle) {
			m.filteredModels = append(m.filteredModels, model)
		}
	}
	m.modelCursor = 0
	m.modelOffset = 0
	if m.cursor >= 0 && m.cursor < len(m.agents) && len(m.filteredModels) > 0 {
		current := m.agents[m.cursor].Model
		for i, model := range m.filteredModels {
			if model == current {
				m.modelCursor = i
				break
			}
		}
	}
	m.ensureModelCursorVisible()
}

func (m *agentStepModel) syncEffortForCurrentAgent() {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return
	}
	a := &m.agents[m.cursor]
	levels := m.effortOptions(a.Harness, a.Model)
	if len(levels) == 0 {
		a.Effort = ""
		return
	}
	for _, lvl := range levels {
		if lvl == a.Effort {
			return
		}
	}
	a.Effort = levels[0]
}

func (m *agentStepModel) effortOptions(harnessName, model string) []string {
	if levels, ok := m.effortLevels[harnessName]; ok && len(levels) > 0 {
		return levels
	}
	if harnessName == "opencode" {
		switch {
		case strings.HasPrefix(model, "anthropic/"):
			return []string{"", "low", "medium", "high", "max"}
		case strings.Contains(model, "codex"):
			return []string{"", "low", "medium", "high", "xhigh"}
		default:
			return []string{"", "low", "medium", "high"}
		}
	}
	return []string{"", "low", "medium", "high"}
}

func (m *agentStepModel) syncTemperatureInput() {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		m.tempInput.Blur()
		return
	}
	m.tempInput.SetValue(m.agents[m.cursor].Temperature)
	if m.editField == 3 && m.currentHarnessSupportsTemperature() {
		m.tempInput.Focus()
	} else {
		m.tempInput.Blur()
	}
}

func (m *agentStepModel) currentHarnessSupportsTemperature() bool {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return false
	}
	switch m.agents[m.cursor].Harness {
	case "claude":
		return false
	default:
		return true
	}
}

func (m *agentStepModel) maxEditField() int {
	if m.currentHarnessSupportsTemperature() {
		return 3
	}
	if m.editField > 2 {
		m.editField = 2
	}
	return 2
}

func (m *agentStepModel) displayValue(v string) string {
	if v == "" {
		return "default"
	}
	return v
}
