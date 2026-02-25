package ui

import (
	"fmt"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

func (r *InstanceRenderer) Render(i *session.Instance, selected bool, focused bool, hasMultipleRepos bool, rowIndex int, highlighted bool) string {
	prefix := " "
	titleS := selectedTitleStyle
	descS := selectedDescStyle

	// Implementation-complete instances use dim styles unless selected.
	if i.ImplementationComplete && !selected {
		titleS = completedTitleStyle
		descS = completedDescStyle
	} else if selected && !focused {
		// Active but unfocused — muted highlight
		titleS = activeTitleStyle
		descS = activeDescStyle
	} else if !selected {
		if rowIndex%2 == 1 {
			titleS = evenRowTitleStyle
			descS = evenRowDescStyle
		} else {
			titleS = titleStyle
			descS = listDescStyle
		}
	}

	// Dim non-highlighted instances when a highlight filter is active
	if !highlighted && !selected {
		titleS = dimmedTitleStyle
		descS = dimmedDescStyle
	}

	// add spinner next to title if it's running
	// Inherit titleS background so status icons don't break the row color.
	titleBg := titleS.GetBackground()
	var join string
	if i.ImplementationComplete {
		join = completedStyle.Background(titleBg).Render(completedIcon)
	} else {
		switch i.Status {
		case session.Running, session.Loading:
			join = lipgloss.NewStyle().Background(titleBg).Render(fmt.Sprintf("%s ", r.spinner.View()))
		case session.Ready:
			if i.Notified {
				t := (math.Sin(float64(time.Now().UnixMilli())/300.0) + 1.0) / 2.0
				cr := lerpByte(0x51, 0xF0, t)
				cg := lerpByte(0xBD, 0xA8, t)
				cb := lerpByte(0x68, 0x68, t)
				pulseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", cr, cg, cb))).Background(titleBg)
				join = pulseStyle.Render(readyIcon)
			} else {
				join = readyStyle.Background(titleBg).Render(readyIcon)
			}
		case session.Paused:
			join = pausedStyle.Background(titleBg).Render(pausedIcon)
		default:
		}
	}

	// For plan-managed instances, show a clean role-based title instead of the
	// full plan-name prefix. The plan name is already on the branch line.
	titleText := i.Title
	switch {
	case i.WaveNumber > 0 && i.TaskNumber > 0:
		titleText = fmt.Sprintf("wave %d · task %d", i.WaveNumber, i.TaskNumber)
	case i.AgentType == session.AgentTypeReviewer && i.PlanFile != "":
		titleText = "review"
	case i.AgentType == session.AgentTypeCoder && i.PlanFile != "" && i.WaveNumber == 0:
		titleText = "applying fixes"
	}

	widthAvail := r.width - 3 - runewidth.StringWidth(prefix) - 1
	if widthAvail > 0 && runewidth.StringWidth(titleText) > widthAvail {
		titleText = runewidth.Truncate(titleText, widthAvail-3, "...")
	}

	// Add skip-permissions indicator
	skipPermsIndicator := ""
	if i.SkipPermissions {
		skipPermsIndicator = " \uf132"
	}

	titleContentPlain := fmt.Sprintf("%s %s%s", prefix, titleText, skipPermsIndicator)
	titleContent := fmt.Sprintf("%s %s%s", prefix, titleText, skipPermsIndicator)
	// Build title line: content + spaces + status icon, all fitting within r.width
	titleContentWidth := runewidth.StringWidth(titleContentPlain)
	joinWidth := runewidth.StringWidth(join)
	titlePad := r.width - titleContentWidth - joinWidth
	if titlePad < 1 {
		titlePad = 1
	}
	titleLine := titleContent + strings.Repeat(" ", titlePad) + join
	title := titleS.Width(r.width).Render(titleLine)

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := r.width
	remainingWidth -= runewidth.StringWidth(prefix)
	remainingWidth -= runewidth.StringWidth(branchIcon)

	diffWidth := runewidth.StringWidth(addedDiff) + runewidth.StringWidth(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() && hasMultipleRepos {
		repoName, err := i.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name in instance renderer: %v", err)
		} else {
			branch += fmt.Sprintf(" (%s)", repoName)
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	branchWidth := runewidth.StringWidth(branch)
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < branchWidth {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = runewidth.Truncate(branch, remainingWidth-3, "...")
		}
	}
	remainingWidth -= runewidth.StringWidth(branch)

	// Build activity indicator for running instances.
	var activityText string
	if i.Status == session.Running && i.LastActivity != nil {
		act := i.LastActivity
		if act.Detail != "" {
			activityText = fmt.Sprintf(" \u00b7 %s %s", act.Action, act.Detail)
		} else {
			activityText = fmt.Sprintf(" \u00b7 %s", act.Action)
		}
		activityWidth := runewidth.StringWidth(activityText)
		// Only show if there is enough room (at least the separator + a few chars).
		if activityWidth > remainingWidth-1 {
			// Truncate or drop if it doesn't fit.
			avail := remainingWidth - 1 // leave at least 1 space before diff
			if avail > 5 {
				activityText = " " + runewidth.Truncate(activityText[1:], avail-1, "...")
			} else {
				activityText = ""
			}
		}
		remainingWidth -= runewidth.StringWidth(activityText)
	}

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	// Render the activity text in a muted style.
	var renderedActivity string
	if activityText != "" {
		renderedActivity = activityStyle.Background(descS.GetBackground()).Render(activityText)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, renderedActivity, spaces, diff)

	// Build resource usage line for non-paused instances (third line)
	var resourceLine string
	if i.Status != session.Paused && i.MemMB > 0 {
		cpuText := fmt.Sprintf("\U000f0d46 %.0f%%", i.CPUPercent)
		memText := fmt.Sprintf("\uefc5 %.0fM", i.MemMB)
		resourceContent := fmt.Sprintf("%s %s  %s", strings.Repeat(" ", len(prefix)), cpuText, memText)
		resourcePad := r.width - runewidth.StringWidth(resourceContent)
		if resourcePad < 0 {
			resourcePad = 0
		}
		resourceLine = resourceStyle.Background(descS.GetBackground()).Render(resourceContent) + strings.Repeat(" ", resourcePad)
	}

	// join title, branch, and optionally resource line
	lines := []string{
		title,
		descS.Width(r.width).Render(branchLine),
	}
	if resourceLine != "" {
		lines = append(lines, descS.Width(r.width).Render(resourceLine))
	}
	text := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return text
}

func (l *List) String() string {
	const autoYesText = " auto-yes "

	// Border frame: 2 border + 2 padding = 4 chars horizontal, 2 chars vertical.
	const borderH = 4
	const borderV = 2

	// Inner width available inside the border.
	innerWidth := l.width - borderH
	if innerWidth < 8 {
		innerWidth = 8
	}

	// Build header separately (tabs + sort/autyes row + blank line).
	var header strings.Builder

	// Write filter tabs
	titleWidth := AdjustPreviewWidth(innerWidth) + 2

	allTab := inactiveFilterTab
	activeTab := inactiveFilterTab
	if l.statusFilter == StatusFilterAll {
		allTab = activeFilterTab
	} else {
		activeTab = activeFilterTab
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom,
		allTab.Render(allTabText),
		activeTab.Render(activeTabText),
	)

	sortLabel := sortDropdownStyle.Render("3 \uf0dc " + sortModeLabels[l.sortMode])

	if !l.autoyes {
		left := tabs
		right := sortLabel
		gap := titleWidth - runewidth.StringWidth(left) - runewidth.StringWidth(right)
		if gap < 1 {
			gap = 1
		}
		header.WriteString(left + strings.Repeat(" ", gap) + right)
	} else {
		left := tabs + " " + sortLabel
		autoYes := autoYesStyle.Render(autoYesText)
		gap := titleWidth - runewidth.StringWidth(left) - runewidth.StringWidth(autoYes)
		if gap < 1 {
			gap = 1
		}
		header.WriteString(left + strings.Repeat(" ", gap) + autoYes)
	}

	header.WriteString("\n")
	header.WriteString("\n")

	// Build item content separately.
	var content strings.Builder
	for i, item := range l.items {
		content.WriteString(l.renderer.Render(item, i == l.selectedIdx, l.focused, len(l.repos) > 1, i, l.IsHighlighted(item)))
		if i != len(l.items)-1 {
			content.WriteString("\n\n")
		}
	}

	// Slice content to visible window.
	allLines := strings.Split(content.String(), "\n")
	avail := l.availContentLines()
	start := l.scrollOffset
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + avail
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := strings.Join(allLines[start:end], "\n")

	// Wrap in border matching the sidebar style.
	borderStyle := listBorderStyle
	if l.focused {
		borderStyle = borderStyle.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	}
	innerHeight := l.height - borderV
	if innerHeight < 4 {
		innerHeight = 4
	}
	bordered := borderStyle.Width(innerWidth).Height(innerHeight).Render(header.String() + visible)
	placed := lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, bordered)

	// Hard-clip to l.height lines to prevent overflow from header wrapping or other layout quirks.
	placedLines := strings.Split(placed, "\n")
	if len(placedLines) > l.height {
		placedLines = placedLines[:l.height]
	}
	return strings.Join(placedLines, "\n")
}

// itemStartLine returns the line offset (0-based) where item idx begins
// in the rendered content buffer (items only, excluding the 2-line header).
func (l *List) itemStartLine(idx int) int {
	line := 0
	for i := 0; i < idx; i++ {
		line += l.itemHeight(i) + 1 // +1 for blank gap between items
	}
	return line
}

// availContentLines returns the number of lines available for item content
// inside the border, excluding the 2-line header (tabs + blank line).
func (l *List) availContentLines() int {
	const borderV = 2
	const headerLines = 2
	avail := l.height - borderV - headerLines
	if avail < 1 {
		avail = 1
	}
	return avail
}

// ensureSelectedVisible adjusts scrollOffset so the selected item is fully visible.
// When an item is taller than the available viewport (e.g. a tall item in a short panel),
// the item's top edge is preferred over its bottom edge.
func (l *List) ensureSelectedVisible() {
	if len(l.items) == 0 {
		l.scrollOffset = 0
		return
	}
	avail := l.availContentLines()
	start := l.itemStartLine(l.selectedIdx)
	end := start + l.itemHeight(l.selectedIdx) - 1

	if start < l.scrollOffset {
		l.scrollOffset = start
	}
	if end >= l.scrollOffset+avail {
		l.scrollOffset = end - avail + 1
		// If the item is taller than avail, prefer showing its top.
		if l.scrollOffset > start {
			l.scrollOffset = start
		}
	}
	if l.scrollOffset < 0 {
		l.scrollOffset = 0
	}
}

// itemHeight returns the rendered row count for an instance entry.
// Title style has Padding(1,0,0,1) top, desc style has Padding(0,1,1,1) bottom.
// The branch icon (U+F126) is a 2-column Nerd Fonts glyph. go-runewidth and
// lipgloss disagree on its width, causing the branch line to wrap onto two rows.
// 2-line item (title+branch) = 5 rows; 3-line (with resource) = 7 rows.
func (l *List) itemHeight(idx int) int {
	inst := l.items[idx]
	base := 5 // title (1 pad top + 1 content) + branch (2 content rows due to wrap + 1 pad bottom)
	if inst.Status != session.Paused && inst.MemMB > 0 {
		base += 2 // resource line (1 content + 1 pad bottom)
	}
	return base
}

// GetItemAtRow maps a row offset (relative to the first item) to an item index.
// Returns -1 if the row doesn't correspond to any item.
func (l *List) GetItemAtRow(row int) int {
	currentRow := 0
	for i := range l.items {
		h := l.itemHeight(i)
		if row >= currentRow && row < currentRow+h {
			return i
		}
		currentRow += h + 1 // +1 for the blank line gap between items
	}
	return -1
}
