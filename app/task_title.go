package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// planTitleMsg is sent when the async AI title derivation completes.
type planTitleMsg struct {
	title string
	err   error
}

// heuristicPlanTitle derives a short title from a plan description.
// Takes the first line, strips common filler prefixes, and uses punctuation-aware
// truncation for single-line inputs (falls back to 6-word truncation).
func heuristicPlanTitle(description string) string {
	text := strings.TrimSpace(description)
	if text == "" {
		return "new plan"
	}

	// Take first line only
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	if text == "" {
		return "new plan"
	}

	// Strip common filler prefixes (case-insensitive)
	lower := strings.ToLower(text)
	fillers := []string{
		"i want to ", "i'd like to ", "we need to ", "we should ",
		"please ", "let's ", "let us ", "can you ", "could you ",
	}
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			text = text[len(f):]
			break
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "new plan"
	}

	words := splitWords(text)
	if len(words) <= 6 {
		return strings.Join(words, " ")
	}

	// Look for a natural break within first 8 words
	limit := min(8, len(words))
	first8 := strings.Join(words[:limit], " ")
	for _, sep := range []string{", ", "; ", ": ", ". ", " - "} {
		if idx := strings.Index(first8, sep); idx > 0 {
			candidate := strings.TrimSpace(first8[:idx])
			if len(splitWords(candidate)) >= 3 {
				return candidate
			}
		}
	}

	// No natural break — truncate to 6 words
	return strings.Join(words[:6], " ")
}

// firstLineIsViableSlug returns true when the description is multiline and the
// first line (after filler-stripping) is short enough to use as a slug without
// truncation — meaning heuristicPlanTitle would return it verbatim.
func firstLineIsViableSlug(description string) bool {
	text := strings.TrimSpace(description)
	if text == "" {
		return false
	}

	// Must be multiline — single-line descriptions benefit from AI summarization
	idx := strings.IndexByte(text, '\n')
	if idx < 0 {
		return false
	}

	firstLine := strings.TrimSpace(text[:idx])
	if firstLine == "" {
		return false
	}

	// Strip filler prefixes (same logic as heuristicPlanTitle)
	lower := strings.ToLower(firstLine)
	fillers := []string{
		"i want to ", "i'd like to ", "we need to ", "we should ",
		"please ", "let's ", "let us ", "can you ", "could you ",
	}
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			firstLine = firstLine[len(f):]
			break
		}
	}
	firstLine = strings.TrimSpace(firstLine)
	if firstLine == "" {
		return false
	}

	// Viable if ≤6 words (no truncation needed)
	return len(splitWords(firstLine)) <= 6
}

// splitWords splits text on whitespace, returning non-empty tokens.
func splitWords(s string) []string {
	raw := strings.Fields(s)
	out := make([]string, 0, len(raw))
	for _, w := range raw {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// aiDerivePlanTitleCmd returns a tea.Cmd that shells out to claude to derive
// a concise plan title from the given description. Returns planTitleMsg.
func aiDerivePlanTitleCmd(description string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		prompt := fmt.Sprintf(
			"Generate a concise 3-5 word title for this software task. "+
				"Respond with ONLY the title in lowercase, nothing else. No quotes, no punctuation.\n\n%s",
			description,
		)

		cmd := exec.CommandContext(ctx, "claude",
			"-p", prompt,
			"--model", "claude-sonnet-4-20250514",
			"--output-format", "text",
		)
		out, err := cmd.Output()
		if err != nil {
			return planTitleMsg{err: err}
		}

		title := strings.TrimSpace(string(out))
		if title == "" {
			return planTitleMsg{err: fmt.Errorf("empty AI response")}
		}
		return planTitleMsg{title: title}
	}
}
