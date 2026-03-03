# Rewrite UI Panels Implementation Plan

**Goal:** Clean-room rewrite all non-overlay panel files in `ui/` to eliminate AGPL-tainted code. The rewrite preserves the identical public API and visual output so all callers (`app/`, `cmd/`) compile without changes, while replacing every line of implementation. Existing test files are the regression suite.

**Architecture:** Ten source files rewritten in-place across the `ui/` package: `gradient.go` (color interpolation utilities), `consts.go` (banner art and animation frames), `statusbar.go` (top status bar), `menu.go` (bottom keybind bar), `diff.go` (diff pane with file sidebar), `info_pane.go` (metadata pane), `audit_pane.go` (activity feed), `preview.go` (agent session preview with scroll/document modes), `navigation_panel.go` (sidebar plan/instance tree), and `tabbed_window.go` (tab container wiring preview+info+diff). Six files are excluded as 100% post-fork original work: `fill.go`, `spring.go`, `termbg.go`, `theme.go`, `zones.go`, `main_test.go`. The `ui/overlay/` subdirectory was already rewritten in plan 02b. The dead file `list_styles.go.bak` is deleted. All existing test files are preserved as the regression gate — they must pass after each task.

**Tech Stack:** Go 1.24, bubbletea v1.3.x, lipgloss v1.1.x, bubbles v0.20+ (viewport, spinner), bubblezone, go-runewidth, harmonica, wordwrap, charmbracelet/x/ansi, testify

**Size:** Large (estimated ~6 hours, 8 tasks, 3 waves)

---

## Wave 1: Leaf Utilities and Standalone Panes

Rewrites files with zero internal `ui/` dependencies (they import only stdlib, external packages, and `session/`). All five tasks are independently implementable — no task reads output from another.

### Task 1: Rewrite gradient.go and consts.go — Color Utilities and Banner Art

**Files:**
- Modify: `ui/gradient.go`
- Modify: `ui/consts.go`
- Delete: `ui/list_styles.go.bak`
- Test: `ui/gradient_test.go` (existing — 98 LOC, covers parseHex, lerpByte, GradientText, GradientBar)
- Test: `ui/consts_test.go` (existing — covers FallBackText, BannerLines)

**Step 1: write the failing test**

No new tests needed. Existing `gradient_test.go` covers `parseHex` edge cases (valid hex, missing `#`, short string), `lerpByte` interpolation (start/end/midpoint), `GradientText` (empty string, single char, multiline, newline preservation, ANSI reset suffix), and `GradientBar` (zero width, full bar, partial bar, overfill clamp). Existing `consts_test.go` covers `FallBackText` frame cycling and `BannerLines` returning exactly 6 lines per frame.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestParseHex|TestLerpByte|TestGradient|TestFallBack|TestBannerLines" -v -count=1
```

expected: PASS — all existing tests green before rewrite

**Step 3: rewrite implementation**

Delete `ui/list_styles.go.bak` (dead backup file, not imported anywhere).

Delete the contents of `ui/gradient.go` and rewrite from scratch:

- **`parseHex(hex string) (uint8, uint8, uint8)`** — strips leading `#`, returns `(0,0,0)` if remaining string is not exactly 6 chars. Uses `fmt.Sscanf` with `%02x%02x%02x` to parse R, G, B components.
- **`lerpByte(a, b uint8, t float64) uint8`** — linear interpolation: `uint8(float64(a) + (float64(b)-float64(a))*t)`.
- **`GradientText(text, startHex, endHex string) string`** — returns empty string if text is empty. Parses both hex colors. Counts visible (non-newline) runes. If zero visible, returns text as-is. Iterates runes: newlines pass through unchanged, visible chars get `\033[38;2;R;G;Bm` prefix with linearly interpolated color based on position `idx/(visible-1)`. Appends `\033[0m` reset at end.
- **`GradientBar(width, filled int, startHex, endHex string) string`** — returns empty for `width <= 0`. Clamps `filled` to `[0, width]`. Renders `filled` `█` chars with gradient color, then `width-filled` `░` chars in dim gray (`rgb(60,60,60)`). Appends ANSI reset.

Imports: `fmt`, `strings`.

Delete the contents of `ui/consts.go` and rewrite from scratch:

- **`fallbackBannerRaw`** — raw string literal containing the 6-row KASMOS block-art banner (exact same Unicode box-drawing characters).
- **`blockPeriod`** — `[6]string` array: 4 blank rows of 3 spaces, then `"██╗"`, `"╚═╝"`.
- **`bannerFrames`** — `[]string` computed by an `init`-style func literal: splits `fallbackBannerRaw` into 6 base lines, defines 4 suffix sets (0, 1, 2, 3 periods), for each suffix set appends the period glyphs column-wise to each row with a space separator, then applies `GradientText` with `GradientStart`/`GradientEnd` to the joined result.
- **`FallBackText(frame int) string`** — returns `bannerFrames[frame % len(bannerFrames)]`.
- **`BannerLines(frame int) []string`** — returns `strings.Split(bannerFrames[frame % len(bannerFrames)], "\n")`.

