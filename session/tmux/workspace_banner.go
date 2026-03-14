package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	workspaceBannerRaw = `██╗  ██╗ █████╗ ███████╗███╗   ███╗ ██████╗ ███████╗
██║ ██╔╝██╔══██╗██╔════╝████╗ ████║██╔═══██╗██╔════╝
█████╔╝ ███████║███████╗██╔████╔██║██║   ██║███████╗
██╔═██╗ ██╔══██║╚════██║██║╚██╔╝██║██║   ██║╚════██║
██║  ██╗██║  ██║███████║██║ ╚═╝ ██║╚██████╔╝███████║
╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚══════╝`
	workspaceBannerStartHex = "#9ccfd8"
	workspaceBannerEndHex   = "#c4a7e7"
	workspaceBannerDelay    = "0.08"
)

var workspaceBannerScriptDir = os.TempDir

var workspaceBannerPeriod = [6]string{
	"   ",
	"   ",
	"   ",
	"   ",
	"██╗",
	"╚═╝",
}

var workspaceBannerFrames = func() []string {
	base := strings.Split(workspaceBannerRaw, "\n")
	type glyph = [6]string
	suffixes := [][]glyph{
		{},
		{workspaceBannerPeriod},
		{workspaceBannerPeriod, workspaceBannerPeriod},
		{workspaceBannerPeriod, workspaceBannerPeriod, workspaceBannerPeriod},
		{},
	}

	frames := make([]string, 0, len(suffixes))
	for _, glyphs := range suffixes {
		lines := make([]string, len(base))
		copy(lines, base)
		for _, g := range glyphs {
			for row := range lines {
				lines[row] += " " + g[row]
			}
		}
		frames = append(frames, workspaceGradientText(strings.Join(lines, "\n"), workspaceBannerStartHex, workspaceBannerEndHex))
	}
	return frames
}()

func buildWorkspacePaneCommand(shell string) (string, error) {
	if shell == "" {
		shell = "/bin/bash"
	}
	path, err := writeWorkspaceBannerScript(shell)
	if err != nil {
		return "", err
	}
	return strconv.Quote(path), nil
}

func writeWorkspaceBannerScript(shell string) (string, error) {
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString("rm -- \"$0\"\n")
	for _, frame := range workspaceBannerFrames {
		script.WriteString("printf '\\033[2J\\033[H'")
		script.WriteString("; printf '%b' '")
		script.WriteString(escapeShellPrintf(frame))
		script.WriteString("'")
		script.WriteString("; sleep ")
		script.WriteString(workspaceBannerDelay)
		script.WriteString("; ")
	}
	script.WriteString("printf '\\033[0m\\n\\n'")
	script.WriteString("; exec ")
	script.WriteString(strconv.Quote(shell))
	script.WriteString(" -i\n")

	path := filepath.Join(workspaceBannerScriptDir(), fmt.Sprintf("kasmos-workspace-banner-%d.sh", os.Getpid()))
	if err := os.WriteFile(path, []byte(script.String()), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func escapeShellPrintf(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\x1b':
			b.WriteString("\\033")
		case '\'':
			b.WriteString("'\\''")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func workspaceGradientText(text, startHex, endHex string) string {
	if text == "" {
		return ""
	}

	r1, g1, b1 := parseWorkspaceHex(startHex)
	r2, g2, b2 := parseWorkspaceHex(endHex)

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
		cr := workspaceLerpByte(r1, r2, t)
		cg := workspaceLerpByte(g1, g2, t)
		cb := workspaceLerpByte(b1, b2, t)
		sb.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm%c", cr, cg, cb, ch))
		idx++
	}
	sb.WriteString("\033[39m")
	return sb.String()
}

func parseWorkspaceHex(hex string) (uint8, uint8, uint8) {
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var r, g, b uint8
	_, _ = fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

func workspaceLerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}
