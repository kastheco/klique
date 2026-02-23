package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestHandleKeyPress_DocumentModeConsumesViewportNavigationKeys(t *testing.T) {
	preview := ui.NewPreviewPane()
	preview.SetSize(30, 5)
	preview.SetDocumentContent(appTestDocumentLines(50))

	tw := ui.NewTabbedWindow(preview, ui.NewDiffPane(), ui.NewGitPane())
	tw.SetActiveTab(ui.PreviewTab)

	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: tw,
		keySent:      true,
	}

	before := preview.String()
	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	homeModel, ok := model.(*home)
	require.True(t, ok)
	require.Nil(t, cmd)
	require.True(t, homeModel.tabbedWindow.IsDocumentMode())
	require.NotEqual(t, before, preview.String())
}

func TestHandleKeyPress_DocumentModeScrollsWithDownKey(t *testing.T) {
	// In the tab focus ring model, scrolling in document mode uses up/down
	// when the agent slot (slotAgent) is focused. Shift+Down is no longer a
	// dedicated scroll binding — Tab-focus + up/down replaces it.
	preview := ui.NewPreviewPane()
	preview.SetSize(30, 5)
	preview.SetDocumentContent(appTestDocumentLines(50))

	tw := ui.NewTabbedWindow(preview, ui.NewDiffPane(), ui.NewGitPane())
	tw.SetActiveTab(ui.PreviewTab)

	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: tw,
		focusSlot:    slotAgent, // agent slot focused → down scrolls preview
		keySent:      true,
	}

	before := preview.String()
	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	homeModel, ok := model.(*home)
	require.True(t, ok)
	require.True(t, homeModel.tabbedWindow.IsDocumentMode())
	require.NotEqual(t, before, preview.String())
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
