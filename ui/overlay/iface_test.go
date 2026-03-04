package overlay

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

// stubOverlay is a minimal Overlay implementation for testing the interface contract.
type stubOverlay struct {
	dismissed bool
	rendered  string
	w, h      int
}

func (s *stubOverlay) HandleKey(msg tea.KeyPressMsg) Result {
	if msg.Code == tea.KeyEscape {
		s.dismissed = true
		return Result{Dismissed: true}
	}
	if msg.Code == tea.KeyEnter {
		return Result{Dismissed: true, Submitted: true, Value: "test-value"}
	}
	return Result{}
}

func (s *stubOverlay) View() string     { return s.rendered }
func (s *stubOverlay) SetSize(w, h int) { s.w = w; s.h = h }

func TestOverlayInterface_Dismiss(t *testing.T) {
	var o Overlay = &stubOverlay{rendered: "hello"}
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.True(t, result.Dismissed)
	assert.False(t, result.Submitted)
}

func TestOverlayInterface_Submit(t *testing.T) {
	var o Overlay = &stubOverlay{rendered: "hello"}
	result := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, result.Dismissed)
	assert.True(t, result.Submitted)
	assert.Equal(t, "test-value", result.Value)
}

func TestOverlayInterface_SetSize(t *testing.T) {
	s := &stubOverlay{}
	var o Overlay = s
	o.SetSize(80, 24)
	assert.Equal(t, 80, s.w)
	assert.Equal(t, 24, s.h)
}

func TestResult_ActionField(t *testing.T) {
	r := Result{Dismissed: true, Action: "kill"}
	assert.Equal(t, "kill", r.Action)
}
