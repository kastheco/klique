package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCycleNext_WrapsToBeginning(t *testing.T) {
	l := makeScrollTestList(t, 5)
	require.Equal(t, 5, l.NumInstances())

	// Move to last item
	for i := 0; i < 4; i++ {
		l.Down()
	}
	assert.Equal(t, 4, l.SelectedIndex())

	// CycleNext should wrap to 0
	l.CycleNext()
	assert.Equal(t, 0, l.SelectedIndex(), "CycleNext at end should wrap to beginning")
}

func TestListCycleNext_AdvancesNormally(t *testing.T) {
	l := makeScrollTestList(t, 5)
	assert.Equal(t, 0, l.SelectedIndex())

	l.CycleNext()
	assert.Equal(t, 1, l.SelectedIndex(), "CycleNext should advance by one")
}

func TestListCyclePrev_WrapsToEnd(t *testing.T) {
	l := makeScrollTestList(t, 5)
	assert.Equal(t, 0, l.SelectedIndex())

	// CyclePrev at beginning should wrap to last item
	l.CyclePrev()
	assert.Equal(t, 4, l.SelectedIndex(), "CyclePrev at beginning should wrap to end")
}

func TestListCyclePrev_MovesBackNormally(t *testing.T) {
	l := makeScrollTestList(t, 5)
	l.Down()
	l.Down()
	assert.Equal(t, 2, l.SelectedIndex())

	l.CyclePrev()
	assert.Equal(t, 1, l.SelectedIndex(), "CyclePrev should move back by one")
}

func TestListCycleNext_SingleItem(t *testing.T) {
	l := makeScrollTestList(t, 1)
	assert.Equal(t, 0, l.SelectedIndex())

	l.CycleNext()
	assert.Equal(t, 0, l.SelectedIndex(), "CycleNext with single item should stay at 0")
}

func TestListCyclePrev_SingleItem(t *testing.T) {
	l := makeScrollTestList(t, 1)
	assert.Equal(t, 0, l.SelectedIndex())

	l.CyclePrev()
	assert.Equal(t, 0, l.SelectedIndex(), "CyclePrev with single item should stay at 0")
}

func TestListCycleNext_EmptyList(t *testing.T) {
	l := makeScrollTestList(t, 0)
	l.CycleNext() // should not panic
}

func TestListCyclePrev_EmptyList(t *testing.T) {
	l := makeScrollTestList(t, 0)
	l.CyclePrev() // should not panic
}
