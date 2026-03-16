package ui

import (
	"strings"
	"testing"
)

func stripPreviewANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
		}
		if !inEsc {
			b.WriteRune(r)
		}
		if inEsc && r == 'm' {
			inEsc = false
		}
	}
	return b.String()
}

func TestPreviewFallbackMessage_RemovesSidebarShortcutHint(t *testing.T) {
	p := NewPreviewPane()
	p.springAnim = nil // avoid animation placeholders in snapshot text assertions
	p.SetSize(120, 30)
	if err := p.UpdateContent(nil); err != nil {
		t.Fatalf("UpdateContent(nil) error: %v", err)
	}

	out := stripPreviewANSI(p.String())
	if strings.Contains(out, "[s]elect existing") {
		t.Fatalf("fallback message should not advertise removed s shortcut; got: %q", out)
	}
	if !strings.Contains(out, "create [n]ew plan or select existing") {
		t.Fatalf("fallback message should show updated copy; got: %q", out)
	}
}
