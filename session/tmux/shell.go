package tmux

import (
	"os"
	"path/filepath"
	"strings"
)

// MaxInlinePromptLen is the threshold above which prompts should not be
// inlined as shell arguments. Long prompts can exceed tmux/exec argument
// limits and silently fail session creation. Callers should fall back to
// send-keys delivery for prompts exceeding this length.
const MaxInlinePromptLen = 8192

// promptDir is the subdirectory within the workdir where prompt files are stored.
// Lives inside the project so Claude Code can read @file references without
// extra permissions.
const promptDir = ".kasmos"

// shellEscapeSingleQuote wraps s in POSIX single quotes, escaping any
// embedded single quotes with the '\" idiom. This is safe for all content
// (newlines, $, backticks, double quotes) except NUL bytes.
func shellEscapeSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// writePromptFile writes the prompt to a temp file under <workDir>/.kasmos/
// and stores the path in t.promptFile for cleanup by Close().
// Returns the absolute path, or "" on error.
func (t *TmuxSession) writePromptFile(workDir string) string {
	dir := filepath.Join(workDir, promptDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}

	f, err := os.CreateTemp(dir, "prompt-*.md")
	if err != nil {
		return ""
	}

	if _, err := f.WriteString(t.initialPrompt); err != nil {
		f.Close()
		os.Remove(f.Name())
		return ""
	}
	f.Close()
	t.promptFile = f.Name()
	return t.promptFile
}
