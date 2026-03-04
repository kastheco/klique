package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kastheco/kasmos/session"
)

var (
	previewPaneStyle    = lipgloss.NewStyle().Foreground(ColorText)
	scrollbarTrackStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	scrollbarThumbStyle = lipgloss.NewStyle().Foreground(ColorIris)
)

// previewState holds the current display state of the preview pane.
type previewState struct {
	// fallback indicates the pane is in fallback (no active session) mode.
	fallback bool
	// fallbackMsg is shown below the banner in banner-fallback mode.
	fallbackMsg string
	// text holds the raw content to display in normal or fallback-content modes.
	text string
}

// PreviewPane renders the agent session preview area.
type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool
	viewport     viewport.Model

	// bannerFrame is the current animation tick index for the idle banner.
	bannerFrame int
	// animateBanner enables the idle banner ticker when true.
	animateBanner bool
	// isDocument is true while showing a rendered plan document.
	// UpdateContent is a no-op when this flag is set.
	isDocument bool
	// isRawTerminal is true when content was pushed via SetRawContent.
	// The VT emulator already fills exactly p.height rows, so String() must
	// not subtract 1 for an ellipsis row or the last line gets dropped.
	isRawTerminal bool
	// springAnim drives the banner load-in animation on first render.
	springAnim *SpringAnim
}

// NewPreviewPane constructs a PreviewPane with initial fallback state.
func NewPreviewPane() *PreviewPane {
	return &PreviewPane{
		viewport:   viewport.New(),
		springAnim: NewSpringAnim(6.0, 15),
		previewState: previewState{
			fallback:    true,
			fallbackMsg: "create [n]ew plan or select existing",
		},
	}
}

// TickSpring advances the spring load-in animation by one frame.
func (p *PreviewPane) TickSpring() {
	if p.springAnim != nil {
		p.springAnim.Tick()
	}
}

// SetRawContent sets preview content from a pre-rendered string (VT emulator path).
// Clears scroll, document, and fallback flags and marks the pane as raw-terminal.
func (p *PreviewPane) SetRawContent(content string) {
	p.previewState = previewState{text: content}
	p.isScrolling = false
	p.isDocument = false
	p.isRawTerminal = true
}

// SetSize stores the pane dimensions and configures the viewport.
// The viewport width is width-1 to reserve one column for the scrollbar.
func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.SetWidth(max(0, width-1))
	p.viewport.SetHeight(maxHeight)
}

// setFallbackState puts the pane into banner+message fallback mode.
func (p *PreviewPane) setFallbackState(message string) {
	p.previewState = previewState{
		fallback:    true,
		fallbackMsg: message,
	}
	p.isRawTerminal = false
}

// SetDocumentContent loads scrollable document content into the viewport.
// The pane remains in document mode (UpdateContent is a no-op) until
// ClearDocumentMode is called.
func (p *PreviewPane) SetDocumentContent(content string) {
	p.previewState = previewState{fallback: false}
	p.isScrolling = false
	p.isDocument = true
	p.isRawTerminal = false
	p.viewport.SetContent(content)
	p.viewport.GotoTop()
}

// IsDocumentMode reports whether the pane is displaying a static document.
func (p *PreviewPane) IsDocumentMode() bool {
	return p.isDocument
}

// ClearDocumentMode exits document mode so UpdateContent resumes normal preview.
func (p *PreviewPane) ClearDocumentMode() {
	p.isDocument = false
}

