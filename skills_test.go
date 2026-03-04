package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsSyncCommand(t *testing.T) {
	// Verify the command exists and has correct metadata
	cmd := newSkillsSyncCmd()
	assert.Equal(t, "sync", cmd.Use)
	assert.Contains(t, cmd.Short, "skill")
}

func TestRunSkillsList_IncludesSymlinkedDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillsDir := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))

	// Real directory skill
	require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "real-skill"), 0o755))

	// Symlink-to-directory skill (externally managed)
	extRepo := filepath.Join(home, "external-repo", "my-skill")
	require.NoError(t, os.MkdirAll(extRepo, 0o755))
	require.NoError(t, os.Symlink(extRepo, filepath.Join(skillsDir, "ext-skill")))

	var buf bytes.Buffer
	cmd := newSkillsListCmd()
	cmd.SetOut(&buf)
	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "real-skill")
	assert.Contains(t, output, "ext-skill")
	assert.Contains(t, output, "external")
}

func TestRunSkillsList_ShowsProjectSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create global skill
	globalSkills := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(globalSkills, "global-skill"), 0o755))

	// Create project skill in cwd
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	projectSkills := filepath.Join(projectDir, ".agents", "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(projectSkills, "project-skill"), 0o755))

	var buf bytes.Buffer
	cmd := newSkillsListCmd()
	cmd.SetOut(&buf)
	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "global-skill")
	assert.Contains(t, output, "project-skill")
	assert.Contains(t, output, "Project skills")
}
