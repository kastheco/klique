package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func architectMetaFilename(planSlug string) string {
	return planSlug + "-architect.json"
}

// SaveArchitectMeta serializes meta to JSON and writes it to cacheDir/<planSlug>-architect.json,
// creating cacheDir with mode 0755 if it does not exist.
func SaveArchitectMeta(cacheDir, planSlug string, meta *ArchitectMeta) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(cacheDir, architectMetaFilename(planSlug))
	encoded = append(encoded, '\n')
	return os.WriteFile(filename, encoded, 0o644)
}

// LoadArchitectMeta reads and deserializes the architect metadata file for planSlug from cacheDir.
// Returns (nil, nil) when the file does not exist.
func LoadArchitectMeta(cacheDir, planSlug string) (*ArchitectMeta, error) {
	filename := filepath.Join(cacheDir, architectMetaFilename(planSlug))
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("read architect meta: %w", err)
	}

	var meta ArchitectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("read architect meta: %w", err)
	}

	return &meta, nil
}

// ArchitectMetaExists reports whether the architect metadata file for planSlug exists in cacheDir.
func ArchitectMetaExists(cacheDir, planSlug string) bool {
	filename := filepath.Join(cacheDir, architectMetaFilename(planSlug))
	_, err := os.Stat(filename)
	return err == nil
}
