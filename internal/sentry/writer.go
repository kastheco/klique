package sentry

import (
	"io"
	"strings"

	gosentry "github.com/getsentry/sentry-go"
)

// Level represents the severity level for the sentry writer.
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelError
)

// Writer wraps an io.Writer and forwards log messages to Sentry.
// Errors become Sentry events; warnings and info become breadcrumbs.
type Writer struct {
	inner io.Writer
	level Level
}

// NewWriter creates a Writer that tees to inner and forwards to Sentry.
func NewWriter(inner io.Writer, level Level) *Writer {
	return &Writer{inner: inner, level: level}
}

func (w *Writer) Write(p []byte) (int, error) {
	// Always write to the original destination first.
	n, err := w.inner.Write(p)

	if !enabled {
		return n, err
	}

	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return n, err
	}

	switch w.level {
	case LevelError:
		gosentry.CaptureMessage(msg)
	case LevelWarning:
		gosentry.AddBreadcrumb(&gosentry.Breadcrumb{
			Level:    gosentry.LevelWarning,
			Category: "log",
			Message:  msg,
		})
	case LevelInfo:
		gosentry.AddBreadcrumb(&gosentry.Breadcrumb{
			Level:    gosentry.LevelInfo,
			Category: "log",
			Message:  msg,
		})
	}

	return n, err
}
