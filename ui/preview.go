package ui

import (
	"fmt"
	"github.com/kastheco/klique/session"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var previewPaneStyle = lipgloss.NewStyle().
	Foreground(ColorText)

type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool
	viewport     viewport.Model

	// bannerFrame tracks the animation frame for the idle banner dots.
	bannerFrame int
}

type previewState struct {
	// fallback is true if the preview pane is displaying fallback text
	fallback bool
	// fallbackMsg is the message shown below the banner in fallback mode
	fallbackMsg string
	// text is the text displayed in the preview pane
	text string
}

func NewPreviewPane() *PreviewPane {
	return &PreviewPane{
		viewport: viewport.New(0, 0),
	}
}

// SetRawContent sets the preview content directly from a pre-rendered string.
// Used by the embedded terminal emulator in focus mode.
func (p *PreviewPane) SetRawContent(content string) {
	p.previewState = previewState{text: content}
	p.isScrolling = false
}

func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.Width = width
	p.viewport.Height = maxHeight
}

// setFallbackState sets the preview state with the animated banner and a message below it.
func (p *PreviewPane) setFallbackState(message string) {
	p.previewState = previewState{
		fallback:    true,
		fallbackMsg: message,
	}
}

// setFallbackContent sets the preview state with arbitrary centered content (no banner).
func (p *PreviewPane) setFallbackContent(content string) {
	p.previewState = previewState{
		fallback: true,
		text:     content,
	}
}

// TickBanner advances the banner animation frame. Call from the app tick loop.
func (p *PreviewPane) TickBanner() {
	if p.previewState.fallback {
		p.bannerFrame++
	}
}

// Updates the preview pane content with the tmux pane content
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	switch {
	case instance == nil:
		p.setFallbackState("No agents running, select a plan or create a 'n'ew one.")
		return nil
	case instance.Status == session.Loading:
		// Real progress from instance startup stages
		stage := instance.LoadingStage
		total := instance.LoadingTotal
		if total == 0 {
			total = 7
		}

		// Progress bar based on actual stage
		barWidth := 20
		filled := (stage * barWidth) / total
		if filled > barWidth {
			filled = barWidth
		}
		bar := GradientBar(barWidth, filled, GradientStart, GradientEnd)

		stepText := instance.LoadingMessage
		if stepText == "" {
			stepText = "Starting..."
		}

		pct := 0
		if total > 0 {
			pct = (stage * 100) / total
		}
		progressText := fmt.Sprintf("%d%%", pct)

		p.setFallbackContent(lipgloss.JoinVertical(lipgloss.Center,
			"",
			lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(GradientStart)).
				Render("Starting instance"),
			"",
			bar,
			"",
			lipgloss.NewStyle().
				Foreground(ColorMuted).
				Render(stepText),
			lipgloss.NewStyle().
				Foreground(ColorMuted).
				Render(progressText),
		))
		return nil
	case instance.Status == session.Paused:
		p.setFallbackContent(lipgloss.JoinVertical(lipgloss.Center,
			"Session is paused. Press 'r' to resume.",
			"",
			lipgloss.NewStyle().
				Foreground(ColorGold).
				Render(fmt.Sprintf(
					"The instance can be checked out at '%s' (copied to your clipboard)",
					instance.Branch,
				)),
		))
		return nil
	}

	var content string
	var err error

	// If in scroll mode but haven't captured content yet, do it now
	if p.isScrolling && p.viewport.Height > 0 && len(p.viewport.View()) == 0 {
		// Capture full pane content including scrollback history using capture-pane -p -S -
		content, err = instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("ESC to exit scroll mode")

		p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	} else if !p.isScrolling {
		// In normal mode, use the usual preview
		content, err = instance.Preview()
		if err != nil {
			return err
		}

		// Always update the preview state with content, even if empty
		// This ensures that newly created instances will display their content immediately
		if len(content) == 0 && !instance.Started() {
			p.setFallbackState("Please enter a name for the instance.")
		} else {
			// Update the preview state with the current content
			p.previewState = previewState{
				fallback: false,
				text:     content,
			}
		}
	}

	return nil
}

