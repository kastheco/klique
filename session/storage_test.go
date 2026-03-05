package session

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStateManager struct {
	helpScreensSeen uint32
	instances       json.RawMessage
}

func (m *mockStateManager) SaveInstances(instancesJSON json.RawMessage) error {
	m.instances = instancesJSON
	return nil
}

func (m *mockStateManager) GetInstances() json.RawMessage {
	if m.instances == nil {
		return json.RawMessage("[]")
	}
	return m.instances
}

func (m *mockStateManager) DeleteAllInstances() error {
	m.instances = json.RawMessage("[]")
	return nil
}

func (m *mockStateManager) GetHelpScreensSeen() uint32 {
	return m.helpScreensSeen
}

func (m *mockStateManager) SetHelpScreensSeen(seen uint32) error {
	m.helpScreensSeen = seen
	return nil
}

func TestLoadInstances_DropsStaleWaveInstancesWithoutTmuxSession(t *testing.T) {
	repoDir := t.TempDir()
	nonce := time.Now().UnixNano()

	records := []InstanceData{
		{
			Title:      fmt.Sprintf("stale-wave-%d", nonce),
			Path:       repoDir,
			Status:     Paused,
			Program:    "opencode",
			TaskFile:   "stale-wave.md",
			TaskNumber: 1,
			WaveNumber: 1,
			Worktree: GitWorktreeData{
				RepoPath:     repoDir,
				WorktreePath: repoDir,
				SessionName:  fmt.Sprintf("stale-wave-%d", nonce),
				BranchName:   "plan/stale-wave",
			},
		},
		{
			Title:   fmt.Sprintf("keep-fixer-%d", nonce),
			Path:    repoDir,
			Status:  Ready,
			Program: "opencode",
		},
	}

	raw, err := json.Marshal(records)
	require.NoError(t, err)

	state := &mockStateManager{instances: raw}
	storage, err := NewStorage(state)
	require.NoError(t, err)

	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, records[1].Title, instances[0].Title)
}

func TestLoadInstances_PreservesPausedNonWaveInstanceWithoutTmuxSession(t *testing.T) {
	repoDir := t.TempDir()
	nonce := time.Now().UnixNano()

	records := []InstanceData{
		{
			Title:   fmt.Sprintf("paused-solo-%d", nonce),
			Path:    repoDir,
			Status:  Paused,
			Program: "opencode",
			Worktree: GitWorktreeData{
				RepoPath:     repoDir,
				WorktreePath: repoDir,
				SessionName:  fmt.Sprintf("paused-solo-%d", nonce),
			},
		},
	}

	raw, err := json.Marshal(records)
	require.NoError(t, err)

	state := &mockStateManager{instances: raw}
	storage, err := NewStorage(state)
	require.NoError(t, err)

	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, records[0].Title, instances[0].Title)
	assert.True(t, instances[0].Paused())
}
