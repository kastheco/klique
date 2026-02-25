package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	kaslog "github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
)

func makeScrollTestList(t *testing.T, n int) *List {
	t.Helper()
	kaslog.Initialize(false)
	sp := spinner.New()
	l := NewList(&sp, false)
	l.SetSize(40, 30)
	for i := 0; i < n; i++ {
		inst := &session.Instance{}
		inst.Title = fmt.Sprintf("inst-%d", i)
		inst.MemMB = 100
		finalize := l.AddInstance(inst)
		finalize()
		inst.MarkStartedForTest()
	}
	return l
}

func TestListScrollOffset_DownScrolls(t *testing.T) {
	l := makeScrollTestList(t, 20)
	l.SetSize(40, 10) // very short — forces scrolling
	initial := l.scrollOffset
	for i := 0; i < 15; i++ {
		l.Down()
	}
	assert.Greater(t, l.scrollOffset, initial, "scrollOffset should increase when selection moves past bottom")
}

func TestListScrollOffset_UpScrollsBack(t *testing.T) {
	l := makeScrollTestList(t, 20)
	l.SetSize(40, 10)
	for i := 0; i < 15; i++ {
		l.Down()
	}
	offset := l.scrollOffset
	for i := 0; i < 15; i++ {
		l.Up()
	}
	assert.Less(t, l.scrollOffset, offset, "scrollOffset should decrease when scrolling back up")
	assert.Equal(t, 0, l.scrollOffset, "scrollOffset should reset to 0 at top")
}

func TestListScrollOffset_ResizeClamps(t *testing.T) {
	l := makeScrollTestList(t, 20)
	l.SetSize(40, 10)
	for i := 0; i < 15; i++ {
		l.Down()
	}
	l.SetSize(40, 60) // make taller — offset might now be invalid
	assert.GreaterOrEqual(t, l.scrollOffset, 0, "scrollOffset must not go negative after resize")
}

func TestListString_DoesNotOverflowHeight(t *testing.T) {
	l := makeScrollTestList(t, 20)
	l.SetSize(40, 14)
	rendered := l.String()
	lines := strings.Split(rendered, "\n")
	assert.LessOrEqual(t, len(lines), 14, "rendered output must not exceed panel height")
}
