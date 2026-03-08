package ui

import (
	"fmt"
	"strings"
)

// parseHex converts a "#RRGGBB" hex string to (r, g, b) uint8 components.
// Returns (0, 0, 0) if the input (after stripping leading '#') is not exactly 6 chars.
func parseHex(hex string) (uint8, uint8, uint8) {
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// lerpByte linearly interpolates between byte a and byte b at position t ∈ [0,1].
func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

// GradientText renders text with a left-to-right truecolor gradient from startHex to endHex.
// Newline characters are passed through unchanged. An ANSI reset sequence is appended at the end.
// Returns an empty string when text is empty. Returns text unchanged when it has no visible runes.
func GradientText(text, startHex, endHex string) string {
	if text == "" {
		return ""
	}

	r1, g1, b1 := parseHex(startHex)
	r2, g2, b2 := parseHex(endHex)

	runes := []rune(text)
	visible := 0
	for _, ch := range runes {
		if ch != '\n' {
			visible++
		}
	}
	if visible == 0 {
		return text
	}

	var sb strings.Builder
	idx := 0
	for _, ch := range runes {
		if ch == '\n' {
			sb.WriteRune('\n')
			continue
		}
		t := 0.0
		if visible > 1 {
			t = float64(idx) / float64(visible-1)
		}
		cr := lerpByte(r1, r2, t)
		cg := lerpByte(g1, g2, t)
		cb := lerpByte(b1, b2, t)
		sb.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm%c", cr, cg, cb, ch))
		idx++
	}
	sb.WriteString("\033[39m")
	return sb.String()
}

// GradientBar renders a horizontal progress bar of `width` characters.
// The `filled` portion uses full-block (█) characters colored with the gradient from
// startHex to endHex. The remainder uses light-shade (░) characters in dim gray.
// Returns empty string when width <= 0. Clamps filled to [0, width].
func GradientBar(width, filled int, startHex, endHex string) string {
	if width <= 0 {
		return ""
	}
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	r1, g1, b1 := parseHex(startHex)
	r2, g2, b2 := parseHex(endHex)

	var sb strings.Builder
	for i := 0; i < filled; i++ {
		t := 0.0
		if filled > 1 {
			t = float64(i) / float64(filled-1)
		}
		cr := lerpByte(r1, r2, t)
		cg := lerpByte(g1, g2, t)
		cb := lerpByte(b1, b2, t)
		sb.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm█", cr, cg, cb))
	}
	if filled < width {
		sb.WriteString("\033[38;2;60;60;60m")
		sb.WriteString(strings.Repeat("░", width-filled))
	}
	sb.WriteString("\033[39m")
	return sb.String()
}
