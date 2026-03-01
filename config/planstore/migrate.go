package planstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jsonPlanEntry is the on-disk format for a single plan in plan-state.json.
type jsonPlanEntry struct {
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Topic       string `json:"topic,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Implemented string `json:"implemented,omitempty"`
}

// jsonTopicEntry is the on-disk format for a single topic in plan-state.json.
type jsonTopicEntry struct {
	CreatedAt string `json:"created_at"`
}

// jsonPlanState is the top-level structure of plan-state.json.
type jsonPlanState struct {
	Plans  map[string]jsonPlanEntry  `json:"plans"`
	Topics map[string]jsonTopicEntry `json:"topics"`
}

// MigrateFromJSON reads plan-state.json from plansDir and imports all plans
// and topics into the store under the given project. If plan-state.json does
// not exist, it returns (0, nil) — a no-op. The migration is idempotent:
// plans and topics that already exist in the store are silently skipped.
// For each plan entry that has a corresponding .md file in plansDir, the file
// content is also imported via SetContent.
//
// Returns the number of plans successfully migrated (newly created).
func MigrateFromJSON(store Store, project, plansDir string) (int, error) {
	stateFile := filepath.Join(plansDir, "plan-state.json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read plan-state.json: %w", err)
	}

	var state jsonPlanState
	if err := json.Unmarshal(data, &state); err != nil {
		return 0, fmt.Errorf("parse plan-state.json: %w", err)
	}

	migrated := 0

	// Migrate plans.
	for filename, jp := range state.Plans {
		entry := PlanEntry{
			Filename:    filename,
			Status:      Status(jp.Status),
			Description: jp.Description,
			Branch:      jp.Branch,
			Topic:       jp.Topic,
			Implemented: jp.Implemented,
		}
		if jp.CreatedAt != "" {
			entry.CreatedAt = parseTime(jp.CreatedAt)
		}

		if err := store.Create(project, entry); err != nil {
			// Skip if already exists (idempotent).
			if strings.Contains(err.Error(), "plan already exists") {
				continue
			}
			return migrated, fmt.Errorf("migrate plan %s: %w", filename, err)
		}
		migrated++

		// Import .md file content if it exists on disk.
		mdPath := filepath.Join(plansDir, filename)
		content, err := os.ReadFile(mdPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return migrated, fmt.Errorf("read plan content %s: %w", filename, err)
			}
			// No .md file — that's fine, content stays empty.
		} else {
			if err := store.SetContent(project, filename, string(content)); err != nil {
				return migrated, fmt.Errorf("set content for %s: %w", filename, err)
			}
		}
	}

	// Migrate topics.
	for name, jt := range state.Topics {
		var createdAt time.Time
		if jt.CreatedAt != "" {
			createdAt = parseTime(jt.CreatedAt)
		}
		entry := TopicEntry{
			Name:      name,
			CreatedAt: createdAt,
		}
		if err := store.CreateTopic(project, entry); err != nil {
			// Skip if already exists (idempotent).
			if strings.Contains(err.Error(), "topic already exists") {
				continue
			}
			return migrated, fmt.Errorf("migrate topic %s: %w", name, err)
		}
	}

	return migrated, nil
}
