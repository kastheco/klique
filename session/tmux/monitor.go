package tmux

import (
	"bytes"
	"crypto/sha256"

	"github.com/charmbracelet/x/ansi"
)

const debounceThreshold = 15

// StatusMonitor tracks pane content changes via SHA-256 hashing with ANSI
// stripping, plus debounce logic to prevent false Running→Ready transitions
// during brief pauses (API waits, tool calls, thinking time).
//
// Callers feed raw pane content to RecordContent on each tick; RecordContent
// returns true while the session is considered "active" (content changed or
// still within the debounce window after the last change).
type StatusMonitor struct {
	// prevOutputHash is the hash of the last observed (ANSI-stripped) content.
	prevOutputHash []byte
	// captureFailures counts consecutive capture-pane errors.
	captureFailures int
	// unchangedTicks counts consecutive ticks where content hash is identical.
	// Once this reaches debounceThreshold, RecordContent reports not-updated.
	unchangedTicks int
}

// NewStatusMonitor returns a fresh StatusMonitor ready for use.
func NewStatusMonitor() *StatusMonitor {
	return &StatusMonitor{}
}

// RecordContent hashes content (after ANSI stripping) and updates internal state.
// Returns true if the session is considered "updated" — either the content
// changed since the last call, or unchanged ticks are still within the debounce
// window (~15 ticks × 200 ms ≈ 3 s of stability required before reporting idle).
func (m *StatusMonitor) RecordContent(content string) bool {
	h := m.hash(content)
	if !bytes.Equal(h, m.prevOutputHash) {
		// Content changed — reset debounce window and report updated.
		m.prevOutputHash = h
		m.unchangedTicks = 0
		return true
	}
	// Content unchanged — increment the stable-tick counter.
	m.unchangedTicks++
	// Return true while still within the debounce window.
	return m.unchangedTicks < debounceThreshold
}

// RecordFailure increments the consecutive capture failure counter.
// Returns true if the failure should be logged: on the first failure, then
// every 75th, to avoid flooding logs for a permanently-gone pane.
func (m *StatusMonitor) RecordFailure() bool {
	m.captureFailures++
	return m.captureFailures == 1 || m.captureFailures%75 == 0
}

// ResetFailures resets the failure counter after a successful capture.
func (m *StatusMonitor) ResetFailures() {
	m.captureFailures = 0
}

// hash returns the SHA-256 of s after stripping ANSI escape sequences.
// Stripping ANSI ensures cursor blink, SGR resets, and other terminal control
// codes don't create false "content changed" signals when semantic content is stable.
func (m *StatusMonitor) hash(s string) []byte {
	stripped := ansi.Strip(s)
	h := sha256.Sum256([]byte(stripped))
	return h[:]
}
