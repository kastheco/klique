---
name: TUI Design
description: >
  This skill should be used when the user asks to "create a TUI",
  "build a terminal UI", "design a bubbletea app", "style with lipgloss",
  "add bubbles components", "design a dashboard", "make a terminal dashboard",
  "terminal color palette", "TUI keybindings", "responsive terminal layout",
  "layout terminal panels", "style a charm TUI", "viewport scrolling",
  or is building any Go full-screen terminal application with the Charm ecosystem
  (bubbletea, lipgloss, bubbles, huh, wish). Provides design-first workflow
  for aesthetically excellent terminal interfaces.
---

# TUI Design with Go & Charm

Guide creation of aesthetically excellent terminal UIs using Go, bubbletea, lipgloss, bubbles, and the broader Charm ecosystem. Most AI-generated TUI code produces functional but visually generic interfaces. This skill ensures TUIs that look designer-built.

## The Design-First Mandate

**No code before a design pass.** When building a TUI, produce these artifacts before writing any Go:

1. **Aesthetic identity** -- one sentence describing the feel (e.g., "minimal, muted, data-dense like k9s" or "warm, approachable, spacious like soft-serve")
2. **Color palette** -- named roles with specific ANSI 256 / hex values (see Color System below)
3. **Layout sketch** -- ASCII art showing the interface at 80x24 minimum and 120x40 standard
4. **Component inventory** -- which bubbles components are used, where, and why
5. **Interaction vocabulary** -- the keybinding grammar grouped by mode

Only after design approval (or explicit skip) begin writing Go code. Before generating any code, always use context7 MCP tools (`resolve-library-id` then `query-docs`) to pull current Charm library APIs -- the ecosystem evolves rapidly.

## TUI Slop: Anti-Patterns to Actively Avoid

These are the terminal equivalent of "Inter font + purple gradient on white." Recognizing and avoiding them is the single highest-leverage improvement.

### 1. The Unstyled Bubbles Dump
Dropping `bubbles/table` or `bubbles/list` with zero lipgloss customization. Default gray borders, default white text, default selection highlight. These components are scaffolding, not finished UI. Every component requires intentional styling -- custom delegates, styled headers, semantic foreground colors.

### 2. Border Everywhere Disease
Applying `lipgloss.RoundedBorder()` to every panel. Borders create visual weight; used everywhere, they create noise instead of structure. Polished Charm apps use borders sparingly -- often just one outer container. Separate inner panels with spacing, color shifts, or thin divider lines (`strings.Repeat("─", width)` in a muted style).

### 3. Status Bar Overload
Cramming 12 pieces of information into one status line. Prioritize ruthlessly: 3-5 elements maximum with clear visual hierarchy. Left-align primary status, right-align help hints, use a gap-fill pattern between them.

### 4. Color Without System
Random ad-hoc color values scattered across the codebase. `lipgloss.Color("9")` here, `Color("#FF5555")` there -- all "red" but visually incoherent. Define a named palette struct at the package level. Every color in the app references that palette, never a raw literal.

### 5. Ignoring Terminal Background
Hardcoding colors that work on dark terminals but vanish on light ones. Always use `lipgloss.AdaptiveColor{Light: "...", Dark: "..."}` for any text-facing color. Test with both backgrounds.

### 6. Width Blindness
Components that ignore `tea.WindowSizeMsg`. Text that wraps randomly on narrow terminals. No minimum dimension protection. Every component must update its dimensions in `Update` when `WindowSizeMsg` arrives. Impose minimums and degrade gracefully.

### 7. The Phantom Help Bar
A help bar listing 15 keybindings in dense tiny text nobody can parse. Either use `bubbles/help` properly (short/long toggle via `?`) or design help as a full overlay with visual grouping. The status bar shows at most 3-4 critical bindings.

### 8. Spinner as Afterthought
Using `spinner.Dot` (the default) for everything. The 12+ spinner styles have different visual weight and rhythm. Match the spinner to context: `MiniDot` for inline table cells, `Points` for status bar ambient state, `Line` for foregrounded loading sequences.

## Terminal Aesthetic Principles

### Visual Hierarchy Without Depth

In web UI there are shadows, z-index, blur. In terminals there are four tools:

1. **Color brightness** -- dim secondary content with `Color("240")`, keep primary near-white. This is the most powerful hierarchy signal.
2. **Bold** -- use sparingly. Reserve for labels, section headers, selected items. Bold everything and nothing is bold.
3. **Spatial separation** -- two blank lines between major sections beats a thin border. Whitespace is free and clean.
4. **Unicode weight** -- `●` vs `○`, `▶` vs `›`, block chars vs dots. Heavier glyphs draw the eye.

What does NOT create hierarchy: italic (barely visible in most terminals), underline (looks like a hyperlink), blink (never use).

### Character-Level Precision

