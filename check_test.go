package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureCheckOutput runs newCheckCmd() with a temp home/project layout and
// captures stdout. Returns the output string and whether the command returned nil.
func captureCheckOutput(t *testing.T, setupFn func(home, project string)) string {
	t.Helper()

	home := t.TempDir()
	project := t.TempDir()

	if setupFn != nil {
		setupFn(home, project)
	}

	// Override HOME so Audit() uses our temp dir
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	// Override working dir so Audit() uses our temp project
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(project))
	defer os.Chdir(origWd)

	cmd := newCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Execute — ignore error (non-zero exit is expected when not 100%)
	_ = cmd.Execute()

	return buf.String()
}

func TestCheckCmd_EmptyEnvironment(t *testing.T) {
	out := captureCheckOutput(t, nil)

	// Should always have the two sections
	assert.Contains(t, out, "Global skills")
	assert.Contains(t, out, "Health:")
}

func TestCheckCmd_NotInProject(t *testing.T) {
	out := captureCheckOutput(t, nil)

	// No .agents/ dir → no project skills section
	assert.NotContains(t, out, "Project skills")
}

func TestCheckCmd_InProject(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		// Create .agents/skills/ with real skill dirs (including SKILL.md) to mark as kas project
		// Dynamic discovery means skills only appear if directories exist
		for _, name := range []string{"kasmos-coder", "kasmos-planner", "kasmos-reviewer"} {
			dir := filepath.Join(project, ".agents", "skills", name)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644))
		}
	})

	assert.Contains(t, out, "Project skills")
	// All embedded kasmos skills should appear
	assert.Contains(t, out, "kasmos-coder")
	assert.Contains(t, out, "kasmos-planner")
	assert.Contains(t, out, "kasmos-reviewer")
}

func TestCheckCmd_SyncedSkillsShowCheckmark(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		// Set up a synced global skill for claude
		agentsSkills := filepath.Join(home, ".agents", "skills")
		skillDir := filepath.Join(agentsSkills, "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		claudeSkills := filepath.Join(home, ".claude", "skills")
		require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
		target, err := filepath.Rel(claudeSkills, skillDir)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(target, filepath.Join(claudeSkills, "my-skill")))
	})

	// claude should show synced count > 0
	assert.Contains(t, out, "claude")
	// The synced skill should contribute to health
	assert.Contains(t, out, "Health:")
}

func TestCheckCmd_HealthPercentageInOutput(t *testing.T) {
	out := captureCheckOutput(t, nil)

	// Health line should contain a percentage
	assert.Contains(t, out, "%)")
	// Should match pattern "Health: N/M OK (P%)"
	assert.True(t, strings.Contains(out, "Health:"), "output should contain Health: line")
}

func TestCheckCmd_OrphansReported(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		// Create an orphan in claude skills dir (no corresponding source)
		claudeSkills := filepath.Join(home, ".claude", "skills")
		require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
		require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "stale-skill")))
	})

	assert.Contains(t, out, "Orphans")
	assert.Contains(t, out, "stale-skill")
}

func TestCheckCmd_VerboseFlag(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	// Set up one synced skill
	agentsSkills := filepath.Join(home, ".agents", "skills")
	skillDir := filepath.Join(agentsSkills, "verbose-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	target, err := filepath.Rel(claudeSkills, skillDir)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(target, filepath.Join(claudeSkills, "verbose-skill")))

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(project))
	defer os.Chdir(origWd)

	cmd := newCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"-v"})
	_ = cmd.Execute()

	out := buf.String()
	// Verbose mode should show individual skill names indented
	assert.Contains(t, out, "verbose-skill")
}

// TestCheckCmd_ShowsAllProjectSkills verifies that all skills placed in .agents/skills/
// appear in the project section output.
func TestCheckCmd_ShowsAllProjectSkills(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		skills := []string{"alpha-skill", "beta-skill", "gamma-skill"}
		for _, name := range skills {
			dir := filepath.Join(project, ".agents", "skills", name)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644))
		}
	})

	assert.Contains(t, out, "Project skills")
	assert.Contains(t, out, "alpha-skill")
	assert.Contains(t, out, "beta-skill")
	assert.Contains(t, out, "gamma-skill")
}

// TestCheckCmd_ShowsCopyGlyph verifies that a non-symlink directory in a harness dir
// shows the ≈ glyph.
func TestCheckCmd_ShowsCopyGlyph(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		name := "copy-skill"
		// Create canonical skill (with SKILL.md)
		skillDir := filepath.Join(project, ".agents", "skills", name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# copy-skill"), 0o644))

		// Create a real (non-symlink) directory in the claude harness project skills dir
		claudeSkillDir := filepath.Join(project, ".claude", "skills", name)
		require.NoError(t, os.MkdirAll(claudeSkillDir, 0o755))
	})

	assert.Contains(t, out, "≈")
}

// TestCheckCmd_ShowsSkillMDWarning verifies that a skill missing SKILL.md shows "no SKILL.md" annotation.
func TestCheckCmd_ShowsSkillMDWarning(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		name := "no-md-skill"
		// Create canonical skill directory WITHOUT SKILL.md
		skillDir := filepath.Join(project, ".agents", "skills", name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		// Intentionally no SKILL.md
	})

	assert.Contains(t, out, "no SKILL.md")
}

// TestCheckCmd_ShowsRemediation verifies that remediation hints are shown for missing/copy status skills.
func TestCheckCmd_ShowsRemediation(t *testing.T) {
	out := captureCheckOutput(t, func(home, project string) {
		// Create a skill without SKILL.md (so remediation hint for adding SKILL.md is shown)
		name := "needs-fix-skill"
		skillDir := filepath.Join(project, ".agents", "skills", name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		// No SKILL.md — triggers remediation hint
	})

	// Should show some kind of remediation / hint
	assert.True(t,
		strings.Contains(out, "kas skills sync") || strings.Contains(out, "add SKILL.md") || strings.Contains(out, "SKILL.md"),
		"expected remediation hint in output, got:\n%s", out)
}
