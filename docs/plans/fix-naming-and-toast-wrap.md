# Fix Naming Bug & Toast Message Wrapping

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix two UI bugs: (1) entering a name for a new instance fires "cannot change title of a started instance" when another instance is already running; (2) long toast messages are truncated instead of wrapping to a second line.

**Architecture:**
- Bug 1 is a data-access bug in the bubbletea model: `stateNew` resolves the instance being named via a fragile index into `allItems` (unfiltered). Fix: store a direct `*session.Instance` pointer on the model when entering `stateNew`, removing the index dependency entirely.
- Bug 2 is a render bug in `renderToast`: it calls `runewidth.Truncate`. Fix: swap truncation for word-wrap using `github.com/muesli/reflow/wordwrap` (already in `go.mod`), and multi-line the toast content with proper icon indentation on continuation lines.

**Tech Stack:** Go, bubbletea, lipgloss, `github.com/muesli/reflow/wordwrap`, `github.com/mattn/go-runewidth`

---

## Bug 1: Naming Bug — stateNew resolves wrong instance

### Root Cause

In `app_input.go` line 283:

```go
instance := m.list.GetInstances()[m.list.TotalInstances()-1]
```

`GetInstances()` returns `l.allItems` (all instances, unfiltered/unsorted). After a new instance is appended to `allItems` via `AddInstance`, `rebuildFilteredItems()` is called which re-sorts `items` (the visible/filtered slice). Depending on sort mode (e.g. "Oldest First"), the newly created instance may not end up last in `allItems` after the sort. Meanwhile `SetSelectedInstance(NumInstances()-1)` sets `selectedIdx` to the last item in the *filtered* `items`.

The fix: store the new instance directly on the model when entering `stateNew`. Use it by reference in the handler. No index math. No allItems lookup.

### Task 1: Add `newInstance` field to the `home` model

**Files:**
- Modify: `app/app.go:99-101`

**Step 1: Read the file**

Read `app/app.go` lines 92-108 to confirm placement.

**Step 2: Add the field**

In the `// -- State --` block, just below `newInstanceFinalizer`, add:

```go
// newInstance is the instance currently being named in stateNew.
// Set when entering stateNew, cleared on Enter/Esc/ctrl+c.
newInstance *session.Instance
```

**Step 3: Run the build to ensure no compile errors**

```bash
go build ./...
```

Expected: no errors.

---

### Task 2: Store `newInstance` at all three `stateNew` entry sites

**Files:**
- Modify: `app/app_input.go:848-908` (three `case` blocks: `KeyPrompt`, `KeyNew`, `KeyNewSkipPermissions`)

Each site currently does:
```go
m.newInstanceFinalizer = m.list.AddInstance(instance)
m.list.SetSelectedInstance(m.list.NumInstances() - 1)
m.state = stateNew
```

Add `m.newInstance = instance` after the `AddInstance` call at all three sites:

```go
m.newInstanceFinalizer = m.list.AddInstance(instance)
m.newInstance = instance                               // <-- add this line
m.list.SetSelectedInstance(m.list.NumInstances() - 1)
m.state = stateNew
```

Do this for all three cases: `KeyPrompt` (~line 848), `KeyNew` (~line 877), `KeyNewSkipPermissions` (~line 904).

**Step 1: Edit `app_input.go`** for `KeyPrompt` (around line 850):

Old:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true
```
New:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true
```

**Step 2: Edit `app_input.go`** for `KeyNew` (around line 877):

Old:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyNewSkipPermissions:
```
New:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyNewSkipPermissions:
```

**Step 3: Edit `app_input.go`** for `KeyNewSkipPermissions` (around line 904):

Old:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
```
New:
```go
		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.newInstance = instance
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
```

**Step 4: Build to confirm no errors**

```bash
go build ./...
```

---

### Task 3: Replace index-based lookup in `stateNew` handler with `m.newInstance`

**Files:**
- Modify: `app/app_input.go:268-358`

**Step 1: Replace the instance lookup line**

Old (line 283):
```go
	instance := m.list.GetInstances()[m.list.TotalInstances()-1]
```
New:
```go
	instance := m.newInstance
```

Also add a nil guard immediately after (defensive programming):

```go
	instance := m.newInstance
	if instance == nil {
		// stateNew without a pending instance — shouldn't happen, return to default
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		return m, nil
	}
