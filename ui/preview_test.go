package ui

import (
	"fmt"
	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/tmux"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// testSetup holds common test setup data
type testSetup struct {
	workdir     string
	instance    *session.Instance
	sessionName string
	cleanupFn   func()
}

// setupTestEnvironment creates a common test environment with git repo and instance
func setupTestEnvironment(t *testing.T, cmdExec cmd_test.MockCmdExec) *testSetup {
	t.Helper()

	// Initialize logging
	log.Initialize(false)

	// Set up a temp working directory
	workdir := t.TempDir()

	// Initialize git repository
	setupGitRepo(t, workdir)

	// Create unique session name
	random := time.Now().UnixNano() % 10000000
	sessionName := fmt.Sprintf("test-preview-%s-%d-%d", t.Name(), time.Now().UnixNano(), random)

	// Clean up any existing tmux session
	cleanupCmd := exec.Command("tmux", "kill-session", "-t", "kas_"+sessionName)
	_ = cleanupCmd.Run() // Ignore errors if session doesn't exist

	// Create instance
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   sessionName,
		Path:    workdir,
		Program: "bash",
		AutoYes: false,
	})
	require.NoError(t, err)

	// Create MockPtyFactory
	ptyFactory := &MockPtyFactory{
		t:       t,
		cmdExec: cmdExec,
	}

	// Set up tmux session with mocks
	tmuxSession := tmux.NewTmuxSessionWithDeps(sessionName, "bash", false, ptyFactory, cmdExec)
	instance.SetTmuxSession(tmuxSession)

	// Start the tmux session
	err = instance.Start(true)
	require.NoError(t, err)

	// Create cleanup function
	cleanupFn := func() {
		if instance != nil {
			_ = instance.Kill() // Ignore errors during cleanup
		}
		log.Close()
	}

	return &testSetup{
		workdir:     workdir,
		instance:    instance,
		sessionName: sessionName,
		cleanupFn:   cleanupFn,
	}
}

