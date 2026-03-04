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
	for _, slot := range []int{slotInfo, slotAgent, slotDiff} {
		t.Run(fmt.Sprintf("from slot %d", slot), func(t *testing.T) {
			spin := spinner.New(spinner.WithSpinner(spinner.Dot))
			h := &home{
				ctx:          context.Background(),
				state:        stateDefault,
				appConfig:    config.DefaultConfig(),
				nav:          ui.NewNavigationPanel(&spin),
				menu:         ui.NewMenu(),
				tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
				focusSlot:    slot,
				keySent:      true,
			}

			model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
			homeModel, ok := model.(*home)
			require.True(t, ok)
			assert.Equal(t, slotNav, homeModel.focusSlot, "Down must focus nav")
		})
	}
}

func TestHandleKeyPress_UpKeyAlwaysFocusesNav(t *testing.T) {
	for _, slot := range []int{slotInfo, slotAgent, slotDiff} {
		t.Run(fmt.Sprintf("from slot %d", slot), func(t *testing.T) {
			spin := spinner.New(spinner.WithSpinner(spinner.Dot))
			h := &home{
				ctx:          context.Background(),
				state:        stateDefault,
				appConfig:    config.DefaultConfig(),
				nav:          ui.NewNavigationPanel(&spin),
				menu:         ui.NewMenu(),
				tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
				focusSlot:    slot,
				keySent:      true,
			}

			model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyUp})
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
