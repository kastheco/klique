package ui

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	zone "github.com/lrstanley/bubblezone/v2"
)

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
	// Deprecated: InfoTab is kept as a compile-time shim until task 4 removes all
	// app-layer references. Internal use in TabbedWindow has been replaced with
	// dynamic instanceTabs indexing.
	InfoTab int = iota // 0 — legacy info tab index
	// Deprecated: PreviewTab is kept as a compile-time shim until task 4 removes
	// all app-layer references. Internal use in TabbedWindow has been replaced with
	// dynamic instanceTabs indexing.
	PreviewTab // 1 — legacy agent session tab index
)

// Tab describes a single tab entry (kept for API compatibility).
type Tab struct {
	Name   string
	Render func(width int, height int) string
}

// InstanceTab describes a single session tab in the dynamic tab bar.
type InstanceTab struct {
	Title string // short label shown in the tab header
	Key   string // stable lookup key; use instance title for now
}

// TabbedWindow composes two content panes (info, preview) behind a tab bar.
// It tracks which tab is active, manages focus state, and delegates rendering
// and scroll operations to the appropriate child pane.
type TabbedWindow struct {
	activeTab  int // currently visible tab index
	focusedTab int // tab that has keyboard-ring focus (-1 = none)
	height     int // total allocated height (post AdjustPreviewWidth)
	width      int // total allocated width (post AdjustPreviewWidth)

	preview  *PreviewPane
	info     *InfoPane
	instance *session.Instance // last known selected instance

	focused   bool // true when this panel owns keyboard focus
	focusMode bool // true when user is typing directly into the agent pane

	// showWelcome is true on startup. While true the preview pane shows the
	// animated banner until the user navigates for the first time.
	showWelcome bool

	// instanceTabs is the dynamic list of per-session tabs.
	instanceTabs []InstanceTab
	// showInfo controls whether the compact info summary is visible above the tab bar.
	showInfo bool
}

