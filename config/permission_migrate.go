package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MigratePermissionCache reads a legacy permission-cache.json from cacheDir,
// imports all "allow_always" entries into store under the given project name,
// and removes the JSON file. If the file does not exist, it is a no-op.
func MigratePermissionCache(cacheDir, project string, store PermissionStore) error {
	path := filepath.Join(cacheDir, permissionCacheFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	for key, value := range entries {
		if value == "allow_always" {
			store.Remember(project, key)
		}
	}

	return os.Remove(path)
}
