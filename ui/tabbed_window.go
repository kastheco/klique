package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone"
)

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

var (
	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")
	activeTabBorder   = tabBorderWithBottom("┘", " ", "└")
	inactiveTabStyle  = lipgloss.NewStyle().
				Border(inactiveTabBorder, true).
				BorderForeground(ColorIris).
				AlignHorizontal(lipgloss.Center)
	activeTabStyle = inactiveTabStyle.
			Border(activeTabBorder, true).
			AlignHorizontal(lipgloss.Center)
	windowBorder = lipgloss.RoundedBorder()
	windowStyle  = lipgloss.NewStyle().
			BorderForeground(ColorIris).
			Border(windowBorder, false, true, true, true)
)

const (
	InfoTab int = iota
	PreviewTab
	DiffTab
)

type Tab struct {
	Name   string
	Render func(width int, height int) string
}

// TabbedWindow has tabs at the top of a pane which can be selected. The tabs
// take up one rune of height.
type TabbedWindow struct {
	tabs []string

	activeTab  int
	focusedTab int // which specific tab (0=info, 1=agent, 2=diff) has Tab-ring focus; -1 = none
	height     int
	width      int

	preview   *PreviewPane
	diff      *DiffPane
	info      *InfoPane
	instance  *session.Instance
	focused   bool // true when this panel has keyboard focus (panel == 1)
	focusMode bool // true when user is typing directly into the agent pane
}

// SetFocusMode enables or disables the focus/insert mode visual indicator.
func (w *TabbedWindow) SetFocusMode(enabled bool) {
	w.focusMode = enabled
}

// IsFocusMode returns whether the window is in focus/insert mode.
func (w *TabbedWindow) IsFocusMode() bool {
	return w.focusMode
}

// SetFocused sets whether this panel has keyboard focus (panel == 1).
func (w *TabbedWindow) SetFocused(focused bool) {
	w.focused = focused
}

func NewTabbedWindow(preview *PreviewPane, diff *DiffPane, info *InfoPane) *TabbedWindow {
	return &TabbedWindow{
		tabs: []string{
			"\uea74 info",
			"\uea85 agent",
			"\ueae1 diff",
		},
		preview:    preview,
		diff:       diff,
		info:       info,
		focusedTab: -1,
	}
}

// SetFocusedTab sets which specific tab has focus ring focus. -1 = none.
func (w *TabbedWindow) SetFocusedTab(tab int) {
	w.focusedTab = tab
}

func (w *TabbedWindow) SetInstance(instance *session.Instance) {
	w.instance = instance
}

// AdjustPreviewWidth adjusts the width of the preview pane to be 90% of the provided width.
func AdjustPreviewWidth(width int) int {
	return width - 2 // just enough margin for borders
}

func (w *TabbedWindow) SetSize(width, height int) {
	w.width = AdjustPreviewWidth(width)
	w.height = height

	// Calculate the content height by subtracting the tab row and window border.
	tabHeight := activeTabStyle.GetVerticalFrameSize() + 1
	contentHeight := height - tabHeight - windowStyle.GetVerticalFrameSize()
	contentWidth := w.width - windowStyle.GetHorizontalFrameSize()

	w.preview.SetSize(contentWidth, contentHeight)
	w.diff.SetSize(contentWidth, contentHeight)
	w.info.SetSize(contentWidth, contentHeight)
}

func (w *TabbedWindow) GetPreviewSize() (width, height int) {
	return w.preview.width, w.preview.height
}

func (w *TabbedWindow) Toggle() {
	w.activeTab = (w.activeTab + 1) % len(w.tabs)
}

// ToggleWithReset toggles the tab and resets preview pane to normal mode
func (w *TabbedWindow) ToggleWithReset(instance *session.Instance) error {
	// Reset preview pane to normal mode before switching
	if err := w.preview.ResetToNormalMode(instance); err != nil {
		return err
	}
	w.activeTab = (w.activeTab + 1) % len(w.tabs)
	return nil
}

// UpdatePreview updates the content of the preview pane. instance may be nil.
// No-op when focusMode is true - the embedded terminal owns the pane.
func (w *TabbedWindow) UpdatePreview(instance *session.Instance) error {
	if w.activeTab != PreviewTab || w.focusMode {
		return nil
	}
	return w.preview.UpdateContent(instance)
}

