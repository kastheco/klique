package planstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusReady      Status = "ready"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusReviewing  Status = "reviewing"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"

	// Lifecycle-stage aliases used by the plan/implement/review/finished sidebar stages.
	StatusPlanning     Status = "planning"
	StatusImplementing Status = "implementing"
	StatusFinished     Status = "finished"
)

type PlanEntry struct {
	Status      Status    `json:"status"`
	Description string    `json:"description,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Topic       string    `json:"topic,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	Implemented string    `json:"implemented,omitempty"`
}

type TopicEntry struct {
	CreatedAt time.Time `json:"created_at"`
}

type PlanState struct {
	Dir          string
	Plans        map[string]PlanEntry
	TopicEntries map[string]TopicEntry
}

type PlanInfo struct {
	Filename    string
	Status      Status
	Description string
	Branch      string
	Topic       string
	CreatedAt   time.Time
}

type TopicInfo struct {
	Name      string
	CreatedAt time.Time
}

const stateFile = "plan-state.json"

// wrappedFormat is the new on-disk format with "plans" and "topics" keys.
type wrappedFormat struct {
	Topics map[string]TopicEntry `json:"topics,omitempty"`
	Plans  map[string]PlanEntry  `json:"plans"`
}

// Load reads plan-state.json from dir. Returns empty state if file missing.
func Load(dir string) (*PlanState, error) {
	path := filepath.Join(dir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &PlanState{Dir: dir, Plans: make(map[string]PlanEntry), TopicEntries: make(map[string]TopicEntry)}, nil
		}
		return nil, fmt.Errorf("read plan state: %w", err)
	}

	// Try new wrapped format first
	var wrapped wrappedFormat
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse plan state: %w", err)
	}

	// Detect legacy flat format: if "plans" key is absent, the top-level
	// object IS the plan map. We detect this by checking if wrapped.Plans
	// is nil/empty AND the raw JSON has keys ending in ".md".
	if len(wrapped.Plans) == 0 {
		var flat map[string]PlanEntry
		if err := json.Unmarshal(data, &flat); err == nil && len(flat) > 0 {
			// Check if any key looks like a plan filename
			isLegacy := false
			for k := range flat {
				if strings.HasSuffix(k, ".md") {
					isLegacy = true
					break
				}
			}
			if isLegacy {
				wrapped.Plans = flat
				wrapped.Topics = make(map[string]TopicEntry)
			}
		}
	}

	if wrapped.Plans == nil {
		wrapped.Plans = make(map[string]PlanEntry)
	}
	if wrapped.Topics == nil {
		wrapped.Topics = make(map[string]TopicEntry)
	}

	return &PlanState{Dir: dir, Plans: wrapped.Plans, TopicEntries: wrapped.Topics}, nil
}

