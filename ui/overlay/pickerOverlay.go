package overlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// PickerOverlay shows a searchable list of options for selection.
type PickerOverlay struct {
	title       string
	allItems    []string
	filtered    []string
	selectedIdx int
	searchQuery string
	width       int
	submitted   bool
	cancelled   bool
	allowCustom bool // when true, typing a non-matching query offers "Create: <query>"
}

// NewPickerOverlay creates a picker with a title and list of items.
func NewPickerOverlay(title string, items []string) *PickerOverlay {
	filtered := make([]string, len(items))
	copy(filtered, items)
	return &PickerOverlay{
		title:    title,
		allItems: items,
		filtered: filtered,
		width:    40,
	}
}

// SetTitle updates the picker's title text (used when AI title arrives async).
func (p *PickerOverlay) SetTitle(title string) {
	p.title = title
}

// SetAllowCustom enables free-text entry when the search query doesn't match any item.
func (p *PickerOverlay) SetAllowCustom(allow bool) {
	p.allowCustom = allow
}

const customPrefix = "+ Create: "

func (p *PickerOverlay) applyFilter() {
	if p.searchQuery == "" {
		p.filtered = make([]string, len(p.allItems))
		copy(p.filtered, p.allItems)
	} else {
		query := strings.ToLower(p.searchQuery)
		p.filtered = nil
		for _, item := range p.allItems {
			if strings.Contains(strings.ToLower(item), query) {
				p.filtered = append(p.filtered, item)
			}
		}
		// When allowCustom is on and query doesn't exactly match an existing item,
		// offer to create a new entry with the raw query text.
		if p.allowCustom && !p.hasExactMatch() {
			p.filtered = append(p.filtered, customPrefix+p.searchQuery)
		}
	}
	if p.selectedIdx >= len(p.filtered) {
		p.selectedIdx = len(p.filtered) - 1
	}
	if p.selectedIdx < 0 {
		p.selectedIdx = 0
	}
}

// hasExactMatch returns true if any item matches the search query exactly (case-insensitive).
func (p *PickerOverlay) hasExactMatch() bool {
	query := strings.ToLower(p.searchQuery)
	for _, item := range p.allItems {
		if strings.ToLower(item) == query {
			return true
		}
	}
	return false
}

// Value returns the selected item, or empty string if cancelled or nothing selected.
// When a custom "Create: <name>" entry is selected, returns just the name.
func (p *PickerOverlay) Value() string {
	if p.cancelled || len(p.filtered) == 0 {
		return ""
	}
	val := p.filtered[p.selectedIdx]
	if strings.HasPrefix(val, customPrefix) {
		return strings.TrimPrefix(val, customPrefix)
	}
	return val
}

// SetSize implements Overlay. Updates available dimensions; width is used for layout.
func (p *PickerOverlay) SetSize(width, height int) {
	p.width = width
}

// HandleKey implements Overlay. It processes a key event and returns a Result
// indicating whether the overlay should close and what was selected.
func (p *PickerOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	switch msg.String() {
	case "esc":
		p.cancelled = true
		return Result{Dismissed: true, Submitted: false}
	case "enter":
		p.submitted = true
		return Result{Dismissed: true, Submitted: true, Value: p.Value()}
	case "up", "shift+tab":
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
	case "down", "tab":
		if p.selectedIdx < len(p.filtered)-1 {
			p.selectedIdx++
		}
	case "backspace":
		if len(p.searchQuery) > 0 {
			runes := []rune(p.searchQuery)
			p.searchQuery = string(runes[:len(runes)-1])
			p.applyFilter()
		}
	default:
		if len(msg.Text) > 0 {
			p.searchQuery += msg.Text
			p.applyFilter()
		}
	}
	return Result{}
}

// View implements Overlay using DefaultStyles for rendering.
func (p *PickerOverlay) View() string {
	st := DefaultStyles()
	var b strings.Builder

	b.WriteString(st.Title.Render(p.title))
	b.WriteString("\n")

	innerWidth := p.width - 8
	if innerWidth < 10 {
		innerWidth = 10
	}
	searchText := p.searchQuery
	if searchText == "" {
		searchText = "\uf002 Type to filter..."
	}
	b.WriteString(st.SearchBar.Width(innerWidth).Render(searchText))
	b.WriteString("\n")

	if len(p.filtered) == 0 {
		b.WriteString(st.Hint.Render("  No matches"))
		b.WriteString("\n")
	} else {
		for i, item := range p.filtered {
			if i == p.selectedIdx {
				b.WriteString(st.SelectedItem.Width(innerWidth).Render("▸ " + item))
			} else {
				b.WriteString(st.Item.Width(innerWidth).Render("  " + item))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(st.Hint.Render("↑↓ navigate • enter select • esc cancel"))

	return st.FloatingBorder.Width(p.width).Render(b.String())
}
