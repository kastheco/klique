package app

import (
	"context"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestStatusBarIncludedInView(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager: overlay.NewToastManager(&spin),
		overlays:     overlay.NewManager(),
		statusBar:    ui.NewStatusBar(),
	}

	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := h.View()
	firstLine := strings.SplitN(view.Content, "\n", 2)[0]
	// App name is gradient-rendered (per-char ANSI), so check individual chars.
	for _, c := range "kasmos" {
		assert.Contains(t, firstLine, string(c))
	}
}
