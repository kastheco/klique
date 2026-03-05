package session

import "strings"

// cliPromptPrograms lists programs that accept an initial user prompt via a
// CLI flag (opencode --prompt) or positional argument (claude).
//
// NOTE: Values correspond to session/tmux.ProgramOpenCode and
// session/tmux.ProgramClaude. Keep in sync if those constants change;
// importing tmux from here would create a circular dependency.
var cliPromptPrograms = []string{"opencode", "claude"}

// programSupportsCliPrompt reports whether program accepts a startup prompt
// passed directly on the command line. It extracts the base program name
// (first whitespace-delimited token, then the filepath base) so that both
// bare names ("claude"), absolute paths ("/usr/local/bin/claude"), and
// commands with flags ("opencode --variant low") are matched correctly.
func programSupportsCliPrompt(program string) bool {
	// Extract the executable name: first token handles flags, then
	// filepath-style base handles absolute paths.
	base := program
	if idx := strings.IndexByte(program, ' '); idx > 0 {
		base = program[:idx]
	}
	if slashIdx := strings.LastIndexByte(base, '/'); slashIdx >= 0 {
		base = base[slashIdx+1:]
	}
	for _, name := range cliPromptPrograms {
		if base == name {
			return true
		}
	}
	return false
}
