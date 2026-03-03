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

	// Only examine the last 25 lines — the permission dialog renders at the
	// bottom of opencode's TUI. Limiting the scan window avoids false-positives
	// from conversation text that may mention "Permission required".
	const tailLines = 25
	if start := len(lines) - tailLines; start > 0 {
		lines = lines[start:]
	}

	// Structural check 1: locate the "△ Permission required" header line.
	// The △ glyph is unique to opencode's UI chrome; its presence on the same
	// line as "Permission required" is the primary signal.
	permIdx := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.Contains(t, "△") && strings.Contains(t, "Permission required") {
			permIdx = i
			break
		}
	}
	if permIdx < 0 {
		return nil
	}

	// Structural check 2: button bar with both "Allow once" and "Allow always"
	// must appear after the header line. Without this guard a bare mention of
	// "Permission required" in conversation text would create false-positives.
	buttonFound := false
	for _, line := range lines[permIdx:] {
		if strings.Contains(line, "Allow once") && strings.Contains(line, "Allow always") {
			buttonFound = true
			break
		}
	}
	if !buttonFound {
		return nil
	}

	prompt := &PermissionPrompt{}

	// Description: first non-empty line after the header.
	// opencode prefixes the description with "← " or "→ " arrow glyphs.
	for i := permIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		t = strings.TrimPrefix(t, "← ")
		t = strings.TrimPrefix(t, "←")
		t = strings.TrimPrefix(t, "→ ")
		t = strings.TrimPrefix(t, "→")
		prompt.Description = strings.TrimSpace(t)
		break
	}

	// Pattern: locate "Patterns" header, then take the first non-empty line
	// that starts with "- ".
	for i := permIdx; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "Patterns" {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" {
				continue
			}
			if strings.HasPrefix(t, "- ") {
				prompt.Pattern = strings.TrimPrefix(t, "- ")
			}
			break
		}
		break
	}

	return prompt
}
