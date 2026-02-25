package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/session"
)

var (
	previewPaneStyle = lipgloss.NewStyle().
				Foreground(ColorText)
	scrollbarTrackStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	scrollbarThumbStyle = lipgloss.NewStyle().Foreground(ColorIris)
)

type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool
	viewport     viewport.Model

	// bannerFrame tracks the animation frame for the idle banner dots.
	bannerFrame int
	// animateBanner gates the idle banner animation (disabled by default).
	animateBanner bool
	// isDocument is true when the preview is showing a rendered document (plan markdown).
	// While set, UpdateContent() is a no-op so the tick loop doesn't overwrite the content.
	isDocument bool
	// springAnim drives the banner load-in animation on launch.
	springAnim *SpringAnim
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
		viewport:   viewport.New(0, 0),
		springAnim: NewSpringAnim(6.0, 15), // 6 rows, 750ms CTA delay
	}
}

// TickSpring advances the spring load-in animation by one frame.
func (p *PreviewPane) TickSpring() {
	if p.springAnim != nil {
		p.springAnim.Tick()
	}
}

// SetRawContent sets the preview content directly from a pre-rendered string.
// Used by the embedded terminal emulator in focus mode.
func (p *PreviewPane) SetRawContent(content string) {
	p.previewState = previewState{text: content}
	p.isScrolling = false
	p.isDocument = false
}

func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.Width = max(0, width-1)
	p.viewport.Height = maxHeight
}

// setFallbackState sets the preview state with the animated banner and a message below it.
func (p *PreviewPane) setFallbackState(message string) {
	p.previewState = previewState{
		fallback:    true,
		fallbackMsg: message,
	}
}

// SetDocumentContent sets the preview to show a rendered document (e.g. plan markdown)
// using the viewport for scrollable display. Sets isDocument so the periodic
// UpdateContent tick won't overwrite the content.
func (p *PreviewPane) SetDocumentContent(content string) {
	p.previewState = previewState{fallback: false}
	p.isScrolling = false
	p.isDocument = true
	p.viewport.SetContent(content)
	p.viewport.GotoTop()
}

// IsDocumentMode returns true when the preview is showing a static document.
func (p *PreviewPane) IsDocumentMode() bool {
	return p.isDocument
}

// ClearDocumentMode exits document mode so UpdateContent resumes normal preview.
func (p *PreviewPane) ClearDocumentMode() {
	p.isDocument = false
}

