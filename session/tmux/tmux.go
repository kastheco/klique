package tmux

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/log"
)

const ProgramClaude = "claude"

const ProgramAider = "aider"
const ProgramGemini = "gemini"
const ProgramOpenCode = "opencode"

// ansiRe strips ANSI escape sequences (SGR, cursor movement, etc.) so that
// content hashing is not affected by cursor blink, color resets, or other
// terminal control codes that change between captures of an otherwise-idle pane.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// TmuxSession represents a managed tmux session
type TmuxSession struct {
	// Initialized by NewTmuxSession
	//
	// The name of the tmux session and the sanitized name used for tmux commands.
	sanitizedName string
	program       string
	// ptyFactory is used to create a PTY for the tmux session.
	ptyFactory PtyFactory
	// cmdExec is used to execute commands in the tmux session.
	cmdExec cmd.Executor
	// skipPermissions appends --dangerously-skip-permissions to Claude commands
	skipPermissions bool
	// agentType, when non-empty, appends --agent <type> to the program command.
	agentType string
	// initialPrompt, when non-empty, is baked into the CLI command at Start()
	// using per-program syntax (--prompt for opencode, positional for claude).
	initialPrompt string
	// taskNumber, waveNumber, peerCount are set for wave task instances.
	// When non-zero, they are prepended as KASMOS_TASK, KASMOS_WAVE, KASMOS_PEERS
	// env vars to the program command string.
	taskNumber int
	waveNumber int
	peerCount  int
	// ProgressFunc is called with (stage, description) during Start() to report progress.
	ProgressFunc func(stage int, desc string)
	// promptFile is the path to a temporary file containing the initial prompt.
	// Set during Start() when the prompt exceeds maxInlinePromptLen. Cleaned up by Close().
	promptFile string

	// Initialized by Start or Restore
	//
	// ptmx is a PTY is running the tmux attach command. This can be resized to change the
	// stdout dimensions of the tmux pane. On detach, we close it and set a new one.
	// This should never be nil.
	ptmx *os.File
	// monitor monitors the tmux pane content and sends signals to the UI when it's status changes
	monitor *statusMonitor

	// Initialized by Attach
	// Deinitialized by Detach
	//
	// Channel to be closed at the very end of detaching. Used to signal callers.
	attachCh chan struct{}
	// While attached, we use some goroutines to manage the window size and stdin/stdout. This stuff
	// is used to terminate them on Detach. We don't want them to outlive the attached window.
	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup
}

const TmuxPrefix = "kas_"

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// cleanupSessionsRe matches current kas_ sessions and legacy klique_/hivemind_ sessions.
var cleanupSessionsRe = regexp.MustCompile(`(?:kas_|klique_|hivemind_).*:`)

func toKasTmuxName(str string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_") // tmux replaces all . with _
	return fmt.Sprintf("%s%s", TmuxPrefix, str)
}

// NewTmuxSession creates a new TmuxSession with the given name and program.
func NewTmuxSession(name string, program string, skipPermissions bool) *TmuxSession {
	return newTmuxSession(name, program, skipPermissions, MakePtyFactory(), cmd.MakeExecutor())
}

// NewTmuxSessionWithDeps creates a new TmuxSession with provided dependencies for testing.
func NewTmuxSessionWithDeps(name string, program string, skipPermissions bool, ptyFactory PtyFactory, cmdExec cmd.Executor) *TmuxSession {
	return newTmuxSession(name, program, skipPermissions, ptyFactory, cmdExec)
}

func newTmuxSession(name string, program string, skipPermissions bool, ptyFactory PtyFactory, cmdExec cmd.Executor) *TmuxSession {
	return &TmuxSession{
		sanitizedName:   toKasTmuxName(name),
		program:         program,
		skipPermissions: skipPermissions,
		ptyFactory:      ptyFactory,
		cmdExec:         cmdExec,
	}
}

// SetAgentType sets the agent type flag to inject at startup (planner/coder/reviewer).
func (t *TmuxSession) SetAgentType(agentType string) {
	t.agentType = strings.TrimSpace(agentType)
}

// SetInitialPrompt sets the initial prompt to bake into the CLI command at launch.
// Supported programs: opencode (--prompt), claude (positional arg).
// For unsupported programs the prompt is ignored; callers should keep
// QueuedPrompt set so the send-keys fallback fires.
func (t *TmuxSession) SetInitialPrompt(prompt string) {
	t.initialPrompt = prompt
}

// SetTaskEnv sets the task identity env vars for parallel wave execution.
// When set, KASMOS_TASK, KASMOS_WAVE, and KASMOS_PEERS are prepended to the
// program command string at Start() time.
func (t *TmuxSession) SetTaskEnv(taskNumber, waveNumber, peerCount int) {
	t.taskNumber = taskNumber
	t.waveNumber = waveNumber
	t.peerCount = peerCount
}

