package ui

import (
	"fmt"
	"strings"
)

// testDocumentLines builds a multi-line string with n numbered lines.
// Used by tabbed_window_test.go to populate document-mode content.
func testDocumentLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&b, "line %d", i)
	}
	return b.String()
}
