package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusMonitor_DetectsChange(t *testing.T) {
	m := NewStatusMonitor()
	assert.True(t, m.RecordContent("hello"))
	// Same content is still within the debounce window — reports updated.
	assert.True(t, m.RecordContent("hello"))
}

func TestStatusMonitor_DebounceThreshold(t *testing.T) {
	m := NewStatusMonitor()
	m.RecordContent("stable")
	// First 14 unchanged ticks still report "updated" (debouncing)
	for i := 0; i < 14; i++ {
		assert.True(t, m.RecordContent("stable"), "tick %d should still debounce", i)
	}
	// 15th unchanged tick reports "not updated"
	assert.False(t, m.RecordContent("stable"))
}

func TestStatusMonitor_ResetOnChange(t *testing.T) {
	m := NewStatusMonitor()
	m.RecordContent("a")
	for i := 0; i < 20; i++ {
		m.RecordContent("a")
	}
	// Now change — should report updated and reset debounce
	assert.True(t, m.RecordContent("b"))
}
