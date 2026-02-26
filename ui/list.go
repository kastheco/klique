package ui

import (
	"errors"
	"fmt"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
)

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool
	focused       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int

	statusFilter StatusFilter // status filter (All or Active)
	sortMode     SortMode     // how instances are sorted
	allItems     []*session.Instance

	highlightKind  string // "plan", "topic", or "" (no highlight)
	highlightValue string // plan filename or topic name

	scrollOffset int // line offset from top of rendered item content
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
		focused:  true,
	}
}

func (l *List) SetFocused(focused bool) {
	l.focused = focused
}

// SetStatusFilter sets the status filter and rebuilds the filtered items.
func (l *List) SetStatusFilter(filter StatusFilter) {
	l.statusFilter = filter
	l.rebuildFilteredItems()
}

// CycleSortMode advances to the next sort mode and rebuilds.
func (l *List) CycleSortMode() {
	l.sortMode = (l.sortMode + 1) % 4
	l.rebuildFilteredItems()
}

// GetSortMode returns the current sort mode.
func (l *List) GetSortMode() SortMode {
	return l.sortMode
}

// GetStatusFilter returns the current status filter.
func (l *List) GetStatusFilter() StatusFilter {
	return l.statusFilter
}

// GetSelectedIdx returns the index of the currently selected item in the filtered list.
func (l *List) GetSelectedIdx() int {
	return l.selectedIdx
}

// SelectedIndex returns the current selection index (alias for GetSelectedIdx).
func (l *List) SelectedIndex() int {
	return l.selectedIdx
}

// CycleNext selects the next instance, wrapping to the beginning at the end.
func (l *List) CycleNext() {
	if len(l.items) == 0 {
		return
	}
	l.selectedIdx = (l.selectedIdx + 1) % len(l.items)
	l.ensureSelectedVisible()
}

// CyclePrev selects the previous instance, wrapping to the end at the beginning.
func (l *List) CyclePrev() {
	if len(l.items) == 0 {
		return
	}
	l.selectedIdx = (l.selectedIdx - 1 + len(l.items)) % len(l.items)
	l.ensureSelectedVisible()
}

// CycleNextActive selects the next non-paused instance, wrapping around.
// If no active instance exists, selection stays unchanged.
func (l *List) CycleNextActive() {
	n := len(l.items)
	if n == 0 {
		return
	}
	for i := 1; i <= n; i++ {
		idx := (l.selectedIdx + i) % n
		if !l.items[idx].Paused() {
			l.selectedIdx = idx
			l.ensureSelectedVisible()
			return
		}
	}
}

// CyclePrevActive selects the previous non-paused instance, wrapping around.
// If no active instance exists, selection stays unchanged.
func (l *List) CyclePrevActive() {
	n := len(l.items)
	if n == 0 {
		return
	}
	for i := 1; i <= n; i++ {
		idx := (l.selectedIdx - i + n) % n
		if !l.items[idx].Paused() {
			l.selectedIdx = idx
			l.ensureSelectedVisible()
			return
		}
	}
}

// allTabText and activeTabText are the rendered tab labels with hotkey indicators.
const allTabText = "1 All"
const activeTabText = "2 Active"

