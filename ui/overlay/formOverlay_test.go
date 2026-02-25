package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormOverlay_SubmitWithName(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "auth-refactor" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "auth-refactor", f.Name())
	assert.Equal(t, "", f.Description())
}

func TestFormOverlay_SubmitWithNameAndDescription(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "auth" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "refactor jwt" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, f.IsSubmitted())
	assert.Equal(t, "auth", f.Name())
	assert.Equal(t, "refactor jwt", f.Description())
}

func TestFormOverlay_EmptyNameDoesNotSubmit(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, closed)
	assert.False(t, f.IsSubmitted())
}

func TestFormOverlay_EscCancels(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, f.IsSubmitted())
}

func TestFormOverlay_ArrowDownNavigates(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "test" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	for _, r := range "desc" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, closed)
	assert.Equal(t, "test", f.Name())
	assert.Equal(t, "desc", f.Description())
}

func TestFormOverlay_TabCyclesFromDescriptionBackToName(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "a" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "b" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "c" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, closed)
	assert.Equal(t, "ac", f.Name())
	assert.Equal(t, "b", f.Description())
}

func TestFormOverlay_ShiftTabCyclesFromNameToDescription(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "a" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyShiftTab})
	for _, r := range "b" {
		f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	closed := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, closed)
	assert.Equal(t, "a", f.Name())
	assert.Equal(t, "b", f.Description())
}

func TestFormOverlay_Render(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	output := f.Render()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "new plan")
}
