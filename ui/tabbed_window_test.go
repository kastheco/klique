package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestUpdatePreview_SkipsWhenFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	diff := NewDiffPane()
	git := NewGitPane()
	tw := NewTabbedWindow(preview, diff, git)
	tw.SetActiveTab(PreviewTab)

	// Set focus mode - simulates embedded terminal owning the pane.
	tw.SetFocusMode(true)

	// Attempt to update preview with nil instance.
	// Without the guard this would overwrite the preview state.
	// With the guard it should be a no-op (return nil, state unchanged).
	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// Preview should still have its default zero state, not the fallback
	// banner that UpdateContent(nil) would set.
	assert.False(t, preview.previewState.fallback,
		"UpdatePreview should be a no-op when focusMode is true")
}

func TestUpdatePreview_WorksWhenNotFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	diff := NewDiffPane()
	git := NewGitPane()
	tw := NewTabbedWindow(preview, diff, git)
	tw.SetActiveTab(PreviewTab)

	// Focus mode OFF - normal path.
	tw.SetFocusMode(false)

	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// With nil instance, UpdateContent sets fallback state.
	assert.True(t, preview.previewState.fallback,
		"UpdatePreview should update content when focusMode is false")
}

func TestViewportUpdate_DelegatesOnlyForPreviewTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(30, 5)
	preview.SetDocumentContent(testDocumentLines(40))
	diff := NewDiffPane()
	git := NewGitPane()
	tw := NewTabbedWindow(preview, diff, git)

	before := preview.viewport.View()

	tw.SetActiveTab(PreviewTab)
	cmd := tw.ViewportUpdate(tea.KeyMsg{Type: tea.KeyPgDown})
	afterPreview := preview.viewport.View()
	assert.Nil(t, cmd)
	assert.NotEqual(t, before, afterPreview)

	tw.SetActiveTab(DiffTab)
	beforeDiff := preview.viewport.View()
	cmd = tw.ViewportUpdate(tea.KeyMsg{Type: tea.KeyPgDown})
	afterDiff := preview.viewport.View()
	assert.Nil(t, cmd)
	assert.Equal(t, beforeDiff, afterDiff)
}
