package ui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone/v2"
)

// ── Minimal PreviewPane stub ──────────────────────────────────────────────────
//
// The full PreviewPane (VT emulator, banner animation, scroll capture) has been
// removed as part of the Wave 2→3 migration to a native tmux pane layout.
// This stub preserves the public API surface so callers in app/ and tests in
// ui/ continue to compile unchanged until Wave 3 completes the migration.

// previewState holds the current display state of the preview pane.
type previewState struct {
	// fallback indicates the pane is in fallback (no active session) mode.
	fallback bool
	// fallbackMsg is shown in fallback mode.
	fallbackMsg string
	// text holds the raw content to display.
	text string
}

// PreviewPane is a minimal stub replacing the full VT-emulator preview pane.
// It retains the fields and methods required by tabbed_window_test.go and
// callers in app/ so the codebase compiles during the Wave 2→3 transition.
type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool // always false in stub; kept for API compatibility
	viewport     viewport.Model

	// isDocument is true while showing a rendered plan document.
	// UpdateContent is a no-op when this flag is set.
	isDocument bool
}

// NewPreviewPane constructs a PreviewPane with initial fallback state.
func NewPreviewPane() *PreviewPane {
	return &PreviewPane{
		viewport: viewport.New(),
		previewState: previewState{
			fallback:    true,
			fallbackMsg: "create [n]ew plan or select existing",
		},
	}
}

// SetSize stores the pane dimensions and configures the viewport.
func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.SetWidth(max(0, width-1))
	p.viewport.SetHeight(maxHeight)
}

// setFallbackState puts the pane into fallback mode with the given message.
func (p *PreviewPane) setFallbackState(message string) {
	p.previewState = previewState{
		fallback:    true,
		fallbackMsg: message,
	}
}

// SetRawContent is a no-op stub (VT emulator path removed).
func (p *PreviewPane) SetRawContent(_ string) {}

// SetDocumentContent loads scrollable document content into the viewport.
func (p *PreviewPane) SetDocumentContent(content string) {
	p.previewState = previewState{fallback: false}
	p.isScrolling = false
	p.isDocument = true
	p.viewport.SetContent(content)
	p.viewport.GotoTop()
}

// IsDocumentMode reports whether the pane is displaying a static document.
func (p *PreviewPane) IsDocumentMode() bool { return p.isDocument }

// ClearDocumentMode exits document mode so UpdateContent resumes normal preview.
func (p *PreviewPane) ClearDocumentMode() { p.isDocument = false }

// ViewportUpdate forwards a tea.Msg to the viewport when in document mode.
func (p *PreviewPane) ViewportUpdate(msg tea.Msg) tea.Cmd {
	if !p.isDocument {
		return nil
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return cmd
}

// ViewportHandlesKey reports whether the viewport keymap matches the given key
// when the pane is in document mode.
func (p *PreviewPane) ViewportHandlesKey(msg tea.KeyMsg) bool {
	if !p.isDocument {
		return false
	}
	km := p.viewport.KeyMap
	return key.Matches(msg,
		km.Up, km.Down, km.Left, km.Right,
		km.PageUp, km.PageDown,
		km.HalfPageUp, km.HalfPageDown,
	)
}

// UpdateContent refreshes the pane based on the instance state.
// It is a no-op when in document mode.
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	if p.isDocument {
		return nil
	}
	if instance == nil {
		p.setFallbackState("create [n]ew plan or select existing")
		return nil
	}
	switch instance.Status {
	case session.Paused:
		p.setFallbackState("session paused")
	default:
		if instance.Exited {
			p.setFallbackState("session exited")
		}
	}
	return nil
}

// ScrollUp is a no-op stub (scroll capture removed; tmux pane handles scrolling).
func (p *PreviewPane) ScrollUp(_ *session.Instance) error { return nil }

// ScrollDown is a no-op stub.
func (p *PreviewPane) ScrollDown(_ *session.Instance) error { return nil }

