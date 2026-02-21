# Design Principles for Terminal UIs

Deep reference for terminal aesthetics, visual hierarchy, color construction, typography, animation, and analysis of polished TUI applications. Load this file when designing a new application's visual identity.

---

## 1. Terminal as a Design Medium

Treat the terminal as a constrained design medium with its own strengths, not as a degraded version of a GUI. The constraints are specific and absolute -- understanding them prevents fighting the medium and enables leveraging its unique qualities.

### Character-Cell Constraints

Accept the grid. Every terminal renders on a fixed character grid where each cell occupies approximately 8-16 pixels wide and 16-20 pixels tall depending on font and DPI. There is no sub-pixel positioning, no fractional spacing, no kerning control. Colors apply to entire cells -- foreground and background, nothing in between. A "1px border" does not exist; the thinnest possible divider is a full character wide (`│` or `─`), which at typical font sizes is 8-16 pixels. This is why borders carry so much visual weight in terminals and must be used deliberately.

Wide characters (CJK, some emoji) occupy exactly two cells. Combining characters (diacritics, some ZWJ sequences) occupy zero additional cells but render atop the previous character. Always use `lipgloss.Width()` or `runewidth.StringWidth()` for measuring string width -- never `len()`, never `utf8.RuneCountInString()`. A single miscounted rune breaks every alignment downstream.

### Terminal Capability Tiers

Design for capability tiers and degrade gracefully. Not all terminals are equal.

**Tier 1 -- True Color + Unicode + Wide Characters**
Full design freedom. 16.7 million colors, all Unicode glyphs, correct wide-character rendering. Kitty, Alacritty, WezTerm, Ghostty, iTerm2, Windows Terminal. This is the primary design target. Use hex colors (`Color("#5FB4E0")`), emoji status indicators, box-drawing characters, braille patterns -- the full vocabulary.

**Tier 2 -- 256-Color + Unicode**
Solid experience with minor palette compromises. Most modern Linux terminal emulators (GNOME Terminal, Konsole, xfce4-terminal), tmux with `set -g default-terminal "tmux-256color"`. Map hex colors to nearest ANSI 256 equivalents. The visual identity holds; only subtle gradations between similar shades may collapse.

**Tier 3 -- 256-Color + ASCII Only**
Fall back to ASCII box-drawing (`+`, `-`, `|`) instead of Unicode line-drawing characters. Replace emoji indicators with ASCII equivalents: `[x]` instead of `✓`, `[!]` instead of `⚠`, `*` instead of `●`. No wide characters. This tier covers older xterm configurations, some SSH sessions through legacy jump hosts, and screen (not tmux).

**Tier 4 -- 16-Color**
Minimal palette of the original 16 ANSI colors (8 normal + 8 bright). Rely entirely on structure, spatial hierarchy, bold, and dim attributes. The design must communicate through layout and weight rather than color nuance. This tier matters for bare Linux console (no framebuffer), very old terminals, and some CI/log-viewer contexts.

**Target Tier 1/2 as primary.** Provide Tier 3 graceful degradation where feasible. Tier 4 is a courtesy, not a requirement -- document that the application targets modern terminals.

### Detecting and Adapting

Use `lipgloss.HasDarkBackground()` to detect terminal background polarity at startup. Pair this with `lipgloss.CompleteAdaptiveColor` to provide full three-tier color fallback (true color, 256-color, and 16-color variants for both light and dark backgrounds) for every semantic color role. Query `$TERM` and `$COLORTERM` to determine capability tier, but prefer feature detection over string matching where the library supports it.

```go
accent := lipgloss.CompleteAdaptiveColor{
    Light: lipgloss.CompleteColor{TrueColor: "#1A6BC4", ANSI256: "32", ANSI: "4"},
    Dark:  lipgloss.CompleteColor{TrueColor: "#5FB4E0", ANSI256: "74", ANSI: "12"},
}
```

Store the detected background polarity once at startup and thread it through the palette rather than querying on every render.

---

## 2. Visual Hierarchy in Pure Text

Web and native UI have shadows, z-index, blur, transparency, and variable font weights. Terminals have none of these. Hierarchy must be constructed from five tools, listed here in order of perceptual power (strongest first).

### Tool 1: Color Brightness and Saturation

