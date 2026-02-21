package ui

import "strings"

// The base KLIQUE banner — 6 rows tall.
var fallbackBannerRaw = `██╗  ██╗██╗     ██╗ ██████╗ ██╗   ██╗███████╗
██║ ██╔╝██║     ██║██╔═══██╗██║   ██║██╔════╝
█████╔╝ ██║     ██║██║   ██║██║   ██║█████╗
██╔═██╗ ██║     ██║██║▄▄ ██║██║   ██║██╔══╝
██║  ██╗███████╗██║╚██████╔╝╚██████╔╝███████╗
╚═╝  ╚═╝╚══════╝╚═╝ ╚══▀▀═╝  ╚═════╝ ╚══════╝`

// Block-art glyphs, each 6 rows to match the banner height.
// period: small block sitting at the bottom.
var blockPeriod = [6]string{
	"   ",
	"   ",
	"   ",
	"   ",
	"██╗",
	"╚═╝",
}

// bannerFrames are precomputed gradient-rendered banner strings.
// Animation: base → . → .. → ... → .. → . → (loop)
var bannerFrames = func() []string {
	base := strings.Split(fallbackBannerRaw, "\n")

	type glyph = [6]string
	suffixes := [][]glyph{
		{},                                      // KLIQUE
		{blockPeriod},                           // KLIQUE.
		{blockPeriod, blockPeriod},              // KLIQUE..
		{blockPeriod, blockPeriod, blockPeriod}, // KLIQUE...
	}

	frames := make([]string, len(suffixes))
	for i, glyphs := range suffixes {
		lines := make([]string, 6)
		copy(lines, base)
		for _, g := range glyphs {
			for row := 0; row < 6; row++ {
				lines[row] += " " + g[row]
			}
		}
		frames[i] = GradientText(strings.Join(lines, "\n"), GradientStart, GradientEnd)
	}
	return frames
}()

// FallBackText returns the precomputed banner frame for the given tick.
func FallBackText(frame int) string {
	return bannerFrames[frame%len(bannerFrames)]
}