Imports: `strings`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestParseHex|TestLerpByte|TestGradient|TestFallBack|TestBannerLines" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
rm -f ui/list_styles.go.bak
git add ui/gradient.go ui/consts.go
git rm -f ui/list_styles.go.bak 2>/dev/null || true
git commit -m "feat(clean-room): rewrite gradient.go and consts.go from scratch"
```

### Task 2: Rewrite statusbar.go — Top Status Bar

**Files:**
- Modify: `ui/statusbar.go`
- Test: `ui/statusbar_test.go` (existing — 269 LOC, covers data model, rendering, plan status styles, task glyphs, layout)

**Step 1: write the failing test**

No new tests needed. Existing `statusbar_test.go` covers: `NewStatusBar` defaults, `SetSize`/`SetData` state changes, `String` output containing app name gradient, branch display, plan status coloring, wave label + task glyph rendering, tmux session count display, project directory right-alignment, narrow-width graceful degradation, and focus mode indicator.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestStatusBar" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/statusbar.go` and rewrite from scratch:

- **`TaskGlyph` type** — `int` enum with constants: `TaskGlyphComplete`, `TaskGlyphRunning`, `TaskGlyphFailed`, `TaskGlyphPending` (iota 0–3).
- **`StatusBarData` struct** — fields: `Branch string`, `PlanName string`, `PlanStatus string`, `WaveLabel string`, `TaskGlyphs []TaskGlyph`, `FocusMode bool`, `TmuxSessionCount int`, `ProjectDir string`.
- **`StatusBar` struct** — fields: `width int`, `data StatusBarData`.
- **`NewStatusBar() *StatusBar`** — returns `&StatusBar{}`.
- **`SetSize(width int)`** — stores width.
- **`SetData(data StatusBarData)`** — stores data.
- **Styles** — package-level `lipgloss.Style` vars: `statusBarStyle` (bg=`ColorSurface`, fg=`ColorText`, horizontal padding 1), `statusBarAppNameStyle` (bg=`ColorSurface`, bold), `statusBarSepStyle` (fg=`ColorOverlay`, bg=`ColorSurface`), `statusBarBranchStyle` (fg=`ColorFoam`, bg=`ColorSurface`), `statusBarWaveLabelStyle` (fg=`ColorSubtle`, bg=`ColorSurface`), `statusBarTmuxCountStyle` (fg=`ColorMuted`, bg=`ColorSurface`), `statusBarProjectDirStyle` (fg=`ColorMuted`, bg=`ColorSurface`).
- **`planStatusStyle(status string) string`** — returns styled status string with color based on status: "implementing"/"planning" → `ColorFoam`, "reviewing"/"done" → `ColorRose`, default → `ColorMuted`. All with `ColorSurface` background.
- **`taskGlyphStr(g TaskGlyph) string`** — renders single glyph: Complete→`✓` in `ColorFoam`, Running→`●` in `ColorIris`, Failed→`✕` in `ColorLove`, Pending→`○` in `ColorMuted`. All with `ColorSurface` background.
- **`centerBranchGroup() string`** — returns empty if no branch. Otherwise renders git icon `\ue725` + branch name in branch style.
- **`leftStatusGroup() string`** — if wave label + task glyphs present: joins glyph strings with spaces, appends wave label. Else if plan status present: renders plan status. Returns joined with ` · ` separator.
- **`String() string`** — returns empty if width < 10. Computes content width (width - 2 for padding). Builds left group (app name gradient + left status), center group (branch), right group (project dir). Uses cursor-based positioning: left at 0, center at `(contentWidth - centerWidth) / 2` (clamped after left), right at `contentWidth - rightWidth`. Drops center/right if they overlap. Pads remaining width with spaces. Wraps in `statusBarStyle.Width(width)`.

Imports: `strings`, `github.com/charmbracelet/lipgloss`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestStatusBar" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/statusbar.go
git commit -m "feat(clean-room): rewrite statusbar.go from scratch"
```

### Task 3: Rewrite menu.go — Bottom Keybind Bar

**Files:**
- Modify: `ui/menu.go`
- Test: `ui/menu_test.go` (existing — covers state transitions, option generation, rendering, focus mode)

**Step 1: write the failing test**

No new tests needed. Existing `menu_test.go` covers: `NewMenu` defaults, `SetState` transitions (empty/default/new-instance/prompt), `SetInstance` with nil/non-nil, `SetFocusMode` toggle, `SetFocusSlot` sidebar/agent, `SetSidebarSpaceAction` variants, `Keydown`/`ClearKeydown`, `SetInDiffTab`, `String` output containing expected keybind labels, focus mode rendering with spinner and exit hint, tmux session count display, and group separator placement.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestMenu" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/menu.go` and rewrite from scratch:

