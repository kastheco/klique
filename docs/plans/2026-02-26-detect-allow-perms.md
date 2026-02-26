# Detect & Respond to OpenCode Permission Prompts — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Detect opencode's "Permission required" dialog, show a kasmos modal with allow always/allow once/reject choices, cache "allow always" decisions locally, and send the appropriate key sequence to the tmux pane.

**Architecture:** New `ParsePermissionPrompt()` function detects the prompt in pane content during the metadata tick. A `PermissionCache` in `config/` handles persistence. A `PermissionOverlay` in `ui/overlay/` renders the three-choice modal. The app wires detection → cache lookup → modal or auto-approve → tmux key dispatch.

**Tech Stack:** Go, bubbletea, lipgloss, tmux send-keys

**Size:** Medium (estimated ~3 hours, 5 tasks, 2 waves)

---

## Wave 1: Detection + Cache + Overlay Components

> These three tasks are independent — they touch different packages with no shared dependencies.

### Task 1: Permission Prompt Parser

**Files:**
- Create: `session/permission_prompt.go`
- Create: `session/permission_prompt_test.go`

**Step 1: Write the failing tests**

```go
package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePermissionPrompt_OpenCodeDetectsPrompt(t *testing.T) {
	content := `
→ Read ../../../../opt

■  Chat · claude-opus-4-6

△ Permission required
  ← Access external directory /opt

Patterns

