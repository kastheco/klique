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
// passed directly on the command line. It checks that program ends with one
// of the known program names, so both bare names ("claude") and absolute
// paths ("/usr/local/bin/claude") are matched correctly.
func programSupportsCliPrompt(program string) bool {
	for _, name := range cliPromptPrograms {
		if strings.HasSuffix(program, name) {
			return true
		}
	}
	return false
}