- **Styles** — package-level vars: `keyStyle` (fg=`ColorSubtle`), `descStyle` (fg=`ColorMuted`), `sepStyle` (fg=`ColorOverlay`), `actionGroupStyle` (fg=`ColorRose`), `menuStyle` (fg=`ColorFoam`). Separators: `separator = " • "`, `verticalSeparator = " │ "`.
- **`MenuState` type** — `int` enum: `StateDefault`, `StateEmpty`, `StateNewInstance`, `StatePrompt`.
- **Focus slot constants** — `MenuSlotSidebar=0`, `MenuSlotInfo=1`, `MenuSlotAgent=2`, `MenuSlotDiff=3`, `MenuSlotList=4`.
- **`Menu` struct** — fields: `options []keys.KeyName`, `height int`, `width int`, `state MenuState`, `instance *session.Instance`, `isInDiffTab bool`, `isFocusMode bool`, `focusSlot int`, `sidebarSpaceAction string`, `keyDown keys.KeyName`, `systemGroupSize int`, `tmuxSessionCount int`.
- **Default option slices** — `defaultMenuOptions`, `emptyMenuOptions`, `newInstanceMenuOptions`, `promptMenuOptions` using `keys.Key*` constants. `defaultSystemGroupSize=4`, `emptySystemGroupSize=3`.
- **`NewMenu() *Menu`** — returns menu with `StateEmpty`, `keyDown: -1`, `sidebarSpaceAction: "toggle"`, default options.
- **Setter methods** — `SetTmuxSessionCount`, `Keydown`, `ClearKeydown`, `SetState` (updates state + calls `updateOptions`), `SetInstance` (updates instance, auto-transitions state if not in special state, calls `updateOptions`), `SetInDiffTab`, `SetFocusMode`, `SetFocusSlot`, `SetSidebarSpaceAction` (validates "expand"/"collapse", defaults to "toggle"), `SetSize`.
- **`updateOptions()`** — dispatches on focus mode (single exit key) or state: `StateEmpty` → sidebar or empty options based on focus slot; `StateDefault` → instance options if instance selected, else sidebar options; `StateNewInstance`/`StatePrompt` → submit-only options.
- **`addSidebarOptions(includeNewPlan bool)`** — builds options: optional new-plan, action group (enter, space-expand, view-plan, audit-toggle), system group (search, help, quit).
- **`addInstanceOptions()`** — builds options: management group (new-plan, kill), action group (enter, send-prompt, space, conditional yes/resume), system group (search, tab, help, quit).
- **Focus mode styles** — `focusModeFrames` (braille spinner), `focusDotStyle` (fg=`ColorLove`, bold), `focusLabelStyle` (fg=`ColorLove`, bold), `focusHintKeyStyle` (fg=`ColorSubtle`), `focusHintDescStyle` (fg=`ColorMuted`).
- **`renderFocusMode() string`** — animated spinner frame from wall-clock `time.Now().UnixMilli()/100`, badge "interactive" + spinner, hint "ctrl+space exit". Cursor-positioned: centered content + right-aligned tmux count.
- **`String() string`** — if focus mode, delegates to `renderFocusMode`. Otherwise: iterates options, renders each with key+desc styling (action group uses `actionGroupStyle`, others use `keyStyle`+`descStyle`). Applies underline if `keyDown` matches. Inserts group separators (vertical `│` between groups, bullet `•` within). Centers the menu text, right-aligns tmux count. Uses cursor-based positioning to avoid overlap.

Imports: `fmt`, `strings`, `time`, `github.com/kastheco/kasmos/keys`, `github.com/kastheco/kasmos/session`, `github.com/charmbracelet/lipgloss`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestMenu" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/menu.go
git commit -m "feat(clean-room): rewrite menu.go from scratch"
```

### Task 4: Rewrite diff.go — Diff Pane with File Sidebar

**Files:**
- Modify: `ui/diff.go`
- Test: (no dedicated diff_test.go — tested through `tabbed_window_test.go` and app integration tests)

**Step 1: write the failing test**

No new tests needed. The diff pane is exercised through `tabbed_window_test.go` and app-layer integration tests. The `parseFileChunks` and `colorizeDiff` functions are pure and deterministic — the rewrite spec below is sufficient.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/diff.go` and rewrite from scratch:

- **Styles** — exported: `AdditionStyle` (fg=`ColorDiffAdd`), `DeletionStyle` (fg=`ColorDiffDelete`), `HunkStyle` (fg=`ColorDiffHunk`). Unexported: `fileItemStyle` (fg=`ColorIris`), `fileItemSelectedStyle` (bg=`ColorIris`, fg=`ColorBase`, bold), `fileItemDimStyle` (fg=`ColorMuted`), `filePanelBorderStyle` (rounded border, fg=`ColorOverlay`), `filePanelBorderFocusedStyle` (rounded border, fg=`ColorIris`), `diffHeaderStyle` (fg=`ColorIris`, bold), `diffHintStyle` (fg=`ColorMuted`).
- **`fileChunk` struct** — fields: `path string`, `added int`, `removed int`, `diff string`.
- **`DiffPane` struct** — fields: `viewport viewport.Model`, `width int`, `height int`, `files []fileChunk`, `totalAdded int`, `totalRemoved int`, `fullDiff string`, `selectedFile int` (-1=all, 0..N=specific), `sidebarWidth int`.
- **`NewDiffPane() *DiffPane`** — returns pane with fresh viewport, `selectedFile: 0`.
- **`SetSize(width, height int)`** — stores dimensions, recomputes sidebar width, updates viewport width/height, rebuilds viewport content.
- **`computeSidebarWidth()`** — computes inner width needed from file names (base name + stats + padding), caps at 35% of total width, adds border frame size.
- **`updateViewportWidth()`** — sets viewport width to `width - sidebarWidth - 1`, minimum 10.
- **`SetDiff(instance *session.Instance)`** — returns early if nil/not-started. Gets diff stats from instance. If empty/error, clears state. Otherwise: stores totals, parses file chunks, colorizes full diff, clamps selected file, recomputes layout.
- **`rebuildViewport()`** — sets viewport content to full diff (if selectedFile < 0) or selected file's colorized diff.
- **`String() string`** — if no files: centers "No changes" or error message. Otherwise: joins sidebar + " " + viewport view horizontally.
- **`renderSidebar() string`** — renders bordered file list: header with colored +N -N totals, "All" entry (selected style if selectedFile==-1), per-file entries showing `dir/`(dimmed) + `filename`(accent) + colored stats. Selected file gets full-width highlight. Fills remaining height, adds "shift+↑↓" hint at bottom. Wraps in border style.
- **Navigation** — `FileUp()`/`FileDown()`: cycle selectedFile with wrap-around (-1 ↔ len-1), rebuild viewport, go to top. `ScrollUp()`/`ScrollDown()`: viewport line up/down by 3. `HasFiles() bool`.
- **`parseFileChunks(content string) []fileChunk`** — splits unified diff on `"diff --git "` boundaries. For each chunk: extracts path from `" b/"` portion, counts `+`/`-` lines (excluding `+++`/`---` headers).
- **`colorizeDiff(diff string) string`** — line-by-line colorization: `@@` lines → `HunkStyle`, `+` lines (not `++`) → `AdditionStyle`, `-` lines (not `--`) → `DeletionStyle`, others pass through.

