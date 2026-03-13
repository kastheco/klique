package app

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
)

func TestHandleMouseClick_HelpOverlay_OutsideClickTriggersOnDismiss(t *testing.T) {
	dismissed := false
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))

	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		nav:       ui.NewNavigationPanel(&spin),
		menu:      ui.NewMenu(),
		overlays:  overlay.NewManager(),
		appState:  &mockAppState{},
	}

	to := overlay.NewTextOverlay("help")
	to.OnDismiss = func() { dismissed = true }
	h.overlays.ShowPositioned(to, 5, 5, false)
	h.state = stateHelp

	_, _ = h.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})

	assert.True(t, dismissed)
	assert.Equal(t, stateDefault, h.state)
	assert.False(t, h.overlays.IsActive())
}
