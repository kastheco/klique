package wizard

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestRosePineMoonPalette(t *testing.T) {
	assert.Equal(t, lipgloss.Color("#232136"), colorBase)
	assert.Equal(t, lipgloss.Color("#2a273f"), colorSurface)
	assert.Equal(t, lipgloss.Color("#393552"), colorOverlay)
	assert.Equal(t, lipgloss.Color("#6e6a86"), colorMuted)
	assert.Equal(t, lipgloss.Color("#908caa"), colorSubtle)
	assert.Equal(t, lipgloss.Color("#e0def4"), colorText)

	assert.Equal(t, lipgloss.Color("#eb6f92"), colorLove)
	assert.Equal(t, lipgloss.Color("#f6c177"), colorGold)
	assert.Equal(t, lipgloss.Color("#ea9a97"), colorRose)
	assert.Equal(t, lipgloss.Color("#3e8fb0"), colorPine)
	assert.Equal(t, lipgloss.Color("#9ccfd8"), colorFoam)
	assert.Equal(t, lipgloss.Color("#c4a7e7"), colorIris)

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