The single most powerful hierarchy signal. Dim secondary content to `Color("240")` (dark gray); render primary content near-white at `Color("252")` or `Color("255")`. The human eye resolves brightness differences faster than any other visual property. A 30-point gap in ANSI 256 grayscale (232-255 range) between "important" and "not important" text creates instant, effortless hierarchy without any other styling.

Saturation works similarly for chromatic elements. A muted `Color("241")` timestamp next to a bright `Color("#5FB4E0")` link creates hierarchy through saturation contrast alone, even at similar luminance.

Apply this principle aggressively: in a typical TUI, 60-70% of visible text should be in muted/secondary tones. Only the currently relevant, actionable content gets full brightness.

### Tool 2: Bold

Reserve bold for labels, section headers, selected items, and mode indicators. Bold text in a sea of regular text creates strong focal points. Bold text in a sea of bold text creates nothing -- the signal is lost. Audit every use: if removing the bold attribute from a particular element would not reduce clarity, remove it.

Bold works best as a binary toggle on small pieces of text (1-3 words). Bold entire paragraphs or data rows look heavy and defeat the purpose.

### Tool 3: Spatial Separation

Two blank lines between major sections beat one thin border for visual grouping. Whitespace is free, renders instantly, never has alignment bugs, works in every terminal, and creates cleaner visual rhythm than character-based dividers.

Use spatial separation as the primary structural tool:
- One blank line between items within a group
- Two blank lines between groups
- Three or more characters of horizontal padding between columns
- Consistent left margin (2-3 characters) from the terminal edge

When a divider is genuinely needed (e.g., separating a status bar from content), prefer a full-width thin line (`strings.Repeat("─", width)`) in a very muted color (`Color("237")`) over a box border. It provides separation without the visual weight of a full border.

### Tool 4: Unicode Glyph Weight

Different glyphs carry different visual weight. Exploit this deliberately:
- `●` (filled circle) draws the eye more than `○` (empty circle)
- `▶` is heavier than `›`, which is heavier than `>`
- `█` block characters create solid visual mass; `░` and `▒` create texture
- `⣿` braille patterns can simulate sub-cell density for progress indicators

Assign heavier glyphs to active/selected/important states and lighter glyphs to inactive/unselected/secondary states. A status column using `●` for running (bright green) and `○` for stopped (muted gray) communicates two dimensions of information (state + importance) in a single character.

### Tool 5: Indentation

Indentation creates group membership without any visible delimiter. Indent child items 2-4 characters from their parent. This leverages the same Gestalt principle (proximity) that makes code indentation readable.

Combine indentation with color dimming for nested hierarchies: each level slightly more muted than its parent. Two levels of nesting is the practical maximum in a terminal before horizontal space becomes critical.

### What Does NOT Work

**Italic** -- Barely visible in most terminal fonts. Many monospace fonts have no italic variant at all; the terminal fakes it with a slight slant that is nearly indistinguishable from regular weight. Never rely on italic as a sole differentiator.

**Underline** -- Reads as a hyperlink to anyone who has used a web browser in the past 25 years. Using underline for emphasis creates false affordance ("is this clickable?"). The only acceptable use is column headers in a table context, where the convention is established.

**Blink** -- An accessibility nightmare. Causes genuine problems for photosensitive users, violates WCAG, and is disabled by default in many terminals. Never use it.

---

## 3. Color Palette Construction

### Worked Example: Dark-Terminal-Primary Palette

Build a palette from structural roles, not aesthetic preferences. Start with the darkest elements (barely visible structure) and progress to the brightest (primary content and accents).

```
Panel dividers:   Color("237")      -- barely visible, structural only
Secondary text:   Color("241")      -- readable but visually recessed
Primary text:     Color("252")      -- near-white, comfortable reading brightness
Accent/identity:  Color("#5FB4E0")  -- cool blue, the app's single identity color
Success:          Color("#A8CC8C")  -- muted green, task complete
Warning:          Color("#DBAB79")  -- amber, needs attention
Error:            Color("#E88388")  -- soft red, something failed
Running/active:   Color("#89B4FA")  -- blue-tinted, process in flight
Pending/inactive: Color("243")      -- gray, waiting or disabled
Selection bg:     Color("237")      -- subtle highlight row
Selection fg:     Color("255")      -- white text on selection
```