// Returns the preview pane content as a string.
func (p *PreviewPane) String() string {
	if p.width == 0 || p.height == 0 {
		return strings.Repeat("\n", p.height)
	}

	if p.previewState.fallback {
		// Build fallback text: either animated banner + message, or raw content
		var fallbackText string
		if p.previewState.fallbackMsg != "" {
			// Banner mode: animated cursor + message
			fallbackText = lipgloss.JoinVertical(lipgloss.Left, FallBackText(p.bannerFrame), "", p.previewState.fallbackMsg)
		} else if p.previewState.text != "" {
			// Content mode: loading spinner, paused state, etc.
			fallbackText = p.previewState.text
		} else {
			fallbackText = FallBackText(p.bannerFrame)
		}

		// Calculate available height for fallback text
		availableHeight := p.height - 3 - 4 // 2 for borders, 1 for margin, 1 for padding

		// Count the number of lines in the fallback text
		fallbackLines := len(strings.Split(fallbackText, "\n"))

		// Calculate padding needed above and below to center the content
		totalPadding := availableHeight - fallbackLines
		topPadding := 0
		bottomPadding := 0
		if totalPadding > 0 {
			topPadding = totalPadding / 2
			bottomPadding = totalPadding - topPadding // accounts for odd numbers
		}

		// Build the centered content
		var lines []string
		if topPadding > 0 {
			lines = append(lines, strings.Repeat("\n", topPadding))
		}
		lines = append(lines, fallbackText)
		if bottomPadding > 0 {
			lines = append(lines, strings.Repeat("\n", bottomPadding))
		}

		// Center both vertically and horizontally
		return previewPaneStyle.
			Width(p.width).
			Align(lipgloss.Center).
			Render(strings.Join(lines, ""))
	}

	// If in copy mode, use the viewport to display scrollable content
	if p.isScrolling {
		return p.viewport.View()
	}

	// Normal mode display
	// Calculate available height accounting for border and margin
	availableHeight := p.height - 1 //  1 for ellipsis

	lines := strings.Split(p.previewState.text, "\n")

	// Truncate if we have more lines than available height
	if availableHeight > 0 {
		if len(lines) > availableHeight {
			lines = lines[:availableHeight]
			lines = append(lines, "...")
		} else {
			// Pad with empty lines to fill available height
			padding := availableHeight - len(lines)
			lines = append(lines, make([]string, padding)...)
		}
	}

	content := strings.Join(lines, "\n")
	rendered := previewPaneStyle.Width(p.width).Render(content)
	return rendered
}

// ScrollUp scrolls up in the viewport
func (p *PreviewPane) ScrollUp(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - capture entire pane content including scrollback history
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("ESC to exit scroll mode")

		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, content, footer)
		p.viewport.SetContent(contentWithFooter)

		// Position the viewport at the bottom initially
		p.viewport.GotoBottom()

		p.isScrolling = true
		return nil
	}

	// Already in scroll mode, just scroll the viewport
	p.viewport.LineUp(1)
	return nil
}

// ScrollDown scrolls down in the viewport
func (p *PreviewPane) ScrollDown(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - capture entire pane content including scrollback history
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("ESC to exit scroll mode")

		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, content, footer)
		p.viewport.SetContent(contentWithFooter)

		// Position the viewport at the bottom initially
		p.viewport.GotoBottom()

		p.isScrolling = true
		return nil
	}

	// Already in copy mode, just scroll the viewport
	p.viewport.LineDown(1)
	return nil
}

// ResetToNormalMode exits scroll mode and returns to normal mode
func (p *PreviewPane) ResetToNormalMode(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if p.isScrolling {
		p.isScrolling = false
		// Reset viewport
		p.viewport.SetContent("")
		p.viewport.GotoTop()

		// Immediately update content instead of waiting for next UpdateContent call
		content, err := instance.Preview()
		if err != nil {
			return err
		}
		p.previewState.text = content
	}

	return nil
}
