package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteTaskUpdateContent(t *testing.T) {
	t.Run("ingests stdin and updates parsed metadata", func(t *testing.T) {
		store := taskstore.NewTestSQLiteStore(t)
		project := "test-project"
		err := store.Create(project, taskstore.TaskEntry{
			Filename:  "my-plan",
			Status:    taskstore.StatusReady,
			Branch:    "plan/my-plan",
			CreatedAt: time.Now(),
		})
		require.NoError(t, err)

		content := "# Updated Plan\n\n**Goal:** new goal\n\n## Wave 1\n\n### Task 1: foo\n\nDo it.\n"
		err = executeTaskUpdateContent(project, "my-plan.md", strings.NewReader(content), store)
		require.NoError(t, err)

		got, err := store.GetContent(project, "my-plan")
		require.NoError(t, err)
		assert.Equal(t, content, got)

		entry, err := store.Get(project, "my-plan")
		require.NoError(t, err)
		assert.Equal(t, "new goal", entry.Goal)

		subtasks, err := store.GetSubtasks(project, "my-plan")
		require.NoError(t, err)
		require.Len(t, subtasks, 1)
		assert.Equal(t, 1, subtasks[0].TaskNumber)
		assert.Equal(t, "foo", subtasks[0].Title)
		assert.Equal(t, taskstore.SubtaskStatusPending, subtasks[0].Status)
	})

	t.Run("rejects empty stdin", func(t *testing.T) {
		store := taskstore.NewTestSQLiteStore(t)
		project := "test-project"
		require.NoError(t, store.Create(project, taskstore.TaskEntry{
			Filename:  "my-plan",
			Status:    taskstore.StatusReady,
			CreatedAt: time.Now(),
		}))

		err := executeTaskUpdateContent(project, "my-plan", strings.NewReader(" \n"), store)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no content provided")
	})

	t.Run("rejects tty stdin", func(t *testing.T) {
		stdinFile, err := os.Open("/dev/null")
		require.NoError(t, err)
		t.Cleanup(func() { _ = stdinFile.Close() })

		err = validateUpdateContentStdin(stdinFile)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stdin is a tty")
	})
}
