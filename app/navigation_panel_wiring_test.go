package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHomeInitializesNavigationPanel(t *testing.T) {
	h := newHome(context.Background(), "opencode", false)
	require.NotNil(t, h.nav)
}