// setupGitRepo initializes a git repository in the given directory
func setupGitRepo(t *testing.T, workdir string) {
	t.Helper()

	// Initialize git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = workdir
	err := initCmd.Run()
	require.NoError(t, err)

	// Create basic git config (local to this repo only)
	configCmd := exec.Command("git", "config", "--local", "user.email", "test@example.com")
	configCmd.Dir = workdir
	err = configCmd.Run()
	require.NoError(t, err)

	configCmd = exec.Command("git", "config", "--local", "user.name", "Test User")
	configCmd.Dir = workdir
	err = configCmd.Run()
	require.NoError(t, err)

	// Create and commit a test file
	testFile := filepath.Join(workdir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = workdir
	err = addCmd.Run()
	require.NoError(t, err)

	commitCmd := exec.Command("git", "commit", "-m", "initial commit")
	commitCmd.Dir = workdir
	err = commitCmd.Run()
	require.NoError(t, err)
}

// TestPreviewScrolling tests the scrolling functionality in the preview pane
func TestPreviewScrolling(t *testing.T) {
	// Track what commands were executed and their order
	var executedCommands []string
	inCopyMode := false
	scrollPosition := 0 // 0 = bottom, positive = scrolled up
	sessionCreated := false

	// Create test content with line numbers for scrolling
	const numLines = 100
	lines := make([]string, numLines+1)
	lines[0] = "$ seq 100" // Command that was run
	for i := 1; i <= numLines; i++ {
		lines[i] = fmt.Sprintf("%d", i)
	}
	fullContent := strings.Join(lines, "\n")

	// Mock command execution
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			executedCommands = append(executedCommands, cmdStr)

			// Handle tmux session creation and existence checking
			if strings.Contains(cmdStr, "has-session") {
				if sessionCreated {
					return nil // Session exists
				} else {
					return fmt.Errorf("session does not exist")
				}
			}

			// Handle session creation
			if strings.Contains(cmdStr, "new-session") {
				sessionCreated = true
				return nil
			}

			// Handle attach-session
			if strings.Contains(cmdStr, "attach-session") {
				return nil
			}

			// Handle copy mode commands
			if strings.Contains(cmdStr, "copy-mode") {
				inCopyMode = true
			}
			if strings.Contains(cmdStr, "send-keys") && strings.Contains(cmdStr, "q") {
				inCopyMode = false
				scrollPosition = 0 // Reset position when exiting copy mode
			}
			if strings.Contains(cmdStr, "send-keys") && strings.Contains(cmdStr, "Up") {
				if inCopyMode {
					scrollPosition++
				}
			}
			if strings.Contains(cmdStr, "send-keys") && strings.Contains(cmdStr, "Down") {
				if inCopyMode && scrollPosition > 0 {
					scrollPosition--
				}
			}

			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()

			// Handle capture-pane commands
			if strings.Contains(cmdStr, "capture-pane") {
				// Check if this is a request for cursor position
				if strings.Contains(cmdStr, "display-message") && strings.Contains(cmdStr, "copy_cursor_y") {
					var buf []byte
					buf = fmt.Appendf(buf, "%d", scrollPosition)
					return buf, nil
				}

				// Check if this is a copy mode capture with full history (-S -)
				if strings.Contains(cmdStr, "-S -") {
					// Always return the full content for PreviewFullHistory
					return []byte(fullContent), nil
				}

				// Regular capture for normal preview mode - show the last 20 lines
				const visibleLines = 20
				startLine := max(0, numLines+1-visibleLines)
				visibleContent := strings.Join(lines[startLine:], "\n")
				return []byte(visibleContent), nil
			}

			return []byte(""), nil
		},
	}

	// Setup test environment
	setup := setupTestEnvironment(t, cmdExec)
	defer setup.cleanupFn()

	// Simulate running a command that produces lots of output
	err := setup.instance.SendKeys("seq 100")
	require.NoError(t, err)
	err = setup.instance.SendKeys("") // Simulate pressing Enter
	require.NoError(t, err)

	// Create the preview pane
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, 30) // Set reasonable size for testing

	// Step 1: Check initial content - should show normal preview mode
	err = previewPane.UpdateContent(setup.instance)
	require.NoError(t, err)

	// Verify we're not in scrolling mode initially
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode initially")

	// Step 2: Check that PreviewFullHistory returns all content
	fullHistory, err := setup.instance.PreviewFullHistory()
	require.NoError(t, err)

	// Verify that the full history contains both the command and early output
	require.Contains(t, fullHistory, "$ seq 100", "Full history should contain the command")
	require.Contains(t, fullHistory, "1", "Full history should contain earliest output")

	// Step 3: Enter scroll mode
	err = previewPane.ScrollUp(setup.instance)
	require.NoError(t, err)

	// Verify we entered scrolling mode
	require.True(t, previewPane.isScrolling, "Should be in scrolling mode after ScrollUp")

	// Step 4: Get the content directly from the viewport
	viewportContent := previewPane.viewport.View()
	t.Logf("Viewport content: %q", viewportContent)

	// With proper implementation, the viewport should have the full history content
	// Note: The viewport will be positioned at the bottom initially, so we need to scroll up

	// Step 5: Scroll up multiple times to get to the top
	for range 50 {
		err = previewPane.ScrollUp(setup.instance)
		require.NoError(t, err)
	}

	// Now get the viewport content after scrolling up
	viewportAfterScrollUp := previewPane.viewport.View()
	t.Logf("Viewport after scrolling up: %q", viewportAfterScrollUp)

	// Step 6: Scroll down multiple times
	for range 25 {
		err = previewPane.ScrollDown(setup.instance)
		require.NoError(t, err)
	}

	// Get updated viewport content after scrolling down
	viewportAfterScrollDown := previewPane.viewport.View()
	t.Logf("Viewport after scrolling down: %q", viewportAfterScrollDown)

	// Step 7: Reset to normal mode
	err = previewPane.ResetToNormalMode(setup.instance)
	require.NoError(t, err)

	// Verify we exited scrolling mode
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode after reset")
}

// MockPtyFactory for testing tmux sessions
type MockPtyFactory struct {
	t       *testing.T
	cmdExec cmd_test.MockCmdExec

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", pt.t.Name(), len(pt.cmds)))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)

		// Execute the command through our mock to trigger session creation logic
		_ = pt.cmdExec.Run(cmd)
	}
	return f, err
}

func (pt *MockPtyFactory) Close() {}

// TestPreviewContentWithoutScrolling tests that the preview pane correctly displays content
// pushed via SetRawContent (the VT emulator path) without requiring scrolling.
// In the new architecture, UpdateContent no longer fetches content in normal mode —
// live content arrives via SetRawContent from the VT emulator tick loop.
func TestPreviewContentWithoutScrolling(t *testing.T) {
	// Create the preview pane
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, 30)

	// Simulate the VT emulator pushing rendered content via SetRawContent.
	expectedContent := "$ echo test\ntest"
	previewPane.SetRawContent(expectedContent)

	// Verify we're not in scrolling mode
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode")

	// Verify that the preview state is not in fallback mode
	require.False(t, previewPane.previewState.fallback, "Preview should not be in fallback mode")

	// Verify that the preview state contains the expected content
	require.Equal(t, expectedContent, previewPane.previewState.text, "Preview state should contain the expected content")

	// Verify the rendered string contains the content
	renderedString := previewPane.String()
	require.Contains(t, renderedString, "test", "Rendered preview should contain the test content")
}

