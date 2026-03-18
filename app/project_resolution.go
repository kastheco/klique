package app

import (
	"path/filepath"

	daemonpkg "github.com/kastheco/kasmos/daemon"
	"github.com/kastheco/kasmos/daemon/api"
)

var listDaemonRepoStatuses = func() ([]api.RepoStatus, error) {
	return daemonpkg.NewSocketClient(daemonpkg.DefaultSocketPath()).ListRepos()
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

func resolveTaskStoreProject(repoPath string) string {
	if project := daemonProjectForRepo(repoPath); project != "" {
		return project
	}
	return filepath.Base(repoPath)
}
