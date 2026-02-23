package planfsm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransition_ValidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
		to    Status
	}{
		{StatusReady, PlanStart, StatusPlanning},
		{StatusPlanning, PlannerFinished, StatusReady},
		{StatusReady, ImplementStart, StatusImplementing},
		{StatusImplementing, ImplementFinished, StatusReviewing},
		{StatusReviewing, ReviewApproved, StatusDone},
		{StatusReviewing, ReviewChangesRequested, StatusImplementing},
		{StatusDone, StartOver, StatusPlanning},
		{StatusReady, Cancel, StatusCancelled},
		{StatusPlanning, Cancel, StatusCancelled},
		{StatusImplementing, Cancel, StatusCancelled},
		{StatusReviewing, Cancel, StatusCancelled},
		{StatusCancelled, Reopen, StatusPlanning},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			result, err := ApplyTransition(tc.from, tc.event)
			require.NoError(t, err)
			assert.Equal(t, tc.to, result)
		})
	}
}

func TestTransition_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
	}{
		{StatusReady, PlannerFinished},    // not planning
		{StatusReady, ImplementFinished},  // not implementing
		{StatusReady, ReviewApproved},     // not reviewing
		{StatusPlanning, ImplementStart},  // must go through ready
		{StatusImplementing, PlanStart},   // can't go backwards
		{StatusDone, PlanStart},           // terminal
		{StatusDone, ImplementFinished},   // terminal
		{StatusCancelled, ImplementStart}, // must reopen first
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			_, err := ApplyTransition(tc.from, tc.event)
			assert.Error(t, err)
		})
	}
}

func TestIsUserOnly(t *testing.T) {
	assert.True(t, StartOver.IsUserOnly())
	assert.True(t, Cancel.IsUserOnly())
	assert.True(t, Reopen.IsUserOnly())
	assert.False(t, PlannerFinished.IsUserOnly())
	assert.False(t, ReviewApproved.IsUserOnly())
}