// HalfPageUp scrolls the viewport up half a page when in document mode.
func (p *PreviewPane) HalfPageUp(_ *session.Instance) error {
	if p.isDocument {
		p.viewport.HalfPageUp()
	}
	return nil
}

// HalfPageDown scrolls the viewport down half a page when in document mode.
func (p *PreviewPane) HalfPageDown(_ *session.Instance) error {
	if p.isDocument {
		p.viewport.HalfPageDown()
	}
	return nil
}

// ResetToNormalMode is a no-op stub (scroll mode removed).
func (p *PreviewPane) ResetToNormalMode(_ *session.Instance) error { return nil }

// TickBanner is a no-op stub (banner animation removed).
func (p *PreviewPane) TickBanner() {}

// TickSpring is a no-op stub (spring animation removed).
func (p *PreviewPane) TickSpring() {}

// SetAnimateBanner is a no-op stub (banner animation removed).
func (p *PreviewPane) SetAnimateBanner(_ bool) {}

// String renders the preview pane to a string.
func (p *PreviewPane) String() string {
	if p.width == 0 || p.height == 0 {
		return strings.Repeat("\n", p.height)
	}
	if p.isDocument {
		return p.viewport.View()
	}
	// Fallback / placeholder: centered empty space.
	return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, "")
}

// ── tabBorderWithBottom ───────────────────────────────────────────────────────

// tabBorderWithBottom constructs a rounded lipgloss border where the bottom
// edge uses the three supplied characters (left corner, fill, right corner).
func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	b := lipgloss.RoundedBorder()
	b.BottomLeft = left
	b.Bottom = middle
	b.BottomRight = right
	return b
}

var (
	// inactiveTabBorder blends the inactive tab into the window border below it.
	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")

	// activeTabBorder lifts the active tab by making the bottom edge invisible.
	activeTabBorder = tabBorderWithBottom("┘", " ", "└")

	inactiveTabStyle = lipgloss.NewStyle().
				Border(inactiveTabBorder, true).
				BorderForeground(ColorIris).
				AlignHorizontal(lipgloss.Center)

	activeTabStyle = inactiveTabStyle.
			Border(activeTabBorder, true).
			AlignHorizontal(lipgloss.Center)

	windowBorder = lipgloss.RoundedBorder()

	// windowStyle draws the right, bottom, and left borders of the content area.
	// The top border is omitted because the tab row sits flush against it.
	windowStyle = lipgloss.NewStyle().
			BorderForeground(ColorIris).
			Border(windowBorder, false, true, true, true)
)

// Tab index constants.
const (
	InfoTab    int = iota // 0 — plan / metadata pane
	PreviewTab            // 1 — live agent session pane
)

// Tab describes a single tab entry (kept for API compatibility).
type Tab struct {
	Name   string
	Render func(width int, height int) string
}

// TabbedWindow composes two content panes (info, preview) behind a tab bar.
// It tracks which tab is active, manages focus state, and delegates rendering
// and scroll operations to the appropriate child pane.
type TabbedWindow struct {
	// tabs holds the icon+label string for each tab header.
	tabs []string

	activeTab  int // currently visible tab index
	focusedTab int // tab that has keyboard-ring focus (-1 = none)
	height     int // total allocated height (post AdjustPreviewWidth)
	width      int // total allocated width (post AdjustPreviewWidth)

	preview  *PreviewPane
	info     *InfoPane
	instance *session.Instance // last known selected instance

	focused   bool // true when this panel owns keyboard focus
	focusMode bool // true when user is typing directly into the agent pane

	// showWelcome is true on startup. While true the info tab shows the
	// animated banner instead of plan metadata so the logo animation plays.
	showWelcome bool
}

// NewTabbedWindow creates a TabbedWindow wiring the two child panes together.
// The info tab is active by default and the welcome banner is shown.
func NewTabbedWindow(preview *PreviewPane, info *InfoPane) *TabbedWindow {
	return &TabbedWindow{
		tabs: []string{
			"\uea74 info",
			"\uea85 agent",
		},
		// activeTab defaults to InfoTab (0) via zero value.
		preview:     preview,
		info:        info,
		focusedTab:  -1,
		showWelcome: true,
	}
}