// SetPreviewContent sets the preview pane content directly from a pre-rendered string.
// Used by the embedded terminal in focus mode to bypass tmux capture-pane.
func (w *TabbedWindow) SetPreviewContent(content string) {
	w.preview.SetRawContent(content)
}

// SetConnectingState shows the animated banner with a "connecting…" message.
// Called while the preview terminal is being attached to the selected instance.
func (w *TabbedWindow) SetConnectingState() {
	w.preview.setFallbackState("connecting…")
}

// SetDocumentContent sets the preview pane to show a rendered document (plan markdown etc.)
// with top-aligned display and scroll support.
func (w *TabbedWindow) SetDocumentContent(content string) {
	w.preview.SetDocumentContent(content)
}

// ClearDocumentMode exits document mode so UpdatePreview resumes normal behaviour.
func (w *TabbedWindow) ClearDocumentMode() {
	w.preview.ClearDocumentMode()
}

// IsDocumentMode returns true when the preview is showing a static document.
func (w *TabbedWindow) IsDocumentMode() bool {
	return w.preview.IsDocumentMode()
}

// ViewportUpdate forwards a tea.Msg to the preview viewport when in document
// or scroll mode, enabling native key handling (PgUp/PgDn, Home/End, etc.).
func (w *TabbedWindow) ViewportUpdate(msg tea.Msg) tea.Cmd {
	if w.activeTab != PreviewTab {
		return nil
	}
	return w.preview.ViewportUpdate(msg)
}

// ViewportHandlesKey reports whether the preview viewport keymap handles this key.
func (w *TabbedWindow) ViewportHandlesKey(msg tea.KeyMsg) bool {
	if w.activeTab != PreviewTab {
		return false
	}
	return w.preview.ViewportHandlesKey(msg)
}

func (w *TabbedWindow) UpdateDiff(instance *session.Instance) {
	if w.activeTab != DiffTab {
		return
	}
	w.diff.SetDiff(instance)
}

// SetInfoData updates the info pane data.
func (w *TabbedWindow) SetInfoData(data InfoData) {
	w.info.SetData(data)
}

// ResetPreviewToNormalMode resets the preview pane to normal mode
func (w *TabbedWindow) ResetPreviewToNormalMode(instance *session.Instance) error {
	return w.preview.ResetToNormalMode(instance)
}

// ScrollUp scrolls content. In preview tab, scrolls the preview. In diff tab,
// navigates to the previous file if files exist, otherwise scrolls.
// In info tab, scrolls the info pane.
func (w *TabbedWindow) ScrollUp() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollUp(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll up: %v", err)
		}
	case DiffTab:
		if w.diff.HasFiles() {
			w.diff.FileUp()
		} else {
			w.diff.ScrollUp()
		}
	case InfoTab:
		w.info.ScrollUp()
	}
}

// ScrollDown scrolls content. In preview tab, scrolls the preview. In diff tab,
// navigates to the next file if files exist, otherwise scrolls.
// In info tab, scrolls the info pane.
func (w *TabbedWindow) ScrollDown() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollDown(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll down: %v", err)
		}
	case DiffTab:
		if w.diff.HasFiles() {
			w.diff.FileDown()
		} else {
			w.diff.ScrollDown()
		}
	case InfoTab:
		w.info.ScrollDown()
	}
}

// HalfPageUp scrolls the preview pane up by half a page.
// Always targets the preview (agent session) regardless of which tab is active,
// since ctrl+u/d is the primary way to paginate agent output.
func (w *TabbedWindow) HalfPageUp() {
	if err := w.preview.HalfPageUp(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to half page up: %v", err)
	}
}

// HalfPageDown scrolls the preview pane down by half a page.
// Always targets the preview (agent session) regardless of which tab is active,
// since ctrl+u/d is the primary way to paginate agent output.
func (w *TabbedWindow) HalfPageDown() {
	if err := w.preview.HalfPageDown(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to half page down: %v", err)
	}
}

// ContentScrollUp scrolls content without file navigation (for mouse wheel).
// In info tab, scrolls the info pane.
func (w *TabbedWindow) ContentScrollUp() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollUp(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll up: %v", err)
		}
	case DiffTab:
		w.diff.ScrollUp()
	case InfoTab:
		w.info.ScrollUp()
	}
}

