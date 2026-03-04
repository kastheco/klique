package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PermissionChoice mirrors tmux.PermissionChoice to avoid import cycle.
// Ordering matches opencode's left-to-right menu navigation and tmux.PermissionChoice,
// so app_input.go can cast directly without a mapping switch.
type PermissionChoice int

const (
	PermissionAllowOnce   PermissionChoice = iota // 0 — opencode's default cursor position
	PermissionAllowAlways                         // 1 — one Right arrow from default
	PermissionReject                              // 2 — two Right arrows from default
)

var permissionChoiceLabels = []string{"allow once", "allow always", "reject"}

// PermissionOverlay shows a three-choice modal for opencode permission prompts.
type PermissionOverlay struct {
	instanceTitle string
	description   string
	pattern       string
	selectedIdx   int
	confirmed     bool
	width         int
}

// NewPermissionOverlay creates a permission overlay with extracted prompt data.
func NewPermissionOverlay(instanceTitle, description, pattern string) *PermissionOverlay {
	return &PermissionOverlay{
		instanceTitle: instanceTitle,
		description:   description,
		pattern:       pattern,
		selectedIdx:   1, // default to "allow always"
		width:         50,
	}
}

// Choice returns the selected permission choice.
func (p *PermissionOverlay) Choice() PermissionChoice {
	return PermissionChoice(p.selectedIdx)
}

// IsConfirmed returns true if the user pressed Enter.
func (p *PermissionOverlay) IsConfirmed() bool {
	return p.confirmed
}

// Pattern returns the permission pattern string extracted from the agent pane.
// Use this on confirm instead of re-parsing CachedContent, which may have changed.
func (p *PermissionOverlay) Pattern() string {
	return p.pattern
}

// Description returns the permission description shown in the overlay.
func (p *PermissionOverlay) Description() string {
	return p.description
}

// render draws the permission overlay.
func (p *PermissionOverlay) render() string {
	st := DefaultStyles()

	var b strings.Builder
	b.WriteString(st.WarningTitle.Render("△ permission required"))
	b.WriteString("\n")
	b.WriteString(st.Item.Render(p.description))
	if p.pattern != "" {
		b.WriteString("\n")
		b.WriteString(st.Muted.Render(fmt.Sprintf("pattern: %s", p.pattern)))
	}
	if p.instanceTitle != "" {
		b.WriteString("\n")
		b.WriteString(st.Muted.Render(fmt.Sprintf("instance: %s", p.instanceTitle)))
	}
	b.WriteString("\n\n")

	// Render choices horizontally
	var choices []string
	for i, label := range permissionChoiceLabels {
		if i == p.selectedIdx {
			choices = append(choices, st.SelectedItem.Render("▸ "+label))
		} else {
			choices = append(choices, st.Item.Render("  "+label))
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, choices...))
	b.WriteString("\n\n")
	b.WriteString(st.Muted.Render("←→ select · enter confirm · esc dismiss"))

	return st.WarningBorder.Width(p.width).Render(b.String())
}

// permissionActionLabels maps selectedIdx to the action string returned by HandleKey.
var permissionActionLabels = []string{"allow_once", "allow_always", "reject"}

// HandleKey implements Overlay. Processes a key event and returns a Result.
func (p *PermissionOverlay) HandleKey(msg tea.KeyMsg) Result {
	switch msg.Type {
	case tea.KeyLeft:
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
		return Result{}
	case tea.KeyRight:
		if p.selectedIdx < len(permissionChoiceLabels)-1 {
			p.selectedIdx++
		}
		return Result{}
	case tea.KeyEnter:
		p.confirmed = true
		action := permissionActionLabels[p.selectedIdx]
		return Result{Dismissed: true, Submitted: true, Action: action}
	case tea.KeyEsc:
		return Result{Dismissed: true}
	}
	return Result{}
}

// View implements Overlay. Returns the rendered overlay string.
func (p *PermissionOverlay) View() string {
	return p.render()
}

// SetSize implements Overlay. Updates the available dimensions for the overlay.
func (p *PermissionOverlay) SetSize(w, h int) {
	p.width = w
}
