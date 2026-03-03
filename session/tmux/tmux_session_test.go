package tmux

import (
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
)

func TestToKasTmuxName(t *testing.T) {
	assert.Equal(t, "kas_asdf", toKasTmuxName("asdf"))
	assert.Equal(t, "kas_asdf__asdf", toKasTmuxName("a sd f . . asdf"))
}

func TestNewTmuxSession_SanitizesName(t *testing.T) {
	s := NewTmuxSession("my.session", "claude", false)
	assert.Equal(t, "kas_my_session", s.GetSanitizedName())
}

func TestSetAgentType(t *testing.T) {
	s := NewTmuxSession("test", "opencode", false)
	s.SetAgentType("  planner  ")
	// agentType should be trimmed — verified indirectly via Start command construction
	assert.Equal(t, "planner", s.agentType)
}

func TestSetTaskEnv(t *testing.T) {
	s := NewTmuxSession("test", "claude", false)
	s.SetTaskEnv(3, 2, 4)
	assert.Equal(t, 3, s.taskNumber)
	assert.Equal(t, 2, s.waveNumber)
	assert.Equal(t, 4, s.peerCount)
}

func TestNewReset_PreservesDeps(t *testing.T) {
	pty := NewMockPtyFactory(t)
	exec := cmd_test.NewMockExecutor()
	s := NewTmuxSessionWithDeps("orig", "claude", false, pty, exec)
	reset := s.NewReset("new", "opencode", true)
	assert.Equal(t, "kas_new", reset.GetSanitizedName())
}

func TestNewTmuxSessionFromExisting(t *testing.T) {
	s := NewTmuxSessionFromExisting("kas_orphan", "claude", false)
	assert.Equal(t, "kas_orphan", s.GetSanitizedName())
}

func TestToKasTmuxNamePublic(t *testing.T) {
	assert.Equal(t, "kas_foo", ToKasTmuxNamePublic("foo"))
}
