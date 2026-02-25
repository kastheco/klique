package sentry

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriter_PassthroughToInner(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, LevelError)

	msg := []byte("test error message\n")
	n, err := w.Write(msg)

	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
	assert.Equal(t, string(msg), buf.String())
}

func TestWriter_DisabledPassthrough(t *testing.T) {
	enabled = false
	var buf bytes.Buffer
	w := NewWriter(&buf, LevelError)

	msg := []byte("test message\n")
	n, err := w.Write(msg)

	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
	assert.Equal(t, string(msg), buf.String())
}
