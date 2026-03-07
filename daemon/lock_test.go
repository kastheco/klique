package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPIDLock_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lock, err := AcquirePIDLock(filepath.Join(dir, "daemon.pid"))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "daemon.pid"))

	err = lock.Release()
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "daemon.pid"))
	assert.True(t, os.IsNotExist(err))
}

func TestPIDLock_DoubleAcquire(t *testing.T) {
	dir := t.TempDir()
	lock1, err := AcquirePIDLock(filepath.Join(dir, "daemon.pid"))
	require.NoError(t, err)
	defer lock1.Release()

	_, err = AcquirePIDLock(filepath.Join(dir, "daemon.pid"))
	assert.Error(t, err, "second lock should fail")
}