### The Muted-Over-Saturated Principle

Saturated red (`#FF0000`) looks alarming and cheap -- it triggers a visceral "something is broken with the UI" response rather than "a task in this UI failed." Muted red (`#E88388`) communicates "error" without visual aggression. The same applies across the spectrum: `#00FF00` screams; `#A8CC8C` informs.

Desaturate all semantic colors by 30-40% from their pure form. The resulting palette feels intentional and cohesive. Saturated colors should appear only in exceptional circumstances (e.g., a critical alert that genuinely demands immediate attention, if even then).

### Light Terminal Support

Provide `AdaptiveColor` pairs for every palette role. Light terminal palettes invert the brightness hierarchy: structural elements become light gray instead of dark gray, and primary text becomes near-black instead of near-white.

```go
type Palette struct {
    Divider  lipgloss.AdaptiveColor
    Muted    lipgloss.AdaptiveColor
    Text     lipgloss.AdaptiveColor
    Accent   lipgloss.AdaptiveColor
    Success  lipgloss.AdaptiveColor
    Warning  lipgloss.AdaptiveColor
    Error    lipgloss.AdaptiveColor
    Active   lipgloss.AdaptiveColor
    Inactive lipgloss.AdaptiveColor
    SelBg    lipgloss.AdaptiveColor
    SelFg    lipgloss.AdaptiveColor
}

var DefaultPalette = Palette{
    Divider:  lipgloss.AdaptiveColor{Light: "254", Dark: "237"},
    Muted:    lipgloss.AdaptiveColor{Light: "245", Dark: "241"},
    Text:     lipgloss.AdaptiveColor{Light: "235", Dark: "252"},
    Accent:   lipgloss.AdaptiveColor{Light: "#1A6BC4", Dark: "#5FB4E0"},
    Success:  lipgloss.AdaptiveColor{Light: "#4E8A3E", Dark: "#A8CC8C"},
    Warning:  lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#DBAB79"},
    Error:    lipgloss.AdaptiveColor{Light: "#C4384B", Dark: "#E88388"},
    Active:   lipgloss.AdaptiveColor{Light: "#2B6CB0", Dark: "#89B4FA"},
    Inactive: lipgloss.AdaptiveColor{Light: "250", Dark: "243"},
    SelBg:    lipgloss.AdaptiveColor{Light: "255", Dark: "237"},
    SelFg:    lipgloss.AdaptiveColor{Light: "232", Dark: "255"},
}
```

Note the pattern: light-mode accent colors are darker and more saturated than their dark-mode counterparts, because they must contrast against a bright background. Muted colors shift in the opposite direction on the grayscale.

### Terminal Palettes as Starting Points

Popular terminal color schemes provide battle-tested color relationships:

- **Catppuccin** (Mocha/Macchiato/Latte) -- Excellent range of surface colors and well-balanced semantic palette. The four variants (Latte for light, Frappe/Macchiato/Mocha for dark) provide natural AdaptiveColor pairs. Strong community adoption means users' terminals may already harmonize.
- **Rose Pine** -- Muted, warm palette with distinctive rose and gold accents. Works well for applications with a softer, less technical aesthetic.
- **Dracula** -- High-contrast with saturated accents. Good for applications that need strong visual differentiation between many simultaneous states.
- **Gruvbox** -- Warm retro tones, strong dark/light duality. Distinctive amber/orange character suits certain tool aesthetics.
- **Tokyo Night** -- Cool blue-purple tones, modern feel, good balance between muted backgrounds and readable text.

Derive a custom palette rather than copying wholesale. Extract the *relationships* (the brightness gaps between surface levels, the saturation of semantic colors relative to text, the warmth/coolness of grays) and apply them to chosen identity colors. An app that uses the exact Catppuccin palette looks like "a Catppuccin-themed app" rather than having its own identity.

---

## 4. Typography in Terminals

### The Label/Value Pattern

Structure data display as explicit label/value pairs with distinct styling for each role. Render labels in dim/muted (`Color("241")`), values in primary text weight (`Color("252")`). "Worker:" in gray, "coder-1" in white. This pattern creates scannable information architecture -- the eye learns to skip the dim labels and find bright values.