// Topics returns all topic entries sorted by name.
func (ps *PlanState) Topics() []TopicInfo {
	result := make([]TopicInfo, 0, len(ps.TopicEntries))
	for name, entry := range ps.TopicEntries {
		result = append(result, TopicInfo{Name: name, CreatedAt: entry.CreatedAt})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// PlansByTopic returns all plans in the given topic, sorted by filename.
func (ps *PlanState) PlansByTopic(topic string) []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Topic == topic {
			result = append(result, PlanInfo{
				Filename: filename, Status: entry.Status,
				Description: entry.Description, Branch: entry.Branch,
				Topic: entry.Topic, CreatedAt: entry.CreatedAt,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// UngroupedPlans returns all plans with no topic that are not done/completed/cancelled, sorted by filename.
func (ps *PlanState) UngroupedPlans() []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Topic == "" && entry.Status != StatusDone && entry.Status != StatusCompleted && entry.Status != StatusCancelled {
			result = append(result, PlanInfo{
				Filename: filename, Status: entry.Status,
				Description: entry.Description, Branch: entry.Branch,
				Topic: "", CreatedAt: entry.CreatedAt,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// HasRunningCoderInTopic checks if any plan in the given topic (other than
// excludePlan) has status StatusInProgress. Returns the conflicting plan filename.
func (ps *PlanState) HasRunningCoderInTopic(topic, excludePlan string) (bool, string) {
	if topic == "" {
		return false, ""
	}
	for filename, entry := range ps.Plans {
		if filename == excludePlan {
			continue
		}
		if entry.Topic == topic && entry.Status == StatusInProgress {
			return true, filename
		}
	}
	return false, ""
}

// Unfinished returns plans that are not done, completed, or cancelled, sorted by filename.
func (ps *PlanState) Unfinished() []PlanInfo {
	result := make([]PlanInfo, 0, len(ps.Plans))
	for filename, entry := range ps.Plans {
		if entry.Status == StatusDone || entry.Status == StatusCompleted || entry.Status == StatusCancelled {
			continue
		}
		result = append(result, PlanInfo{
			Filename: filename, Status: entry.Status,
			Description: entry.Description, Branch: entry.Branch,
			Topic: entry.Topic, CreatedAt: entry.CreatedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// Finished returns plans that are done or completed, sorted by creation time (newest first).
func (ps *PlanState) Finished() []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Status != StatusDone && entry.Status != StatusCompleted {
			continue
		}
		result = append(result, PlanInfo{
			Filename: filename, Status: entry.Status,
			Description: entry.Description, Branch: entry.Branch,
			Topic: entry.Topic, CreatedAt: entry.CreatedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].Filename > result[j].Filename
	})
	return result
}

// Cancelled returns all cancelled plans, sorted by filename.
func (ps *PlanState) Cancelled() []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Status != StatusCancelled {
			continue
		}
		result = append(result, PlanInfo{
			Filename: filename, Status: entry.Status,
			Description: entry.Description, Branch: entry.Branch,
			Topic: entry.Topic, CreatedAt: entry.CreatedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// IsDone returns true only if the given plan has status StatusDone.
// StatusCompleted intentionally returns false to prevent re-triggering a reviewer.
func (ps *PlanState) IsDone(filename string) bool {
	entry, ok := ps.Plans[filename]
	if !ok {
		return false
	}
	return entry.Status == StatusDone
}

// SetStatus updates a plan's status and persists to disk.
func (ps *PlanState) SetStatus(filename string, status Status) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	entry := ps.Plans[filename]
	entry.Status = status
	ps.Plans[filename] = entry
	return ps.save()
}

// Create adds a new plan entry to the state and auto-creates the topic if needed.
func (ps *PlanState) Create(filename, description, branch, topic string, createdAt time.Time) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	if _, exists := ps.Plans[filename]; exists {
		return fmt.Errorf("plan already exists: %s", filename)
	}
	ps.Plans[filename] = PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		Topic:       topic,
		CreatedAt:   createdAt.UTC(),
	}
	// Auto-create topic entry if it doesn't exist
	if topic != "" {
		if ps.TopicEntries == nil {
			ps.TopicEntries = make(map[string]TopicEntry)
		}
		if _, exists := ps.TopicEntries[topic]; !exists {
			ps.TopicEntries[topic] = TopicEntry{CreatedAt: createdAt.UTC()}
		}
	}
	return ps.save()
}

// Register adds a new plan entry with metadata and persists to disk.
// Returns an error if the plan already exists.
func (ps *PlanState) Register(filename, description, branch string, createdAt time.Time) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	if _, exists := ps.Plans[filename]; exists {
		return fmt.Errorf("plan already exists: %s", filename)
	}
	ps.Plans[filename] = PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		CreatedAt:   createdAt.UTC(),
	}
	return ps.save()
}

// Entry returns the PlanEntry for the given filename, and whether it exists.
func (ps *PlanState) Entry(filename string) (PlanEntry, bool) {
	entry, ok := ps.Plans[filename]
	return entry, ok
}

// Save persists the current state to disk.
func (ps *PlanState) Save() error {
	return ps.save()
}

// DisplayName strips the date prefix and .md extension from a plan filename.
// "2026-02-20-my-feature.md" → "my-feature"
// "plain-plan.md" → "plain-plan"
func DisplayName(filename string) string {
	name := strings.TrimSuffix(filename, ".md")
	if len(name) > 11 && name[4] == '-' && name[7] == '-' && name[10] == '-' {
		name = name[11:]
	}
	return name
}

func (ps *PlanState) save() error {
	wrapped := wrappedFormat{
		Topics: ps.TopicEntries,
		Plans:  ps.Plans,
	}
	data, err := json.MarshalIndent(wrapped, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan state: %w", err)
	}
	path := filepath.Join(ps.Dir, stateFile)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write plan state: %w", err)
	}
	return nil
}
