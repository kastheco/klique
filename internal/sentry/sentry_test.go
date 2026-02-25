package sentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit_Disabled(t *testing.T) {
	err := Init("1.0.0", false)
	assert.NoError(t, err)
	// Flush and RecoverPanic should be safe no-ops
	Flush()
}

func TestInit_EmptyDSN(t *testing.T) {
	origDSN := dsn
	dsn = ""
	defer func() { dsn = origDSN }()

	err := Init("1.0.0", true)
	assert.NoError(t, err)
	Flush()
}

func TestIsEnabled(t *testing.T) {
	enabled = false
	assert.False(t, IsEnabled())
	enabled = true
	assert.True(t, IsEnabled())
	enabled = false // reset
}
