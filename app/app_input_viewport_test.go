package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/ui"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleKeyPress_DownKeyNavigatesNav(t *testing.T) {
	// Down always navigates the sidebar nav panel.
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		nav:       ui.NewNavigationPanel(&spin),
		menu:      ui.NewMenu(),
		keySent:   true,
	}

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyDown})
	_, ok := model.(*home)
	require.True(t, ok)
}

func TestHandleKeyPress_UpKeyNavigatesNav(t *testing.T) {
	// Up always navigates the sidebar nav panel.
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		nav:       ui.NewNavigationPanel(&spin),
		menu:      ui.NewMenu(),
		keySent:   true,
	}

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyUp})
	_, ok := model.(*home)
	require.True(t, ok)
}

func appTestDocumentLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&b, "line %d", i)
	}
	return b.String()
}

func TestHandleMouseWheel_IsNoOp(t *testing.T) {
	// With the tabbed window removed, mouse wheel events are no-ops.
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		nav:       ui.NewNavigationPanel(&spin),
		menu:      ui.NewMenu(),
		keySent:   true,
	}

	model, cmd := h.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, h, model)
	assert.Nil(t, cmd)
}
