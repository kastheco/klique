package cmd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kastheco/kasmos/daemon/api"
)

var listDaemonRepoStatuses = func() ([]api.RepoStatus, error) {
	socketPath := defaultDaemonSocketPath()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			d.Timeout = 300 * time.Millisecond
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 500 * time.Millisecond}

	resp, err := client.Get("http://daemon/v1/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, nil
	}

	var status api.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return status.Repos, nil
}

func defaultDaemonSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "kasmos", "kas.sock")
	}
	return filepath.Join(os.TempDir(), "kasmos-"+itoa(os.Getuid()), "kas.sock")
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func canonicalRepoPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	if root, err := resolveRepoRoot(repoPath); err == nil && root != "" {
		repoPath = root
	}
	if realPath, err := filepath.EvalSymlinks(repoPath); err == nil && realPath != "" {
		repoPath = realPath
	}
	return filepath.Clean(repoPath)
}

func daemonProjectForRepo(repoPath string) string {
	repos, err := listDaemonRepoStatuses()
	if err != nil {
		return ""
	}
	cleanRepoPath := canonicalRepoPath(repoPath)
	for _, repo := range repos {
		if canonicalRepoPath(repo.Path) == cleanRepoPath {
			return repo.Project
		}
	}
	return ""
}

func resolveTaskProject(repoPath string) string {
	if project := daemonProjectForRepo(repoPath); project != "" {
		return project
	}
	return filepath.Base(repoPath)
}