// ViewportUpdate forwards a tea.Msg to the viewport when in document or scroll
// mode, enabling the viewport's built-in key handling (PgUp/PgDn, Home/End,
// arrow keys, mouse wheel). Returns any command the viewport emits.
func (p *PreviewPane) ViewportUpdate(msg tea.Msg) tea.Cmd {
	if !p.isDocument && !p.isScrolling {
		return nil
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return cmd
}

// ViewportHandlesKey reports whether the viewport keymap handles this key.
func (p *PreviewPane) ViewportHandlesKey(msg tea.KeyMsg) bool {
	if !p.isDocument && !p.isScrolling {
		return false
	}

	km := p.viewport.KeyMap
	return key.Matches(msg,
		km.Up,
		km.Down,
		km.Left,
		km.Right,
		km.PageUp,
		km.PageDown,
		km.HalfPageUp,
		km.HalfPageDown,
	)
}

// setFallbackContent sets the preview state with arbitrary centered content (no banner).
func (p *PreviewPane) setFallbackContent(content string) {
	p.previewState = previewState{
		fallback: true,
		text:     content,
	}
}

// SetAnimateBanner enables or disables the idle banner animation.
func (p *PreviewPane) SetAnimateBanner(enabled bool) {
	p.animateBanner = enabled
}

// TickBanner advances the banner animation frame. Call from the app tick loop.
func (p *PreviewPane) TickBanner() {
	if p.animateBanner && p.previewState.fallback {
		p.bannerFrame++
	}
}

// UpdateContent updates the preview pane with fallback states for nil/loading/paused instances.
// In normal mode the live content is pushed via SetRawContent from the VT emulator tick loop.
// No-op when in document mode (isDocument) — the caller must ClearDocumentMode first.
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	if p.isDocument {
		return nil
	}
	switch {
	case instance == nil:
		p.setFallbackState("create [n]ew plan or [s]elect existing")
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

	// If in scroll mode but haven't captured content yet, do it now
	if p.isScrolling && p.viewport.Height > 0 && len(p.viewport.View()) == 0 {
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		footer := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("ESC to exit scroll mode")

		p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	}
	// In normal (non-scroll) mode, live content arrives via SetRawContent from the VT emulator.

	return nil
}

// renderScrollbar builds a vertical scrollbar string of the given height using
// the viewport's current scroll position. Returns an empty string when all
// content fits on screen (no scrolling needed).
func (p *PreviewPane) renderScrollbar(height int) string {
	if height <= 0 {
		return ""
	}

	pct := p.viewport.ScrollPercent()

	// Don't show scrollbar when everything fits on screen.
	if p.viewport.AtBottom() && p.viewport.YOffset == 0 {
		return ""
	}

	thumbSize := max(1, height/5)
	trackLen := height - thumbSize
	thumbPos := int(pct * float64(trackLen))

	var sb strings.Builder
	for i := 0; i < height; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(scrollbarThumbStyle.Render("▐"))
		} else {
			sb.WriteString(scrollbarTrackStyle.Render("│"))
		}
		if i < height-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
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
			bannerLines := BannerLines(p.bannerFrame)

			// Spring load-in: unfold center rows, CTA after delay
			animating := p.springAnim != nil && !p.springAnim.Settled()
			visibleRows := len(bannerLines)
			if animating {
				visibleRows = p.springAnim.VisibleRows()
			}

			if visibleRows <= 0 {
				// Reserve space for banner + blank + CTA so position is stable
				bannerWidth := lipgloss.Width(bannerLines[0])
				blankBanner := strings.Repeat(strings.Repeat(" ", bannerWidth)+"\n", len(bannerLines)-1)
				blankBanner += strings.Repeat(" ", bannerWidth)
				blankCTA := strings.Repeat(" ", lipgloss.Width(p.previewState.fallbackMsg))
				fallbackText = lipgloss.JoinVertical(lipgloss.Left, blankBanner, "", blankCTA)
			} else {
				totalRows := len(bannerLines)
				startRow := (totalRows - visibleRows) / 2
				endRow := startRow + visibleRows
				if startRow < 0 {
					startRow = 0
				}
				if endRow > totalRows {
					endRow = totalRows
				}

				// Pad hidden rows with blank lines to keep total height constant
				bannerWidth := lipgloss.Width(bannerLines[0])
				blankLine := strings.Repeat(" ", bannerWidth)
				var bannerParts []string
				hiddenTop := startRow
				for i := 0; i < hiddenTop; i++ {
					bannerParts = append(bannerParts, blankLine)
				}
				bannerParts = append(bannerParts, bannerLines[startRow:endRow]...)
				hiddenBottom := totalRows - endRow
				for i := 0; i < hiddenBottom; i++ {
					bannerParts = append(bannerParts, blankLine)
				}
				banner := strings.Join(bannerParts, "\n")

				// Always reserve CTA space; horizontal reveal after delay
				ctaMsg := p.previewState.fallbackMsg
				ctaRunes := []rune(ctaMsg)
				ctaFullWidth := lipgloss.Width(ctaMsg)
				ctaPad := (bannerWidth - ctaFullWidth) / 2
				if ctaPad < 0 {
					ctaPad = 0
				}

				// Tell the spring how long the CTA is
				if p.springAnim != nil {
					p.springAnim.SetCTALen(len(ctaRunes))
				}

				visChars := len(ctaRunes) // default: show all
				if p.springAnim != nil && !p.springAnim.Settled() {
					visChars = p.springAnim.CTAVisibleChars()
				}

				if visChars <= 0 {
					// Blank placeholder
					blankCTA := strings.Repeat(" ", ctaFullWidth)
					centeredCTA := strings.Repeat(" ", ctaPad) + blankCTA
					fallbackText = lipgloss.JoinVertical(lipgloss.Left, banner, "", centeredCTA)
				} else if visChars >= len(ctaRunes) {
					centeredCTA := strings.Repeat(" ", ctaPad) + ctaMsg
					fallbackText = lipgloss.JoinVertical(lipgloss.Left, banner, "", centeredCTA)
				} else {
					// Partial reveal: show visChars, pad the rest
					shown := string(ctaRunes[:visChars])
					remaining := ctaFullWidth - lipgloss.Width(shown)
					partialCTA := strings.Repeat(" ", ctaPad) + shown + strings.Repeat(" ", remaining)
					fallbackText = lipgloss.JoinVertical(lipgloss.Left, banner, "", partialCTA)
				}
			}
		} else if p.previewState.text != "" {
			// Content mode: loading spinner, paused state, etc.
			fallbackText = p.previewState.text
		} else {
			bannerLines := BannerLines(p.bannerFrame)
			if p.springAnim != nil && !p.springAnim.Settled() {
				visibleRows := p.springAnim.VisibleRows()
				if visibleRows <= 0 {
					fallbackText = ""
				} else {
					totalRows := len(bannerLines)
					startRow := (totalRows - visibleRows) / 2
					endRow := startRow + visibleRows
					if startRow < 0 {
						startRow = 0
					}
					if endRow > totalRows {
						endRow = totalRows
					}
					fallbackText = strings.Join(bannerLines[startRow:endRow], "\n")
				}
			} else {
				fallbackText = FallBackText(p.bannerFrame)
			}
		}

		// Center both vertically and horizontally.
		return lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			previewPaneStyle.Render(fallbackText),
		)
	}

	// If in document or scroll mode, use the viewport to display scrollable content
	if p.isDocument || p.isScrolling {
		viewContent := p.viewport.View()
		scrollbar := p.renderScrollbar(p.viewport.Height)
		if scrollbar == "" {
			return viewContent
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, viewContent, scrollbar)
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
	// In document mode the viewport already has the content — just scroll it.
	if p.isDocument {
		p.viewport.LineUp(1)
		return nil
	}

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
	// In document mode the viewport already has the content — just scroll it.
	if p.isDocument {
		p.viewport.LineDown(1)
		return nil
	}

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
