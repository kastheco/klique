package cmd

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/session/git"
	"github.com/stretchr/testify/assert"
)

func TestBuildCLIPRMetadata(t *testing.T) {
	entry := taskstore.TaskEntry{
		Description: "Auth Middleware",
		Goal:        "add JWT auth to all routes",
		Branch:      "plan/auth-middleware",
		Content:     "# Auth\n\n**Goal:** add JWT auth\n\n**Architecture:** middleware chain\n\n**Tech Stack:** Go\n\n## Wave 1\n\n### Task 1: JWT middleware\n\nbody\n",
	}
	subtasks := []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "JWT middleware", Status: taskstore.SubtaskStatusComplete},
	}

	meta := buildCLIPRMetadata(entry, subtasks, "file1.go", "abc123 fix: auth", "1 file changed")
	assert.Equal(t, "Auth Middleware", meta.Description)
	assert.Equal(t, "add JWT auth to all routes", meta.Goal)
	assert.Equal(t, "middleware chain", meta.Architecture)
	assert.Equal(t, "Go", meta.TechStack)
	assert.Len(t, meta.Subtasks, 1)
	assert.Equal(t, "file1.go", meta.GitChanges)
}

func TestBuildCLIPRMetadata_EmptyContent(t *testing.T) {
	entry := taskstore.TaskEntry{
		Description: "quick fix",
		Goal:        "fix the bug",
	}
	meta := buildCLIPRMetadata(entry, nil, "", "", "")
	assert.Equal(t, "quick fix", meta.Description)
	assert.Equal(t, "fix the bug", meta.Goal)
	assert.Empty(t, meta.Architecture)
	assert.Empty(t, meta.TechStack)
}

func TestBuildCLIPRTitleFallback(t *testing.T) {
	assert.Equal(t, "Auth Middleware", git.BuildPRTitle("Auth Middleware", "my-feature"))
	assert.Equal(t, "my-feature", git.BuildPRTitle("", "my-feature"))
}
