package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
