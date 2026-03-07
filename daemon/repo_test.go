package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoManager_AddAndList(t *testing.T) {
	rm := NewRepoManager()
	require.NoError(t, rm.Add("/home/user/project-a"))
	require.NoError(t, rm.Add("/home/user/project-b"))

	repos := rm.List()
	assert.Len(t, repos, 2)
	assert.Equal(t, "/home/user/project-a", repos[0].Path)
}

func TestRepoManager_AddDuplicate(t *testing.T) {
	rm := NewRepoManager()
	require.NoError(t, rm.Add("/home/user/project-a"))
	err := rm.Add("/home/user/project-a")
	assert.Error(t, err)
}

func TestRepoManager_Remove(t *testing.T) {
	rm := NewRepoManager()
	require.NoError(t, rm.Add("/home/user/project-a"))
	require.NoError(t, rm.Remove("/home/user/project-a"))
	assert.Len(t, rm.List(), 0)
}

func TestRepoManager_ProjectName(t *testing.T) {
	rm := NewRepoManager()
	require.NoError(t, rm.Add("/home/user/my-project"))
	repos := rm.List()
	assert.Equal(t, "my-project", repos[0].Project)
}

func TestRepoManager_AddDuplicateBasename(t *testing.T) {
	rm := NewRepoManager()
	require.NoError(t, rm.Add("/org-a/my-project"))
	// Different absolute path but same basename — must be rejected.
	err := rm.Add("/org-b/my-project")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "my-project")
}
