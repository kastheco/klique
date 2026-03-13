package app

import (
	"context"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNavViewIncludedInView(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		toastManager: overlay.NewToastManager(&spin),
		overlays:     overlay.NewManager(),
	}

	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := h.View()
	// The nav-only layout renders successfully and produces non-empty content.
	assert.NotEmpty(t, view.Content)
}
