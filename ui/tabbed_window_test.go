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
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, diff, info)
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
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, diff, info)
	tw.SetActiveTab(PreviewTab)

	// Focus mode OFF - normal path.
	tw.SetFocusMode(false)

	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// With nil instance, UpdateContent sets fallback state.
	assert.True(t, preview.previewState.fallback,
		"UpdatePreview should update content when focusMode is false")
}

func TestHalfPageUp_WorksRegardlessOfActiveTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	preview.SetDocumentContent(testDocumentLines(100))
	diff := NewDiffPane()
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, diff, info)

	// Default activeTab is InfoTab (0), NOT PreviewTab.
	// Ctrl+U/D should still scroll the preview pane.
	assert.Equal(t, InfoTab, tw.GetActiveTab())

	initialOffset := preview.viewport.YOffset
	tw.HalfPageUp() // should still scroll even though activeTab != PreviewTab
	// In document mode, HalfPageUp calls viewport.HalfViewUp which decreases YOffset
	// Since we're at the top, this won't change. Let's scroll down first then up.
	tw.HalfPageDown()
	afterDown := preview.viewport.YOffset
	assert.Greater(t, afterDown, initialOffset,
		"HalfPageDown should scroll the preview even when activeTab is InfoTab")

	tw.HalfPageUp()
	afterUp := preview.viewport.YOffset
	assert.Less(t, afterUp, afterDown,
		"HalfPageUp should scroll the preview even when activeTab is InfoTab")
}

func TestHalfPageDown_WorksRegardlessOfActiveTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	preview.SetDocumentContent(testDocumentLines(100))
	diff := NewDiffPane()
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, diff, info)

	// Set to DiffTab — ctrl+u/d should still scroll the preview.
	tw.SetActiveTab(DiffTab)

	initialOffset := preview.viewport.YOffset
	tw.HalfPageDown()
	afterDown := preview.viewport.YOffset
	assert.Greater(t, afterDown, initialOffset,
		"HalfPageDown should scroll the preview even when activeTab is DiffTab")
}

func TestViewportUpdate_DelegatesOnlyForPreviewTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(30, 5)
	preview.SetDocumentContent(testDocumentLines(40))
	diff := NewDiffPane()
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, diff, info)

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