Apply this consistently: `STATUS:` dim, `running` bright green. `DURATION:` dim, `4m23s` bright. `ERROR:` dim, `connection refused` bright red. The label is structural; the value is content.

### ALL-CAPS for Structural Labels

Use ALL-CAPS for section headers and structural labels: `WORKERS`, `OUTPUT`, `TASKS`. ALL-CAPS in a monospace context feels more intentional than title case. It signals "this is a label, not content" through a typographic convention that requires zero additional styling. Combined with muted color, ALL-CAPS labels become the scaffolding that organizes the interface without competing for attention with the data they label.

Reserve mixed-case and sentence-case for user-generated content, values, and messages. The visual distinction between `SECTION HEADER` and `actual content beneath it` is immediate.

### Monospace Alignment

Every character occupies the same width. This is the terminal's single greatest typographic advantage over proportional-font environments. Exploit it:
- Column alignment is trivial -- pad with spaces to a fixed width, and columns line up perfectly across every row.
- Decimal alignment in numeric columns is free.
- Table-like layouts without any table component -- just `fmt.Sprintf("%-12s %6d %s", ...)` with consistent format strings.
- Vertical pipelines (e.g., tree views, dependency graphs) align perfectly with simple indentation.

Use `lipgloss.Width()` for measuring and `lipgloss.PlaceHorizontal()` / padding for aligning. Never assume ASCII -- account for wide characters when calculating padding.

### Line Density

Limit data rows to 2-3 meaningful pieces of information per line for scannability. A row reading `● worker-1  idle  0 tasks  12m uptime` carries four data points and is near the legibility limit. Adding a fifth element (say, memory usage) creates visual congestion -- split into columns or move less-critical data to a detail view.

Status bars can carry higher density (5-7 elements) because they occupy a fixed, expected position and users learn their layout. Even so, separate elements with clear visual gaps -- use `lipgloss.JoinHorizontal()` with a spacer style of 2-3 characters minimum between groups.

### Consistent Padding

Pick one or two padding values and apply them everywhere. If the left margin is 2 characters, it is 2 characters on every line of every view. If the gap between columns is 3 characters, it is 3 characters in every table-like layout. This creates an invisible grid discipline -- the regularity is not consciously noticed but its absence is immediately felt as "something looks off."

Define padding constants at the package level:

```go
const (
    padLeft   = 2
    padRight  = 2
    colGap    = 3
    sectionGap = 1  // blank lines between sections
)
```

Reference these everywhere. Never write a literal space count in a format string without relating it to these constants.

---

## 5. What Makes Polished TUIs Polished

Analyze real applications to extract concrete, replicable patterns rather than vague principles.

### lazydocker

Strict panel grid layout. Borders appear only on the outer container of each panel, never on internal elements within a panel. This creates clear zone separation without border noise. Status indicators use filled circles (`●`) in green, red, and yellow with the container name immediately adjacent -- never verbose status text like "Status: Running." The selected item renders as a full-width background color change (the entire row highlights), not a `>` prefix cursor. This is critical: full-width selection communicates "this entire row is active" while a prefix cursor communicates "the cursor is here, somewhere." The footer contains a single `press ? for help` hint -- one source of keybinding discovery, not a dense key dump.

### k9s

No borders on the main list at all. The primary navigation indicator is a colored top bar spanning the full terminal width, with a distinct background color. This bar anchors the entire layout -- remove it and the interface loses its structural anchor. Column headers use bold combined with underline (one of the rare valid uses of underline). Error states manifest as a red background on the entire row, not just a red icon or text prefix. The full-row treatment makes errors impossible to miss even when scanning quickly. The header row's distinct background color creates a visual ledge that anchors all column data below it.

### soft-serve

Exceptional typography that demonstrates what is possible when borders are eliminated almost entirely. A consistent 3-character left margin runs through every view, creating a vertical rhythm line. Navigation is pure text highlight with zero border elements -- the selected item is brighter, everything else is dimmer. Color appears only for interactive and selected states; everything else renders in exactly two shades of gray (dim and regular). The result reads like a well-typeset document rather than software. This is the aesthetic to aspire to for tools that prioritize readability over information density.

### Common Patterns Across All

Extract the shared principles:

