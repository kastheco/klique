package tmux

import "strings"

// shellEscapeSingleQuote wraps s in POSIX single quotes, escaping any
// embedded single quotes with the '\” idiom. This is safe for all content
// (newlines, $, backticks, double quotes) except NUL bytes.
func shellEscapeSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
