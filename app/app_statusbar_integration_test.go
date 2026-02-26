package app

import (
	"context"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager: overlay.NewToastManager(&spin),
		statusBar:    ui.NewStatusBar(),
	}

	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := h.View()
	firstLine := strings.SplitN(view, "\n", 2)[0]
	// App name is gradient-rendered (per-char ANSI), so check individual chars.
	for _, c := range "kasmos" {
		assert.Contains(t, firstLine, string(c))
	}
}