// NewTabbedWindow creates a TabbedWindow wiring the two child panes together.
// The welcome banner is shown on startup until the user navigates.
func NewTabbedWindow(preview *PreviewPane, info *InfoPane) *TabbedWindow {
	return &TabbedWindow{
		// activeTab defaults to 0 via zero value.
		preview:     preview,
		info:        info,
		focusedTab:  -1,
		showWelcome: true,
		showInfo:    true,
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

// compactInfo renders the compact info header and returns its content and
// rendered height. Returns ("", 0) when showInfo is false, width is
// non-positive, or the info pane has no compact content to show.
func (w *TabbedWindow) compactInfo(width int) (string, int) {
	if !w.showInfo || width <= 0 {
		return "", 0
	}
	compact := w.info.RenderCompact(width)
	if compact == "" {
		return "", 0
	}
	return compact, lipgloss.Height(compact)
}

// SetSize allocates space to the window and propagates the resulting content
// dimensions to the preview and info child panes.
func (w *TabbedWindow) SetSize(width, height int) {
	w.width = AdjustPreviewWidth(width)
	w.height = height

	// Height consumed by the compact info header (may be 0).
	_, compactH := w.compactInfo(w.width)

	// Height consumed by the tab row (0 when there are no instance tabs).
	tabH := 0
	if len(w.instanceTabs) > 0 {
		tabH = activeTabStyle.GetVerticalFrameSize() + 1
	}

	// Remaining content dimensions after compact header, tab row, and window border.
	contentH := height - compactH - tabH - windowStyle.GetVerticalFrameSize()
	if contentH < 0 {
		contentH = 0
	}
	contentW := w.width - windowStyle.GetHorizontalFrameSize()
	if contentW < 0 {
		contentW = 0
	}

	w.preview.SetSize(contentW, contentH)
	w.info.SetSize(contentW, contentH)
}

// GetPreviewSize returns the dimensions currently allocated to the preview pane.
func (w *TabbedWindow) GetPreviewSize() (int, int) {
	return w.preview.width, w.preview.height
}

// ── Tab navigation ────────────────────────────────────────────────────────────

// Toggle advances to the next dynamic instance tab, wrapping around at the end.
// No-op when there are no dynamic instance tabs.
func (w *TabbedWindow) Toggle() {
	if len(w.instanceTabs) == 0 {
		return
	}
	w.activeTab = (w.activeTab + 1) % len(w.instanceTabs)
}

// ToggleWithReset resets the preview pane to normal mode first, then advances
// to the next dynamic instance tab. No-op when there are no instance tabs.
func (w *TabbedWindow) ToggleWithReset(instance *session.Instance) error {
	if err := w.preview.ResetToNormalMode(instance); err != nil {
		return err
	}
	if len(w.instanceTabs) == 0 {
		return nil
	}
	w.activeTab = (w.activeTab + 1) % len(w.instanceTabs)
	return nil
}

// SetActiveTab selects the tab at the given index. Clears the welcome banner
// so that the preview pane shows live content on the next render.
// When instanceTabs is non-empty, the index must be in [0, len(instanceTabs)).
func (w *TabbedWindow) SetActiveTab(tab int) {
	if tab < 0 {
		return
	}
	if len(w.instanceTabs) > 0 && tab >= len(w.instanceTabs) {
		return
	}
	w.activeTab = tab
	w.focusedTab = tab
	w.showWelcome = false
}

// GetActiveTab returns the currently active tab index.
func (w *TabbedWindow) GetActiveTab() int { return w.activeTab }

// IsInInfoTab reports whether the active tab index matches the legacy InfoTab
// constant (0). Retained for compatibility until task 4 updates the app layer.
func (w *TabbedWindow) IsInInfoTab() bool { return w.activeTab == InfoTab }

// ── Dynamic instance tab API ──────────────────────────────────────────────────

// SetTabs replaces the dynamic instance tab list. The incoming slice is copied
// so callers cannot mutate it afterward. When the current activeTab index is
// out of range after the update, it is clamped to the last valid index (or 0
// when the new list is empty). Does not clear showWelcome; use SetActiveTab for that.
func (w *TabbedWindow) SetTabs(tabs []InstanceTab) {
	w.instanceTabs = append([]InstanceTab(nil), tabs...)
	if len(w.instanceTabs) == 0 {
		w.activeTab = 0
		w.focusedTab = -1
		return
	}
	if w.activeTab >= len(w.instanceTabs) {
		w.activeTab = len(w.instanceTabs) - 1
	}
	if w.focusedTab >= len(w.instanceTabs) {
		w.focusedTab = len(w.instanceTabs) - 1
	}
}

// TabCount returns the number of dynamic instance tabs.
func (w *TabbedWindow) TabCount() int { return len(w.instanceTabs) }

// ActiveTabKey returns the Key of the currently active dynamic tab, or ""
// when there are no dynamic tabs or the active index is out of range.
func (w *TabbedWindow) ActiveTabKey() string {
	if len(w.instanceTabs) == 0 || w.activeTab < 0 || w.activeTab >= len(w.instanceTabs) {
		return ""
	}
	return w.instanceTabs[w.activeTab].Key
}

// NextTab advances to the next dynamic instance tab, wrapping around at the end.
// No-op when there are no dynamic tabs.
func (w *TabbedWindow) NextTab() {
	if len(w.instanceTabs) == 0 {
		return
	}
	w.activeTab = (w.activeTab + 1) % len(w.instanceTabs)
}

// PrevTab moves to the previous dynamic instance tab, wrapping around at the start.
// No-op when there are no dynamic tabs.
func (w *TabbedWindow) PrevTab() {
	if len(w.instanceTabs) == 0 {
		return
	}
	w.activeTab = (w.activeTab - 1 + len(w.instanceTabs)) % len(w.instanceTabs)
}

// SetShowInfo enables or disables the compact info summary above the tab bar.
func (w *TabbedWindow) SetShowInfo(show bool) { w.showInfo = show }

// IsShowingInfo reports whether the compact info summary is currently visible.
func (w *TabbedWindow) IsShowingInfo() bool { return w.showInfo }

// ── Preview pane delegation ───────────────────────────────────────────────────

// UpdatePreview refreshes the preview pane content from the given instance.
// No-op when focus mode is active (the embedded terminal owns the pane).
func (w *TabbedWindow) UpdatePreview(instance *session.Instance) error {
	if w.focusMode {
		return nil
	}
	return w.preview.UpdateContent(instance)
}

// SetPreviewContent sets preview content directly from a pre-rendered string.
// Used by the embedded terminal in focus mode to bypass tmux capture-pane.
func (w *TabbedWindow) SetPreviewContent(content string) {
	w.preview.SetRawContent(content)
}

// SetConnectingState shows the animated banner with a "connecting…" message.
func (w *TabbedWindow) SetConnectingState() {
	w.preview.setFallbackState("connecting…")
}

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
// handling (PgUp/PgDn, Home/End, etc.) regardless of active tab.
func (w *TabbedWindow) ViewportUpdate(msg tea.Msg) tea.Cmd {
	return w.preview.ViewportUpdate(msg)
}

// ViewportHandlesKey reports whether the preview viewport keymap handles msg,
// regardless of active tab.
func (w *TabbedWindow) ViewportHandlesKey(msg tea.KeyMsg) bool {
	return w.preview.ViewportHandlesKey(msg)
}

// ResetPreviewToNormalMode resets the preview pane to normal (live) mode.
func (w *TabbedWindow) ResetPreviewToNormalMode(instance *session.Instance) error {
	return w.preview.ResetToNormalMode(instance)
}

// IsPreviewInScrollMode reports whether the preview pane is in scroll mode.
func (w *TabbedWindow) IsPreviewInScrollMode() bool { return w.preview.isScrolling }

// ── Info pane delegation ──────────────────────────────────────────────────────

// SetInfoData updates the metadata shown in the info pane.
func (w *TabbedWindow) SetInfoData(data InfoData) { w.info.SetData(data) }

// GetInfoData returns the current InfoData held by the info pane.
func (w *TabbedWindow) GetInfoData() InfoData { return w.info.data }

// ── Scroll / pagination ───────────────────────────────────────────────────────

// ScrollUp scrolls the preview pane upward, regardless of active tab.
func (w *TabbedWindow) ScrollUp() {
	if err := w.preview.ScrollUp(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to scroll up: %v", err)
	}
}

// ScrollDown scrolls the preview pane downward, regardless of active tab.
func (w *TabbedWindow) ScrollDown() {
	if err := w.preview.ScrollDown(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to scroll down: %v", err)
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

// ContentScrollUp scrolls the preview pane upward without file navigation,
// regardless of active tab. Intended for mouse-wheel events.
func (w *TabbedWindow) ContentScrollUp() {
	if err := w.preview.ScrollUp(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to content scroll up: %v", err)
	}
}

// ContentScrollDown scrolls the preview pane downward without file navigation,
// regardless of active tab. Intended for mouse-wheel events.
func (w *TabbedWindow) ContentScrollDown() {
	if err := w.preview.ScrollDown(w.instance); err != nil {
		log.InfoLog.Printf("tabbed window failed to content scroll down: %v", err)
	}
}

// ── Banner animation ──────────────────────────────────────────────────────────

// TickBanner advances the preview pane's banner animation by one frame.
func (w *TabbedWindow) TickBanner() { w.preview.TickBanner() }

// TickSpring advances the spring load-in animation on the preview pane.
func (w *TabbedWindow) TickSpring() { w.preview.TickSpring() }

// SetAnimateBanner enables or disables the idle banner animation.
func (w *TabbedWindow) SetAnimateBanner(enabled bool) { w.preview.SetAnimateBanner(enabled) }

// ── Rendering ─────────────────────────────────────────────────────────────────

// String renders the compact info header, tab bar, and content window as a
// single string. Returns an empty string when no size has been allocated.
func (w *TabbedWindow) String() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

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

	// ── Compact info header ───────────────────────────────────────────────────

	compact, compactH := w.compactInfo(w.width)

	// ── Tab row ───────────────────────────────────────────────────────────────

	var row string
	tabH := 0
	if len(w.instanceTabs) > 0 {
		// Height consumed by the tab row.
		tabH = activeTabStyle.GetVerticalFrameSize() + 1

		// Divide available width evenly across tabs; last tab absorbs the remainder.
		tabW := w.width / len(w.instanceTabs)
		lastTabW := w.width - tabW*(len(w.instanceTabs)-1)

		renderedTabs := make([]string, 0, len(w.instanceTabs))
		for i, tab := range w.instanceTabs {
			width := tabW
			if i == len(w.instanceTabs)-1 {
				width = lastTabW
			}

			isFirst := i == 0
			isLast := i == len(w.instanceTabs)-1
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

			label := tab.Title
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

			renderedTabs = append(renderedTabs, zone.Mark(InstanceTabZoneID(i), cell))
		}

		row = lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
	}

	// ── Content window ────────────────────────────────────────────────────────

	// Every tab renders preview content; the old info pane full-content path is
	// replaced by the compact header above the tab bar.
	content := w.preview.String()

	ws := windowStyle.BorderForeground(borderColor)
	innerW := w.width - ws.GetHorizontalFrameSize()
	innerH := w.height - ws.GetVerticalFrameSize() - tabH - compactH

	window := ws.Render(lipgloss.Place(innerW, innerH, lipgloss.Left, lipgloss.Top, content))
	// Wrap the preview content in a zone so mouse clicks are detected.
	window = zone.Mark(ZoneAgentPane, window)

	// ── Assemble ──────────────────────────────────────────────────────────────

	parts := make([]string, 0, 3)
	if compact != "" {
		parts = append(parts, compact)
	}
	if row != "" {
		parts = append(parts, row)
	}
	parts = append(parts, window)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
