package session

import (
	"testing"

	"github.com/kastheco/kasmos/session/headless"
	"github.com/kastheco/kasmos/session/tmux"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeExecutionMode(t *testing.T) {
	tests := []struct {
		name     string
		in       ExecutionMode
		expected ExecutionMode
	}{
		{name: "empty defaults to tmux", in: "", expected: ExecutionModeTmux},
		{name: "tmux stays tmux", in: ExecutionModeTmux, expected: ExecutionModeTmux},
		{name: "headless stays headless", in: ExecutionModeHeadless, expected: ExecutionModeHeadless},
		{name: "whitespace headless", in: "  headless  ", expected: ExecutionModeHeadless},
		{name: "unknown defaults to tmux", in: ExecutionMode("bogus"), expected: ExecutionModeTmux},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, NormalizeExecutionMode(tc.in))
		})
	}
}

func TestNewExecutionSession(t *testing.T) {
	tests := []struct {
		name     string
		mode     ExecutionMode
		wantType interface{}
	}{
		{name: "tmux mode creates tmux session", mode: ExecutionModeTmux, wantType: &tmux.TmuxSession{}},
		{name: "empty mode creates tmux session", mode: "", wantType: &tmux.TmuxSession{}},
		{name: "unknown mode creates tmux session", mode: ExecutionMode("bogus"), wantType: &tmux.TmuxSession{}},
		{name: "headless mode creates headless session", mode: ExecutionModeHeadless, wantType: &headless.Session{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess := NewExecutionSession(tc.mode, "test", "claude", false)
			switch tc.wantType.(type) {
			case *tmux.TmuxSession:
				_, ok := sess.(*tmux.TmuxSession)
				assert.True(t, ok)
			case *headless.Session:
				_, ok := sess.(*headless.Session)
				assert.True(t, ok)
			}
		})
	}
}
