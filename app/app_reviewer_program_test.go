package app

import "testing"

func TestWithOpenCodeModelFlag(t *testing.T) {
	tests := []struct {
		name    string
		program string
		model   string
		want    string
	}{
		{
			name:    "opencode appends explicit provider model",
			program: "opencode",
			model:   "anthropic/claude-opus-4-6",
			want:    "opencode --model anthropic/claude-opus-4-6",
		},
		{
			name:    "opencode normalizes bare claude model",
			program: "opencode --agent reviewer",
			model:   "claude-opus-4-6",
			want:    "opencode --agent reviewer --model anthropic/claude-opus-4-6",
		},
		{
			name:    "does not duplicate model flag",
			program: "opencode --agent reviewer --model anthropic/claude-sonnet-4-6",
			model:   "anthropic/claude-opus-4-6",
			want:    "opencode --agent reviewer --model anthropic/claude-sonnet-4-6",
		},
		{
			name:    "non opencode command unchanged",
			program: "claude --agent reviewer",
			model:   "anthropic/claude-opus-4-6",
			want:    "claude --agent reviewer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withOpenCodeModelFlag(tt.program, tt.model)
			if got != tt.want {
				t.Fatalf("withOpenCodeModelFlag() = %q, want %q", got, tt.want)
			}
		})
	}
}
