package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRosePineMoonPalette(t *testing.T) {
	assert.Equal(t, "#232136", string(colorBase))
	assert.Equal(t, "#2a273f", string(colorSurface))
	assert.Equal(t, "#393552", string(colorOverlay))
	assert.Equal(t, "#6e6a86", string(colorMuted))
	assert.Equal(t, "#908caa", string(colorSubtle))
	assert.Equal(t, "#e0def4", string(colorText))

	assert.Equal(t, "#eb6f92", string(colorLove))
	assert.Equal(t, "#f6c177", string(colorGold))
	assert.Equal(t, "#ea9a97", string(colorRose))
	assert.Equal(t, "#3e8fb0", string(colorPine))
	assert.Equal(t, "#9ccfd8", string(colorFoam))
	assert.Equal(t, "#c4a7e7", string(colorIris))

	assert.Equal(t, "#9ccfd8", gradientStart)
	assert.Equal(t, "#c4a7e7", gradientEnd)
}

func TestCoreStyles(t *testing.T) {
	assert.Equal(t, colorText, titleStyle.GetForeground())
	assert.True(t, titleStyle.GetBold())

	assert.Equal(t, colorMuted, subtitleStyle.GetForeground())
	assert.Equal(t, colorOverlay, separatorStyle.GetForeground())
	assert.Equal(t, colorSubtle, hintKeyStyle.GetForeground())
	assert.Equal(t, colorMuted, hintDescStyle.GetForeground())
	assert.Equal(t, colorIris, harnessSelectedStyle.GetForeground())
	assert.Equal(t, colorText, harnessNormalStyle.GetForeground())
	assert.Equal(t, colorSubtle, harnessDimStyle.GetForeground())
	assert.Equal(t, colorMuted, harnessDescStyle.GetForeground())
	assert.Equal(t, colorSubtle, pathStyle.GetForeground())

	assert.Equal(t, colorIris, roleActiveStyle.GetForeground())
	assert.Equal(t, colorText, roleNormalStyle.GetForeground())
	assert.Equal(t, colorMuted, roleMutedStyle.GetForeground())
	assert.Equal(t, colorFoam, dotEnabledStyle.GetForeground())
	assert.Equal(t, colorMuted, dotDisabledStyle.GetForeground())

	assert.Equal(t, colorSubtle, labelStyle.GetForeground())
	assert.Equal(t, colorText, valueStyle.GetForeground())

	assert.Equal(t, colorIris, fieldActiveStyle.GetForeground())
	assert.Equal(t, colorText, fieldNormalStyle.GetForeground())
	assert.Equal(t, colorGold, defaultTagStyle.GetForeground())

	assert.Equal(t, colorFoam, stepDoneStyle.GetForeground())
	assert.Equal(t, colorIris, stepActiveStyle.GetForeground())
	assert.Equal(t, colorOverlay, stepPendingStyle.GetForeground())
}
