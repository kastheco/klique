package ui

import (
	"bytes"
	"testing"
)

func TestSetTerminalBackground_EmitsOSC11(t *testing.T) {
	var buf bytes.Buffer
	restore := setTermBg(&buf, "#232136")
	got := buf.String()
	want := "\033]11;#232136\033\\"
	if got != want {
		t.Errorf("OSC 11 set: got %q, want %q", got, want)
	}

	buf.Reset()
	restore()
	got = buf.String()
	want = "\033]111\033\\"
	if got != want {
		t.Errorf("OSC 111 restore: got %q, want %q", got, want)
	}
}

func TestSetTerminalBackground_InvalidColor(t *testing.T) {
	var buf bytes.Buffer
	restore := setTermBg(&buf, "")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty color, got %q", buf.String())
	}
	restore() // should not panic
}