Imports: `fmt`, `path/filepath`, `strings`, `github.com/charmbracelet/bubbles/viewport`, `github.com/charmbracelet/lipgloss`, `github.com/mattn/go-runewidth`, `github.com/kastheco/kasmos/session`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/diff.go
git commit -m "feat(clean-room): rewrite diff.go from scratch"
```

### Task 5: Rewrite info_pane.go and audit_pane.go — Metadata and Activity Panes

**Files:**
- Modify: `ui/info_pane.go`
- Modify: `ui/audit_pane.go`
- Test: `ui/info_pane_test.go` (existing — 152 LOC, covers data rendering, plan summary, wave progress, empty state)
- Test: `ui/audit_pane_test.go` (existing — 158 LOC, covers event rendering, scroll, toggle, empty state)

**Step 1: write the failing test**

No new tests needed. Existing `info_pane_test.go` covers: `NewInfoPane` defaults, `SetData` with instance data, plan header selection, plan summary with instance counts and line changes, wave progress rendering, status color mapping, empty state "no instance selected" message, and `SetSize` viewport refresh. Existing `audit_pane_test.go` covers: `NewAuditPane` defaults, `SetEvents` with event list, `Events()` accessor, minute header rendering, word-wrap continuation lines, scroll up/down, toggle visibility, empty state "no events" message, `EventKindIcon` mapping for all event kinds, and `Height()` accessor.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestInfoPane|TestAuditPane|TestEventKindIcon" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/info_pane.go` and rewrite from scratch:

- **Styles** — `infoSectionStyle` (fg=`ColorFoam`, bold), `infoDividerStyle` (fg=`ColorOverlay`), `infoLabelStyle` (fg=`ColorMuted`, width 20), `infoValueStyle` (fg=`ColorText`).
- **`InfoData` struct** — all fields as currently defined: instance fields (Title, Program, Branch, Path, Created, Status), plan fields (PlanName, PlanDescription, PlanStatus, PlanTopic, PlanBranch, PlanCreated), plan summary fields (PlanInstanceCount, PlanRunningCount, PlanReadyCount, PlanPausedCount, PlanAddedLines, PlanRemovedLines), resource fields (CPUPercent, MemMB), wave fields (AgentType, WaveNumber, TotalWaves, TaskNumber, TotalTasks, WaveTasks), flags (HasPlan, HasInstance, IsPlanHeaderSelected).
- **`WaveTaskInfo` struct** — `Number int`, `State string` ("complete"/"running"/"failed"/"pending").
- **`InfoPane` struct** — `width`, `height int`, `data InfoData`, `viewport viewport.Model`.
- **`NewInfoPane() *InfoPane`** — fresh viewport.
- **`SetSize(width, height int)`** — stores dimensions, updates viewport, re-renders content.
- **`SetData(data InfoData)`** — stores data, re-renders, goes to top.
- **`ScrollUp()`/`ScrollDown()`** — viewport line up/down by 1.
- **`String() string`** — returns "no instance selected" if no instance and no plan header. Otherwise returns viewport view.
- **`statusColor(status string) lipgloss.TerminalColor`** — maps status strings to theme colors.
- **Helper renderers** — `renderRow(label, value)`, `renderStatusRow(label, value)` (with color), `renderDivider()` (dashes).
- **Section renderers** — `renderPlanSection()` (plan metadata rows), `renderInstanceSection()` (instance metadata + wave/task + CPU/mem), `renderPlanSummary()` (plan header view with instance counts, line changes, "view plan doc" button via `zone.Mark(ZoneViewPlan, ...)`), `renderWaveSection()` (task glyphs with state icons).
- **`render() string`** — dispatches: plan header selected → plan summary + optional wave section; instance selected → optional plan section + instance section + optional wave section. Joins with double newline.

Imports: `fmt`, `math`, `strings`, `github.com/charmbracelet/bubbles/viewport`, `github.com/charmbracelet/lipgloss`, `github.com/lrstanley/bubblezone`.

Delete the contents of `ui/audit_pane.go` and rewrite from scratch:

- **`AuditEventDisplay` struct** — `Time string` (HH:MM), `Kind string`, `Icon string`, `Message string`, `Color lipgloss.Color`, `Level string` ("info"/"warn"/"error").
- **`AuditPane` struct** — `events []AuditEventDisplay`, `viewport viewport.Model`, `width int`, `height int`, `visible bool`.
- **`NewAuditPane() *AuditPane`** — visible by default, fresh viewport.
- **`SetSize(w, h int)`** — stores dimensions, viewport body height = h-1 (header), re-renders body, goes to bottom.
- **`SetEvents(events []AuditEventDisplay)`** — stores events, re-renders, goes to bottom.
- **`Events() []AuditEventDisplay`** — accessor for tests.
- **`ScrollDown(n int)`/`ScrollUp(n int)`** — viewport line down/up.
- **`Visible() bool`**, **`Height() int`**, **`ToggleVisible()`**.
- **Styles** — `auditDividerStyle` (fg=`ColorMuted`), `auditMinuteStyle` (fg=`ColorMuted`), `auditMsgStyle` (fg=`ColorSubtle`), `auditWarnMsgStyle` (fg=`ColorGold`), `auditErrMsgStyle` (fg=`ColorLove`), `auditEmptyStyle` (fg=`ColorMuted`), `auditRowPad` (padding-left 1).
- **`String() string`** — joins header + viewport view vertically.
- **`renderHeader() string`** — centered divider `────── log ──────` spanning width.
- **`renderMinuteHeader(minute string) string`** — same pattern with minute string.
- **`renderBody() string`** — if no events: "· no events". Otherwise iterates events oldest-first (reverse order), emits minute header on boundary changes, word-wraps messages at available width (width - 4 overhead), renders with level-based styling (error/warn/info), continuation lines indented to align under message text.
- **`EventKindIcon(kind string) (string, lipgloss.Color)`** — maps event kind strings to (icon, color) pairs for all known event types (agent_spawned→◆/Foam, agent_finished→✓/Gold, agent_killed→✕/Love, etc.). Default: `·`/`ColorMuted`.
- **`debugAudit(format string, args ...any)`** — writes to `/tmp/audit_debug.log` (append mode). Fire-and-forget, ignores errors.

Imports: `fmt`, `os`, `strings`, `github.com/charmbracelet/bubbles/viewport`, `github.com/charmbracelet/lipgloss`, `github.com/muesli/reflow/wordwrap`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestInfoPane|TestAuditPane|TestEventKindIcon" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/info_pane.go ui/audit_pane.go
git commit -m "feat(clean-room): rewrite info_pane.go and audit_pane.go from scratch"
```

## Wave 2: Preview Pane and Navigation Panel

> **depends on wave 1:** `preview.go` uses `GradientText`, `BannerLines`, `FallBackText`, `GradientBar`, `GradientStart`, `GradientEnd` from `gradient.go`/`consts.go` (rewritten in Task 1). `navigation_panel.go` uses `GradientText` and theme colors. Both tasks in this wave are independent of each other.

### Task 6: Rewrite preview.go — Agent Session Preview Pane

**Files:**
- Modify: `ui/preview.go`
- Test: `ui/preview_test.go` (existing — 459 LOC, covers scrolling, content modes, viewport, fallback, raw terminal, document mode)
- Test: `ui/preview_fallback_test.go` (existing — covers fallback centering in short heights)

**Step 1: write the failing test**

No new tests needed. Existing `preview_test.go` covers: scroll mode entry/exit, content truncation with ellipsis, `SetSize` viewport width reservation for scrollbar, `ViewportUpdate` in document/scroll/normal modes, `ViewportHandlesKey` delegation, `String` scrollbar rendering (only when scrollable), fallback content centering, raw terminal content (no ellipsis), document mode (`SetDocumentContent`/`ClearDocumentMode`/`IsDocumentMode`), and `SetRawContent`. Existing `preview_fallback_test.go` covers banner centering in short pane heights.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestPreview" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/preview.go` and rewrite from scratch:

- **Styles** — `previewPaneStyle` (fg=`ColorText`), `scrollbarTrackStyle` (fg=`ColorOverlay`), `scrollbarThumbStyle` (fg=`ColorIris`).
- **`previewState` struct** — `fallback bool`, `fallbackMsg string`, `text string`.
- **`PreviewPane` struct** — `width int`, `height int`, `previewState previewState`, `isScrolling bool`, `viewport viewport.Model`, `bannerFrame int`, `animateBanner bool`, `isDocument bool`, `isRawTerminal bool`, `springAnim *SpringAnim`.
- **`NewPreviewPane() *PreviewPane`** — fresh viewport, spring anim (6.0 target, 15 tick delay), initial fallback state with "create [n]ew plan or select existing" message.
- **`TickSpring()`** — advances spring animation if non-nil.
- **`SetRawContent(content string)`** — sets text directly, clears scroll/document/fallback flags, sets `isRawTerminal = true`.
- **`SetSize(width, maxHeight int)`** — stores dimensions, viewport width = `max(0, width-1)` (reserve 1 col for scrollbar), viewport height = maxHeight.
- **`setFallbackState(message string)`** — sets fallback mode with message, clears `isRawTerminal`.
- **`SetDocumentContent(content string)`** — sets viewport content, clears fallback, sets `isDocument = true`, goes to top.
- **`IsDocumentMode() bool`**, **`ClearDocumentMode()`**.
- **`ViewportUpdate(msg tea.Msg) tea.Cmd`** — forwards to viewport only in document or scroll mode. Returns nil otherwise.
- **`ViewportHandlesKey(msg tea.KeyMsg) bool`** — checks viewport keymap matches only in document or scroll mode.
- **`setFallbackContent(content string)`** — sets fallback with arbitrary centered content (no banner).
- **`SetAnimateBanner(enabled bool)`**, **`TickBanner()`** — animation control.
- **`UpdateContent(instance *session.Instance) error`** — no-op if `isDocument`. Handles nil (fallback CTA), Loading (progress bar via `GradientBar`), Paused (paused message with branch), Exited (exited message with remove hint). In scroll mode with empty viewport: captures full history. In normal mode: live content arrives via `SetRawContent`.
- **`renderScrollbar(height int) string`** — vertical scrollbar using viewport scroll percent. Thumb size = max(1, height/5). Track char `│`, thumb char `▐`. Returns empty when all content fits.
- **`String() string`** — returns blank lines if zero dimensions. Fallback mode: builds animated banner with spring load-in (center unfold, CTA delay + horizontal character reveal), centers vertically and horizontally. Document/scroll mode: viewport view + optional scrollbar joined horizontally. Normal mode: splits text into lines, truncates at available height (height-1 for ellipsis, unless `isRawTerminal`), pads with empty lines, renders with `previewPaneStyle.Width(width)`.
- **Scroll methods** — `ScrollUp(instance)`, `ScrollDown(instance)`, `HalfPageUp(instance)`, `HalfPageDown(instance)`: each enters scroll mode on first call (captures full history, sets viewport content with "ESC to exit scroll mode" footer, goes to bottom), then scrolls viewport. Document mode variants just scroll directly. `ResetToNormalMode(instance)`: exits scroll mode, clears viewport, fetches fresh preview.

