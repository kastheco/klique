package taskfsm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyHook_Name(t *testing.T) {
	h := NewNotifyHook()
	assert.Equal(t, "notify", h.Name())
}

func TestNotifyHook_CallsNotifyFunc(t *testing.T) {
	h := NewNotifyHook()

	var capturedTitle, capturedBody string
	h.notifyFunc = func(title, body string) {
		capturedTitle = title
		capturedBody = body
	}

	ev := TransitionEvent{
		PlanFile:   "my-plan",
		FromStatus: StatusReady,
		ToStatus:   StatusPlanning,
		Event:      PlanStart,
		Timestamp:  time.Now(),
		Project:    "test-proj",
	}

	err := h.Run(context.Background(), ev)
	require.NoError(t, err)

	assert.True(t, strings.Contains(capturedTitle, ev.PlanFile),
		"title should contain plan file, got: %q", capturedTitle)
	assert.True(t, strings.Contains(capturedBody, string(ev.Event)),
		"body should contain the event string, got: %q", capturedBody)
}
