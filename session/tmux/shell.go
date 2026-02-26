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

// promptArg returns the shell argument for the initial prompt. Short prompts
// are shell-escaped inline; long prompts are written to a file under
// <workDir>/.kasmos/ and referenced via Claude Code's @file syntax.
// The temp file path is stored in t.promptFile for cleanup by Close().
func (t *TmuxSession) promptArg(workDir string) string {
	if len(t.initialPrompt) <= MaxInlinePromptLen {
		return shellEscapeSingleQuote(t.initialPrompt)
	}

	dir := filepath.Join(workDir, promptDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return shellEscapeSingleQuote(t.initialPrompt)
	}

	f, err := os.CreateTemp(dir, "prompt-*.md")
	if err != nil {
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
	// Use a relative path from the workdir since that's the tmux session's cwd.
	rel, err := filepath.Rel(workDir, t.promptFile)
	if err != nil {
		rel = t.promptFile
	}
	return "@" + rel
}