func (t *TmuxSession) reportProgress(stage int, desc string) {
	if t.ProgressFunc != nil {
		t.ProgressFunc(stage, desc)
	}
}

// isClaudeProgram returns true if the program string refers to Claude Code.
func isClaudeProgram(program string) bool {
	return strings.HasSuffix(program, ProgramClaude)
}

// isAiderProgram returns true if the program string refers to Aider.
func isAiderProgram(program string) bool {
	return strings.HasPrefix(program, ProgramAider)
}

// isGeminiProgram returns true if the program string refers to Gemini.
func isGeminiProgram(program string) bool {
	return strings.HasPrefix(program, ProgramGemini)
}

// isOpenCodeProgram returns true if the program string refers to OpenCode.
func isOpenCodeProgram(program string) bool {
	return strings.HasSuffix(program, ProgramOpenCode)
}

type statusMonitor struct {
	// Store hashes to save memory.
	prevOutputHash []byte
	// captureFailures counts consecutive capture-pane failures.
	// After a threshold we stop logging every tick to avoid spam.
	captureFailures int
	// unchangedTicks counts consecutive ticks where the pane content hash is identical.
	// HasUpdated only reports !updated after the count exceeds the debounce threshold,
	// preventing false Running→Ready transitions during brief pauses (API waits, thinking).
	unchangedTicks int
}

func newStatusMonitor() *statusMonitor {
	return &statusMonitor{}
}

// hash hashes the string after stripping ANSI escape sequences.
// Stripping ANSI ensures that cursor blink, color resets, and other
// terminal control codes don't cause false "content changed" detections
// when the semantic content is actually stable.
func (m *statusMonitor) hash(s string) []byte {
	h := sha256.New()
	stripped := ansiRe.ReplaceAllString(s, "")
	h.Write([]byte(stripped))
	return h.Sum(nil)
}

