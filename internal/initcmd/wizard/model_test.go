package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootModelInitializesSteps(t *testing.T) {
	m := newRootModel(nil, nil)
	require.Len(t, m.steps, 3)
	assert.Equal(t, 3, m.totalSteps)
	_, ok := m.steps[0].(*harnessStep)
	assert.True(t, ok)
}

func TestRootModelStepTransitions(t *testing.T) {
	t.Run("initial step is 0", func(t *testing.T) {
		m := newRootModel(nil, nil)
		assert.Equal(t, 0, m.step)
	})

	t.Run("nextStep advances and caps at maxStep", func(t *testing.T) {
		m := newRootModel(nil, nil)
		m.totalSteps = 3
		m.step = 1
		m.nextStep()
		assert.Equal(t, 2, m.step)
		m.nextStep()
		assert.Equal(t, 2, m.step) // capped
	})

	t.Run("prevStep decrements and floors at 0", func(t *testing.T) {
		m := newRootModel(nil, nil)
		m.step = 1
		m.prevStep()
		assert.Equal(t, 0, m.step)
		m.prevStep()
		assert.Equal(t, 0, m.step) // floored
	})
}

func TestStepIndicator(t *testing.T) {
	// Visual output test — just check it doesn't panic and has expected structure
	indicator := renderStepIndicator(1, 3)
	assert.Contains(t, indicator, "●")
	assert.Contains(t, indicator, "○")
}

func TestRootModelHandlesStepCancelMsg(t *testing.T) {
	m := newRootModel(nil, nil)
	next, cmd := m.Update(stepCancelMsg{})
	rm, ok := next.(rootModel)
	require.True(t, ok)
	assert.True(t, rm.cancelled)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok = msg.(tea.QuitMsg)
	assert.True(t, ok)
}
