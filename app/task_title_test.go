package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeuristicPlanTitle_FirstLine(t *testing.T) {
	desc := "refactor auth to use JWT tokens\nThis needs to handle refresh tokens too"
	got := heuristicPlanTitle(desc)
	assert.Equal(t, "refactor auth to use JWT tokens", got)
}

func TestHeuristicPlanTitle_StripsFiller(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"i want to refactor the auth module", "refactor the auth module"},
		{"we need to add dark mode support", "add dark mode support"},
		{"please add search functionality", "add search functionality"},
		{"let's build a new dashboard", "build a new dashboard"},
		{"can you implement caching", "implement caching"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := heuristicPlanTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHeuristicPlanTitle_TruncatesLongInput(t *testing.T) {
	desc := "refactor the entire authentication subsystem to use JSON web tokens instead of session cookies across all microservices"
	got := heuristicPlanTitle(desc)
	// Should be at most 6 words (punctuation-aware truncation)
	words := len(splitWords(got))
	assert.LessOrEqual(t, words, 6)
}

func TestHeuristicPlanTitle_EmptyInput(t *testing.T) {
	got := heuristicPlanTitle("")
	assert.Equal(t, "new plan", got)
}

func TestHeuristicPlanTitle_WhitespaceOnly(t *testing.T) {
	got := heuristicPlanTitle("   \n\n  ")
	assert.Equal(t, "new plan", got)
}

func TestHeuristicPlanTitle_SingleLineLong_UsesNaturalBreak(t *testing.T) {
	desc := "implement a custom verification process, including static analysis and reality checks for all plans"
	got := heuristicPlanTitle(desc)
	assert.Equal(t, "implement a custom verification process", got)
}

func TestHeuristicPlanTitle_SingleLineNoPunctuation_Truncates(t *testing.T) {
	desc := "refactor the entire authentication subsystem to use JSON web tokens instead of session cookies"
	got := heuristicPlanTitle(desc)
	words := len(splitWords(got))
	assert.LessOrEqual(t, words, 6)
}

func TestFirstLineIsViableSlug_MultilineShortFirstLine(t *testing.T) {
	desc := "fix auth token refresh\ndetails about the bug and how to reproduce it"
	assert.True(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_SingleLine(t *testing.T) {
	// Single-line descriptions should NOT be considered viable — they need AI
	desc := "fix auth token refresh"
	assert.False(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_MultilineLongFirstLine(t *testing.T) {
	// First line is too long (>6 words after filler strip) — needs AI
	desc := "refactor the entire authentication subsystem to use JWT tokens\nmore details here"
	assert.False(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_MultilineWithFiller(t *testing.T) {
	// First line has filler prefix but after stripping is ≤6 words
	desc := "i want to fix auth refresh\ndetails about the bug"
	assert.True(t, firstLineIsViableSlug(desc))
}

func TestFirstLineIsViableSlug_EmptyFirstLine(t *testing.T) {
	desc := "\nactual description here"
	assert.False(t, firstLineIsViableSlug(desc))
}