**Selection is full-width row background change.** Never use a `>` cursor prefix, never use only foreground color change on the text. Highlight the entire row with a subtle background color shift (`Color("237")` on a dark terminal) and brighten the text to maximum. This is the universal convention in polished TUIs.

**Error states affect the entire row or element, not just an icon.** A red `●` at the start of a row is easy to miss. A row with a red-tinted background is impossible to miss. Scale the visual treatment to match the severity: informational states can be icon-only, warnings get colored text, errors get row-level background treatment.

**Status bar is always exactly one line.** Not two, not zero, not variable. One line, pinned to the bottom, always visible. Left side: primary context (current mode, active filter, item count). Right side: help hint (`?` for help) and maybe one critical status indicator. The single-line constraint forces ruthless prioritization.

**Help is a separate mode, not an always-visible key dump.** Press `?` to enter help, see a well-organized overlay or panel with grouped keybindings, press `?` or `Esc` to return. Never display more than 3-4 keybindings in the always-visible UI. Users learn key bindings from the help mode; they do not read a 15-item footer on every frame.

**Primary accent color appears in 1-2 elements maximum.** Everything else is achromatic (grayscale). The accent color identifies interactive or selected elements. When primary blue appears on the selected row and the active tab and nowhere else, those elements pop. When primary blue appears on the header, the footer, the borders, the selected row, and the active tab, it becomes background noise and nothing pops. Constraint creates impact.

---

## 6. Animation and Motion

### Spinners: When and How

Use spinners exclusively for operations of genuinely uncertain duration. If a network call takes 100ms, do not animate -- render the stale state, then render the result. The spinner frame update and the result would race visually, creating a flash rather than a meaningful animation. The threshold for "worth animating" is roughly 500ms -- below that, the user perceives instantaneity or near-instantaneity and a spinner adds visual noise.

Match spinner rhythm to task tempo. Fast-iterating background tasks (file I/O, rapid API polling) suit fast spinners like `MiniDot` (80ms frame time) -- the rapid motion communicates "actively working." Long-running processes (builds, deployments, large data transfers) suit slower spinners like `Points` (200ms) or `Meter` (300ms) -- the slower rhythm communicates "working, be patient" without the anxious energy of rapid animation.

For inline status within a table row, use `MiniDot` or `Line` -- small, contained spinners that do not visually dominate the row. For a full-screen loading state, use `Dots` or `Globe` -- larger patterns that justify the empty screen real estate.

### Progress Bars: Honesty Over Aesthetics

Use `bubbles/progress` only when real percentage data is available. A download with `Content-Length` gets a progress bar. A compilation with a known number of files gets a progress bar. An API call with unknown response time does not -- use a spinner with a descriptive label instead.

Never fake progress. A progress bar that races to 90% and then stalls is worse than a spinner. It creates a false expectation ("almost done!") followed by uncertainty ("is it stuck?"). Honest spinners with text labels ("Compiling dependencies...", "Waiting for API response...") give more useful information than a dishonest progress bar.

When using progress bars, update smoothly. Jumping from 0% to 47% to 100% in three frames looks broken. If only coarse progress data is available (e.g., 3 of 10 files processed), consider a step indicator (`[3/10]`) instead of a smooth bar.

### Structural Changes: Always Immediate

Never animate layout changes. Panel resizes, view switches, tab changes, modal appearances and dismissals -- all must be instantaneous. Terminal users develop muscle memory around navigation speed; introducing a 200ms slide animation for a panel switch creates a perception of sluggishness that outweighs any aesthetic benefit.

The animation budget for a TUI goes entirely to status indicators (spinners, progress bars, and blinking cursors in text inputs). Navigation, structural changes, and data updates render in a single frame.

### Transitions Between Views

Switching from a list view to a detail view, or from the main screen to a help overlay, should be a single-frame replacement. No fade, no slide, no crossfade. Render the new view completely in the next `View()` call. The perceived speed of a TUI comes primarily from the absence of transition delay.

If orientation is a concern (the user needs to understand "where they went"), communicate it through persistent structural elements (a breadcrumb in the header, a mode indicator in the status bar) rather than through animated transitions. A header that reads `WORKERS > coder-1 > logs` after pressing Enter on a worker row tells the user exactly where they are without any animation.
