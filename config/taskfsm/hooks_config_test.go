package taskfsm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHookRegistry_NilInput(t *testing.T) {
	reg := BuildHookRegistry(nil)
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Len())
}

func TestBuildHookRegistry_EmptySlice(t *testing.T) {
	reg := BuildHookRegistry([]HookConfig{})
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Len())
}

func TestBuildHookRegistry_Webhook(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:    "webhook",
			URL:     "https://example.com/hook",
			Headers: map[string]string{"X-Token": "abc123"},
			Events:  []string{"plan_start", "implement_finished"},
		},
	}
	reg := BuildHookRegistry(cfg)
	require.NotNil(t, reg)
	assert.Equal(t, 1, reg.Len())
}

func TestBuildHookRegistry_WebhookEmptyURL(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:   "webhook",
			URL:    "", // invalid — should be skipped
			Events: []string{"plan_start"},
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 0, reg.Len(), "webhook with empty url should be skipped")
}

func TestBuildHookRegistry_Notify(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:   "notify",
			Events: []string{"implement_finished", "review_approved"},
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 1, reg.Len())
}

func TestBuildHookRegistry_Command(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:    "command",
			Command: "echo done",
			Events:  []string{"review_approved"},
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 1, reg.Len())
}

func TestBuildHookRegistry_CommandEmptyCommand(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:    "command",
			Command: "", // invalid — should be skipped
			Events:  []string{"review_approved"},
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 0, reg.Len(), "command with empty command should be skipped")
}

func TestBuildHookRegistry_UnknownType(t *testing.T) {
	cfg := []HookConfig{
		{
			Type: "unknown_type",
			URL:  "https://example.com",
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 0, reg.Len(), "unknown type should be skipped")
}

func TestBuildHookRegistry_MultipleHooks(t *testing.T) {
	cfg := []HookConfig{
		{Type: "webhook", URL: "https://example.com/hook"},
		{Type: "notify"},
		{Type: "command", Command: "echo done"},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 3, reg.Len())
}

func TestBuildHookRegistry_InvalidEventNamesIgnored(t *testing.T) {
	cfg := []HookConfig{
		{
			Type: "notify",
			// Mix of valid and invalid event names.
			Events: []string{"plan_start", "not_a_real_event", "implement_finished"},
		},
	}
	// The hook should still be registered (notify is valid), but the
	// internal filter should only contain the two valid events.
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 1, reg.Len(), "hook should be registered despite unknown event names")

	// Fire a transition for a known event — hook should fire.
	// Fire for an unknown event name would simply not match.
	// We can't inspect internal filter from outside, but we verify
	// the registry was built (not skipped) even with bad event names.
}

func TestBuildHookRegistry_AllEventsInvalid_HookSkipped(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:   "notify",
			Events: []string{"not_a_real_event", "another_bogus_event"},
		},
	}
	// When every event name is unknown, the hook must be skipped entirely
	// rather than registering with an empty filter (which would fire on all
	// transitions — the exact opposite of what the user intended).
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 0, reg.Len(), "hook with all-unknown events should be skipped")
}

func TestBuildHookRegistry_AllEventsInvalid_WebhookSkipped(t *testing.T) {
	cfg := []HookConfig{
		{
			Type:   "webhook",
			URL:    "https://example.com/hook",
			Events: []string{"totally_bogus"},
		},
	}
	reg := BuildHookRegistry(cfg)
	assert.Equal(t, 0, reg.Len(), "webhook with all-unknown events should be skipped")
}

func TestParseHookEvents_AllKnownEvents(t *testing.T) {
	all := []string{
		"plan_start",
		"planner_finished",
		"implement_start",
		"implement_finished",
		"review_approved",
		"review_changes_requested",
		"request_review",
		"start_over",
		"reimplement",
		"cancel",
		"reopen",
	}
	events := parseHookEvents(all)
	assert.Len(t, events, len(all), "all known event strings should parse successfully")
}

func TestParseHookEvents_UnknownStringsSkipped(t *testing.T) {
	raw := []string{"plan_start", "bogus_event", "implement_finished"}
	events := parseHookEvents(raw)
	assert.Len(t, events, 2, "unknown event strings should be skipped")
	assert.Equal(t, PlanStart, events[0])
	assert.Equal(t, ImplementFinished, events[1])
}
