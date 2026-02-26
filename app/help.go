package app

import (
	"fmt"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpText interface {
	// toContent returns the help UI content.
	toContent() string
	// mask returns the bit mask for this help text. These are used to track which help screens
	// have been seen in the config and app state.
	mask() uint32
}

type helpTypeGeneral struct{}

type helpTypeInstanceStart struct {
	instance *session.Instance
}

type helpTypeInstanceAttach struct{}

type helpTypeInstanceCheckout struct{}

func helpStart(instance *session.Instance) helpText {
	return helpTypeInstanceStart{instance: instance}
}

func (h helpTypeGeneral) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		ui.GradientText("kasmos", ui.GradientStart, ui.GradientEnd),
		"",
		descStyle.Render("multi-agent orchestration IDE. manages concurrent ai agent sessions"),
		descStyle.Render("(opencode, claude, codex, gemini, amp, etc.) in isolated git worktrees"),
		descStyle.Render("with unified tui control over plans, topics, and lifecycle stages."),
		"",
		headerStyle.Render("sessions:"),
		keyStyle.Render("↵/o")+descStyle.Render("           - attach to tmux session fullscreen"),
		keyStyle.Render("i")+descStyle.Render("             - interactive mode (type in pane)"),
		keyStyle.Render("ctrl+space")+descStyle.Render("    - exit fullscreen or interactive mode"),
		keyStyle.Render("k")+descStyle.Render("             - kill tmux session (keeps instance)"),
		keyStyle.Render("K")+descStyle.Render("             - abort session (removes worktree)"),
		keyStyle.Render("r")+descStyle.Render("             - resume paused session"),
		keyStyle.Render("c")+descStyle.Render("             - checkout branch (pause + copy branch name)"),
		keyStyle.Render("P")+descStyle.Render("             - create pull request"),
		keyStyle.Render("R")+descStyle.Render("             - switch repo"),
		keyStyle.Render("1/2")+descStyle.Render("           - filter: all / active only"),
		keyStyle.Render("3")+descStyle.Render("             - cycle sort mode"),
		"",
		headerStyle.Render("plans:"),
		keyStyle.Render("n")+descStyle.Render("             - new plan"),
		keyStyle.Render("space")+descStyle.Render("         - expand/collapse plan, topic, or history"),
		keyStyle.Render("↵/o")+descStyle.Render("           - select (context menu or run stage)"),
		keyStyle.Render("v")+descStyle.Render("             - view selected plan"),
		"",
		headerStyle.Render("navigation:"),
		keyStyle.Render("s")+descStyle.Render("             - focus sidebar"),
		keyStyle.Render("t")+descStyle.Render("             - focus instance list"),
		keyStyle.Render("tab/shift+tab")+descStyle.Render(" - cycle tabs (agent → diff → info)"),
		keyStyle.Render("!/@ /#")+descStyle.Render("        - jump to agent/diff/info tab"),
		keyStyle.Render("g")+descStyle.Render("             - info tab"),
		keyStyle.Render("↑↓")+descStyle.Render("            - navigate within focused pane"),
		keyStyle.Render("←→")+descStyle.Render("            - move between panes"),
		keyStyle.Render("ctrl+s")+descStyle.Render("        - toggle sidebar visibility"),
		keyStyle.Render("/")+descStyle.Render("             - search plans and instances"),
		keyStyle.Render("q")+descStyle.Render("             - quit"),
	)
	return content
}

func (h helpTypeInstanceStart) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("instance created"),
		"",
		descStyle.Render("new session created:"),
		descStyle.Render(fmt.Sprintf("• git branch: %s (isolated worktree)",
			lipgloss.NewStyle().Bold(true).Render(h.instance.Branch))),
		descStyle.Render(fmt.Sprintf("• %s running in background tmux session",
			lipgloss.NewStyle().Bold(true).Render(h.instance.Program))),
		"",
		headerStyle.Render("managing:"),
		keyStyle.Render("↵/o")+descStyle.Render("   - attach to session"),
		keyStyle.Render("tab")+descStyle.Render("   - cycle panes (!/@ /# to jump to agent/diff/info)"),
		keyStyle.Render("k")+descStyle.Render("     - kill tmux session"),
		keyStyle.Render("K")+descStyle.Render("     - abort session (removes worktree)"),
		"",
		headerStyle.Render("handoff:"),
		keyStyle.Render("c")+descStyle.Render("     - checkout this instance's branch"),
		keyStyle.Render("P")+descStyle.Render("     - create a pull request for this branch"),
	)
	return content
}

func (h helpTypeInstanceAttach) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("attaching to instance"),
		"",
		descStyle.Render("to detach from a session, press ")+keyStyle.Render("ctrl-q"),
	)
	return content
}

func (h helpTypeInstanceCheckout) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("checkout instance"),
		"",
		descStyle.Render("changes will be committed locally. the branch name has been copied to your clipboard."),
		"",
		descStyle.Render("feel free to make changes and commit them. when resuming, the session continues from where you left off."),
		"",
		headerStyle.Render("commands:"),
		keyStyle.Render("c")+descStyle.Render(" - checkout: commit changes locally and pause session"),
		keyStyle.Render("r")+descStyle.Render(" - resume a paused session"),
	)
	return content
}
func (h helpTypeGeneral) mask() uint32 {
	return 1
}

func (h helpTypeInstanceStart) mask() uint32 {
	return 1 << 1
}
func (h helpTypeInstanceAttach) mask() uint32 {
	return 1 << 2
}
func (h helpTypeInstanceCheckout) mask() uint32 {
	return 1 << 3
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(ui.ColorIris)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorFoam)
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorGold)
	descStyle   = lipgloss.NewStyle().Foreground(ui.ColorText)
)

// showHelpScreen displays the help screen overlay if it hasn't been shown before
func (m *home) showHelpScreen(helpType helpText, onDismiss func()) (tea.Model, tea.Cmd) {
	// Get the flag for this help type
	var alwaysShow bool
	switch helpType.(type) {
	case helpTypeGeneral:
		alwaysShow = true
	}

	flag := helpType.mask()

	// Check if this help screen has been seen before
	// Only show if we're showing the general help screen or the corresponding flag is not set
	// in the seen bitmask.
	if alwaysShow || (m.appState.GetHelpScreensSeen()&flag) == 0 {
		// Mark this help screen as seen and save state
		if err := m.appState.SetHelpScreensSeen(m.appState.GetHelpScreensSeen() | flag); err != nil {
			log.WarningLog.Printf("Failed to save help screen state: %v", err)
		}

		content := helpType.toContent()

		m.textOverlay = overlay.NewTextOverlay(content)
		m.textOverlay.OnDismiss = onDismiss
		m.state = stateHelp
		return m, nil
	}

	// Skip displaying the help screen
	if onDismiss != nil {
		onDismiss()
	}
	return m, nil
}

// handleHelpState handles key events when in help state
func (m *home) handleHelpState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key press will close the help overlay
	shouldClose := m.textOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.state = stateDefault
		return m, tea.Sequence(
			tea.WindowSize(),
			func() tea.Msg {
				m.menu.SetState(ui.StateDefault)
				return nil
			},
		)
	}

	return m, nil
}
