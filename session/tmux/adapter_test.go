package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaudeAdapter_ReadyString(t *testing.T) {
	a := claudeAdapter{}
	assert.Equal(t, "Do you trust the files in this folder?", a.ReadyString())
}

func TestClaudeAdapter_DetectPrompt(t *testing.T) {
	a := claudeAdapter{}
	assert.True(t, a.DetectPrompt("No, and tell Claude what to do differently"))
	assert.False(t, a.DetectPrompt("Working on it..."))
}

func TestClaudeAdapter_ReadyTap(t *testing.T) {
	a := claudeAdapter{}
	assert.True(t, a.NeedsTrustTap())
}

func TestOpenCodeAdapter_ReadyString(t *testing.T) {
	a := opencodeAdapter{}
	assert.Equal(t, "Ask anything", a.ReadyString())
}

func TestOpenCodeAdapter_DetectPrompt(t *testing.T) {
	a := opencodeAdapter{}
	// opencode idle = no "esc interrupt" shown
	assert.True(t, a.DetectPrompt("some content without interrupt"))
	assert.False(t, a.DetectPrompt("some content esc interrupt more"))
}

func TestOpenCodeAdapter_ReadyTap(t *testing.T) {
	a := opencodeAdapter{}
	assert.False(t, a.NeedsTrustTap())
}

func TestAdapterFor_Claude(t *testing.T) {
	a := AdapterFor("claude")
	assert.NotNil(t, a)
	assert.Equal(t, "Do you trust the files in this folder?", a.ReadyString())
}

func TestAdapterFor_OpenCode(t *testing.T) {
	a := AdapterFor("opencode")
	assert.NotNil(t, a)
	assert.Equal(t, "Ask anything", a.ReadyString())
}

func TestAdapterFor_Unknown(t *testing.T) {
	a := AdapterFor("vim")
	assert.Nil(t, a)
}
