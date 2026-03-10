package daemon

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration/loop"
)

// RepoEntry holds per-repo registration metadata.
type RepoEntry struct {
	// Path is the absolute path to the repository root.
	Path string
	// Project is the basename of the repository directory (e.g. "my-project").
	Project string
	// Store is the per-repo task store (embedded SQLite).
	// It may be nil when the store has not yet been opened or is unavailable.
	Store taskstore.Store
	// SignalGateway is the DB-backed signal gateway for this repo.
	// It may be nil when the gateway has not yet been opened or is unavailable.
	SignalGateway taskstore.SignalGateway
	// SignalsDir is the path to the signals directory (<repo>/.kasmos/signals/).
	SignalsDir string
	// Processor is the signal processor for this repo. It persists across ticks
	// so that wave orchestrator state is maintained between poll cycles.
	Processor *loop.Processor
}

// RepoManager tracks registered repositories for the daemon.
// It is safe for concurrent use.
type RepoManager struct {
	mu                 sync.RWMutex
	repos              []RepoEntry
	autoReviewFix      bool
	maxReviewFixCycles int
}

// NewRepoManager returns an empty, ready-to-use RepoManager.
func NewRepoManager() *RepoManager {
	return &RepoManager{}
}

// Add registers a repository by absolute path.
// It derives the project name from the directory basename and sets the signals dir.
// A per-repo SQLite taskstore is opened at <path>/.kasmos/taskstore.db; any
// error opening the store is non-fatal — the entry is added with a nil Store.
// Returns an error if path is already registered.
func (m *RepoManager) Add(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project := filepath.Base(path)

	for _, r := range m.repos {
		if r.Path == path {
			return fmt.Errorf("repo already registered: %s", path)
		}
		if r.Project == project {
			return fmt.Errorf("repo with basename %q already registered (path: %s); rename one of the directories or use distinct names", project, r.Path)
		}
	}
	kasmosDir := filepath.Join(path, ".kasmos")
	signalsDir := filepath.Join(kasmosDir, "signals")
	dbPath := filepath.Join(kasmosDir, "taskstore.db")

	var store taskstore.Store
	if s, err := taskstore.NewSQLiteStore(dbPath); err == nil {
		store = s
	}

	var gw taskstore.SignalGateway
	if g, err := taskstore.NewSQLiteSignalGateway(dbPath); err == nil {
		gw = g
	}

	// Create a per-repo processor that persists across poll ticks so that wave
	// orchestrator state is maintained between cycles.
	proc := loop.NewProcessor(loop.ProcessorConfig{
		AutoReviewFix:      m.autoReviewFix,
		Store:              store,
		Project:            project,
		MaxReviewFixCycles: m.maxReviewFixCycles,
	})

	m.repos = append(m.repos, RepoEntry{
		Path:          path,
		Project:       project,
		Store:         store,
		SignalGateway: gw,
		SignalsDir:    signalsDir,
		Processor:     proc,
	})
	return nil
}

// Remove deregisters a repository by absolute path, closing its store if open.
// Returns an error if path is not registered.
func (m *RepoManager) Remove(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, r := range m.repos {
		if r.Path == path {
			if r.Store != nil {
				_ = r.Store.Close()
			}
			if r.SignalGateway != nil {
				_ = r.SignalGateway.Close()
			}
			m.repos = append(m.repos[:i], m.repos[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("repo not registered: %s", path)
}

// RemoveByProject deregisters a repository by its project name (the basename
// of the repo path). Closing its store if open. Returns an error if not found.
func (m *RepoManager) RemoveByProject(project string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, r := range m.repos {
		if r.Project == project {
			if r.Store != nil {
				_ = r.Store.Close()
			}
			if r.SignalGateway != nil {
				_ = r.SignalGateway.Close()
			}
			m.repos = append(m.repos[:i], m.repos[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("repo not registered: %s", project)
}

// List returns a snapshot of all currently registered repositories.
// The returned slice is a copy — modifications do not affect internal state.
func (m *RepoManager) List() []RepoEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]RepoEntry, len(m.repos))
	copy(out, m.repos)
	return out
}

// Get returns the RepoEntry for the given path, or an error if not registered.
func (m *RepoManager) Get(path string) (RepoEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.repos {
		if r.Path == path {
			return r, nil
		}
	}
	return RepoEntry{}, fmt.Errorf("repo not registered: %s", path)
}
