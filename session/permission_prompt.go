package session

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// PermissionPrompt represents a detected permission request from an agent.
type PermissionPrompt struct {
	// Description is the human-readable description, e.g. "Access external directory /opt".
	Description string
	// Pattern is the permission pattern, e.g. "/opt/*".
	Pattern string
}

// ParsePermissionPrompt scans pane content for an opencode "Permission required" dialog.
// Returns nil if no permission prompt is detected or if the program is not opencode.
func ParsePermissionPrompt(content string, program string) *PermissionPrompt {
	if !strings.Contains(strings.ToLower(program), "opencode") {
		return nil
	}

	clean := ansi.Strip(content)
	lines := strings.Split(clean, "\n")

	// Only scan the bottom 25 lines of the pane. The permission dialog is
	// rendered at the bottom of opencode's TUI (~10 lines for the dialog
	// plus status bar). Scanning the full pane false-positives on
	// conversation text that discusses permissions.
	const tailLines = 25
	startLine := len(lines) - tailLines
	if startLine < 0 {
		startLine = 0
	}
	lines = lines[startLine:]

	// Two structural checks to avoid false-positives from conversation text:
	//  1. "△ Permission required" header (the △ glyph is opencode UI chrome)
	//  2. Button bar with "Allow once" + "Allow always" on the same line
	// Both must appear within the tail window.
	permIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "△") && strings.Contains(trimmed, "Permission required") {
			permIdx = i
			break
		}
	}
	if permIdx < 0 {
		return nil
	}

	hasButtons := false
	for _, line := range lines[permIdx:] {
		if strings.Contains(line, "Allow once") && strings.Contains(line, "Allow always") {
			hasButtons = true
			break
		}
	}
	if !hasButtons {
		return nil
	}

	prompt := &PermissionPrompt{}

	// Description is on the next non-empty line after "Permission required".
	// Strip leading arrow prefixes — opencode uses both "← " and "→ ".
	for i := permIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "← ")
		trimmed = strings.TrimPrefix(trimmed, "←")
		trimmed = strings.TrimPrefix(trimmed, "→ ")
		trimmed = strings.TrimPrefix(trimmed, "→")
		trimmed = strings.TrimSpace(trimmed)
		prompt.Description = trimmed
		break
	}

	// Pattern: find "Patterns" header, then first line starting with "- ".
	for i := permIdx; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "Patterns" {
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed == "" {
					continue
				}
				if strings.HasPrefix(trimmed, "- ") {
					prompt.Pattern = strings.TrimPrefix(trimmed, "- ")
					break
				}
				break // non-empty, non-pattern line — stop
			}
			break
		}
	}

	return prompt
}