// ── Focus helpers ─────────────────────────────────────────────────────────────

// SetFocusMode enables or disables insert / focus mode. When enabled the
// embedded terminal owns the pane and most updates become no-ops.
func (w *TabbedWindow) SetFocusMode(enabled bool) { w.focusMode = enabled }

// IsFocusMode reports whether the window is in focus/insert mode.
func (w *TabbedWindow) IsFocusMode() bool { return w.focusMode }

// SetFocused marks whether this panel currently holds keyboard focus.
func (w *TabbedWindow) SetFocused(focused bool) { w.focused = focused }

// SetFocusedTab sets which tab has keyboard-ring focus. Pass -1 to clear.
func (w *TabbedWindow) SetFocusedTab(tab int) { w.focusedTab = tab }

// SetInstance stores the currently selected session instance.
func (w *TabbedWindow) SetInstance(instance *session.Instance) { w.instance = instance }

// ── Layout helpers ────────────────────────────────────────────────────────────

// AdjustPreviewWidth returns the usable width after subtracting the margin
// needed for borders (width - 2).
func AdjustPreviewWidth(width int) int { return width - 2 }

// SetSize allocates space to the window and propagates the resulting content
// dimensions to all three child panes.
func (w *TabbedWindow) SetSize(width, height int) {
	w.width = AdjustPreviewWidth(width)
	w.height = height

	// Height consumed by the tab row (border frame + one character row).
	tabH := activeTabStyle.GetVerticalFrameSize() + 1
	// Remaining content dimensions after window border and tab row.
	contentH := height - tabH - windowStyle.GetVerticalFrameSize()
	contentW := w.width - windowStyle.GetHorizontalFrameSize()

	w.preview.SetSize(contentW, contentH)
	w.info.SetSize(contentW, contentH)
}

// GetPreviewSize returns the dimensions currently allocated to the preview pane.
func (w *TabbedWindow) GetPreviewSize() (int, int) {
	return w.preview.width, w.preview.height
}

// ── Tab navigation ────────────────────────────────────────────────────────────

// Toggle advances to the next tab, wrapping from agent back to info.
func (w *TabbedWindow) Toggle() {
	w.activeTab = (w.activeTab + 1) % len(w.tabs)
}

// ToggleWithReset resets the preview pane to normal mode first, then toggles.
func (w *TabbedWindow) ToggleWithReset(instance *session.Instance) error {
	if err := w.preview.ResetToNormalMode(instance); err != nil {
		return err
	}
	w.activeTab = (w.activeTab + 1) % len(w.tabs)
	return nil
}

// SetActiveTab selects the tab at the given index. Clears the welcome banner
// so that the info pane shows real plan content on the next render.
func (w *TabbedWindow) SetActiveTab(tab int) {
	if tab < 0 || tab >= len(w.tabs) {
		return
	}
	w.activeTab = tab
	w.focusedTab = tab
	w.showWelcome = false
}

// GetActiveTab returns the currently active tab index.
func (w *TabbedWindow) GetActiveTab() int { return w.activeTab }

// IsInInfoTab reports whether the info tab is currently active.
func (w *TabbedWindow) IsInInfoTab() bool { return w.activeTab == InfoTab }

// ── Preview pane delegation ───────────────────────────────────────────────────

// UpdatePreview refreshes the preview pane content from the given instance.
// No-op when focus mode is active (the embedded terminal owns the pane) or
// when the preview tab is not currently visible.
func (w *TabbedWindow) UpdatePreview(instance *session.Instance) error {
	if w.activeTab != PreviewTab || w.focusMode {
		return nil
	}
	return w.preview.UpdateContent(instance)
}

// SetPreviewContent is a no-op stub (VT emulator path removed; tmux pane owns content).
func (w *TabbedWindow) SetPreviewContent(_ string) {}

// SetConnectingState is a no-op stub (banner animation removed).
func (w *TabbedWindow) SetConnectingState() {}

