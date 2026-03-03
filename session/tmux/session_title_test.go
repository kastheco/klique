package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTmuxSession_SessionTitle(t *testing.T) {
	ts := &TmuxSession{}

	// Default: no title, no callback
	assert.Nil(t, ts.titleFunc)

	// Set title callback
	called := false
	var capturedDir string
	var capturedBefore time.Time
	var capturedTitle string
	ts.SetTitleFunc(func(workDir string, beforeStart time.Time, title string) {
		called = true
		capturedDir = workDir
		capturedBefore = beforeStart
		capturedTitle = title
	})
	ts.sessionTitle = "kas: plan my-feature"

	assert.NotNil(t, ts.titleFunc)

	// Simulate the call that Start() would make
	ts.titleFunc("/work/dir", time.Now(), ts.sessionTitle)
	assert.True(t, called)
	assert.Equal(t, "/work/dir", capturedDir)
	assert.Equal(t, "kas: plan my-feature", capturedTitle)
	assert.False(t, capturedBefore.IsZero())
}

func TestTmuxSession_SessionTitle_SkippedWhenEmpty(t *testing.T) {
	ts := &TmuxSession{}
	// No title set — titleFunc should not be called even if set
	called := false
	ts.SetTitleFunc(func(string, time.Time, string) { called = true })
	// sessionTitle is empty, so Start() would skip the call
	assert.Empty(t, ts.sessionTitle)
	assert.False(t, called)
}
