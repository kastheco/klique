package session

import (
	"regexp"
	"strings"
)

// PermissionPrompt represents a detected permission request from an agent.
type PermissionPrompt struct {
	// Description is the human-readable description, e.g. "Access external directory /opt".
	Description string
	// Pattern is the permission pattern, e.g. "/opt/*".
	Pattern string
}

// ansiStripRe strips ANSI escape sequences for permission prompt parsing.
var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// ParsePermissionPrompt scans pane content for an opencode "Permission required" dialog.
// Returns nil if no permission prompt is detected or if the program is not opencode.
func ParsePermissionPrompt(content string, program string) *PermissionPrompt {
	if !strings.Contains(strings.ToLower(program), "opencode") {
		return nil
	}

	clean := ansiStripRe.ReplaceAllString(content, "")
	lines := strings.Split(clean, "\n")

	permIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), "Permission required") {
			permIdx = i
			break
		}
	}
	if permIdx < 0 {
		return nil
	}

	prompt := &PermissionPrompt{}

	// Description is on the next non-empty line after "Permission required", strip leading "← ".
	for i := permIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "← ")
		trimmed = strings.TrimPrefix(trimmed, "←")
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
