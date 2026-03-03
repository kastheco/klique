package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "double quote",
			input: `say "hello"`,
			want:  `say \"hello\"`,
		},
		{
			name:  "backslash",
			input: `path\to\file`,
			want:  `path\\to\\file`,
		},
		{
			name:  "backslash before quote",
			input: `\"`,
			want:  `\\\"`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeAppleScript(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSendNotificationDisabled(t *testing.T) {
	// When notifications are disabled, SendNotification must be a no-op.
	// We verify by temporarily disabling and ensuring it doesn't panic or exec.
	orig := NotificationsEnabled
	NotificationsEnabled = false
	defer func() { NotificationsEnabled = orig }()

	// Should return without error and without launching any process.
	assert.NotPanics(t, func() {
		SendNotification("test title", "test body")
	})
}

func TestSendNotificationEnabled(t *testing.T) {
	// When enabled, SendNotification should not panic on any platform.
	// (We cannot verify the external command fires without running the OS
	// notification stack — this just ensures fire-and-forget doesn't crash.)
	orig := NotificationsEnabled
	NotificationsEnabled = true
	defer func() { NotificationsEnabled = orig }()

	assert.NotPanics(t, func() {
		SendNotification("klique", "agent finished")
	})
}