// SetDocumentContent puts the preview pane into document mode, showing the
// supplied content (e.g. plan markdown) with scroll support.
func (w *TabbedWindow) SetDocumentContent(content string) {
	w.preview.SetDocumentContent(content)
}

// ClearDocumentMode exits document mode so UpdatePreview resumes normal behaviour.
func (w *TabbedWindow) ClearDocumentMode() { w.preview.ClearDocumentMode() }

// IsDocumentMode reports whether the preview pane is showing a static document.
func (w *TabbedWindow) IsDocumentMode() bool { return w.preview.IsDocumentMode() }

// ViewportUpdate forwards a tea.Msg to the preview viewport for native key
// handling (PgUp/PgDn, Home/End, etc.) when the preview tab is active.
func (w *TabbedWindow) ViewportUpdate(msg tea.Msg) tea.Cmd {
	if w.activeTab != PreviewTab {
		return nil
	}
	return w.preview.ViewportUpdate(msg)
}

// ViewportHandlesKey reports whether the preview viewport keymap handles msg.
func (w *TabbedWindow) ViewportHandlesKey(msg tea.KeyMsg) bool {
	if w.activeTab != PreviewTab {
		return false
	}
	return w.preview.ViewportHandlesKey(msg)
}

// ResetPreviewToNormalMode resets the preview pane to normal (live) mode.
func (w *TabbedWindow) ResetPreviewToNormalMode(instance *session.Instance) error {
	return w.preview.ResetToNormalMode(instance)
}

// IsPreviewInScrollMode reports whether the preview pane is in scroll mode.
// Always false in the stub (scroll capture removed).
func (w *TabbedWindow) IsPreviewInScrollMode() bool { return w.preview.isScrolling }

// ── Info pane delegation ──────────────────────────────────────────────────────

// SetInfoData updates the metadata shown in the info pane.
func (w *TabbedWindow) SetInfoData(data InfoData) { w.info.SetData(data) }

// GetInfoData returns the current InfoData held by the info pane.
func (w *TabbedWindow) GetInfoData() InfoData { return w.info.data }

// ── Scroll / pagination ───────────────────────────────────────────────────────

// ScrollUp scrolls the active pane upward.
func (w *TabbedWindow) ScrollUp() {
	switch w.activeTab {
	case PreviewTab:
		if err := w.preview.ScrollUp(w.instance); err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll up: %v", err)
		}
	case InfoTab:
		w.info.ScrollUp()
	}
}

// ScrollDown scrolls the active pane downward.
func (w *TabbedWindow) ScrollDown() {
	switch w.activeTab {
	case PreviewTab:
		if err := w.preview.ScrollDown(w.instance); err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll down: %v", err)
		}
	case InfoTab:
		w.info.ScrollDown()
	}
}

// HalfPageUp scrolls the preview pane up by half a page, regardless of which
// tab is active. Ctrl+U always targets the agent session output.
func (w *TabbedWindow) HalfPageUp() {
	if err := w.preview.HalfPageUp(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to half page up: %v", err)
	}
}

// HalfPageDown scrolls the preview pane down by half a page, regardless of
// which tab is active. Ctrl+D always targets the agent session output.
func (w *TabbedWindow) HalfPageDown() {
	if err := w.preview.HalfPageDown(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to half page down: %v", err)
	}
}

// ContentScrollUp scrolls the active pane upward without file navigation.
// Intended for mouse-wheel events.
func (w *TabbedWindow) ContentScrollUp() {
	switch w.activeTab {
	case PreviewTab:
		if err := w.preview.ScrollUp(w.instance); err != nil {
			log.InfoLog.Printf("tabbed window failed to content scroll up: %v", err)
		}
	case InfoTab:
		w.info.ScrollUp()
	}
}