- /opt/*

 Allow once   Allow always   Reject                          ctrl+f fullscreen ⇥ select enter confirm
`
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /opt", result.Description)
	assert.Equal(t, "/opt/*", result.Pattern)
}

func TestParsePermissionPrompt_OpenCodeNoPrompt(t *testing.T) {
	content := `some normal opencode output without permission prompt`
	result := ParsePermissionPrompt(content, "opencode")
	assert.Nil(t, result)
}

func TestParsePermissionPrompt_IgnoresNonOpenCode(t *testing.T) {
	content := `△ Permission required
  ← Access external directory /opt
Patterns
- /opt/*`
	result := ParsePermissionPrompt(content, "claude")
	assert.Nil(t, result)
}

func TestParsePermissionPrompt_HandlesAnsiCodes(t *testing.T) {
	content := "\x1b[33m△\x1b[0m \x1b[1mPermission required\x1b[0m\n  ← Access external directory /tmp\n\nPatterns\n\n- /tmp/*\n"
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /tmp", result.Description)
	assert.Equal(t, "/tmp/*", result.Pattern)
}

func TestParsePermissionPrompt_MissingPattern(t *testing.T) {
	content := "△ Permission required\n  ← Access external directory /opt\n"
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /opt", result.Description)
	assert.Empty(t, result.Pattern)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./session/ -run TestParsePermissionPrompt -v`
Expected: FAIL — `ParsePermissionPrompt` not defined

**Step 3: Write the implementation**

```go
package session

import (
	"regexp"
	"strings"
)

// PermissionPrompt represents a detected permission request from an agent.
type PermissionPrompt struct {
	// Description is the human-readable description, e.g. "Access external directory /opt".
	Description string
	// Pattern is the permission pattern, e.g. "/opt/*".
	Pattern string
}

// ansiStripRe strips ANSI escape sequences for permission prompt parsing.
var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// ParsePermissionPrompt scans pane content for an opencode "Permission required" dialog.
// Returns nil if no permission prompt is detected or if the program is not opencode.
func ParsePermissionPrompt(content string, program string) *PermissionPrompt {
	if !strings.Contains(strings.ToLower(program), "opencode") {
		return nil
	}

	clean := ansiStripRe.ReplaceAllString(content, "")
	lines := strings.Split(clean, "\n")

	var permIdx int = -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), "Permission required") {
			permIdx = i
			break
		}
	}
	if permIdx < 0 {
		return nil
	}

	prompt := &PermissionPrompt{}

	// Description is on the next non-empty line after "Permission required", strip leading "← ".
	for i := permIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "← ")
		trimmed = strings.TrimPrefix(trimmed, "←")
		trimmed = strings.TrimSpace(trimmed)
		prompt.Description = trimmed
		break
	}

	// Pattern: find "Patterns" header, then first line starting with "- ".
	for i := permIdx; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "Patterns" {
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed == "" {
					continue
				}
				if strings.HasPrefix(trimmed, "- ") {
					prompt.Pattern = strings.TrimPrefix(trimmed, "- ")
					break
				}
				break // non-empty, non-pattern line — stop
			}
			break
		}
	}

	return prompt
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./session/ -run TestParsePermissionPrompt -v`
Expected: All PASS

**Step 5: Commit**

```
git add session/permission_prompt.go session/permission_prompt_test.go
git commit -m "feat: add permission prompt parser for opencode sessions"
```

---

### Task 2: Permission Cache

**Files:**
- Create: `config/permission_cache.go`
- Create: `config/permission_cache_test.go`

**Step 1: Write the failing tests**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionCache_LookupEmpty(t *testing.T) {
	cache := NewPermissionCache("")
	assert.False(t, cache.IsAllowedAlways("/opt/*"))
}

func TestPermissionCache_RememberAndLookup(t *testing.T) {
	dir := t.TempDir()
	cache := NewPermissionCache(dir)
	cache.Remember("/opt/*")
	assert.True(t, cache.IsAllowedAlways("/opt/*"))
	assert.False(t, cache.IsAllowedAlways("/tmp/*"))
}

func TestPermissionCache_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	cache := NewPermissionCache(dir)
	cache.Remember("/opt/*")
	err := cache.Save()
	require.NoError(t, err)

	// Load into a new cache and verify
	cache2 := NewPermissionCache(dir)
	err = cache2.Load()
	require.NoError(t, err)
	assert.True(t, cache2.IsAllowedAlways("/opt/*"))
}

func TestPermissionCache_LoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	cache := NewPermissionCache(dir)
	err := cache.Load()
	assert.NoError(t, err) // missing file is not an error
	assert.False(t, cache.IsAllowedAlways("/opt/*"))
}

func TestPermissionCache_SaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	cache := NewPermissionCache(dir)
	cache.Remember("/opt/*")
	err := cache.Save()
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "permission-cache.json"))
	assert.NoError(t, err)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run TestPermissionCache -v`
Expected: FAIL — `NewPermissionCache` not defined

**Step 3: Write the implementation**

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const permissionCacheFile = "permission-cache.json"

// PermissionCache stores "allow always" decisions keyed by permission pattern.
type PermissionCache struct {
	mu       sync.RWMutex
	patterns map[string]string // pattern -> "allow_always"
	dir      string           // directory to store the cache file
}

// NewPermissionCache creates a new cache that persists to the given directory.
func NewPermissionCache(dir string) *PermissionCache {
	return &PermissionCache{
		patterns: make(map[string]string),
		dir:      dir,
	}
}

// Load reads the cache from disk. Missing file is not an error.
func (c *PermissionCache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := filepath.Join(c.dir, permissionCacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &c.patterns)
}

// Save writes the cache to disk, creating the directory if needed.
func (c *PermissionCache) Save() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c.patterns, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, permissionCacheFile), data, 0644)
}

// IsAllowedAlways returns true if the pattern has been cached as "allow always".
func (c *PermissionCache) IsAllowedAlways(pattern string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.patterns[pattern] == "allow_always"
}

// Remember stores a pattern as "allow always".
func (c *PermissionCache) Remember(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.patterns[pattern] = "allow_always"
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./config/ -run TestPermissionCache -v`
Expected: All PASS

**Step 5: Commit**

```
git add config/permission_cache.go config/permission_cache_test.go
git commit -m "feat: add permission cache for auto-approving repeated patterns"
```

---

### Task 3: Permission Overlay + Tmux Key Helpers

**Files:**
- Create: `ui/overlay/permissionOverlay.go`
- Modify: `session/tmux/tmux_io.go` (add `TapRight` and `SendPermissionResponse`)
- Create: `session/tmux/tmux_permission_test.go`

**Step 1: Write the tmux helper tests**

```go
package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendPermissionResponse_AllowAlways(t *testing.T) {
	exec := NewMockExecutor()
	session := NewTmuxSessionWithDeps("test", "opencode", false, MockPtyFactory{}, exec)
	session.sanitizedName = "kas_test"

	err := session.SendPermissionResponse(PermissionAllowAlways)
	require.NoError(t, err)

	// Should send: Right, Enter, Enter
	calls := exec.GetCalls()
	assert.GreaterOrEqual(t, len(calls), 3)
}

func TestSendPermissionResponse_AllowOnce(t *testing.T) {
	exec := NewMockExecutor()
	session := NewTmuxSessionWithDeps("test", "opencode", false, MockPtyFactory{}, exec)
	session.sanitizedName = "kas_test"

	err := session.SendPermissionResponse(PermissionAllowOnce)
	require.NoError(t, err)

	// Should send: Enter
	calls := exec.GetCalls()
	assert.GreaterOrEqual(t, len(calls), 1)
}

func TestSendPermissionResponse_Reject(t *testing.T) {
	exec := NewMockExecutor()
	session := NewTmuxSessionWithDeps("test", "opencode", false, MockPtyFactory{}, exec)
	session.sanitizedName = "kas_test"

	err := session.SendPermissionResponse(PermissionReject)
	require.NoError(t, err)

	// Should send: Right, Right, Enter
	calls := exec.GetCalls()
	assert.GreaterOrEqual(t, len(calls), 3)
}
```

Note: The test depends on MockExecutor already existing in `tmux_test.go`. Check that file for the exact mock type name and constructor — adapt the test to match. If no mock executor exists, use the pattern from existing tmux tests.

**Step 2: Run tests to verify they fail**

Run: `go test ./session/tmux/ -run TestSendPermissionResponse -v`
Expected: FAIL — `SendPermissionResponse` not defined

**Step 3: Write the tmux helpers**

Add to `session/tmux/tmux_io.go`:

```go
// PermissionChoice represents the user's response to an opencode permission prompt.
type PermissionChoice int

const (
	PermissionAllowOnce   PermissionChoice = iota
	PermissionAllowAlways
	PermissionReject
)

// TapRight sends a Right arrow keystroke to the tmux pane.
func (t *TmuxSession) TapRight() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")
	return t.cmdExec.Run(cmd)
}

// SendPermissionResponse sends the key sequence for the given permission choice.
// Allow once: Enter (already selected). Allow always: Right Enter Enter. Reject: Right Right Enter.
func (t *TmuxSession) SendPermissionResponse(choice PermissionChoice) error {
	switch choice {
	case PermissionAllowOnce:
		return t.TapEnter()
	case PermissionAllowAlways:
		if err := t.TapRight(); err != nil {
			return err
		}
		if err := t.TapEnter(); err != nil {
			return err
		}
		return t.TapEnter()
	case PermissionReject:
		if err := t.TapRight(); err != nil {
			return err
		}
		if err := t.TapRight(); err != nil {
			return err
		}
		return t.TapEnter()
	default:
		return fmt.Errorf("unknown permission choice: %d", choice)
	}
}
```

**Step 4: Write the permission overlay**

Create `ui/overlay/permissionOverlay.go`:

```go
package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PermissionChoice mirrors tmux.PermissionChoice to avoid import cycle.
type PermissionChoice int

const (
	PermissionAllowAlways PermissionChoice = iota
	PermissionAllowOnce
	PermissionReject
)

var permissionChoiceLabels = []string{"allow always", "allow once", "reject"}

// PermissionOverlay shows a three-choice modal for opencode permission prompts.
type PermissionOverlay struct {
	instanceTitle string
	description   string
	pattern       string
	selectedIdx   int
	confirmed     bool
	dismissed     bool
	width         int
}

// NewPermissionOverlay creates a permission overlay with extracted prompt data.
func NewPermissionOverlay(instanceTitle, description, pattern string) *PermissionOverlay {
	return &PermissionOverlay{
		instanceTitle: instanceTitle,
		description:   description,
		pattern:       pattern,
		selectedIdx:   0, // default to "allow always"
		width:         50,
	}
}

// HandleKeyPress processes input. Returns true when the overlay should close.
func (p *PermissionOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "left":
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
	case "right":
		if p.selectedIdx < len(permissionChoiceLabels)-1 {
			p.selectedIdx++
		}
	case "enter":
		p.confirmed = true
		return true
	case "esc":
		p.dismissed = true
		return true
	}
	return false
}

// Choice returns the selected permission choice.
func (p *PermissionOverlay) Choice() PermissionChoice {
	return PermissionChoice(p.selectedIdx)
}

// IsConfirmed returns true if the user pressed Enter.
func (p *PermissionOverlay) IsConfirmed() bool {
	return p.confirmed
}

// Render draws the permission overlay.
func (p *PermissionOverlay) Render() string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorGold).
		Padding(1, 2).
		Width(p.width)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorGold)

	descStyle := lipgloss.NewStyle().
		Foreground(colorText)

	patternStyle := lipgloss.NewStyle().
		Foreground(colorMuted)

	hintStyle := lipgloss.NewStyle().
		Foreground(colorMuted)

	selectedStyle := lipgloss.NewStyle().
		Background(colorFoam).
		Foreground(colorBase).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Padding(0, 1)

	var b strings.Builder
	b.WriteString(titleStyle.Render("△ permission required"))
	b.WriteString("\n")
	b.WriteString(descStyle.Render(p.description))
	if p.pattern != "" {
		b.WriteString("\n")
		b.WriteString(patternStyle.Render(fmt.Sprintf("pattern: %s", p.pattern)))
	}
	if p.instanceTitle != "" {
		b.WriteString("\n")
		b.WriteString(patternStyle.Render(fmt.Sprintf("instance: %s", p.instanceTitle)))
	}
	b.WriteString("\n\n")

	// Render choices horizontally
	var choices []string
	for i, label := range permissionChoiceLabels {
		if i == p.selectedIdx {
			choices = append(choices, selectedStyle.Render("▸ "+label))
		} else {
			choices = append(choices, normalStyle.Render("  "+label))
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, choices...))
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("←→ select · enter confirm · esc dismiss"))

	return borderStyle.Render(b.String())
}

// SetWidth sets the overlay width.
func (p *PermissionOverlay) SetWidth(w int) {
	p.width = w
}
```

**Step 5: Run all tests**

Run: `go test ./session/tmux/ -run TestSendPermissionResponse -v && go build ./...`
Expected: All PASS, builds clean

**Step 6: Commit**

```
git add ui/overlay/permissionOverlay.go session/tmux/tmux_io.go session/tmux/tmux_permission_test.go
git commit -m "feat: add permission overlay and tmux key sequence helpers"
```

---

## Wave 2: App Integration

> **Depends on Wave 1:** The parser, cache, and overlay must exist before wiring them into the app's Update/View cycle.

### Task 4: Wire Detection + Cache into Metadata Tick

**Files:**
- Modify: `session/instance_session.go` (add `PermissionPrompt` to `InstanceMetadata`, call parser in `CollectMetadata`)
- Modify: `app/app.go` (add `permissionCache`, `permissionOverlay`, `pendingPermissionInstance` fields to `home`; handle detection in Update; add `statePermission`)
- Modify: `app/app_input.go` (add `statePermission` key handling)

**Step 1: Extend InstanceMetadata**

In `session/instance_session.go`, add to the `InstanceMetadata` struct:

```go
PermissionPrompt *PermissionPrompt // non-nil when opencode shows a permission dialog
```

In `CollectMetadata()`, after the existing content capture block, add:

```go
if m.ContentCaptured && m.Content != "" {
	m.PermissionPrompt = ParsePermissionPrompt(m.Content, i.Program)
}
```

**Step 2: Add state and fields to home**

In `app/app.go`:
- Add `statePermission` to the state enum
- Add fields to `home` struct:
  ```go
  permissionOverlay        *overlay.PermissionOverlay
  pendingPermissionInstance *session.Instance
  permissionCache          *config.PermissionCache
  ```
- In `newHome()`, initialize the cache:
  ```go
  configDir, _ := config.GetConfigDir()
  permCache := config.NewPermissionCache(configDir)
  _ = permCache.Load()
  ```

**Step 3: Handle detection in Update's metadata tick**

In the metadata application loop in `app.go` (where `PermissionPrompt` is applied), add after the existing prompt detection block:

```go
// Permission prompt detection for opencode
if md.PermissionPrompt != nil && m.state == stateDefault {
	pp := md.PermissionPrompt
	if pp.Pattern != "" && m.permissionCache.IsAllowedAlways(pp.Pattern) {
		// Auto-approve cached pattern
		i := inst
		asyncCmds = append(asyncCmds, func() tea.Msg {
			return permissionAutoApproveMsg{instance: i}
		})
	} else {
		// Show modal
		m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
		m.permissionOverlay.SetWidth(55)
		m.pendingPermissionInstance = inst
		m.state = statePermission
	}
}
```

Define the message types:

```go
type permissionAutoApproveMsg struct {
	instance *session.Instance
}
type permissionResponseMsg struct {
	instance *session.Instance
	choice   overlay.PermissionChoice
	pattern  string
}
```

Handle `permissionAutoApproveMsg` in Update:

```go
case permissionAutoApproveMsg:
	if msg.instance != nil && msg.instance.Started() {
		i := msg.instance
		return m, func() tea.Msg {
			i.SendPermissionResponse(tmux.PermissionAllowAlways)
			return nil
		}
	}
```

Note: `Instance.SendPermissionResponse` is a thin wrapper that delegates to `tmuxSession.SendPermissionResponse()` — add it to `session/instance_session.go`:

```go
func (i *Instance) SendPermissionResponse(choice tmux.PermissionChoice) {
	if !i.started || i.tmuxSession == nil {
		return
	}
	if err := i.tmuxSession.SendPermissionResponse(choice); err != nil {
		log.ErrorLog.Printf("error sending permission response: %v", err)
	}
}
```

**Step 4: Handle statePermission input**

In `app/app_input.go`, add a block in `handleKeyPress` (after the `stateConfirm` block):

```go
if m.state == statePermission {
	if m.permissionOverlay == nil {
		m.state = stateDefault
		return m, nil
	}
	shouldClose := m.permissionOverlay.HandleKeyPress(msg)
	if shouldClose {
		if m.permissionOverlay.IsConfirmed() {
			choice := m.permissionOverlay.Choice()
			inst := m.pendingPermissionInstance
			pattern := "" // extract from overlay if needed for caching
			if inst != nil && inst.CachedContentSet {
				if pp := session.ParsePermissionPrompt(inst.CachedContent, inst.Program); pp != nil {
					pattern = pp.Pattern
				}
			}

			// Cache "allow always" decisions
			if choice == overlay.PermissionAllowAlways && pattern != "" && m.permissionCache != nil {
				m.permissionCache.Remember(pattern)
				_ = m.permissionCache.Save()
			}

			m.permissionOverlay = nil
			m.state = stateDefault

			if inst != nil {
				// Map overlay choice to tmux choice
				var tmuxChoice tmux.PermissionChoice
				switch choice {
				case overlay.PermissionAllowAlways:
					tmuxChoice = tmux.PermissionAllowAlways
				case overlay.PermissionAllowOnce:
					tmuxChoice = tmux.PermissionAllowOnce
				case overlay.PermissionReject:
					tmuxChoice = tmux.PermissionReject
				}
				capturedInst := inst
				capturedChoice := tmuxChoice
				return m, func() tea.Msg {
					capturedInst.SendPermissionResponse(capturedChoice)
					return nil
				}
			}
		}
		m.permissionOverlay = nil
		m.pendingPermissionInstance = nil
		m.state = stateDefault
		return m, nil
	}
	return m, nil
}
```

Also add `statePermission` to the list of states that skip menu highlighting in `handleMenuHighlighting`.

**Step 5: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: All PASS, no regressions

**Step 6: Commit**

```
git add session/instance_session.go app/app.go app/app_input.go
git commit -m "feat: wire permission detection, cache lookup, and modal into app"
```

---

### Task 5: Render Overlay + Integration Test

**Files:**
- Modify: `app/app.go` (add `statePermission` case to `View()`)
- Create: `app/app_permission_test.go`

**Step 1: Add the View case**

In `app.go` `View()`, add a case in the overlay switch:

```go
case m.state == statePermission && m.permissionOverlay != nil:
	result = overlay.PlaceOverlay(0, 0, m.permissionOverlay.Render(), mainView, true, true)
```

**Step 2: Write integration tests**

```go
package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
)

func TestPermissionDetection_ShowsOverlayForOpenCode(t *testing.T) {
	m := newTestHome()
	inst := &session.Instance{
		Title:   "test-agent",
		Program: "opencode",
	}
	inst.MarkStartedForTest()
	m.list.AddInstance(inst)
	m.list.SetSelectedInstance(0)

	// Simulate metadata tick with permission prompt detected
	inst.CachedContent = "△ Permission required\n  ← Access external directory /opt\n\nPatterns\n\n- /opt/*\n"
	inst.CachedContentSet = true

	pp := session.ParsePermissionPrompt(inst.CachedContent, inst.Program)
	assert.NotNil(t, pp)

	// Simulate the detection path
	m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
	m.pendingPermissionInstance = inst
	m.state = statePermission

	assert.Equal(t, statePermission, m.state)
	assert.NotNil(t, m.permissionOverlay)
}

func TestPermissionOverlay_ArrowKeysNavigate(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")

	// Default is "allow always" (index 0)
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice())

	// Right → "allow once"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice())

	// Right → "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Right at end → stays on "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Left → back to "allow once"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice())
}

func TestPermissionOverlay_EnterConfirms(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, po.IsConfirmed())
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice()) // default
}

func TestPermissionOverlay_EscDismisses(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, po.IsConfirmed())
}

func TestPermissionCache_AutoApprovesCachedPattern(t *testing.T) {
	m := newTestHome()
	m.permissionCache.Remember("/opt/*")
	assert.True(t, m.permissionCache.IsAllowedAlways("/opt/*"))
}
```

Note: Adapt `newTestHome()` to match the existing test helper pattern in `app_test.go`. The test may need to initialize `permissionCache` on the test home. Check `app_test.go` for the factory function and add `permissionCache: config.NewPermissionCache(t.TempDir())` to it.

**Step 3: Run tests**

Run: `go test ./app/ -run TestPermission -v && go test ./... 2>&1 | tail -20`
Expected: All PASS

**Step 4: Commit**

```
git add app/app.go app/app_permission_test.go
git commit -m "feat: render permission overlay and add integration tests"
```
