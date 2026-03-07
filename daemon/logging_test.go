package daemon

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemonLogger_JSONOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDaemonLogger(buf, false)
	logger.Info("test message", "repo", "/tmp/test", "plan", "my-plan.md")

	var entry map[string]any
	require.NoError(t, json.NewDecoder(buf).Decode(&entry))
	assert.Equal(t, "test message", entry["msg"])
	assert.Equal(t, "/tmp/test", entry["repo"])
}

func TestNewDaemonLogger_TextOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDaemonLogger(buf, true)
	logger.Info("test message")
	assert.Contains(t, buf.String(), "test message")
}

func TestWithRepo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDaemonLogger(buf, false)
	scoped := WithRepo(logger, "/home/user/project")
	scoped.Info("scoped log")

	var entry map[string]any
	require.NoError(t, json.NewDecoder(buf).Decode(&entry))
	assert.Equal(t, "/home/user/project", entry["repo"])
}

func TestWithPlan(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDaemonLogger(buf, false)
	scoped := WithPlan(logger, "plan.md")
	scoped.Info("plan log")

	var entry map[string]any
	require.NoError(t, json.NewDecoder(buf).Decode(&entry))
	assert.Equal(t, "plan.md", entry["plan"])
}