// ViewportUpdate forwards a tea.Msg to the viewport when in document or scroll
// mode, enabling native viewport key handling (PgUp/PgDn, arrows, mouse wheel).
// Returns nil when the pane is not in a scrollable mode.
func (p *PreviewPane) ViewportUpdate(msg tea.Msg) tea.Cmd {
	if !p.isDocument && !p.isScrolling {
		return nil
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return cmd
}

// ViewportHandlesKey reports whether the viewport keymap matches the given key
// when the pane is in document or scroll mode.
func (p *PreviewPane) ViewportHandlesKey(msg tea.KeyMsg) bool {
	if !p.isDocument && !p.isScrolling {
		return false
	}
	km := p.viewport.KeyMap
	return key.Matches(msg,
		km.Up, km.Down, km.Left, km.Right,
		km.PageUp, km.PageDown,
		km.HalfPageUp, km.HalfPageDown,
	)
}

// setFallbackContent sets fallback mode with arbitrary centered content (no banner).
func (p *PreviewPane) setFallbackContent(content string) {
	p.previewState = previewState{
		fallback: true,
		text:     content,
	}
	p.isRawTerminal = false
}

// SetAnimateBanner enables or disables the idle banner animation ticker.
func (p *PreviewPane) SetAnimateBanner(enabled bool) {
	p.animateBanner = enabled
}

// TickBanner advances the banner animation frame. Call from the app tick loop.
func (p *PreviewPane) TickBanner() {
	if p.animateBanner && p.previewState.fallback {
		p.bannerFrame++
	}
}

// UpdateContent refreshes the pane based on the instance state. It is a no-op
// when in document mode. In normal (non-scroll) mode live content arrives via
// SetRawContent from the VT emulator; this method only handles nil/Loading/
// Paused/Exited special cases plus initial scroll-mode capture.
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	if p.isDocument {
		return nil
	}

	switch {
	case instance == nil:
		p.setFallbackState("create [n]ew plan or select existing")
		return nil

	case instance.Status == session.Loading:
		stage := instance.LoadingStage
		total := instance.LoadingTotal
		if total == 0 {
			total = 7
		}
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

		p.setFallbackContent(lipgloss.JoinVertical(lipgloss.Center,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(GradientStart)).Render("Starting instance"),
			"",
			bar,
			"",
			lipgloss.NewStyle().Foreground(ColorMuted).Render(stepText),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("%d%%", pct)),
		))
		return nil

	case instance.Status == session.Paused:
		p.setFallbackContent(lipgloss.JoinVertical(lipgloss.Center,
			"Session is paused. Press 'r' to resume.",
			"",
			lipgloss.NewStyle().Foreground(ColorGold).Render(fmt.Sprintf(
				"The instance can be checked out at '%s' (copied to your clipboard)",
				instance.Branch,
			)),
		))
		return nil

	case instance.Exited:
		p.setFallbackContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(ColorMuted).Render("session exited"),
			"",
			lipgloss.NewStyle().Foreground(ColorMuted).Render("press shift+k to remove"),
		))
		return nil
	}

	// If in scroll mode but haven't loaded content yet, capture full history now.
	if p.isScrolling && p.viewport.Height() > 0 && len(p.viewport.View()) == 0 {
		content, err := instance.PreviewFullHistory()
		if err != nil {
			return err
		}
		footer := lipgloss.NewStyle().Foreground(ColorMuted).Render("ESC to exit scroll mode")
		p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	}
	// Normal mode: live content arrives via SetRawContent from the VT emulator.
	return nil
}

