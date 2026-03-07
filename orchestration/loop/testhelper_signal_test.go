package loop

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/require"
)

func newTestGateway(t *testing.T) taskstore.SignalGateway {
	t.Helper()
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })
	return gw
}
