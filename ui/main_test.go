package ui

import (
	"os"
	"testing"

	zone "github.com/lrstanley/bubblezone/v2"
)

// TestMain initializes package-level state needed by all ui tests.
func TestMain(m *testing.M) {
	// Initialize bubblezone global manager (required for zone.Mark/zone.Get in tests)
	zone.NewGlobal()

	os.Exit(m.Run())
}
