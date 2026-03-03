package overlay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStyles_ModalBorder(t *testing.T) {
	s := DefaultStyles()
	// Modal border should use the iris color
	rendered := s.ModalBorder.Render("content")
	assert.NotEmpty(t, rendered)
}

func TestStyles_FloatingBorder(t *testing.T) {
	s := DefaultStyles()
	rendered := s.FloatingBorder.Render("content")
	assert.NotEmpty(t, rendered)
}

func TestStyles_Title(t *testing.T) {
	s := DefaultStyles()
	rendered := s.Title.Render("my title")
	assert.Contains(t, rendered, "my title")
}

func TestStyles_Hint(t *testing.T) {
	s := DefaultStyles()
	rendered := s.Hint.Render("press esc")
	assert.Contains(t, rendered, "press esc")
}

func TestStyles_SelectedItem(t *testing.T) {
	s := DefaultStyles()
	rendered := s.SelectedItem.Render("item")
	assert.Contains(t, rendered, "item")
}

func TestStyles_Item(t *testing.T) {
	s := DefaultStyles()
	rendered := s.Item.Render("item")
	assert.Contains(t, rendered, "item")
}

func TestStyles_DisabledItem(t *testing.T) {
	s := DefaultStyles()
	rendered := s.DisabledItem.Render("disabled")
	assert.Contains(t, rendered, "disabled")
}

func TestStyles_SearchBar(t *testing.T) {
	s := DefaultStyles()
	rendered := s.SearchBar.Render("query")
	assert.Contains(t, rendered, "query")
}

func TestStyles_WarningBorder(t *testing.T) {
	s := DefaultStyles()
	rendered := s.WarningBorder.Render("warning")
	assert.NotEmpty(t, rendered)
}

func TestStyles_DangerBorder(t *testing.T) {
	s := DefaultStyles()
	rendered := s.DangerBorder.Render("danger")
	assert.NotEmpty(t, rendered)
}

func TestThemeRosePine_NotNil(t *testing.T) {
	theme := ThemeRosePine()
	assert.NotNil(t, theme)
}
