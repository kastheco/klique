package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBannerLines_ReturnsCorrectRowCount(t *testing.T) {
	lines := BannerLines(0)
	assert.Equal(t, 6, len(lines), "banner should have 6 rows")
}

func TestBannerLines_FrameWraps(t *testing.T) {
	// Should not panic on any frame index
	for i := 0; i < 20; i++ {
		lines := BannerLines(i)
		assert.Equal(t, 6, len(lines))
	}
}
