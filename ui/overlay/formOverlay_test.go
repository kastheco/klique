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
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "auth-refactor", f.Name())
	assert.Equal(t, "", f.Description())
}

func TestFormOverlay_SubmitWithNameAndDescription(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "auth" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "refactor jwt" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "auth", f.Name())
	assert.Equal(t, "refactor jwt", f.Description())
}

func TestFormOverlay_EmptyNameDoesNotSubmit(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestFormOverlay_EscCancels(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestFormOverlay_ArrowDownNavigates(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "test" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	for _, r := range "desc" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, result.Dismissed)
	assert.Equal(t, "test", f.Name())
	assert.Equal(t, "desc", f.Description())
}

func TestFormOverlay_TabCyclesFromDescriptionBackToName(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "a" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "b" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "c" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, result.Dismissed)
	assert.Equal(t, "ac", f.Name())
	assert.Equal(t, "b", f.Description())
}

func TestFormOverlay_ShiftTabCyclesFromNameToDescription(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "a" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	for _, r := range "b" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, result.Dismissed)
	assert.Equal(t, "a", f.Name())
	assert.Equal(t, "b", f.Description())
}

func TestSpawnFormOverlay_SubmitWithNameOnly(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "my-task" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "my-task", f.Name())
	assert.Equal(t, "", f.Branch())
	assert.Equal(t, "", f.WorkPath())
}

func TestSpawnFormOverlay_SubmitWithAllFields(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "task" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "feature/login" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "/tmp/worktree" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "task", f.Name())
	assert.Equal(t, "feature/login", f.Branch())
	assert.Equal(t, "/tmp/worktree", f.WorkPath())
}

func TestSpawnFormOverlay_EmptyNameDoesNotSubmit(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestSpawnFormOverlay_TabCyclesThreeFields(t *testing.T) {
	f := NewSpawnFormOverlay("spawn agent", 60)
	for _, r := range "n" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to branch
	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "b" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to path
	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "p" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab wraps to name
	f.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "!" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, result.Dismissed)
	assert.Equal(t, "n!", f.Name())
	assert.Equal(t, "b", f.Branch())
	assert.Equal(t, "p", f.WorkPath())
}

func TestFormOverlay_Render(t *testing.T) {
	f := NewFormOverlay("new plan", 60)

	output := f.View()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "new plan")
}

func TestFormOverlay_ImplementsOverlay(t *testing.T) {
	var _ Overlay = NewFormOverlay("title", 60)
}

func TestFormOverlay_HandleKey_Submit(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	for _, r := range "test-name" {
		f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "test-name", result.Value)
}

func TestFormOverlay_HandleKey_Cancel(t *testing.T) {
	f := NewFormOverlay("new plan", 60)
	result := f.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}
