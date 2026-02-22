package app

import (
	"strings"
	"testing"

	"github.com/kastheco/klique/session"
)

func TestBuildPlanPrompt(t *testing.T) {
	prompt := buildPlanPrompt("Auth Refactor", "Refactor JWT auth")
	if !strings.Contains(prompt, "Plan Auth Refactor") {
		t.Fatalf("prompt missing title")
	}
	if !strings.Contains(prompt, "Goal: Refactor JWT auth") {
		t.Fatalf("prompt missing goal")
	}
}

func TestBuildImplementPrompt(t *testing.T) {
	prompt := buildImplementPrompt("2026-02-21-auth-refactor.md")
	if !strings.Contains(prompt, "Implement docs/plans/2026-02-21-auth-refactor.md") {
		t.Fatalf("prompt missing plan path")
	}
}

func TestAgentTypeForSubItem(t *testing.T) {
	tests := map[string]string{
		"plan":      session.AgentTypePlanner,
		"implement": session.AgentTypeCoder,
		"review":    session.AgentTypeReviewer,
	}
	for action, want := range tests {
		got, ok := agentTypeForSubItem(action)
		if !ok {
			t.Fatalf("agentTypeForSubItem(%q) returned ok=false", action)
		}
		if got != want {
			t.Fatalf("agentTypeForSubItem(%q) = %q, want %q", action, got, want)
		}
	}
}
