package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/session"
	"github.com/mattn/go-runewidth"
)

// Exported diff line styles — callers may render individual lines with these.
var (
	AdditionStyle = lipgloss.NewStyle().Foreground(ColorDiffAdd)
	DeletionStyle = lipgloss.NewStyle().Foreground(ColorDiffDelete)
	HunkStyle     = lipgloss.NewStyle().Foreground(ColorDiffHunk)
)

// Unexported styles used internally by DiffPane rendering.
var (
	fileItemStyle = lipgloss.NewStyle().Foreground(ColorIris)

	fileItemSelectedStyle = lipgloss.NewStyle().
				Background(ColorIris).
				Foreground(ColorBase).
				Bold(true)

	fileItemDimStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	filePanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorOverlay)

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(ColorIris).
			Bold(true)

	diffHintStyle = lipgloss.NewStyle().Foreground(ColorMuted)
)

// fileChunk holds the parsed diff data for a single file.
type fileChunk struct {
	path    string
	added   int
	removed int
	diff    string
}

// DiffPane renders a two-column diff view: a file sidebar on the left and a
// scrollable diff viewport on the right.
type DiffPane struct {
	viewport viewport.Model
	width    int
	height   int

	files        []fileChunk
	totalAdded   int
	totalRemoved int
	fullDiff     string

	// selectedFile is -1 for "all files", or a 0-based index into files.
	selectedFile int

	// sidebarWidth is the total outer width of the sidebar (content + border).
	sidebarWidth int
}

// NewDiffPane creates a DiffPane ready for use.
func NewDiffPane() *DiffPane {
	return &DiffPane{
		viewport:     viewport.New(0, 0),
		selectedFile: 0,
	}
}

// SetSize updates the pane dimensions and re-flows the layout.
func (d *DiffPane) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.computeSidebarWidth()
	d.updateViewportWidth()
	d.viewport.Height = height
	d.rebuildViewport()
}

// computeSidebarWidth calculates the sidebar outer width from the longest file
// entry, capped at 35 % of the total pane width.
func (d *DiffPane) computeSidebarWidth() {
	hframe := filePanelBorderStyle.GetHorizontalFrameSize()

	innerMin := 18
	innerMax := innerMin
	for _, f := range d.files {
		base := filepath.Base(f.path)
		statsLen := len(fmt.Sprintf(" +%d -%d", f.added, f.removed))
		needed := runewidth.StringWidth(base) + statsLen + 4
		if needed > innerMax {
			innerMax = needed
		}
	}

	cap := d.width*35/100 - hframe
	if cap < innerMin {
		cap = innerMin
	}
	if innerMax > cap {
		innerMax = cap
	}
	d.sidebarWidth = innerMax + hframe
}

// updateViewportWidth sets the diff viewport width from the remaining space.
func (d *DiffPane) updateViewportWidth() {
	w := d.width - d.sidebarWidth - 1
	if w < 10 {
		w = 10
	}
	d.viewport.Width = w
}

// SetDiff loads diff data from the given instance into the pane.
func (d *DiffPane) SetDiff(instance *session.Instance) {
	if instance == nil || !instance.Started() {
		d.files = nil
		d.fullDiff = ""
		return
	}

	stats := instance.GetDiffStats()
	if stats == nil || stats.Error != nil || stats.IsEmpty() {
		d.files = nil
		d.fullDiff = ""
		if stats != nil && stats.Error != nil {
			d.fullDiff = fmt.Sprintf("Error: %v", stats.Error)
		}
		return
	}

	d.totalAdded = stats.Added
	d.totalRemoved = stats.Removed
	d.files = parseFileChunks(stats.Content)
	d.fullDiff = colorizeDiff(stats.Content)

	// Clamp selectedFile into the valid range.
	last := len(d.files) - 1
	if d.selectedFile > last {
		d.selectedFile = last
	}
	if d.selectedFile < -1 {
		d.selectedFile = -1
	}

	d.computeSidebarWidth()
	d.updateViewportWidth()
	d.rebuildViewport()
}

// rebuildViewport reloads the viewport content for the active selection.
func (d *DiffPane) rebuildViewport() {
	if len(d.files) == 0 {
		return
	}
	var content string
	switch {
	case d.selectedFile < 0:
		content = d.fullDiff
	case d.selectedFile < len(d.files):
		content = colorizeDiff(d.files[d.selectedFile].diff)
	}
	d.viewport.SetContent(content)
}

// String returns the rendered pane as a string.
func (d *DiffPane) String() string {
	if len(d.files) == 0 {
		msg := "No changes"
		if d.fullDiff != "" {
			msg = d.fullDiff
		}
		return lipgloss.Place(d.width, d.height, lipgloss.Center, lipgloss.Center, msg)
	}
	sidebar := d.renderSidebar()
	right := d.viewport.View()
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", right)
}

