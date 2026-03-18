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

func TestHandleKeyPress_DownKeyAlwaysFocusesNav(t *testing.T) {
	// Up/Down always refocus the sidebar and navigate it, regardless of which
	// pane was previously focused (when no document/scroll mode is active).
	for _, slot := range []int{slotAgent} {
		t.Run(fmt.Sprintf("from slot %d", slot), func(t *testing.T) {
			spin := spinner.New(spinner.WithSpinner(spinner.Dot))
			h := &home{
				ctx:          context.Background(),
				state:        stateDefault,
				appConfig:    config.DefaultConfig(),
				nav:          ui.NewNavigationPanel(&spin),
				menu:         ui.NewMenu(),
				tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
				focusSlot:    slot,
				keySent:      true,
			}

			model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyDown})
			homeModel, ok := model.(*home)
			require.True(t, ok)
			assert.Equal(t, slotNav, homeModel.focusSlot, "Down must focus nav")
		})
	}
}

func TestHandleKeyPress_UpKeyAlwaysFocusesNav(t *testing.T) {
	for _, slot := range []int{slotAgent} {
		t.Run(fmt.Sprintf("from slot %d", slot), func(t *testing.T) {
			spin := spinner.New(spinner.WithSpinner(spinner.Dot))
			h := &home{
				ctx:          context.Background(),
				state:        stateDefault,
				appConfig:    config.DefaultConfig(),
				nav:          ui.NewNavigationPanel(&spin),
				menu:         ui.NewMenu(),
				tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
				focusSlot:    slot,
				keySent:      true,
			}

			model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyUp})
			homeModel, ok := model.(*home)
			require.True(t, ok)
			assert.Equal(t, slotNav, homeModel.focusSlot, "Up must focus nav")
		})
	}
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

func TestHandleMouseWheel_DocumentModeScrollsWithoutSelectedInstance(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		keySent:      true,
	}

	h.tabbedWindow.SetSize(100, 16)
	h.tabbedWindow.SetActiveTab(ui.PreviewTab)
	h.tabbedWindow.SetDocumentContent(appTestDocumentLines(120))

	before := h.tabbedWindow.String()

	model, cmd := h.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Equal(t, h, model)
	assert.Nil(t, cmd)

	after := h.tabbedWindow.String()
	assert.NotEqual(t, before, after, "mouse wheel should scroll plan document in preview tab")
}