// ContentScrollDown scrolls content without file navigation (for mouse wheel).
// In info tab, scrolls the info pane.
func (w *TabbedWindow) ContentScrollDown() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollDown(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll down: %v", err)
		}
	case DiffTab:
		w.diff.ScrollDown()
	case InfoTab:
		w.info.ScrollDown()
	}
}

// IsInDiffTab returns true if the diff tab is currently active
func (w *TabbedWindow) IsInDiffTab() bool {
	return w.activeTab == DiffTab
}

// IsInInfoTab returns true if the info tab is currently active
func (w *TabbedWindow) IsInInfoTab() bool {
	return w.activeTab == InfoTab
}

// SetActiveTab sets the active tab by index.
func (w *TabbedWindow) SetActiveTab(tab int) {
	if tab >= 0 && tab < len(w.tabs) {
		w.activeTab = tab
	}
}

// GetActiveTab returns the currently active tab index.
func (w *TabbedWindow) GetActiveTab() int {
	return w.activeTab
}

// TickBanner advances the preview pane's banner animation frame.
func (w *TabbedWindow) TickBanner() {
	w.preview.TickBanner()
}

// TickSpring advances the preview pane's spring load-in animation.
func (w *TabbedWindow) TickSpring() {
	w.preview.TickSpring()
}

// SetAnimateBanner enables or disables the idle banner animation on the preview pane.
func (w *TabbedWindow) SetAnimateBanner(enabled bool) {
	w.preview.SetAnimateBanner(enabled)
}

// IsPreviewInScrollMode returns true if the preview pane is in scroll mode
func (w *TabbedWindow) IsPreviewInScrollMode() bool {
	return w.preview.isScrolling
}

func (w *TabbedWindow) String() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

	var renderedTabs []string

	tabWidth := w.width / len(w.tabs)
	lastTabWidth := w.width - tabWidth*(len(w.tabs)-1)
	tabHeight := activeTabStyle.GetVerticalFrameSize() + 1 // get padding border margin size + 1 for character height

	// Determine tab/window border color based on focus state.
	var borderColor lipgloss.TerminalColor
	switch {
	case w.focusMode:
		borderColor = ColorFoam
	case w.focused:
		borderColor = ColorIris
	default:
		borderColor = ColorOverlay
	}
	for i, t := range w.tabs {
		width := tabWidth
		if i == len(w.tabs)-1 {
			width = lastTabWidth
		}

		var style lipgloss.Style
		isFirst, isLast, isActive := i == 0, i == len(w.tabs)-1, i == w.activeTab
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
		}
		style = style.BorderForeground(borderColor)
		border, _, _, _, _ := style.GetBorder()
		if isFirst && isActive {
			border.BottomLeft = "│"
		} else if isFirst {
			border.BottomLeft = "├"
		} else if isLast && isActive {
			border.BottomRight = "│"
		} else if isLast {
			border.BottomRight = "┤"
		}
		style = style.Border(border)
		style = style.Width(width - style.GetHorizontalFrameSize())
		var rendered string
		switch {
		case isActive && i == w.focusedTab && !w.focusMode:
			// Focused tab in the ring: foam→iris gradient
			rendered = style.Render(GradientText(t, GradientStart, GradientEnd))
		case isActive:
			// Active but not ring-focused: normal text color
			rendered = style.Render(lipgloss.NewStyle().Foreground(ColorText).Render(t))
		default:
			// Inactive tab: muted
			rendered = style.Render(lipgloss.NewStyle().Foreground(ColorMuted).Render(t))
		}
		renderedTabs = append(renderedTabs, zone.Mark(TabZoneIDs[i], rendered))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
	var content string
	switch w.activeTab {
	case PreviewTab:
		content = w.preview.String()
	case DiffTab:
		content = w.diff.String()
	case InfoTab:
		content = w.info.String()
	}
	ws := windowStyle.BorderForeground(borderColor)
	// Subtract the window border width so the total rendered width
	// (content + borders) matches the tab row width.
	innerWidth := w.width - ws.GetHorizontalFrameSize()
	window := ws.Render(
		lipgloss.Place(
			innerWidth, w.height-ws.GetVerticalFrameSize()-tabHeight,
			lipgloss.Left, lipgloss.Top, content))

	if w.activeTab == PreviewTab {
		window = zone.Mark(ZoneAgentPane, window)
	}

	return lipgloss.JoinVertical(lipgloss.Left, row, window)
}