Imports: `fmt`, `strings`, `github.com/charmbracelet/bubbles/key`, `github.com/charmbracelet/bubbles/viewport`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/kastheco/kasmos/session`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestPreview" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/preview.go
git commit -m "feat(clean-room): rewrite preview.go from scratch"
```

### Task 7: Rewrite navigation_panel.go — Sidebar Plan/Instance Tree

**Files:**
- Modify: `ui/navigation_panel.go`
- Test: `ui/nav_panel_test.go` (existing — 1004 LOC, 40+ test functions covering row building, sorting, navigation, expand/collapse, search, rendering, cycle, selection persistence)

**Step 1: write the failing test**

No new tests needed. Existing `nav_panel_test.go` is the most comprehensive test file in the package, covering: empty panel, plans with instances, solo instances, mixed plan+solo, history/cancelled sections, dead section partitioning (done plans with running/non-running/mixed instances), history expansion, dead section collapsibility, ClickUp import action, sort ordering (notifications first, instance sort within plan), navigation (up/down/left/right with solo header skipping), expand/collapse (plan header, instance returns false, auto-collapse, user override persistence), selection API (GetSelectedInstance, GetSelectedPlanFile, IsSelectedPlanHeader, SelectByID, SelectInstance, GetSelectedID), selection persistence across rebuild, Kill/Remove, search (activate/deactivate, filter visible rows), rendering (section headers, instance display titles with wave/task, legend, review cycle numbers, implementing/reviewing plans in active section), CycleNextActive/CyclePrevActive (skips paused, auto-expands collapsed plans), FindPlanInstance (prefers running), and misc (SetSize, SetFocused, SelectedSpaceAction, AddInstance, Clear, SelectFirst, ClickItem).

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "Test" -v -count=1 2>&1 | tail -5
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/navigation_panel.go` and rewrite from scratch:

- **Constants** — `SidebarPlanPrefix = "__plan__"`, `SidebarTopicPrefix = "__topic__"`, `SidebarPlanHistoryToggle = "__plan_history_toggle__"`, `SidebarImportClickUp = "__import_clickup__"`.
- **`PlanDisplay` struct** — `Filename`, `Status`, `Description`, `Branch`, `Topic string`.
- **`TopicStatus` struct** — `HasRunning`, `HasNotification bool`.
- **`TopicDisplay` struct** — `Name string`, `Plans []PlanDisplay`.
- **`navRowKind` type** — `int` enum: `navRowPlanHeader`, `navRowInstance`, `navRowSoloHeader`, `navRowTopicHeader`, `navRowImportAction`, `navRowDeadToggle`, `navRowDeadPlan`, `navRowHistoryToggle`, `navRowHistoryPlan`, `navRowCancelled`.
- **`navRow` struct** — `Kind navRowKind`, `ID string`, `Label string`, `TaskFile string`, `PlanStatus string`, `Instance *session.Instance`, `Collapsed bool`, `HasRunning bool`, `HasNotification bool`, `Indent int`.
- **Styles** — 15+ package-level `lipgloss.Style` vars for nav items, selection, sections, icons, search box (matching current visual output exactly).
- **`NavigationPanel` struct** — fields: `spinner *spinner.Model`, `rows []navRow`, `selectedIdx int`, `scrollOffset int`, `plans []PlanDisplay`, `topics []TopicDisplay`, `instances []*session.Instance`, `deadPlans []PlanDisplay`, `historyPlans []PlanDisplay`, `promotedPlans []PlanDisplay`, `cancelled []PlanDisplay`, `planStatuses map[string]TopicStatus`, `collapsed map[string]bool`, `userOverrides map[string]bool`, `inspectedPlans map[string]bool`, `deadExpanded bool`, `historyExpanded bool`, `searchActive bool`, `searchQuery string`, `clickUpAvail bool`, `auditView string`, `auditHeight int`, `width int`, `height int`, `focused bool`.
- **`NewNavigationPanel(sp *spinner.Model) *NavigationPanel`** — initializes maps, `deadExpanded: true`, `focused: true`.
- **Data setters** — `SetData(plans, instances, history, cancelled, planStatuses)`, `SetPlans(plans)`, `SetTopicsAndPlans(topics, ungrouped, history, cancelled...)`, `SetPlanStatuses(statuses)`, `SetItems(...)` (legacy compat).
- **`splitDeadFromHistory(finished []PlanDisplay)`** — partitions finished plans into three buckets based on instance state: promoted (has running instances → appended to active plans), dead (has non-running instances or manually inspected), history (no instances). Removes previously promoted plans before re-partitioning.
- **`resplitDead()`** — recombines all finished plans and re-partitions.
- **`InspectPlan(planFile string)`** — marks plan as inspected, re-partitions, expands dead section, rebuilds rows.
- **`rebuildRows()`** — core row-building logic: preserves selected ID across rebuild. Groups instances by plan file, sorts instances within each plan (running < notified < paused < completed, alphabetical tiebreak). Sorts plans by sort key (notification=0, running/implementing/reviewing=1, idle=2, alphabetical tiebreak). Builds row list: optional ClickUp import action, dead section (toggle + plans if expanded), active plans (sort key < 2), solo instances with divider header, idle plans grouped by topic (with topic headers, collapsible), ungrouped idle plans, history section (toggle + plans if expanded), cancelled plans. Restores selection by ID or clamps to previous numeric position (skipping non-selectable dividers).
- **Sort helpers** — `navInstanceSortKey(inst)` (implementation-complete=3, running/loading=0, paused=2, notified=1, default=3), `navPlanSortKey(p, insts, st)` (notification=0, running=1, implementing/reviewing=1, idle=2), `aggregateNavPlanStatus(insts, st)`.
- **`isPlanCollapsed(planFile, hasRunning, hasNotification) bool`** — user override wins; default: collapsed unless running or notified.
- **Layout** — `SetSize(width, height)`, `SetAuditView(view, h)`, `SetFocused(focused)`, `IsFocused()`, `SetClickUpAvailable(a)`, `availRows()` (height - 8, min 1), `clampScroll()`.
- **Search** — `ActivateSearch()`, `DeactivateSearch()`, `IsSearchActive()`, `GetSearchQuery()`, `SetSearchQuery(q)` (snaps selection to first matching row), `rowMatchesSearch(idx)` (case-insensitive label/taskfile match).
- **Navigation** — `Up()`/`Down()` (skip solo headers and search-hidden rows), `Left()` (collapse or jump to parent), `Right()` (expand or descend), `ToggleSelectedExpand()`.
- **Selection API** — `GetSelectedInstance()`, `GetSelectedPlanFile()`, `IsSelectedPlanHeader()`, `IsSelectedHistoryPlan()`, `GetSelectedID()`, `GetSelectedIdx()`, `GetScrollOffset()`, `ClickItem(row)`, `SelectByID(id)`, `SelectInstance(inst)` (auto-expands collapsed plan), `SetSelectedInstance(idx)`, `SelectedIndex()`, `SelectFirst()` (skips solo header), `SelectedSpaceAction()`.
- **Cycle** — `CycleNextActive()`/`CyclePrevActive()` via `cycleActive(step)`: builds ordered list of non-paused instances in visual order (including hidden instances under collapsed plan headers at their header position), wraps around, auto-expands via `SelectInstance`.
- **Instance management** — `GetInstances()`, `TotalInstances()`, `NumInstances()`, `AddInstance(inst)`, `RemoveByTitle(title)`, `Remove()`, `Kill()`, `Attach()`, `Clear()`, `SetSessionPreviewSize(width, height)`, `FindPlanInstance(planFile)` (prefers running over ready, skips paused).
- **Display helpers** — `navInstanceTitle(inst)` (wave/task format, review cycle, planner, solo agent, coder fix labels), `navInstanceStatusIcon(inst)` (spinner for running, colored glyphs for ready/notified/paused/exited/completed), `navPlanStatusIcon(row)`, `navSectionLabel(key)`, `navDividerLine(label, w)`.
- **`renderNavRow(row, contentWidth) string`** — per-kind rendering: plan header (indent + chevron + label + gap + status icon), instance (indent + title + gap + status icon, solo vs plan-child styling, dead instance strikethrough), solo header divider, topic header (chevron + label), dead/history toggles, history plan (label + done icon), cancelled (strikethrough + ✕), import action (right-aligned).
- **`String() string`** — builds bordered panel: border style (double+iris when focused, rounded+overlay otherwise). Inner content: search bar (active/inactive styling), visible items with section dividers between plan sort-key groups (active vs plans), scroll window centered on selected item, legend (icon key centered), optional audit view (bottom-pinned with gap splitting). Wraps in `zone.Mark(ZoneNavPanel, ...)`. Each row wrapped in `zone.Mark(NavRowZoneID(idx), ...)`.
- **`isAuditContinuationLine(line string) bool`** — strips ANSI + leading spaces, returns true if first char is not a digit (not a timestamp line).
- **`RowCount() int`** — returns `len(rows)`.

Imports: `fmt`, `sort`, `strings`, `github.com/charmbracelet/bubbles/spinner`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/x/ansi`, `github.com/kastheco/kasmos/config/taskstate`, `github.com/kastheco/kasmos/session`, `github.com/lrstanley/bubblezone`, `github.com/mattn/go-runewidth`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "Test" -v -count=1 2>&1 | tail -5
```

expected: PASS

**Step 5: commit**

```bash
git add ui/navigation_panel.go
git commit -m "feat(clean-room): rewrite navigation_panel.go from scratch"
```

## Wave 3: Tab Container

> **depends on wave 2:** `tabbed_window.go` composes `PreviewPane` (Task 6), `DiffPane` (Task 4), and `InfoPane` (Task 5). It calls methods on all three panes and delegates rendering to them. Must be rewritten after the panes it wraps.

### Task 8: Rewrite tabbed_window.go — Tab Container

**Files:**
- Modify: `ui/tabbed_window.go`
- Test: `ui/tabbed_window_test.go` (existing — covers tab switching, size propagation, preview delegation, focus mode, welcome banner)

**Step 1: write the failing test**

No new tests needed. Existing `tabbed_window_test.go` covers: `NewTabbedWindow` defaults (info tab active, welcome banner on), `SetSize` propagation to child panes, `Toggle` cycling through tabs, `SetActiveTab` with welcome banner clearing, `GetActiveTab`, `IsInDiffTab`/`IsInInfoTab`, `SetFocusMode`/`IsFocusMode`, `SetFocused`, `UpdatePreview` delegation (no-op in focus mode or non-preview tab), `SetDocumentContent`/`ClearDocumentMode`/`IsDocumentMode`, `ScrollUp`/`ScrollDown` dispatch to active tab's pane, `HalfPageUp`/`HalfPageDown` always targeting preview, `ContentScrollUp`/`ContentScrollDown` (mouse wheel), `IsPreviewInScrollMode`, `String` output with tab labels and border styling.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestTabbedWindow" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete the contents of `ui/tabbed_window.go` and rewrite from scratch:

- **Tab border helpers** — `tabBorderWithBottom(left, middle, right string) lipgloss.Border`: creates rounded border with custom bottom chars.
- **Styles** — `inactiveTabBorder` (bottom: `┴─┴`), `activeTabBorder` (bottom: `┘ └`), `inactiveTabStyle` (inactive border, fg=`ColorIris`, center-aligned), `activeTabStyle` (active border, center-aligned), `windowBorder` (rounded), `windowStyle` (iris border, right+bottom+left sides only).
- **Tab index constants** — `InfoTab=0`, `PreviewTab=1`, `DiffTab=2`.
- **`Tab` struct** — `Name string`, `Render func(width, height int) string`.
- **`TabbedWindow` struct** — `tabs []string` (icon+label for each tab), `activeTab int`, `focusedTab int` (-1=none), `height int`, `width int`, `preview *PreviewPane`, `diff *DiffPane`, `info *InfoPane`, `instance *session.Instance`, `focused bool`, `focusMode bool`, `showWelcome bool`.
- **`NewTabbedWindow(preview, diff, info) *TabbedWindow`** — tab labels: `"\uea74 info"`, `"\uea85 agent"`, `"\ueae1 diff"`. `focusedTab: -1`, `showWelcome: true`.
- **`SetFocusMode(enabled bool)`**, **`IsFocusMode() bool`**, **`SetFocused(focused bool)`**, **`SetInstance(instance)`**.
- **`AdjustPreviewWidth(width int) int`** — returns `width - 2`.
- **`SetSize(width, height int)`** — stores adjusted width/height, computes content dimensions (subtracting tab row height and window border frame), propagates to all three child panes.
- **`GetPreviewSize() (int, int)`** — returns preview dimensions.
- **`Toggle()`** — cycles `activeTab` mod 3. **`ToggleWithReset(instance)`** — resets preview to normal mode first, then toggles.
- **Preview delegation** — `UpdatePreview(instance)` (no-op if not preview tab or focus mode), `SetPreviewContent(content)`, `SetConnectingState()`, `SetDocumentContent(content)`, `ClearDocumentMode()`, `IsDocumentMode()`, `ViewportUpdate(msg)`, `ViewportHandlesKey(msg)`.
- **Diff delegation** — `UpdateDiff(instance)` (no-op if not diff tab).
- **Info delegation** — `SetInfoData(data InfoData)`.
- **`ResetPreviewToNormalMode(instance)`**.
- **Scroll methods** — `ScrollUp()`/`ScrollDown()`: dispatches to active tab's pane (preview: scroll, diff: file navigation if files exist else scroll, info: scroll). `HalfPageUp()`/`HalfPageDown()`: always targets preview pane. `ContentScrollUp()`/`ContentScrollDown()`: dispatches without file navigation (for mouse wheel).
- **Tab state queries** — `IsInDiffTab()`, `IsInInfoTab()`, `SetActiveTab(tab)` (clears welcome banner), `GetActiveTab()`.
- **Animation** — `TickBanner()`, `TickSpring()`, `SetAnimateBanner(enabled)`.
- **`IsPreviewInScrollMode() bool`**.
- **`String() string`** — returns empty if zero dimensions. Renders tab row: each tab gets proportional width (last tab absorbs remainder), active tab uses `activeTabStyle`, inactive uses `inactiveTabStyle`. Border color varies: `ColorFoam` in focus mode, `ColorIris` if focused, `ColorOverlay` otherwise. First/last tab get special bottom-corner chars (`│`/`├`/`┤`). Active+ring-focused tab gets gradient text, active-only gets normal text, inactive gets muted text. Each tab wrapped in `zone.Mark(TabZoneIDs[i], ...)`. Content: dispatches to active tab's pane (info tab shows preview banner while `showWelcome` is true). Window border wraps content with `lipgloss.Place` for top-left alignment. Preview tab content wrapped in `zone.Mark(ZoneAgentPane, ...)`. Joins tab row + window vertically.

Imports: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/kastheco/kasmos/log`, `github.com/kastheco/kasmos/session`, `github.com/lrstanley/bubblezone`.

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -count=1
```

expected: PASS — full suite green

**Step 5: commit**

```bash
git add ui/tabbed_window.go
git commit -m "feat(clean-room): rewrite tabbed_window.go from scratch"
```