// Start creates and starts a new tmux session, then attaches to it. Program is the command to run in
// the session (ex. claude). workdir is the git worktree directory.
func (t *TmuxSession) Start(workDir string) error {
	// Reattach to a surviving session from a previous crash/interrupt
	// instead of killing it — the agent may still be running.
	if t.DoesSessionExist() {
		return t.Restore()
	}

	// Append --dangerously-skip-permissions for Claude programs if enabled
	program := t.program
	if t.skipPermissions && isClaudeProgram(program) {
		program = program + " --dangerously-skip-permissions"
	}
	if t.agentType != "" && !strings.Contains(program, "--agent") {
		program = program + " --agent " + t.agentType
	}
	if t.initialPrompt != "" {
		switch {
		case isOpenCodeProgram(t.program):
			program = program + " --prompt " + shellEscapeSingleQuote(t.initialPrompt)
		case isClaudeProgram(t.program):
			program = program + " " + t.promptArg()
		}
		// aider/gemini: no CLI prompt support — callers keep QueuedPrompt
		// set so the send-keys fallback fires from the app tick handler.
	}

	// Prepend KASMOS_MANAGED=1 so the agent process sees it from startup.
	// tmux set-environment (below) only affects new panes, not the initial program.
	program = "KASMOS_MANAGED=1 " + program

	// Prepend task identity env vars for parallel wave execution.
	if t.taskNumber > 0 {
		program = fmt.Sprintf("KASMOS_TASK=%d KASMOS_WAVE=%d KASMOS_PEERS=%d %s",
			t.taskNumber, t.waveNumber, t.peerCount, program)
	}

	t.reportProgress(1, "Creating tmux session...")

	// Create a new detached tmux session and start claude in it
	cmd := exec.Command("tmux", "new-session", "-d", "-s", t.sanitizedName, "-c", workDir, program)

	ptmx, err := t.ptyFactory.Start(cmd)
	if err != nil {
		// Cleanup any partially created session if any exists.
		if t.DoesSessionExist() {
			cleanupCmd := exec.Command("tmux", "kill-session", "-t", t.sanitizedName)
			if cleanupErr := t.cmdExec.Run(cleanupCmd); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
		}
		return fmt.Errorf("error starting tmux session: %w", err)
	}

	t.reportProgress(2, "Waiting for session to start...")

	// Poll for session existence with exponential backoff
	timeout := time.After(2 * time.Second)
	sleepDuration := 5 * time.Millisecond
	for !t.DoesSessionExist() {
		select {
		case <-timeout:
			if cleanupErr := t.Close(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			return fmt.Errorf("timed out waiting for tmux session %s: %v", t.sanitizedName, err)
		default:
			time.Sleep(sleepDuration)
			// Exponential backoff up to 50ms max
			if sleepDuration < 50*time.Millisecond {
				sleepDuration *= 2
			}
		}
	}
	ptmx.Close()

	// Set history limit to enable scrollback (default is 2000, we'll use 10000 for more history)
	historyCmd := exec.Command("tmux", "set-option", "-t", t.sanitizedName, "history-limit", "10000")
	if err := t.cmdExec.Run(historyCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to set history-limit for session %s: %v", t.sanitizedName, err)
	}

	// Enable mouse scrolling for the session
	mouseCmd := exec.Command("tmux", "set-option", "-t", t.sanitizedName, "mouse", "on")
	if err := t.cmdExec.Run(mouseCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to enable mouse scrolling for session %s: %v", t.sanitizedName, err)
	}

	// Inject KASMOS_MANAGED=1 so agents can detect they're running under kasmos orchestration.
	// This enables skills/prompts to use sentinel files instead of editing plan-state.json directly.
	envCmd := exec.Command("tmux", "set-environment", "-t", t.sanitizedName, "KASMOS_MANAGED", "1")
	if err := t.cmdExec.Run(envCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to set KASMOS_MANAGED env for session %s: %v", t.sanitizedName, err)
	}

	t.reportProgress(3, "Configuring session...")

	err = t.Restore()
	if err != nil {
		if cleanupErr := t.Close(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
		}
		return fmt.Errorf("error restoring tmux session: %w", err)
	}

	if isClaudeProgram(t.program) || isAiderProgram(t.program) || isGeminiProgram(t.program) || isOpenCodeProgram(t.program) {
		t.reportProgress(4, "Waiting for program to start...")

		var searchString string
		var tapFunc func() error // nil means no key tap needed (e.g. opencode)
		maxWaitTime := 30 * time.Second

		switch {
		case isClaudeProgram(t.program):
			searchString = "Do you trust the files in this folder?"
			tapFunc = t.TapEnter
		case isOpenCodeProgram(t.program):
			// opencode shows its input placeholder once the TUI is ready; no tap needed.
			searchString = "Ask anything"
			tapFunc = nil
		default: // aider / gemini
			searchString = "Open documentation url for more info"
			tapFunc = t.TapDAndEnter
			maxWaitTime = 45 * time.Second
		}

		// Poll with exponential backoff until the ready string appears or we time out.
		startTime := time.Now()
		sleepDuration := 100 * time.Millisecond

		for time.Since(startTime) < maxWaitTime {
			time.Sleep(sleepDuration)
			content, err := t.CapturePaneContent()
			if err == nil && strings.Contains(content, searchString) {
				if tapFunc != nil {
					if err := tapFunc(); err != nil {
						log.ErrorLog.Printf("could not tap enter on trust screen: %v", err)
					}
				}
				break
			}

			// Exponential backoff with cap at 1 second.
			sleepDuration = time.Duration(float64(sleepDuration) * 1.2)
			if sleepDuration > time.Second {
				sleepDuration = time.Second
			}
		}
	}
	return nil
}

// Restore attaches to an existing session and restores the window size
func (t *TmuxSession) Restore() error {
	ptmx, err := t.ptyFactory.Start(exec.Command("tmux", "attach-session", "-t", t.sanitizedName))
	if err != nil {
		return fmt.Errorf("error opening PTY: %w", err)
	}
	t.ptmx = ptmx
	t.monitor = newStatusMonitor()
	return nil
}

// Close terminates the tmux session and cleans up resources
func (t *TmuxSession) Close() error {
	var errs []error

	if t.ptmx != nil {
		if err := t.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
		}
		t.ptmx = nil
	}

	cmd := exec.Command("tmux", "kill-session", "-t", t.sanitizedName)
	if err := t.cmdExec.Run(cmd); err != nil {
		errs = append(errs, fmt.Errorf("error killing tmux session: %w", err))
	}

	if t.promptFile != "" {
		os.Remove(t.promptFile)
		t.promptFile = ""
	}

	return errors.Join(errs...)
}

func (t *TmuxSession) DoesSessionExist() bool {
	// Using "-t name" does a prefix match, which is wrong. `-t=` does an exact match.
	existsCmd := exec.Command("tmux", "has-session", fmt.Sprintf("-t=%s", t.sanitizedName))
	return t.cmdExec.Run(existsCmd) == nil
}

// CleanupSessions kills all tmux sessions that start with the kas prefix.
// Also cleans up legacy "hivemind_" sessions from before the rename.
func CleanupSessions(cmdExec cmd.Executor) error {
	// First try to list sessions
	cmd := exec.Command("tmux", "ls")
	output, err := cmdExec.Output(cmd)

	// If there's an error and it's because no server is running, that's fine
	// Exit code 1 typically means no sessions exist
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // No sessions to clean up
		}
		return fmt.Errorf("failed to list tmux sessions: %v", err)
	}

	matches := cleanupSessionsRe.FindAllString(string(output), -1)
	for i, match := range matches {
		matches[i] = match[:strings.Index(match, ":")]
	}

	for _, match := range matches {
		log.InfoLog.Printf("cleaning up session: %s", match)
		if err := cmdExec.Run(exec.Command("tmux", "kill-session", "-t", match)); err != nil {
			return fmt.Errorf("failed to kill tmux session %s: %v", match, err)
		}
	}
	return nil
}
