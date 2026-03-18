package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestUpdatePreview_SkipsWhenFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, info)
	tw.SetActiveTab(PreviewTab)

	// Clear the initial welcome fallback state so we can verify the no-op.
	preview.previewState = previewState{}

	// Set focus mode - simulates embedded terminal owning the pane.
	tw.SetFocusMode(true)

	// Attempt to update preview with nil instance.
	// Without the guard this would overwrite the preview state.
	// With the guard it should be a no-op (return nil, state unchanged).
	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// Preview should still have the cleared state, not the fallback
	// banner that UpdateContent(nil) would set.
	assert.False(t, preview.previewState.fallback,
		"UpdatePreview should be a no-op when focusMode is true")
}

func TestUpdatePreview_WorksWhenNotFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, info)
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
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, info)

	// Default activeTab is InfoTab (0), NOT PreviewTab.
	// Ctrl+U/D should still scroll the preview pane.
	assert.Equal(t, InfoTab, tw.GetActiveTab())

	initialOffset := preview.viewport.YOffset()
	tw.HalfPageUp() // should still scroll even though activeTab != PreviewTab
	// In document mode, HalfPageUp calls viewport.HalfViewUp which decreases YOffset
	// Since we're at the top, this won't change. Let's scroll down first then up.
	tw.HalfPageDown()
	afterDown := preview.viewport.YOffset()
	assert.Greater(t, afterDown, initialOffset,
		"HalfPageDown should scroll the preview even when activeTab is InfoTab")

	tw.HalfPageUp()
	afterUp := preview.viewport.YOffset()
	assert.Less(t, afterUp, afterDown,
		"HalfPageUp should scroll the preview even when activeTab is InfoTab")
}

func TestHalfPageDown_WorksRegardlessOfActiveTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	preview.SetDocumentContent(testDocumentLines(100))
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, info)

	// Set to InfoTab — ctrl+u/d should still scroll the preview.
	tw.SetActiveTab(InfoTab)

	initialOffset := preview.viewport.YOffset()
	tw.HalfPageDown()
	afterDown := preview.viewport.YOffset()
	assert.Greater(t, afterDown, initialOffset,
		"HalfPageDown should scroll the preview even when activeTab is InfoTab")
}

func TestViewportUpdate_DelegatesOnlyForPreviewTab(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(30, 5)
	preview.SetDocumentContent(testDocumentLines(40))
	info := NewInfoPane()
	tw := NewTabbedWindow(preview, info)

	before := preview.viewport.View()

	tw.SetActiveTab(PreviewTab)
	cmd := tw.ViewportUpdate(tea.KeyPressMsg{Code: tea.KeyPgDown})
	afterPreview := preview.viewport.View()
	assert.Nil(t, cmd)
	assert.NotEqual(t, before, afterPreview)

	tw.SetActiveTab(InfoTab)
	beforeInfo := preview.viewport.View()
	cmd = tw.ViewportUpdate(tea.KeyPressMsg{Code: tea.KeyPgDown})
	afterInfo := preview.viewport.View()
	assert.Nil(t, cmd)
	assert.Equal(t, beforeInfo, afterInfo)
}

func TestSetTabs_PopulatesAndClampsActiveTab(t *testing.T) {
	tw := NewTabbedWindow(NewPreviewPane(), NewInfoPane())
	tabs := []InstanceTab{
		{Title: "planner", Key: "plan-planner"},
		{Title: "coder-1", Key: "plan-coder-1"},
	}
	tw.SetTabs(tabs)
	assert.Equal(t, 2, tw.TabCount())
	assert.Equal(t, "plan-planner", tw.ActiveTabKey())

	// Setting fewer tabs clamps activeTab
	tw.SetActiveTab(1)   // select coder-1
	tw.SetTabs(tabs[:1]) // only planner remains
	assert.Equal(t, 0, tw.GetActiveTab())
	assert.Equal(t, "plan-planner", tw.ActiveTabKey())
}

func TestNextPrevTab_CyclesWithWrapping(t *testing.T) {
	tw := NewTabbedWindow(NewPreviewPane(), NewInfoPane())
	tw.SetTabs([]InstanceTab{
		{Title: "a", Key: "inst-a"},
		{Title: "b", Key: "inst-b"},
		{Title: "c", Key: "inst-c"},
	})
	assert.Equal(t, "inst-a", tw.ActiveTabKey())

	tw.NextTab()
	assert.Equal(t, "inst-b", tw.ActiveTabKey())
	tw.NextTab()
	assert.Equal(t, "inst-c", tw.ActiveTabKey())
	tw.NextTab() // wraps
	assert.Equal(t, "inst-a", tw.ActiveTabKey())

	tw.PrevTab() // wraps backward
	assert.Equal(t, "inst-c", tw.ActiveTabKey())
	tw.PrevTab()
	assert.Equal(t, "inst-b", tw.ActiveTabKey())
}

func TestActiveTabKey_EmptyTabs(t *testing.T) {
	tw := NewTabbedWindow(NewPreviewPane(), NewInfoPane())
	assert.Equal(t, "", tw.ActiveTabKey())
	assert.Equal(t, 0, tw.TabCount())
}
