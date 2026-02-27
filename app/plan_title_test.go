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
	// Should be at most 8 words
	words := len(splitWords(got))
	assert.LessOrEqual(t, words, 8)
}

func TestHeuristicPlanTitle_EmptyInput(t *testing.T) {
	got := heuristicPlanTitle("")
	assert.Equal(t, "new plan", got)
}

func TestHeuristicPlanTitle_WhitespaceOnly(t *testing.T) {
	got := heuristicPlanTitle("   \n\n  ")
	assert.Equal(t, "new plan", got)
}
