# Simplify Plan Creation — Design

## Problem

Creating a new plan requires clicking through three sequential overlays: name → description → topic picker. The name and description steps are both simple text inputs that belong together conceptually. This feels sluggish and over-fragmented for a two-field form.

## Solution

Collapse the name and description inputs into a single huh form popup. The topic picker remains a separate step (it's a fundamentally different UI widget — list selection vs text input).

### Flow

```
n pressed → stateNewPlan (huh form: name + description)
         → enter → stateNewPlanTopic (picker, unchanged)
         → pick  → createPlanEntry() → stateDefault
```

### Layout

The huh form is wrapped in the same double-border overlay container used by existing overlays, centered with background fade. huh's focused/blurred field styling (thick left border) provides visual focus indication:

```
╔══════════════════════════════════╗
║  new plan                        ║
║                                  ║
║  ┃ name                          ║
║  ┃ auth-refactor█                ║
║                                  ║
║    description (optional)        ║
║    _                             ║
║                                  ║
║  tab/↑↓ navigate · enter create  ║
╚══════════════════════════════════╝
```

### Key handling

Keys are intercepted before huh to get the desired behavior:

| Key | Action |
|-----|--------|
| Enter | name non-empty → submit (extract both values); name empty → validation error |
| Tab / Shift-Tab | passed to huh for field navigation |
| Arrow ↓ / ↑ | translated to Tab/Shift-Tab (huh.Input is single-line, arrows have no in-field meaning) |
| Esc | cancel, close overlay |
| All other keys | passed to huh for text input |

### Rosé Pine huh theme

`ThemeRosePine()` in `ui/overlay/theme.go`, built from `ThemeBase()` following the same pattern as ThemeDracula/ThemeCharm:

| Role | Color |
|------|-------|
| Focused border | colorIris (#c4a7e7) |
| Title | colorIris, bold |
| Text/input | colorText (#e0def4) |
| Placeholder | colorMuted (#6e6a86) |
| Cursor | colorFoam (#9ccfd8) |
| Error | colorLove (#eb6f92) |
| Focused button | colorBase on colorIris |
| Blurred button | colorSubtle on colorOverlay |

### State machine changes

- Remove `stateNewPlanDescription` from the state enum
- Rename `stateNewPlanName` → `stateNewPlan`
- Remove `pendingPlanName` and `pendingPlanDesc` fields from `home` struct (values live in FormOverlay)
- Add `formOverlay *overlay.FormOverlay` field to `home` struct

### New component

`ui/overlay/formOverlay.go` — a `FormOverlay` struct wrapping a `huh.Form` with two `huh.Input` fields. Exposes `HandleKeyPress`, `Render`, `Name()`, `Description()`, `IsSubmitted()`.

### Files touched

| File | Change |
|------|--------|
| `ui/overlay/formOverlay.go` | new — FormOverlay component |
| `ui/overlay/theme.go` | add ThemeRosePine() |
| `app/app.go` | state enum changes, struct field changes, View switch |
| `app/app_input.go` | collapse two handlers into one, update trigger |
| `app/app_plan_creation_test.go` | adapt tests |
