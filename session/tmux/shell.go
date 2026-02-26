package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxInlinePromptLen is the threshold above which prompts are written to a
// temp file and referenced via @file instead of being inlined as a shell arg.
// Long prompts can exceed tmux/exec argument limits and silently fail session
// creation.
const maxInlinePromptLen = 8192

// shellEscapeSingleQuote wraps s in POSIX single quotes, escaping any
// embedded single quotes with the '\" idiom. This is safe for all content
// (newlines, $, backticks, double quotes) except NUL bytes.
func shellEscapeSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// promptArg returns the shell argument for the initial prompt. Short prompts
// are shell-escaped inline; long prompts are written to a temp file and
// referenced via Claude Code's @file syntax. The temp file path is stored
// in t.promptFile for cleanup by Close().
func (t *TmuxSession) promptArg() string {
	if len(t.initialPrompt) <= maxInlinePromptLen {
		return shellEscapeSingleQuote(t.initialPrompt)
	}

	f, err := os.CreateTemp("", "kasmos-prompt-*.md")
	if err != nil {
		// Fall back to inline if temp file creation fails.
		return shellEscapeSingleQuote(t.initialPrompt)
	}

	if _, err := f.WriteString(t.initialPrompt); err != nil {
		f.Close()
		os.Remove(f.Name())
		return shellEscapeSingleQuote(t.initialPrompt)
	}
	f.Close()

	t.promptFile = f.Name()
	// Claude Code's @file syntax reads the file contents as the prompt.
	return fmt.Sprintf("@%s", filepath.Clean(t.promptFile))
}