func TestPreviewPaneSetSize_ReservesScrollbarColumn(t *testing.T) {
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, 24)

	require.Equal(t, 80, previewPane.width)
	require.Equal(t, 79, previewPane.viewport.Width)
	require.Equal(t, 24, previewPane.viewport.Height)
}

func TestPreviewPaneViewportUpdate_DocumentModeHandlesNativeKeys(t *testing.T) {
	previewPane := NewPreviewPane()
	previewPane.SetSize(30, 5)
	previewPane.SetDocumentContent(testDocumentLines(40))

	before := previewPane.viewport.View()
	cmd := previewPane.ViewportUpdate(tea.KeyMsg{Type: tea.KeyPgDown})
	after := previewPane.viewport.View()

	require.Nil(t, cmd)
	require.NotEqual(t, before, after)
}

func TestPreviewPaneViewportUpdate_NoOpOutsideScrollableModes(t *testing.T) {
	previewPane := NewPreviewPane()
	previewPane.SetSize(30, 5)
	previewPane.SetRawContent("plain preview")

	cmd := previewPane.ViewportUpdate(tea.KeyMsg{Type: tea.KeyPgDown})

	require.Nil(t, cmd)
	require.False(t, previewPane.IsDocumentMode())
	require.False(t, previewPane.isScrolling)
}

func TestPreviewPaneString_RendersScrollbarOnlyWhenScrollable(t *testing.T) {
	t.Run("shows scrollbar for long document", func(t *testing.T) {
		previewPane := NewPreviewPane()
		previewPane.SetSize(30, 6)
		previewPane.SetDocumentContent(testDocumentLines(60))

		rendered := previewPane.String()
		require.Contains(t, rendered, "▐")
		require.Contains(t, rendered, "│")
	})

	t.Run("hides scrollbar when content fits", func(t *testing.T) {
		previewPane := NewPreviewPane()
		previewPane.SetSize(30, 6)
		previewPane.SetDocumentContent(testDocumentLines(2))

		rendered := previewPane.String()
		require.NotContains(t, rendered, "▐")
		require.NotContains(t, rendered, "│")
	})
}

func TestPreviewPaneString_CentersFallbackContentInShortHeight(t *testing.T) {
	previewPane := NewPreviewPane()
	previewPane.SetSize(20, 8)
	previewPane.setFallbackContent("X")

	rendered := previewPane.String()
	lines := strings.Split(rendered, "\n")

	markerLine := -1
	for i, line := range lines {
		if strings.Contains(line, "X") {
			markerLine = i
			break
		}
	}

	require.Equal(t, 3, markerLine)
}

// TestPreviewPane_RawTerminalContent_NoEllipsis verifies that content pushed via
// SetRawContent (the VT emulator path) is rendered without clipping or an
// ellipsis marker, even when the number of lines exactly matches the pane height.
// Previously, String() unconditionally subtracted 1 from the available height,
// causing the last line of every embedded terminal frame to be dropped and
// replaced with "...".
func TestPreviewPane_RawTerminalContent_NoEllipsis(t *testing.T) {
	const rows = 24
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, rows)

	// Build a rows-line string that simulates a VT-emulator snapshot.
	// Each line is uniquely identifiable so we can check the last one is present.
	lineStrs := make([]string, rows)
	for i := range rows {
		lineStrs[i] = fmt.Sprintf("terminal line %d", i+1)
	}
	rawContent := strings.Join(lineStrs, "\n")

	previewPane.SetRawContent(rawContent)

	rendered := previewPane.String()
	plain := stripPreviewANSI(rendered)

	// The last line must appear in the output.
	lastLine := fmt.Sprintf("terminal line %d", rows)
	require.Contains(t, plain, lastLine,
		"last VT-emulator line should appear in rendered output (no clipping)")

	// No ellipsis should be injected by the preview pane.
	require.NotContains(t, plain, "...",
		"preview pane must not inject '...' for raw terminal content")
}

func testDocumentLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&b, "line %d", i)
	}
	return b.String()
}