// HandleTabClick checks if a click at the given local coordinates (relative to the
// list's top-left corner) hits a filter tab. Returns the filter and true if a tab was
// clicked, or false if the click was outside the tab area.
func (l *List) HandleTabClick(localX, localY int) (StatusFilter, bool) {
	// The tab row is rendered near the top (inside the bordered panel). Accept
	// clicks on rows 1-3 to cover the tab area generously, since the exact row
	// depends on how lipgloss.Place renders the output.
	if localY < 1 || localY > 3 {
		return 0, false
	}

	// Tab widths include Padding(0,1) so 1 char padding on each side.
	allWidth := len(allTabText) + 2       // "1 All" + 2 padding = 7
	activeWidth := len(activeTabText) + 2 // "2 Active" + 2 padding = 10

	if localX >= 0 && localX < allWidth {
		return StatusFilterAll, true
	} else if localX >= allWidth && localX < allWidth+activeWidth {
		return StatusFilterActive, true
	}
	return 0, false
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	// Renderer content width must fit inside the border (borderH=6 removes border+padding+gap)
	// AND inside item styles which add 2 chars of horizontal padding (1 left + 1 right).
	// So: width - 6 (border) - 2 (item padding) = width - 8.
	// AdjustPreviewWidth subtracts 2, so pass width - 6.
	l.renderer.setWidth(width - 6)
	l.ensureSelectedVisible()
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.allItems {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// Down selects the next item in the list.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx < len(l.items)-1 {
		l.selectedIdx++
	}
	l.ensureSelectedVisible()
}

// RemoveByTitle removes an instance from both allItems and items by title.
// It does NOT kill the tmux session — the caller is responsible for that.
// Returns the removed instance, or nil if not found.
func (l *List) RemoveByTitle(title string) *session.Instance {
	var target *session.Instance
	for i, inst := range l.allItems {
		if inst.Title == title {
			target = inst
			l.allItems = append(l.allItems[:i], l.allItems[i+1:]...)
			break
		}
	}
	if target == nil {
		return nil
	}
	for i, inst := range l.items {
		if inst == target {
			// Adjust selection if removing at or after selected index.
			if i <= l.selectedIdx && l.selectedIdx > 0 {
				l.selectedIdx--
			}
			l.items = append(l.items[:i], l.items[i+1:]...)
			break
		}
	}
	return target
}

// Remove dismisses the currently selected instance from the list without
// touching the tmux session or worktree. Use this to clear finished instances.
func (l *List) Remove() {
	if len(l.items) == 0 {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// If removing the last item, select the previous one.
	if l.selectedIdx == len(l.items)-1 {
		defer l.Up()
	}

	// Unregister the repo name.
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// Remove from both items and allItems.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)
	for i, inst := range l.allItems {
		if inst == targetInstance {
			l.allItems = append(l.allItems[:i], l.allItems[i+1:]...)
			break
		}
	}
}

// Kill removes and kills the currently selected instance.
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// Kill the tmux session
	if err := targetInstance.Kill(); err != nil {
		log.ErrorLog.Printf("could not kill instance: %v", err)
	}

	// If you delete the last one in the list, select the previous one.
	if l.selectedIdx == len(l.items)-1 {
		defer l.Up()
	}

	// Unregister the reponame.
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// Remove from both items and allItems
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)
	for i, inst := range l.allItems {
		if inst == targetInstance {
			l.allItems = append(l.allItems[:i], l.allItems[i+1:]...)
			break
		}
	}
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx > 0 {
		l.selectedIdx--
	}
	l.ensureSelectedVisible()
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.allItems = append(l.allItems, instance)
	l.rebuildFilteredItems()
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
	}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 {
		return nil
	}
	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// SelectInstance finds the given instance in the filtered/sorted list and selects it.
// Returns true if found. This is sort-order safe unlike SetSelectedInstance(index).
func (l *List) SelectInstance(inst *session.Instance) bool {
	for i, item := range l.items {
		if item == inst {
			l.selectedIdx = i
			return true
		}
	}
	return false
}

// GetInstances returns all instances (unfiltered) for persistence and metadata updates.
func (l *List) GetInstances() []*session.Instance {
	return l.allItems
}

// TotalInstances returns the total number of instances regardless of filter.
func (l *List) TotalInstances() int {
	return len(l.allItems)
}

// SetFilter triggers a rebuild of the filtered item list.
// The topicFilter parameter is accepted for API compatibility but is no longer
// used for filtering — highlight-boost (SetHighlightFilter) handles tree-mode
// selection, and flat-mode topic filtering has been superseded.
func (l *List) SetFilter(_ string) {
	l.rebuildFilteredItems()
}

