package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadArchitectMeta(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")
	planSlug := "planner"

	original := &ArchitectMeta{
		PlanID:          "plan-123",
		SchemaVersion:   1,
		ArchitectModel:  "model-alpha",
		ArchitectEffort: "high",
		CacheVersion:    3,
		Waves: []WaveMeta{{
			Wave:             1,
			Parallel:         true,
			ConflictAnalysis: "contentious file conflict",
			Tasks: []TaskMeta{
				{
					TaskNumber:        1,
					Title:             "Implement task one",
					PreferredModel:    "model-small",
					FallbackModel:     "model-medium",
					EscalationPolicy:  "manual",
					EstimatedTokens:   1200,
					FilesToModify:     []string{"file1.go", "file2.go"},
					DependencyNumbers: []int{2},
					VerifyChecks:      []string{"go test ./..."},
					ContextRefs:       []string{"ref://task-1"},
				},
			},
		}},
		CachedSnippets: map[string]string{"snippet:one": "code"},
	}

	require.NoError(t, SaveArchitectMeta(cacheDir, planSlug, original))
	loaded, err := LoadArchitectMeta(cacheDir, planSlug)
	require.NoError(t, err)

	assert.Equal(t, original, loaded)

	filename := filepath.Join(cacheDir, planSlug+"-architect.json")
	require.FileExists(t, filename)
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), data[len(data)-1])

	require.Len(t, loaded.Waves, 1)
	assert.Equal(t, 1, loaded.Waves[0].Tasks[0].TaskNumber)
}

func TestLoadArchitectMeta_Missing(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")

	meta, err := LoadArchitectMeta(cacheDir, "missing")
	require.NoError(t, err)
	assert.Nil(t, meta)
	assert.False(t, ArchitectMetaExists(cacheDir, "missing"))
}

func TestArchitectMetaExists(t *testing.T) {
	tmp := t.TempDir()
	planSlug := "planner"
	cacheDir := filepath.Join(tmp, "cache")

	meta := &ArchitectMeta{}
	require.NoError(t, SaveArchitectMeta(cacheDir, planSlug, meta))

	assert.True(t, ArchitectMetaExists(cacheDir, planSlug))
	assert.False(t, ArchitectMetaExists(cacheDir, "other"))
}

func TestSaveArchitectMeta_Creates(t *testing.T) {
	tmp := t.TempDir()
	nestedDir := filepath.Join(tmp, "level1", "level2")
	planSlug := "nested"

	require.NoError(t, SaveArchitectMeta(nestedDir, planSlug, &ArchitectMeta{}))

	assert.True(t, ArchitectMetaExists(nestedDir, planSlug))
	info, err := os.Stat(nestedDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestTaskMeta_LookupByNumber(t *testing.T) {
	meta := &ArchitectMeta{Waves: []WaveMeta{{
		Tasks: []TaskMeta{{TaskNumber: 1}, {TaskNumber: 3}},
	}, {
		Tasks: []TaskMeta{{TaskNumber: 7}},
	}}}

	found := meta.TaskByNumber(3)
	require.NotNil(t, found)
	assert.Equal(t, 3, found.TaskNumber)

	assert.Nil(t, meta.TaskByNumber(99))

	var nilMeta *ArchitectMeta
	assert.Nil(t, nilMeta.TaskByNumber(1))
}
