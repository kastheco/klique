package wizard

// RoleDescription returns a human-readable description for a known agent role.
func RoleDescription(role string) string {
	descs := map[string]string{
		"coder":    "Handles implementation tasks. Receives code-level instructions,\nwrites and edits files, runs tests.",
		"reviewer": "Reviews code for correctness, style, and architecture.\nProvides structured feedback before merge.",
		"planner":  "Breaks features into implementation plans.\nDecomposes specs into ordered tasks with file paths and tests.",
		"chat":     "General-purpose assistant for questions and exploration.\nAuto-configured for all selected harnesses.",
	}
	return descs[role]
}

// RolePhaseText returns which workflow phases map to this role.
func RolePhaseText(role string) string {
	phases := map[string]string{
		"coder":    "Default for phases: implementing",
		"reviewer": "Default for phases: spec_review, quality_review",
		"planner":  "Default for phases: planning",
		"chat":     "Available in all phases (ad-hoc)",
	}
	return phases[role]
}

// HarnessDescription returns a one-line summary for a known harness.
func HarnessDescription(name string) string {
	descs := map[string]string{
		"claude":   "Anthropic Claude Code · effort levels · MCP plugins",
		"opencode": "Multi-provider agent · temperature · effort · all models",
		"codex":    "OpenAI Codex CLI · temperature · effort",
	}
	return descs[name]
}

// HarnessCapabilities returns a capabilities list for the detail panel.
func HarnessCapabilities(name string) []string {
	caps := map[string][]string{
		"claude":   {"Model selection", "Effort levels", "MCP plugin support", "No temperature control"},
		"opencode": {"Model selection (50+ models)", "Temperature control", "Effort levels", "Provider-agnostic"},
		"codex":    {"Model selection", "Temperature control", "Effort levels", "Reasoning effort config"},
	}
	return caps[name]
}
