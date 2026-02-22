package overlay

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ToastType identifies the kind of toast notification.
type ToastType int

const (
	ToastInfo ToastType = iota
	ToastSuccess
	ToastError
	ToastLoading
)

// AnimPhase represents the current animation phase of a toast.
type AnimPhase int

const (
	PhaseSlidingIn AnimPhase = iota
	PhaseVisible
	PhaseSlidingOut
	PhaseDone
)

// Animation and display constants.
const (
	SlideInDuration  = 300 * time.Millisecond
	SlideOutDuration = 200 * time.Millisecond

	InfoDismissAfter    = 3 * time.Second
	SuccessDismissAfter = 3 * time.Second
	ErrorDismissAfter   = 5 * time.Second

	MinToastWidth = 30
	MaxToastWidth = 60
	MaxToasts     = 5
)

// idCounter is a global atomic counter used to generate unique toast IDs.
var idCounter atomic.Uint64

// toast represents a single toast notification.
type toast struct {
	ID         string
	Type       ToastType
	Message    string
	CreatedAt  time.Time
	Phase      AnimPhase
	PhaseStart time.Time
	Duration   time.Duration // 0 means no auto-dismiss (e.g. loading toasts)
	Width      int           // computed width based on message content
}

// calcToastWidth computes the appropriate width for a toast based on its
// message content. The width includes border (2) + padding (2) + icon (1-2) + space (1).
func calcToastWidth(msg string) int {
	// icon (up to 2 cells) + space + message + padding (2) + border (2)
	contentWidth := 2 + 1 + runewidth.StringWidth(msg) + 4
	return clampInt(contentWidth, MinToastWidth, MaxToastWidth)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ToastManager manages the collection of active toast notifications.
type ToastManager struct {
	toasts  []*toast
	spinner *spinner.Model
	width   int
	height  int
}

// NewToastManager creates a new ToastManager with the given spinner model.
func NewToastManager(s *spinner.Model) *ToastManager {
	return &ToastManager{
		toasts:  make([]*toast, 0),
		spinner: s,
	}
}

// SetSize updates the available viewport dimensions for toast positioning.
func (tm *ToastManager) SetSize(width, height int) {
	tm.width = width
	tm.height = height
}

// Info creates an informational toast and returns its ID.
func (tm *ToastManager) Info(msg string) string {
	return tm.addToast(ToastInfo, msg, InfoDismissAfter)
}

// Success creates a success toast and returns its ID.
func (tm *ToastManager) Success(msg string) string {
	return tm.addToast(ToastSuccess, msg, SuccessDismissAfter)
}

// Error creates an error toast and returns its ID.
func (tm *ToastManager) Error(msg string) string {
	return tm.addToast(ToastError, msg, ErrorDismissAfter)
}

// Loading creates a loading toast with no auto-dismiss and returns its ID.
func (tm *ToastManager) Loading(msg string) string {
	return tm.addToast(ToastLoading, msg, 0)
}

// Resolve transitions an existing loading toast to a new type and message.
// If the given ID does not match any current toast, this is a no-op.
func (tm *ToastManager) Resolve(id string, typ ToastType, msg string) {
	for _, t := range tm.toasts {
		if t.ID == id {
			t.Type = typ
			t.Message = msg
			t.Width = calcToastWidth(msg)
			now := time.Now()
			t.Phase = PhaseVisible
			t.PhaseStart = now
			switch typ {
			case ToastSuccess:
				t.Duration = SuccessDismissAfter
			case ToastError:
				t.Duration = ErrorDismissAfter
			case ToastInfo:
				t.Duration = InfoDismissAfter
			default:
				t.Duration = SuccessDismissAfter
			}
			return
		}
	}
}

// HasActiveToasts returns true if there are any toasts that have not completed
// their animation cycle.
func (tm *ToastManager) HasActiveToasts() bool {
	for _, t := range tm.toasts {
		if t.Phase != PhaseDone {
			return true
		}
	}
	return false
}

// nextID generates a unique toast ID using an atomic counter.
func nextID() string {
	n := idCounter.Add(1)
	return fmt.Sprintf("toast-%d", n)
}

// addToast creates a new toast, enforces the MaxToasts cap, appends it, and
// returns the generated ID.
func (tm *ToastManager) addToast(typ ToastType, msg string, duration time.Duration) string {
	now := time.Now()

	// Deduplicate: if an identical toast (same type + message) already exists
	// and is still visible, reset its timer instead of creating a new one.
	for _, existing := range tm.toasts {
		if existing.Type == typ && existing.Message == msg && existing.Phase != PhaseDone && existing.Phase != PhaseSlidingOut {
			existing.PhaseStart = now
			if existing.Phase == PhaseSlidingIn {
				// Let it finish sliding in, duration starts after.
			} else {
				existing.Phase = PhaseVisible
			}
			return existing.ID
		}
	}

	t := &toast{
		ID:         nextID(),
		Type:       typ,
		Message:    msg,
		CreatedAt:  now,
		Phase:      PhaseSlidingIn,
		PhaseStart: now,
		Duration:   duration,
		Width:      calcToastWidth(msg),
	}

	tm.enforceMaxToasts()
	tm.toasts = append(tm.toasts, t)
	return t.ID
}

// ToastTickMsg is sent by the main app every ~50ms while toasts are active
// to drive animation phase transitions.
type ToastTickMsg struct{}

// Tick advances all toast animation phases based on elapsed time. Toasts that
// have completed their full animation cycle (PhaseDone) are removed from the
// manager's slice.
func (tm *ToastManager) Tick() {
	now := time.Now()
	alive := tm.toasts[:0]
	for _, t := range tm.toasts {
		elapsed := now.Sub(t.PhaseStart)
		switch t.Phase {
		case PhaseSlidingIn:
			if elapsed >= SlideInDuration {
				t.Phase = PhaseVisible
				t.PhaseStart = now
			}
		case PhaseVisible:
			if t.Duration > 0 && elapsed >= t.Duration {
				t.Phase = PhaseSlidingOut
				t.PhaseStart = now
			}
		case PhaseSlidingOut:
			if elapsed >= SlideOutDuration {
				t.Phase = PhaseDone
				continue // don't keep done toasts
			}
		case PhaseDone:
			continue // don't keep done toasts
		}
		alive = append(alive, t)
	}
	tm.toasts = alive
}

// enforceMaxToasts removes the oldest non-loading toasts when the toast count
// would exceed MaxToasts after adding a new one.
func (tm *ToastManager) enforceMaxToasts() {
	for len(tm.toasts) >= MaxToasts {
		removed := false
		// Remove oldest non-loading toast first.
		for i, t := range tm.toasts {
			if t.Type != ToastLoading {
				tm.toasts = append(tm.toasts[:i], tm.toasts[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			// All toasts are loading; remove the oldest one.
			tm.toasts = tm.toasts[1:]
		}
	}
}

// toastColor returns the Rosé Pine Moon palette color for a toast type.
func toastColor(typ ToastType) lipgloss.Color {
	switch typ {
	case ToastInfo:
		return colorFoam
	case ToastSuccess:
		return colorFoam
	case ToastError:
		return colorLove
	case ToastLoading:
		return colorGold
	default:
		// Defensive fallback for future ToastType values not yet handled;
		// renders as an info-style toast rather than breaking the UI.
		return colorFoam
	}
}

// toastStyle returns a lipgloss style for rendering a toast of the given type and width.
func toastStyle(typ ToastType, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(toastColor(typ)).
		Padding(0, 1).
		Width(width)
}

// toastIcon returns a styled icon string for the given toast type.
func (tm *ToastManager) toastIcon(typ ToastType) string {
	style := lipgloss.NewStyle().Foreground(toastColor(typ))
	switch typ {
	case ToastInfo:
		return style.Render("▸")
	case ToastSuccess:
		return style.Render("✓")
	case ToastError:
		return style.Render("✗")
	case ToastLoading:
		return style.Render(tm.spinner.View())
	default:
		return style.Render("▸")
	}
}

// slideOffset returns the horizontal offset for a toast's slide animation.
func (t *toast) slideOffset() int {
	fullOffset := t.Width + 4
	switch t.Phase {
	case PhaseSlidingIn:
		elapsed := time.Since(t.PhaseStart)
		progress := float64(elapsed) / float64(SlideInDuration)
		if progress > 1 {
			progress = 1
		}
		// Ease-out: progress = 1 - (1-progress)*(1-progress)
		progress = 1 - (1-progress)*(1-progress)
		return int(float64(fullOffset) * (1 - progress))
	case PhaseSlidingOut:
		elapsed := time.Since(t.PhaseStart)
		progress := float64(elapsed) / float64(SlideOutDuration)
		if progress > 1 {
			progress = 1
		}
		// Ease-in: progress = progress * progress
		progress = progress * progress
		return int(float64(fullOffset) * progress)
	default:
		return 0
	}
}

// renderToast renders a single toast notification as a styled string.
// Long messages wrap naturally within the toast width via lipgloss.
func (tm *ToastManager) renderToast(t *toast) string {
	icon := tm.toastIcon(t.Type)
	content := icon + " " + t.Message
	return toastStyle(t.Type, t.Width).Render(content)
}

// View renders all active toasts stacked vertically.
func (tm *ToastManager) View() string {
	if len(tm.toasts) == 0 {
		return ""
	}
	var rendered []string
	for _, t := range tm.toasts {
		if t.Phase == PhaseDone {
			continue
		}
		rendered = append(rendered, tm.renderToast(t))
	}
	if len(rendered) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Right, rendered...)
}

// GetPosition returns the x, y coordinates for placing the toast overlay.
func (tm *ToastManager) GetPosition() (int, int) {
	// Use the widest active toast for right-edge alignment.
	widest := MinToastWidth
	for _, t := range tm.toasts {
		if t.Phase != PhaseDone && t.Width > widest {
			widest = t.Width
		}
	}
	x := tm.width - widest - 4
	if x < 0 {
		x = 0
	}
	y := 1

	// Find the maximum slide offset among all animating toasts.
	maxOffset := 0
	for _, t := range tm.toasts {
		offset := t.slideOffset()
		if offset > maxOffset {
			maxOffset = offset
		}
	}
	x += maxOffset
	return x, y
}