Every character is a design decision. The Unicode vocabulary for TUI design:
- **State indicators**: `● ○ ◉ ✓ ✗ ⊘ ⋯ ▶ ■ □`
- **Structural**: `─ │ ╌ · ┊ ╭ ╮ ╰ ╯`
- **Braille/blocks**: `⠋ ⠙ ⠹ ▓ ░ █ ▏ ▎ ▍`
- **Arrows/pointers**: `› » ▸ ◂ ↑ ↓`

Choosing `●` vs `◉` vs `■` for a selected state matters. Build a consistent glyph vocabulary and stick to it.

### Rhythm and Alignment

Vertically aligned columns, consistent padding (pick 1 or 2 characters and use it everywhere), consistent gutter widths. The underlying grid discipline should be invisible but felt. Monospace fonts make alignment trivial -- leverage this.

### The 60-Second Rule

A new user should understand the complete interaction model within 60 seconds. This requires: progressive disclosure (common actions visually obvious, advanced actions discoverable), contextual hints that appear when relevant, and a sensible default focus state on launch.

## Layout System

The canonical TUI layout is a vertical stack:

```
[Header / Title Bar]        -- 1-2 lines
[Main Content Area]         -- fills remaining height
[Status Bar]                -- 1 line
```

The main content area splits horizontally based on terminal width:

| Breakpoint | Layout | Split |
|---|---|---|
| Narrow (<100 cols) | Single column, tab-switch between views | 100% |
| Standard (100-140) | Master-detail split | 40% / 60% |
| Wide (>140 cols) | Three-column | 20% / 35% / 45% |

Compute all dimensions in the `tea.WindowSizeMsg` handler within `Update`. Never compute layout in `View` -- it runs every frame and must be cheap. Propagate sizes to child components via `.SetWidth()` / `.SetHeight()` / `.SetSize()`.

## Color System

Define a named palette, not ad-hoc colors. Every color in the app references the palette, never a raw literal. Required roles:

- **Base** -- terminal default (transparent)
- **Surface** -- subtle panel backgrounds
- **Divider** -- box-drawing, dividers (dim)
- **Primary** -- brand/identity, interactive elements (one color)
- **Muted** -- secondary text, timestamps
- **Text** -- primary readable content
- **Success / Warning / Error / Info** -- semantic state colors
- **Active / Inactive** -- running vs pending states
- **SelBg / SelFg** -- focused row highlight

See `references/design-principles.md` for the canonical `Palette` struct with `AdaptiveColor` values and a worked example palette. See `references/lipgloss-styling.md` for style composition patterns using the palette.

Principles:
- **Muted over saturated** -- `#E88388` (soft red) communicates "error" without visual aggression. `#FF0000` looks cheap.
- **Primary color for 1-2 things maximum** -- everything else achromatic. This is what makes soft-serve and k9s look polished.
- **ANSI 256 fallbacks** -- use `CompleteAdaptiveColor` for terminals without true color support.
- **Test both backgrounds** -- every `AdaptiveColor` needs a Light and Dark variant.

## Interaction Vocabulary

Core keybinding principles:

- **vim-motion defaults**: `j/k` navigation, `/` search, `g/G` jump to top/bottom, `h/l` or `Esc/Enter` for back/forward
- **Modal clarity**: The current mode must always be visually unambiguous. Change the header color, show a mode indicator, or both.
- **Destructive actions**: Require confirmation but not modal dialogs. Use a status bar prompt with a double-key pattern (press `K` once to arm, `K` again within 3 seconds to confirm, `Esc` to cancel).
- **Universal keys**: `?` toggles help, `Esc` goes back/cancels, `q` quits (with confirmation if state would be lost).
- **Avoid terminal collisions**: Never bind `Ctrl+W`, `Ctrl+T`, `Ctrl+Shift+*` -- terminal emulators intercept these.

## Reference Files

Load these on demand when needed, not all at once:

| File | Content | Load When |
|---|---|---|
| **`references/design-principles.md`** | Terminal aesthetics deep dive: hierarchy, color construction, typography, animation, analysis of polished TUIs | Designing a new application's visual identity |
| **`references/bubbletea-patterns.md`** | Elm architecture, subprocess Cmd patterns, sub-model delegation, modal state machines, daemon mode, performance | Implementing the bubbletea Model/Update/View |
| **`references/lipgloss-styling.md`** | Style composition, layout recipes, state indicators, table choice guide, border discipline | Writing View() functions and styling components |
| **`references/component-guide.md`** | Bubbles component selection matrix, list delegates, viewport streaming, spinner styles, huh forms | Choosing and configuring specific components |
| **`references/ux-patterns.md`** | Keybinding grammar, focus rings, destructive confirmations, empty states, error tiers, help overlays, responsive layout | Designing interaction patterns and keybind maps |