// ContentScrollDown scrolls the active pane downward without file navigation.
// Intended for mouse-wheel events.
func (w *TabbedWindow) ContentScrollDown() {
	switch w.activeTab {
	case PreviewTab:
		if err := w.preview.ScrollDown(w.instance); err != nil {
			log.InfoLog.Printf("tabbed window failed to content scroll down: %v", err)
		}
	case InfoTab:
		w.info.ScrollDown()
	}
}

// ── Banner animation (no-ops) ─────────────────────────────────────────────────

// TickBanner is a no-op stub (banner animation removed).
func (w *TabbedWindow) TickBanner() {}

// TickSpring is a no-op stub (spring animation removed).
func (w *TabbedWindow) TickSpring() {}

// SetAnimateBanner is a no-op stub (banner animation removed).
func (w *TabbedWindow) SetAnimateBanner(_ bool) {}

// ── Rendering ─────────────────────────────────────────────────────────────────

// String renders the tab bar and content window as a single string.
// Returns an empty string when no size has been allocated.
func (w *TabbedWindow) String() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

	// ── Tab row ──────────────────────────────────────────────────────────────

	// Divide available width evenly across tabs; last tab absorbs the remainder.
	tabW := w.width / len(w.tabs)
	lastTabW := w.width - tabW*(len(w.tabs)-1)
	// Height consumed by the tab row (used later for content sizing).
	tabH := activeTabStyle.GetVerticalFrameSize() + 1

	// Choose border accent based on focus state.
	var borderColor color.Color
	switch {
	case w.focusMode:
		borderColor = ColorFoam
	case w.focused:
		borderColor = ColorIris
	default:
		borderColor = ColorOverlay
	}

	renderedTabs := make([]string, 0, len(w.tabs))
	for i, label := range w.tabs {
		width := tabW
		if i == len(w.tabs)-1 {
			width = lastTabW
		}

		isFirst := i == 0
		isLast := i == len(w.tabs)-1
		isActive := i == w.activeTab

		var style lipgloss.Style
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
		}
		style = style.BorderForeground(borderColor)

		// Adjust the bottom corner characters so the tab bar merges cleanly
		// with the window border below it.
		border, _, _, _, _ := style.GetBorder()
		switch {
		case isFirst && isActive:
			border.BottomLeft = "│"
		case isFirst:
			border.BottomLeft = "├"
		case isLast && isActive:
			border.BottomRight = "│"
		case isLast:
			border.BottomRight = "┤"
		}
		style = style.Border(border)
		// In lipgloss v2, Width is the total outer dimension (including border).
		style = style.Width(width)

		var cell string
		switch {
		case isActive && i == w.focusedTab && !w.focusMode:
			// Active tab with keyboard-ring focus: foam→iris gradient text.
			cell = style.Render(GradientText(label, GradientStart, GradientEnd))
		case isActive:
			// Active but not ring-focused: normal text color.
			cell = style.Render(lipgloss.NewStyle().Foreground(ColorText).Render(label))
		default:
			// Inactive tab: muted text.
			cell = style.Render(lipgloss.NewStyle().Foreground(ColorMuted).Render(label))
		}

		renderedTabs = append(renderedTabs, zone.Mark(TabZoneIDs[i], cell))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	// ── Content window ────────────────────────────────────────────────────────

	var content string
	switch {
	case w.activeTab == InfoTab && w.showWelcome:
		// Show the animated welcome banner on the info tab until the user
		// navigates in the sidebar for the first time.
		content = w.preview.String()
	case w.activeTab == PreviewTab:
		content = w.preview.String()
	case w.activeTab == InfoTab:
		content = w.info.String()
	}

	ws := windowStyle.BorderForeground(borderColor)
	innerW := w.width - ws.GetHorizontalFrameSize()
	innerH := w.height - ws.GetVerticalFrameSize() - tabH

	window := ws.Render(lipgloss.Place(innerW, innerH, lipgloss.Left, lipgloss.Top, content))

	// Wrap the preview content in a zone so mouse clicks are detected.
	if w.activeTab == PreviewTab {
		window = zone.Mark(ZoneAgentPane, window)
	}

	return lipgloss.JoinVertical(lipgloss.Left, row, window)
}
