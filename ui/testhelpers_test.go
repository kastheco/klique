package ui

import (
	"fmt"
	"strings"
)

// testDocumentLines builds a multi-line string with n numbered lines for tests
// that need scrollable document-style content.
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
