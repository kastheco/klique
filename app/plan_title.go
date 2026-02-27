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
// Takes the first line, strips common filler prefixes, and truncates to 8 words.
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

	// Truncate to 8 words
	words := splitWords(text)
	if len(words) > 8 {
		words = words[:8]
	}
	return strings.Join(words, " ")
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
