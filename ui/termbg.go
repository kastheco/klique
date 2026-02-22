package ui

import (
	"fmt"
	"io"
	"os"
)

// SetTerminalBackground emits OSC 11 to set the terminal's default background
// color. Returns a function that restores the original default via OSC 111.
// This makes every ANSI reset (\033[0m) fall back to the specified color
// instead of the terminal's configured default (usually black).
//
// Supported by: kitty, alacritty, foot, wezterm, ghostty, iTerm2,
// Windows Terminal, and most modern terminal emulators.
func SetTerminalBackground(hexColor string) func() {
	return setTermBg(os.Stdout, hexColor)
}

// setTermBg is the testable core — writes to the given writer instead of stdout.
func setTermBg(w io.Writer, hexColor string) func() {
	if hexColor == "" {
		return func() {}
	}
	// OSC 11 ; <color> ST — set default background color
	fmt.Fprintf(w, "\033]11;%s\033\\", hexColor)

	return func() {
		// OSC 111 ST — reset default background to terminal's configured value
		fmt.Fprint(w, "\033]111\033\\")
	}
}
