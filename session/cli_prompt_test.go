package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgramSupportsCliPrompt(t *testing.T) {
	tests := []struct {
		program  string
		expected bool
	}{
		{"opencode", true},
		{"claude", true},
		{"/usr/local/bin/claude", true},
		{"/home/user/.local/bin/opencode", true},
		{"opencode --variant low", true},
		{"opencode --agent coder", true},
		{"claude --model sonnet", true},
		{"/usr/local/bin/opencode --variant low", true},
		{"aider --model ollama_chat/gemma3:1b", false},
		{"gemini", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.program, func(t *testing.T) {
			assert.Equal(t, tt.expected, programSupportsCliPrompt(tt.program))
		})
	}
}