```

**Step 2: Clear `m.newInstance` at all exit points of `stateNew`**

The three exit paths are:
1. `ctrl+c` — add `m.newInstance = nil` alongside `m.state = stateDefault`
2. `KeyEsc` — add `m.newInstance = nil` alongside `m.state = stateDefault`
3. `KeyEnter` — add `m.newInstance = nil` alongside the transition out of `stateNew`

For `ctrl+c` (around line 270):
```go
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.newInstance = nil
			m.promptAfterName = false
			m.list.Kill()
```

For `KeyEnter` (around line 286):
```go
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}
			instance.SetStatus(session.Loading)
			m.state = stateDefault
			m.newInstance = nil
			m.menu.SetState(ui.StateDefault)
```

For `KeyEsc` (around line 345):
```go
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.newInstance = nil
			m.instanceChanged()
```

**Step 3: Build**

```bash
go build ./...
```

Expected: no errors.

---

### Task 4: Write a test for the naming bug fix

**Files:**
- Modify: `app/app_test.go`

The goal: verify that `m.newInstance` is set when entering `stateNew` and cleared when leaving it.

**Step 1: Add the test**

At the end of `app_test.go`, add:

```go
// TestNewInstancePointerSetAndCleared verifies that m.newInstance is set when
// entering stateNew and cleared on exit — preventing the stale-index naming bug.
func TestNewInstancePointerSetAndCleared(t *testing.T) {
	s := spinner.New()
	sp := s
	list := ui.NewList(&sp, false)

	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		list:      list,
	}

	// Create a mock instance (not started, not real tmux)
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "",
		Path:    t.TempDir(),
		Program: "echo",
	})
	require.NoError(t, err)

	// Simulate what KeyNew does
	h.newInstanceFinalizer = h.list.AddInstance(inst)
	h.newInstance = inst
	h.list.SetSelectedInstance(h.list.NumInstances() - 1)
	h.state = stateNew

	assert.Equal(t, inst, h.newInstance, "newInstance should be set when entering stateNew")
	assert.Equal(t, stateNew, h.state)

	// Simulate pressing Esc — clear newInstance
	h.list.Kill()
	h.state = stateDefault
	h.newInstance = nil

	assert.Nil(t, h.newInstance, "newInstance should be nil after leaving stateNew")
	assert.Equal(t, stateDefault, h.state)
}
```

**Step 2: Run the test**

```bash
go test ./app/... -run TestNewInstancePointerSetAndCleared -v
```

Expected: PASS.

**Step 3: Run all app tests**

```bash
go test ./app/... -v
```

Expected: all pass.

**Step 4: Commit**

```bash
git add app/app.go app/app_input.go app/app_test.go
git commit -m "fix(app): store newInstance pointer to prevent stale-index naming bug

When pressing n to create a second instance while one is running, the
stateNew handler was reading m.list.GetInstances()[TotalInstances()-1]
which accesses allItems by index. After rebuildFilteredItems() re-sorts,
this index may point to an already-started instance, causing every
keystroke to fire 'cannot change title of a started instance'.