// renderSidebar builds the bordered file-list panel.
func (d *DiffPane) renderSidebar() string {
	hframe := filePanelBorderStyle.GetHorizontalFrameSize()
	innerW := d.sidebarWidth - hframe

	var sb strings.Builder

	// Header — coloured totals.
	addStr := AdditionStyle.Render(fmt.Sprintf("+%d", d.totalAdded))
	delStr := DeletionStyle.Render(fmt.Sprintf("-%d", d.totalRemoved))
	sb.WriteString(addStr + " " + delStr + "\n")

	// "All" entry.
	allLabel := "\uf0ce All"
	if d.selectedFile == -1 {
		sb.WriteString(fileItemSelectedStyle.Width(innerW).Render(" " + allLabel))
	} else {
		sb.WriteString(fileItemStyle.Render(" " + allLabel))
	}
	sb.WriteString("\n")

	// Per-file entries.
	for idx, f := range d.files {
		base := filepath.Base(f.path)
		dir := filepath.Dir(f.path)
		selected := idx == d.selectedFile

		// Build plain stats string for width calculations.
		var plainStats string
		if f.added > 0 {
			plainStats += fmt.Sprintf("+%d", f.added)
		}
		if f.removed > 0 {
			if plainStats != "" {
				plainStats += " "
			}
			plainStats += fmt.Sprintf("-%d", f.removed)
		}

		if selected {
			maxName := innerW - runewidth.StringWidth(plainStats) - 3
			name := base
			if maxName > 3 && runewidth.StringWidth(name) > maxName {
				name = runewidth.Truncate(name, maxName, "…")
			}
			line := fmt.Sprintf(" %s %s", name, plainStats)
			sb.WriteString(fileItemSelectedStyle.Width(innerW).Render(line))
		} else {
			maxName := innerW - runewidth.StringWidth(plainStats) - 3

			// Build coloured name with optional dim dir prefix.
			var nameDisplay string
			if dir == "." {
				name := base
				if maxName > 3 && runewidth.StringWidth(name) > maxName {
					name = runewidth.Truncate(name, maxName, "…")
				}
				nameDisplay = fileItemStyle.Render(name)
			} else {
				dirPrefix := dir + "/"
				remaining := maxName - runewidth.StringWidth(dirPrefix)
				if remaining < 4 {
					name := base
					if maxName > 3 && runewidth.StringWidth(name) > maxName {
						name = runewidth.Truncate(name, maxName, "…")
					}
					nameDisplay = fileItemStyle.Render(name)
				} else {
					name := base
					if runewidth.StringWidth(name) > remaining {
						name = runewidth.Truncate(name, remaining, "…")
					}
					nameDisplay = fileItemDimStyle.Render(dirPrefix) + fileItemStyle.Render(name)
				}
			}

			// Build coloured stats.
			var coloredStats string
			if f.added > 0 {
				coloredStats += AdditionStyle.Render(fmt.Sprintf("+%d", f.added))
			}
			if f.removed > 0 {
				if coloredStats != "" {
					coloredStats += " "
				}
				coloredStats += DeletionStyle.Render(fmt.Sprintf("-%d", f.removed))
			}

			sb.WriteString(" " + nameDisplay + " " + coloredStats)
		}
		sb.WriteString("\n")
	}

	// Pad to fill available height (border + hint accounted for).
	occupied := 2 + len(d.files) // header line + "All" + file entries
	for row := occupied; row < d.height-3; row++ {
		sb.WriteString("\n")
	}

	// Navigation hint at the bottom.
	sb.WriteString(diffHintStyle.Render("shift+↑↓"))

	vframe := filePanelBorderStyle.GetVerticalFrameSize()
	innerH := d.height - vframe
	if innerH < 1 {
		innerH = 1
	}
	return filePanelBorderStyle.Width(innerW).Height(innerH).Render(sb.String())
}

// FileUp moves the sidebar selection one entry upward (with wrap-around).
func (d *DiffPane) FileUp() {
	if len(d.files) == 0 {
		return
	}
	d.selectedFile--
	if d.selectedFile < -1 {
		d.selectedFile = len(d.files) - 1
	}
	d.rebuildViewport()
	d.viewport.GotoTop()
}

// FileDown moves the sidebar selection one entry downward (with wrap-around).
func (d *DiffPane) FileDown() {
	if len(d.files) == 0 {
		return
	}
	d.selectedFile++
	if d.selectedFile >= len(d.files) {
		d.selectedFile = -1
	}
	d.rebuildViewport()
	d.viewport.GotoTop()
}

// ScrollUp scrolls the diff viewport up by 3 lines.
func (d *DiffPane) ScrollUp() {
	d.viewport.LineUp(3)
}

// ScrollDown scrolls the diff viewport down by 3 lines.
func (d *DiffPane) ScrollDown() {
	d.viewport.LineDown(3)
}

// HasFiles reports whether any diff files are loaded.
func (d *DiffPane) HasFiles() bool {
	return len(d.files) > 0
}

// parseFileChunks splits a unified diff into per-file chunks, counting
// added/removed lines in each chunk.
func parseFileChunks(content string) []fileChunk {
	var chunks []fileChunk
	var cur *fileChunk
	var buf strings.Builder

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			// Flush previous chunk.
			if cur != nil {
				cur.diff = buf.String()
				buf.Reset()
			}
			// Extract the b/ path from "diff --git a/<path> b/<path>".
			path := ""
			if parts := strings.SplitN(line, " b/", 2); len(parts) == 2 {
				path = parts[1]
			}
			chunks = append(chunks, fileChunk{path: path})
			cur = &chunks[len(chunks)-1]
			buf.WriteString(line + "\n")
			continue
		}
		if cur == nil {
			continue
		}
		buf.WriteString(line + "\n")
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			cur.added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			cur.removed++
		}
	}
	if cur != nil {
		cur.diff = buf.String()
	}
	return chunks
}

// colorizeDiff applies ANSI colour styles to each line of a unified diff.
func colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var out strings.Builder
	out.Grow(len(diff) + len(lines)*8)
	for _, line := range lines {
		if len(line) == 0 {
			out.WriteByte('\n')
			continue
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			out.WriteString(HunkStyle.Render(line))
		case line[0] == '+' && (len(line) == 1 || line[1] != '+'):
			out.WriteString(AdditionStyle.Render(line))
		case line[0] == '-' && (len(line) == 1 || line[1] != '-'):
			out.WriteString(DeletionStyle.Render(line))
		default:
			out.WriteString(line)
		}
		out.WriteByte('\n')
	}
	return out.String()
}
