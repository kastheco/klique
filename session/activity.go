package session

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Activity represents what an agent is currently doing.
type Activity struct {
	// Action is the type of activity (e.g. "editing", "running", "reading", "searching").
	Action string
	// Detail provides additional context (e.g. filename or command).
	Detail string
	// Timestamp is when this activity was detected.
	Timestamp time.Time
}

// ansiRegex matches ANSI escape sequences so they can be stripped from terminal output.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Claude-specific patterns.
var (
	claudeEditingRegex   = regexp.MustCompile(`(?:Editing|Writing)\s+(.+)`)
	claudeReadingRegex   = regexp.MustCompile(`Reading\s+(.+)`)
	claudeRunningRegex   = regexp.MustCompile(`Running\s+(.+)`)
	claudeSearchingRegex = regexp.MustCompile(`Searching`)
	claudeShellRegex     = regexp.MustCompile(`\$\s+(.+)`)
)

// Aider-specific patterns.
var aiderEditingRegex = regexp.MustCompile(`Editing\s+(.+)`)

// ParseActivity scans the last 30 lines of content (bottom-up) and returns the
// most recent recognisable agent activity. program is compared case-insensitively
// against known agent names ("claude", "aider"). Returns nil when nothing matches.
func ParseActivity(content string, program string) *Activity {
	stripped := ansiRegex.ReplaceAllString(content, "")
	lines := strings.Split(stripped, "\n")

	// Restrict scan window to the last 30 lines.
	start := len(lines) - 30
	if start < 0 {
		start = 0
	}
	tail := lines[start:]

	prog := strings.ToLower(program)

	// Scan from bottom to top so the most recent match wins.
	for i := len(tail) - 1; i >= 0; i-- {
		line := strings.TrimSpace(tail[i])
		if line == "" {
			continue
		}

		if strings.Contains(prog, "claude") {
			if a := parseClaudeLine(line); a != nil {
				return a
			}
		} else if strings.Contains(prog, "aider") {
			if a := parseAiderLine(line); a != nil {
				return a
			}
		}

		if a := parseGenericLine(line); a != nil {
			return a
		}
	}

	return nil
}

// parseClaudeLine attempts to match a line against Claude-specific patterns.
func parseClaudeLine(line string) *Activity {
	if m := claudeEditingRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "editing",
			Detail:    truncateDetail(cleanFilename(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	if m := claudeReadingRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "reading",
			Detail:    truncateDetail(cleanFilename(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	if m := claudeRunningRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "running",
			Detail:    truncateDetail(strings.TrimSpace(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	if claudeSearchingRegex.MatchString(line) {
		return &Activity{
			Action:    "searching",
			Timestamp: time.Now(),
		}
	}
	if m := claudeShellRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "running",
			Detail:    truncateDetail(strings.TrimSpace(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	return nil
}

// parseAiderLine attempts to match a line against Aider-specific patterns.
func parseAiderLine(line string) *Activity {
	if m := aiderEditingRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "editing",
			Detail:    truncateDetail(cleanFilename(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	return nil
}

// parseGenericLine matches patterns common to any agent (e.g. shell prompt lines).
func parseGenericLine(line string) *Activity {
	if m := claudeShellRegex.FindStringSubmatch(line); m != nil {
		return &Activity{
			Action:    "running",
			Detail:    truncateDetail(strings.TrimSpace(m[1]), 40),
			Timestamp: time.Now(),
		}
	}
	return nil
}

// cleanFilename returns filepath.Base(s) when s contains a path separator,
// otherwise the trimmed string itself.
func cleanFilename(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return filepath.Base(s)
	}
	return s
}

// truncateDetail shortens s to at most maxLen characters. When truncation is
// needed and maxLen > 3, the result ends with "...".
func truncateDetail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
