package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/klique/log"
	"github.com/kastheco/klique/session"
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
	PreviewTab int = iota
	DiffTab
	GitTab
)

type Tab struct {
	Name   string
	Render func(width int, height int) string
}

// TabbedWindow has tabs at the top of a pane which can be selected. The tabs
// take up one rune of height.
type TabbedWindow struct {
	tabs []string

	activeTab int
	height    int
	width     int

	preview    *PreviewPane
	diff       *DiffPane
	git        *GitPane
	instance   *session.Instance
	focused    bool   // true when this panel has keyboard focus (panel == 1)
	focusMode  bool   // true when user is typing directly into the agent pane
	gitContent string // cached git pane content, set by tick when changed
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

func NewTabbedWindow(preview *PreviewPane, diff *DiffPane, git *GitPane) *TabbedWindow {
	return &TabbedWindow{
		tabs: []string{
			"\uea85 Agent",
			"\ueae1 Diff",
			"\ue725 Git",
		},
		preview: preview,
		diff:    diff,
		git:     git,
	}
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

	// Calculate the content height by subtracting:
	// 1. Tab height (including border and padding)
	// 2. Window style vertical frame size
	// 3. Additional padding/spacing (2 for the newline and spacing)
	tabHeight := activeTabStyle.GetVerticalFrameSize() + 1
	contentHeight := height - tabHeight - windowStyle.GetVerticalFrameSize() - 2
	contentWidth := w.width - windowStyle.GetHorizontalFrameSize()

	w.preview.SetSize(contentWidth, contentHeight)
	w.diff.SetSize(contentWidth, contentHeight)
	w.git.SetSize(contentWidth, contentHeight)
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

// SetGitContent caches the git pane content to avoid re-rendering when unchanged.
func (w *TabbedWindow) SetGitContent(content string) {
	w.gitContent = content
}

func (w *TabbedWindow) UpdateDiff(instance *session.Instance) {
	if w.activeTab != DiffTab {
		return
	}
	w.diff.SetDiff(instance)
}

// ResetPreviewToNormalMode resets the preview pane to normal mode
func (w *TabbedWindow) ResetPreviewToNormalMode(instance *session.Instance) error {
	return w.preview.ResetToNormalMode(instance)
}

// ScrollUp scrolls content. In preview tab, scrolls the preview. In diff tab,
// navigates to the previous file if files exist, otherwise scrolls.
// No-op for git tab (lazygit handles its own scrolling).
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
	}
}

// ScrollDown scrolls content. In preview tab, scrolls the preview. In diff tab,
// navigates to the next file if files exist, otherwise scrolls.
// No-op for git tab (lazygit handles its own scrolling).
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
	}
}

// ContentScrollUp scrolls content without file navigation (for mouse wheel).
// No-op for git tab.
func (w *TabbedWindow) ContentScrollUp() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollUp(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll up: %v", err)
		}
	case DiffTab:
		w.diff.ScrollUp()
	}
}

// ContentScrollDown scrolls content without file navigation (for mouse wheel).
// No-op for git tab.
func (w *TabbedWindow) ContentScrollDown() {
	switch w.activeTab {
	case PreviewTab:
		err := w.preview.ScrollDown(w.instance)
		if err != nil {
			log.InfoLog.Printf("tabbed window failed to scroll down: %v", err)
		}
	case DiffTab:
		w.diff.ScrollDown()
	}
}

// IsInDiffTab returns true if the diff tab is currently active
func (w *TabbedWindow) IsInDiffTab() bool {
	return w.activeTab == DiffTab
}

// IsInGitTab returns true if the git tab is currently active
func (w *TabbedWindow) IsInGitTab() bool {
	return w.activeTab == GitTab
}

// GetGitPane returns the git pane for external control.
func (w *TabbedWindow) GetGitPane() *GitPane {
	return w.git
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

// SetAnimateBanner enables or disables the idle banner animation on the preview pane.
func (w *TabbedWindow) SetAnimateBanner(enabled bool) {
	w.preview.SetAnimateBanner(enabled)
}

// IsPreviewInScrollMode returns true if the preview pane is in scroll mode
func (w *TabbedWindow) IsPreviewInScrollMode() bool {
	return w.preview.isScrolling
}

// HandleTabClick checks if a click at the given local coordinates (relative to
// the tabbed window's top-left) hits a tab header. Returns true and switches
// tabs if a tab was clicked.
func (w *TabbedWindow) HandleTabClick(localX, localY int) bool {
	// Tab row starts at row 0 in String().
	// Accept rows 0-2 to generously cover the tab area with borders.
	if localY < 0 || localY > 2 {
		return false
	}

	tabWidth := w.width / len(w.tabs)
	clickedTab := localX / tabWidth
	if clickedTab >= len(w.tabs) {
		clickedTab = len(w.tabs) - 1
	}
	if clickedTab < 0 {
		return false
	}

	if clickedTab != w.activeTab {
		w.activeTab = clickedTab
	}
	return true
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
		if isActive && !w.focusMode {
			renderedTabs = append(renderedTabs, style.Render(GradientText(t, GradientStart, GradientEnd)))
		} else {
			renderedTabs = append(renderedTabs, style.Render(t))
		}
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
	var content string
	switch w.activeTab {
	case PreviewTab:
		content = w.preview.String()
	case DiffTab:
		content = w.diff.String()
	case GitTab:
		if w.gitContent != "" {
			content = w.gitContent
		} else {
			content = w.git.String()
		}
	}
	ws := windowStyle.BorderForeground(borderColor)
	// Subtract the window border width so the total rendered width
	// (content + borders) matches the tab row width.
	innerWidth := w.width - ws.GetHorizontalFrameSize()
	window := ws.Render(
		lipgloss.Place(
			innerWidth, w.height-2-ws.GetVerticalFrameSize()-tabHeight,
			lipgloss.Left, lipgloss.Top, content))

	return lipgloss.JoinVertical(lipgloss.Left, row, window)
}