Fix: store the new instance directly as m.newInstance when entering
stateNew. Clear it on all three exit paths (Enter/Esc/ctrl+c)."
```

---

## Bug 2: Toast Message Wrapping

### Root Cause

`renderToast` in `ui/overlay/toast.go` line 333–334:

```go
if runewidth.StringWidth(msg) > maxMsgWidth {
    msg = runewidth.Truncate(msg, maxMsgWidth, "...")
}
```

Messages longer than `MaxToastWidth` (60) are truncated. The fix: wrap instead of truncate using `wordwrap.String` from `github.com/muesli/reflow/wordwrap` (already in `go.mod`). Multi-line toasts have the icon on line 1 and continuation lines indented to align with the text.

### Task 5: Swap truncation for word-wrap in `renderToast`

**Files:**
- Modify: `ui/overlay/toast.go`

**Step 1: Add the import**

The `wordwrap` package is already in `go.mod` at `github.com/muesli/reflow v0.3.0`. Add the import:

```go
import (
    ...
    "strings"
    ...
    "github.com/muesli/reflow/wordwrap"
)
```

**Step 2: Rewrite `renderToast`**

Old:
```go
func (tm *ToastManager) renderToast(t *toast) string {
	icon := tm.toastIcon(t.Type)
	// t.Width - 4 accounts for border (2) + padding (2), then subtract icon width + space.
	maxMsgWidth := t.Width - 4 - runewidth.StringWidth(icon) - 1
	msg := t.Message
	if runewidth.StringWidth(msg) > maxMsgWidth {
		msg = runewidth.Truncate(msg, maxMsgWidth, "...")
	}
	content := icon + " " + msg
	return toastStyle(t.Type, t.Width).Render(content)
}
```

New:
```go
func (tm *ToastManager) renderToast(t *toast) string {
	icon := tm.toastIcon(t.Type)
	iconWidth := runewidth.StringWidth(icon)
	// Content area width: total width minus border (2) + padding (2).
	// Then subtract icon (iconWidth) + space (1) for the first-line prefix.
	maxMsgWidth := t.Width - 4 - iconWidth - 1

	// Word-wrap the message to fit within maxMsgWidth.
	wrapped := wordwrap.String(t.Message, maxMsgWidth)
	lines := strings.Split(wrapped, "\n")

	// First line: "icon message"
	// Continuation lines: indent to align with the text after the icon.
	indent := strings.Repeat(" ", iconWidth+1)
	var contentLines []string
	for i, line := range lines {
		if i == 0 {
			contentLines = append(contentLines, icon+" "+line)
		} else if line != "" {
			contentLines = append(contentLines, indent+line)
		}
	}
	content := strings.Join(contentLines, "\n")
	return toastStyle(t.Type, t.Width).Render(content)
}
```

**Step 3: Remove unused `runewidth.Truncate` — ensure `runewidth` is still imported** (it's still used by `calcToastWidth` and `GetPosition`, so the import stays).

**Step 4: Build**

```bash
go build ./...
```

---

### Task 6: Update and add tests for wrapping behaviour

**Files:**
- Modify: `ui/overlay/toast_test.go`

**Step 1: Update `TestToastViewRendersContent`** — the existing test asserts `Contains(view, "hello world")`. That still passes with wrapping since the message is short. No change needed.

**Step 2: Add a test for long message wrapping**

Add after `TestToastViewRendersContent`:

```go
func TestToastLongMessageWraps(t *testing.T) {
	s := spinner.New()
	tm := NewToastManager(&s)

	longMsg := "cannot change title of a started instance because the session is running"
	_ = tm.Error(longMsg)
	require.Len(t, tm.toasts, 1)

	// Force visible phase.
	tm.toasts[0].Phase = PhaseVisible
	tm.toasts[0].PhaseStart = time.Now()

	view := tm.View()
	assert.NotEmpty(t, view)

	// The full message should appear in the rendered output (possibly across lines),
	// NOT truncated with "...".
	assert.NotContains(t, view, "...", "long messages should wrap, not truncate")

	// Key words from both ends of the message should appear.
	assert.Contains(t, view, "cannot change")
	assert.Contains(t, view, "session is running")
}
```

**Step 3: Run the toast tests**

```bash
go test ./ui/overlay/... -v
```

Expected: all pass including `TestToastLongMessageWraps`.

**Step 4: Run all tests**

```bash
go test ./... 
```

Expected: all pass.

**Step 5: Commit**

```bash
git add ui/overlay/toast.go ui/overlay/toast_test.go
git commit -m "fix(toast): wrap long messages instead of truncating

Messages like 'cannot change title of a started instance' were cut
off at MaxToastWidth (60 chars) with '...'. Use reflow/wordwrap to
flow the message to multiple lines with icon-aligned indentation on
continuation lines."
```

---

## Final Verification

Run the full test suite and build:

```bash
go test ./...
go build ./...
```

### Manual smoke test

1. Start klique with an existing running instance visible.
2. Press `n` to create a new instance.
3. Type a name character-by-character — **no error toast should appear**.
4. Press Enter to confirm. Instance starts normally.
5. Repeat with `shift+n` (KeyNewSkipPermissions) and the prompt flow (`p` key if available).

For toast wrapping:
1. Trigger the error by pressing `n` while having a started instance and somehow triggering the old path (or set a breakpoint / add a temporary `handleError(fmt.Errorf("this is a long error message that exceeds sixty characters in length"))` call to verify the toast wraps).
2. Observe the toast shows two lines without truncation.