// SetHighlightFilter sets the highlight context from sidebar selection.
// kind is "plan" or "topic", value is the plan filename or topic name.
// Empty kind means no highlight (all items render normally).
func (l *List) SetHighlightFilter(kind, value string) {
	l.highlightKind = kind
	l.highlightValue = value
	l.rebuildFilteredItems()
}

// IsHighlighted returns true if the instance matches the current highlight filter.
// Returns true for all instances when no filter is active.
func (l *List) IsHighlighted(inst *session.Instance) bool {
	if l.highlightKind == "" || l.highlightValue == "" {
		return true
	}
	return l.matchesHighlight(inst)
}

func (l *List) matchesHighlight(inst *session.Instance) bool {
	switch l.highlightKind {
	case "plan":
		return inst.PlanFile == l.highlightValue
	case "topic":
		return inst.Topic == l.highlightValue
	}
	return false
}

// GetFilteredInstances returns the current display list (filtered and sorted).
func (l *List) GetFilteredInstances() []*session.Instance {
	return l.items
}

// SetSearchFilter filters instances by search query across all topics.
// SetSearchFilter filters instances by search query across all topics.
func (l *List) SetSearchFilter(query string) {
	l.SetSearchFilterWithTopic(query, "")
}

// SetSearchFilterWithTopic filters instances by search query, optionally scoped to a topic.
// topicFilter: "" = all topics, "__ungrouped__" = ungrouped only, otherwise = specific topic.
func (l *List) SetSearchFilterWithTopic(query string, topicFilter string) {
	filtered := make([]*session.Instance, 0)
	for _, inst := range l.allItems {
		// Check status filter
		if l.statusFilter == StatusFilterActive && inst.Paused() {
			continue
		}
		// Topic filter is no longer instance-based; skip it.
		// Then check search query
		if query == "" ||
			strings.Contains(strings.ToLower(inst.Title), query) {
			filtered = append(filtered, inst)
		}
	}
	l.items = filtered
	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
}

// Clear removes all instances from the list.
func (l *List) Clear() {
	l.allItems = nil
	l.items = nil
	l.selectedIdx = 0
}

func (l *List) rebuildFilteredItems() {
	// Apply status filter
	var filtered []*session.Instance
	if l.statusFilter == StatusFilterActive {
		for _, inst := range l.allItems {
			if !inst.Paused() {
				filtered = append(filtered, inst)
			}
		}
	} else {
		filtered = l.allItems
	}

	// Partition into matched/unmatched if highlight is active
	if l.highlightKind != "" && l.highlightValue != "" {
		var matched, unmatched []*session.Instance
		for _, inst := range filtered {
			if l.matchesHighlight(inst) {
				matched = append(matched, inst)
			} else {
				unmatched = append(unmatched, inst)
			}
		}
		l.items = matched
		l.sortItems()
		sortedMatched := make([]*session.Instance, len(l.items))
		copy(sortedMatched, l.items)

		l.items = unmatched
		l.sortItems()

		l.items = append(sortedMatched, l.items...)
	} else {
		l.items = filtered
		l.sortItems()
	}

	l.selectedIdx = 0
	l.scrollOffset = 0
	l.ensureSelectedVisible()
}

func (l *List) sortItems() {
	switch l.sortMode {
	case SortNewest:
		sort.SliceStable(l.items, func(i, j int) bool {
			return l.items[i].UpdatedAt.After(l.items[j].UpdatedAt)
		})
	case SortOldest:
		sort.SliceStable(l.items, func(i, j int) bool {
			return l.items[i].CreatedAt.Before(l.items[j].CreatedAt)
		})
	case SortName:
		sort.SliceStable(l.items, func(i, j int) bool {
			return strings.ToLower(l.items[i].Title) < strings.ToLower(l.items[j].Title)
		})
	case SortStatus:
		sort.SliceStable(l.items, func(i, j int) bool {
			return l.items[i].Status < l.items[j].Status
		})
	}
}