// renderScrollbar builds a vertical scrollbar string for the given height.
// Returns an empty string when all content fits on screen (no scrolling needed).
func (p *PreviewPane) renderScrollbar(height int) string {
	if height <= 0 {
		return ""
	}
	// Hide scrollbar when everything fits on one screen.
	if p.viewport.AtBottom() && p.viewport.YOffset() == 0 {
		return ""
	}

	pct := p.viewport.ScrollPercent()
	thumbSize := max(1, height/5)
	trackLen := height - thumbSize
	thumbPos := int(pct * float64(trackLen))

	var sb strings.Builder
	for i := range height {
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

// String renders the preview pane to a string.
func (p *PreviewPane) String() string {
	if p.width == 0 || p.height == 0 {
		return strings.Repeat("\n", p.height)
	}

	if p.previewState.fallback {
		fallbackText := p.buildFallbackText()
		return lipgloss.Place(
			p.width, p.height,
			lipgloss.Center, lipgloss.Center,
			previewPaneStyle.Render(fallbackText),
		)
	}

	// Document or scroll mode: render via viewport + optional scrollbar.
	if p.isDocument || p.isScrolling {
		viewContent := p.viewport.View()
		scrollbar := p.renderScrollbar(p.viewport.Height())
		if scrollbar == "" {
			return viewContent
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, viewContent, scrollbar)
	}

	// Normal mode: split text, truncate/pad, render.
	availableHeight := p.height
	if !p.isRawTerminal {
		availableHeight-- // reserve one row for the ellipsis overflow indicator
	}

	lines := strings.Split(p.previewState.text, "\n")
	if availableHeight > 0 {
		if len(lines) > availableHeight {
			lines = lines[:availableHeight]
			if !p.isRawTerminal {
				lines = append(lines, "...")
			}
		} else {
			padding := availableHeight - len(lines)
			lines = append(lines, make([]string, padding)...)
		}
	}

	return previewPaneStyle.Width(p.width).Render(strings.Join(lines, "\n"))
}

// buildFallbackText constructs the text for fallback (no active session) rendering.
func (p *PreviewPane) buildFallbackText() string {
	// Content fallback (loading spinner, paused state, exited, etc.)
	if p.previewState.fallbackMsg == "" {
		if p.previewState.text != "" {
			return p.previewState.text
		}
		// Banner only — no CTA message.
		bannerLines := BannerLines(p.bannerFrame)
		if p.springAnim != nil && !p.springAnim.Settled() {
			visibleRows := p.springAnim.VisibleRows()
			if visibleRows <= 0 {
				return ""
			}
			total := len(bannerLines)
			start := (total - visibleRows) / 2
			end := start + visibleRows
			if start < 0 {
				start = 0
			}
			if end > total {
				end = total
			}
			return strings.Join(bannerLines[start:end], "\n")
		}
		return FallBackText(p.bannerFrame)
	}

	// Banner + CTA message fallback.
	bannerLines := BannerLines(p.bannerFrame)
	animating := p.springAnim != nil && !p.springAnim.Settled()
	visibleRows := len(bannerLines)
	if animating {
		visibleRows = p.springAnim.VisibleRows()
	}

	if visibleRows <= 0 {
		// Reserve stable space while spring starts up.
		bannerWidth := lipgloss.Width(bannerLines[0])
		blankBanner := strings.Repeat(strings.Repeat(" ", bannerWidth)+"\n", len(bannerLines)-1)
		blankBanner += strings.Repeat(" ", bannerWidth)
		blankCTA := strings.Repeat(" ", lipgloss.Width(p.previewState.fallbackMsg))
		return lipgloss.JoinVertical(lipgloss.Left, blankBanner, "", blankCTA)
	}

	// Build banner block with hidden rows as blanks to keep height constant.
	totalRows := len(bannerLines)
	startRow := (totalRows - visibleRows) / 2
	endRow := startRow + visibleRows
	if startRow < 0 {
		startRow = 0
	}
	if endRow > totalRows {
		endRow = totalRows
	}

	bannerWidth := lipgloss.Width(bannerLines[0])
	blankLine := strings.Repeat(" ", bannerWidth)
	bannerParts := make([]string, 0, totalRows)
	for range startRow {
		bannerParts = append(bannerParts, blankLine)
	}
	bannerParts = append(bannerParts, bannerLines[startRow:endRow]...)
	for range totalRows - endRow {
		bannerParts = append(bannerParts, blankLine)
	}
	banner := strings.Join(bannerParts, "\n")

	// CTA: horizontal character-by-character reveal after spring delay.
	ctaMsg := p.previewState.fallbackMsg
	ctaRunes := []rune(ctaMsg)
	ctaFullWidth := lipgloss.Width(ctaMsg)
	ctaPad := (bannerWidth - ctaFullWidth) / 2
	if ctaPad < 0 {
		ctaPad = 0
	}

	if p.springAnim != nil {
		p.springAnim.SetCTALen(len(ctaRunes))
	}

	visChars := len(ctaRunes) // default: fully visible when settled
	if p.springAnim != nil && !p.springAnim.Settled() {
		visChars = p.springAnim.CTAVisibleChars()
	}

	switch {
	case visChars <= 0:
		blankCTA := strings.Repeat(" ", ctaFullWidth)
		centeredCTA := strings.Repeat(" ", ctaPad) + blankCTA
		return lipgloss.JoinVertical(lipgloss.Left, banner, "", centeredCTA)
	case visChars >= len(ctaRunes):
		centeredCTA := strings.Repeat(" ", ctaPad) + ctaMsg
		return lipgloss.JoinVertical(lipgloss.Left, banner, "", centeredCTA)
	default:
		shown := string(ctaRunes[:visChars])
		remaining := ctaFullWidth - lipgloss.Width(shown)
		partialCTA := strings.Repeat(" ", ctaPad) + shown + strings.Repeat(" ", remaining)
		return lipgloss.JoinVertical(lipgloss.Left, banner, "", partialCTA)
	}
}

// enterScrollMode captures the full terminal history and sets up the viewport
// for scroll mode. Shared by all scroll entry points.
func (p *PreviewPane) enterScrollMode(instance *session.Instance) error {
	content, err := instance.PreviewFullHistory()
	if err != nil {
		return err
	}
	footer := lipgloss.NewStyle().Foreground(ColorMuted).Render("ESC to exit scroll mode")
	p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	p.viewport.GotoBottom()
	p.isScrolling = true
	return nil
}

// ScrollUp scrolls the preview up one line. Enters scroll mode on first call.
func (p *PreviewPane) ScrollUp(instance *session.Instance) error {
	if p.isDocument {
		p.viewport.ScrollUp(1)
		return nil
	}
	if instance == nil || instance.Status == session.Paused {
		return nil
	}
	if !p.isScrolling {
		if err := p.enterScrollMode(instance); err != nil {
			return err
		}
		return nil
	}
	p.viewport.ScrollUp(1)
	return nil
}

// ScrollDown scrolls the preview down one line. Enters scroll mode on first call.
func (p *PreviewPane) ScrollDown(instance *session.Instance) error {
	if p.isDocument {
		p.viewport.ScrollDown(1)
		return nil
	}
	if instance == nil || instance.Status == session.Paused {
		return nil
	}
	if !p.isScrolling {
		if err := p.enterScrollMode(instance); err != nil {
			return err
		}
		return nil
	}
	p.viewport.ScrollDown(1)
	return nil
}

// HalfPageUp scrolls up half a viewport height. Enters scroll mode on first call.
func (p *PreviewPane) HalfPageUp(instance *session.Instance) error {
	if p.isDocument {
		p.viewport.HalfPageUp()
		return nil
	}
	if instance == nil || instance.Status == session.Paused {
		return nil
	}
	if !p.isScrolling {
		if err := p.enterScrollMode(instance); err != nil {
			return err
		}
	}
	p.viewport.HalfPageUp()
	return nil
}

// HalfPageDown scrolls down half a viewport height. Enters scroll mode on first call.
func (p *PreviewPane) HalfPageDown(instance *session.Instance) error {
	if p.isDocument {
		p.viewport.HalfPageDown()
		return nil
	}
	if instance == nil || instance.Status == session.Paused {
		return nil
	}
	if !p.isScrolling {
		if err := p.enterScrollMode(instance); err != nil {
			return err
		}
	}
	p.viewport.HalfPageDown()
	return nil
}

// ResetToNormalMode exits scroll mode and returns to live preview.
func (p *PreviewPane) ResetToNormalMode(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}
	if !p.isScrolling {
		return nil
	}
	p.isScrolling = false
	p.viewport.SetContent("")
	p.viewport.GotoTop()

	// Fetch fresh preview content immediately rather than waiting for next tick.
	content, err := instance.Preview()
	if err != nil {
		return err
	}
	p.previewState.text = content
	return nil
}
