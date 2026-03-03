# Separate Logs by Minute Implementation Plan

**Goal:** Group audit log events by minute with centered minute headers, and remove per-line timestamps to reclaim horizontal space for message text.

**Architecture:** Modify `AuditPane.renderBody()` in `ui/audit_pane.go` to detect minute boundaries while iterating events, emit a centered `── HH:MM ──` divider when the minute changes, and render each event line without the timestamp prefix. The `AuditEventDisplay` struct and `refreshAuditPane()` in `app/app_state.go` remain unchanged — the `Time` field is still populated but only consumed by the minute-header logic, not rendered per-line. Overhead drops from 11 chars to 4 chars per line (`" ◆  message"` instead of `" HH:MM  ◆  message"`).

**Tech Stack:** Go, lipgloss, bubbles/viewport, wordwrap

**Size:** Small (estimated ~45 min, 1 task, 1 wave)

---

## Wave 1: Minute-Grouped Log Rendering

### Task 1: Refactor renderBody and Update Tests

**Files:**
- Modify: `ui/audit_pane.go`
- Modify: `ui/audit_pane_test.go`

**Step 1: write the failing test**

Add two new tests to `ui/audit_pane_test.go`:

```go
func TestAuditPane_MinuteHeaders(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 20)
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:35", Kind: "agent_finished", Icon: "✓", Message: "coder finished", Color: ColorGold, Level: "info"},
		{Time: "12:34", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam, Level: "info"},
		{Time: "12:34", Kind: "plan_transition", Icon: "⟳", Message: "ready → implementing", Color: ColorIris, Level: "info"},
	})
	output := pane.String()
	// Minute headers should appear as centered dividers
	assert.Contains(t, output, "12:34")
	assert.Contains(t, output, "12:35")
	// Messages should still appear
	assert.Contains(t, output, "spawned coder")
	assert.Contains(t, output, "coder finished")
	assert.Contains(t, output, "ready → implementing")
}

func TestAuditPane_NoPerLineTimestamp(t *testing.T) {
	pane := NewAuditPane()
	pane.SetSize(60, 20)
	pane.SetEvents([]AuditEventDisplay{
		{Time: "12:34", Kind: "agent_spawned", Icon: "◆", Message: "spawned coder", Color: ColorFoam, Level: "info"},
	})
	output := pane.String()
	// The timestamp should appear in the minute header, not next to the icon on the event line.
	// Event lines should be " ◆  message" not " 12:34  ◆  message".
	// Count occurrences of "12:34" — should be exactly 1 (the header), not 2.
	assert.Equal(t, 1, strings.Count(output, "12:34"),
		"timestamp should appear once (in minute header), not per-line")
}
```

**Step 2: run test to verify it fails**

```bash
go test ./ui/... -run 'TestAuditPane_MinuteHeaders|TestAuditPane_NoPerLineTimestamp' -v
```

expected: FAIL — `TestAuditPane_NoPerLineTimestamp` fails because current rendering puts timestamp on every line (count will be > 1 for multi-event minutes, or the format check fails).

**Step 3: write minimal implementation**

Modify `renderBody()` in `ui/audit_pane.go`:

1. Add a `auditMinuteStyle` for the centered minute header divider (muted color, matching `auditDividerStyle`).

2. Replace the current per-event rendering loop with minute-grouped logic:
   - Track `lastMinute string` as events are iterated oldest-first.
   - When `e.Time != lastMinute`, emit a centered `── HH:MM ──` line using the pane width.
   - Render each event line as `" ◆  message"` (4 chars overhead) instead of `" HH:MM  ◆  message"` (11 chars overhead).
   - Update `overhead` constant from 11 to 4 and `contIndent` from 11 to 4 to match.

3. The minute header rendering should use the same centered-divider pattern as `renderHeader()` but with the time string instead of "log".

Update existing tests that assert on the old format:
- `TestAuditPane_RenderEvents` — still asserts message content (no change needed).
- `TestAuditPane_MessageTruncation` — the narrower overhead means more message fits; update the assertion to reflect the wider available message width (the truncated prefix will be longer now).

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run TestAuditPane -v
```

expected: PASS — all audit pane tests pass including the two new ones.

Also run the app-level audit tests to verify no breakage:

```bash
go test ./app/... -run TestAuditPane -v
```

expected: PASS

**Step 5: commit**

```bash
git add ui/audit_pane.go ui/audit_pane_test.go
git commit -m "feat: group audit log events by minute with centered headers, remove per-line timestamps"
```
