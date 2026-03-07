package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemon_StartStop(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	time.Sleep(250 * time.Millisecond)
	cancel()

	err = <-errCh
	assert.NoError(t, err)
}

func TestDaemon_ControlSocket(t *testing.T) {
	dir := t.TempDir()
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
		SocketPath:   filepath.Join(dir, "kas.sock"),
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	// Wait for socket to appear
	require.Eventually(t, func() bool {
		_, err := os.Stat(cfg.SocketPath)
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)

	// Connect and query status
	client := NewSocketClient(cfg.SocketPath)
	status, err := client.Status()
	require.NoError(t, err)
	assert.True(t, status.Running)

	cancel()
	<-errCh
}

func TestDaemon_AddRepo(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = d.AddRepo(tmpDir)
	assert.NoError(t, err)

	repos := d.ListRepos()
	assert.Len(t, repos, 1)
}
