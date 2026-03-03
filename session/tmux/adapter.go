package tmux

import (
	"path/filepath"
	"strings"
	"time"
)

// ProgramAdapter encapsulates program-specific behavior for readiness detection,
// prompt detection, trust-screen handling, and CLI prompt syntax.
// Implementations exist for claude and opencode; aider/gemini support is dropped.
type ProgramAdapter interface {
	// ReadyString returns the substring to look for in pane content that signals
	// the program has fully started and is ready for input.
	ReadyString() string

	// NeedsTrustTap returns true if the program shows a trust/confirmation screen
	// on first launch that requires an Enter keystroke to dismiss.
	NeedsTrustTap() bool

	// DetectPrompt returns true when the plain (ANSI-stripped) pane content
	// indicates the program is idle and waiting for user input.
	DetectPrompt(plainContent string) bool

	// MaxWaitTime returns the maximum time to wait for the program to reach the
	// ready state before giving up.
	MaxWaitTime() time.Duration

	// BuildPromptArg returns the shell argument for the program's initial-prompt flag.
	// Short prompts are inlined (shell-escaped). Long prompts are written to a temp file
	// via writeFile (which receives the prompt text and returns the absolute path), then
	// referenced using the program's file-argument syntax.
	BuildPromptArg(prompt, workDir string, writeFile func(string) string) string

	// SupportsCliPrompt reports whether this program supports receiving a prompt
	// directly from the CLI (as opposed to via send-keys after startup).
	SupportsCliPrompt() bool
}

// AdapterFor returns the ProgramAdapter for the given program string, or nil
// if the program has no special adapter (unknown/unsupported program).
func AdapterFor(program string) ProgramAdapter {
	switch {
	case strings.HasSuffix(program, "claude"):
		return claudeAdapter{}
	case strings.HasSuffix(program, "opencode"):
		return opencodeAdapter{}
	default:
		return nil
	}
}

// claudeAdapter implements ProgramAdapter for Claude Code.
type claudeAdapter struct{}

func (a claudeAdapter) ReadyString() string {
	return "Do you trust the files in this folder?"
}

func (a claudeAdapter) NeedsTrustTap() bool {
	return true
}

// DetectPrompt returns true when Claude is idle at its input prompt.
// The idle indicator is the text "No, and tell Claude what to do differently"
// appearing in the plain (ANSI-stripped) pane content.
func (a claudeAdapter) DetectPrompt(plainContent string) bool {
	return strings.Contains(plainContent, "No, and tell Claude what to do differently")
}

func (a claudeAdapter) MaxWaitTime() time.Duration {
	return 30 * time.Second
}

// BuildPromptArg returns the shell argument for Claude's positional prompt.
// Short prompts are single-quote escaped inline; long prompts are written to a
// file under .kasmos/ and referenced via Claude Code's @file syntax.
func (a claudeAdapter) BuildPromptArg(prompt, workDir string, writeFile func(string) string) string {
	if len(prompt) <= MaxInlinePromptLen {
		return shellEscapeSingleQuote(prompt)
	}
	path := writeFile(prompt)
	if path == "" {
		return shellEscapeSingleQuote(prompt)
	}
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		rel = path
	}
	return "@" + rel
}

func (a claudeAdapter) SupportsCliPrompt() bool {
	return true
}

// opencodeAdapter implements ProgramAdapter for OpenCode.
type opencodeAdapter struct{}

func (a opencodeAdapter) ReadyString() string {
	return "Ask anything"
}

func (a opencodeAdapter) NeedsTrustTap() bool {
	return false
}

// DetectPrompt returns true when opencode is idle and waiting for input.
// opencode shows "esc interrupt" in its bottom bar only while a task is running.
// When idle the bar disappears, so absence of "esc interrupt" signals idle state.
// The caller must pass plain (ANSI-stripped) content.
func (a opencodeAdapter) DetectPrompt(plainContent string) bool {
	return !strings.Contains(plainContent, "esc interrupt")
}

func (a opencodeAdapter) MaxWaitTime() time.Duration {
	return 30 * time.Second
}

// BuildPromptArg returns the shell argument for opencode's --prompt flag.
// Short prompts are single-quote escaped inline; long prompts are written to a
// file and read back via shell command substitution ($(cat ...)) since opencode
// has no @file syntax.
func (a opencodeAdapter) BuildPromptArg(prompt, workDir string, writeFile func(string) string) string {
	if len(prompt) <= MaxInlinePromptLen {
		return shellEscapeSingleQuote(prompt)
	}
	path := writeFile(prompt)
	if path == "" {
		return shellEscapeSingleQuote(prompt)
	}
	return "\"$(cat " + shellEscapeSingleQuote(path) + ")\""
}

func (a opencodeAdapter) SupportsCliPrompt() bool {
	return true
}
