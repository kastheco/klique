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
	StatusReady     Status = "ready"
	StatusDone      Status = "done"
	StatusReviewing Status = "reviewing"
	StatusCancelled Status = "cancelled"

	// Lifecycle-stage statuses — canonical names used by the FSM.
	StatusPlanning     Status = "planning"
	StatusImplementing Status = "implementing"
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

	// Migrate legacy status values to canonical names.
	for filename, entry := range wrapped.Plans {
		switch entry.Status {
		case "in_progress":
			entry.Status = StatusImplementing
			wrapped.Plans[filename] = entry
		case "completed", "finished":
			entry.Status = StatusDone
			wrapped.Plans[filename] = entry
		}
	}

	ps := &PlanState{Dir: dir, Plans: wrapped.Plans, TopicEntries: wrapped.Topics}

	// Reconcile stale filenames: if a plan-state key doesn't match an actual
	// file on disk (e.g. date changed between planning and follow-up), fuzzy-
	// match by slug and rekey the entry. This is self-healing and persisted.
	if rekeyed := ps.reconcileFilenames(); rekeyed > 0 {
		_ = ps.save()
	}

	return ps, nil
}

// Topics returns all topic entries sorted by name.
func (ps *PlanState) Topics() []TopicInfo {
	// Discover topics from both TopicEntries and plan topic fields.
	seen := make(map[string]TopicInfo)
	for name, entry := range ps.TopicEntries {
		seen[name] = TopicInfo{Name: name, CreatedAt: entry.CreatedAt}
	}
	for _, entry := range ps.Plans {
		if entry.Topic != "" {
			if _, ok := seen[entry.Topic]; !ok {
				seen[entry.Topic] = TopicInfo{Name: entry.Topic}
			}
		}
	}
	result := make([]TopicInfo, 0, len(seen))
	for _, info := range seen {
		result = append(result, info)
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

// UngroupedPlans returns all active plans with no topic, sorted by filename.
func (ps *PlanState) UngroupedPlans() []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Status == StatusDone || entry.Status == StatusCancelled {
			continue
		}
		if entry.Topic == "" {
			result = append(result, PlanInfo{
				Filename: filename, Status: entry.Status,
				Description: entry.Description, Branch: entry.Branch,
				CreatedAt: entry.CreatedAt,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Filename < result[j].Filename
	})
	return result
}

// hasTopicEntry returns true if the given topic name exists in TopicEntries.
func (ps *PlanState) hasTopicEntry(topic string) bool {
	if ps.TopicEntries == nil {
		return false
	}
	_, exists := ps.TopicEntries[topic]
	return exists
}

// HasRunningCoderInTopic checks if any plan in the given topic (other than
// excludePlan) has status StatusImplementing. Returns the conflicting plan filename.
func (ps *PlanState) HasRunningCoderInTopic(topic, excludePlan string) (bool, string) {
	if topic == "" {
		return false, ""
	}
	for filename, entry := range ps.Plans {
		if filename == excludePlan {
			continue
		}
		if entry.Topic == topic && entry.Status == StatusImplementing {
			return true, filename
		}
	}
	return false, ""
}

// Unfinished returns plans that are not done or cancelled, sorted by filename.
func (ps *PlanState) Unfinished() []PlanInfo {
	result := make([]PlanInfo, 0, len(ps.Plans))
	for filename, entry := range ps.Plans {
		if entry.Status == StatusDone || entry.Status == StatusCancelled {
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

// Finished returns plans that are done, sorted by creation time (newest first).
func (ps *PlanState) Finished() []PlanInfo {
	result := make([]PlanInfo, 0)
	for filename, entry := range ps.Plans {
		if entry.Status != StatusDone {
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
func (ps *PlanState) IsDone(filename string) bool {
	entry, ok := ps.Plans[filename]
	if !ok {
		return false
	}
	return entry.Status == StatusDone
}

// setStatus updates a plan's status and persists to disk.
// Unexported: only for use within this package (tests). Production code must use planfsm.Transition.
func (ps *PlanState) setStatus(filename string, status Status) error {
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

// SetBranch assigns a branch name to an existing plan entry and persists to disk.
func (ps *PlanState) SetBranch(filename, branch string) error {
	entry, ok := ps.Plans[filename]
	if !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry.Branch = branch
	ps.Plans[filename] = entry
	return ps.save()
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

// reconcileFilenames checks each plan key against the actual files on disk.
// If the exact filename is missing but a file with the same slug (date-stripped
// suffix) exists, the entry is rekeyed to the real filename. Returns the number
// of entries rekeyed.
func (ps *PlanState) reconcileFilenames() int {
	rekeyed := 0
	for filename, entry := range ps.Plans {
		path := filepath.Join(ps.Dir, filename)
		if _, err := os.Stat(path); err == nil {
			continue // file exists, nothing to do
		}

		slug := DisplayName(filename) // strip date + .md
		if slug == "" {
			continue
		}

		// Glob for *-<slug>.md in the plans directory.
		matches, err := filepath.Glob(filepath.Join(ps.Dir, "*-"+slug+".md"))
		if err != nil || len(matches) == 0 {
			continue
		}

		// Use the most recent match (last alphabetically = latest date).
		sort.Strings(matches)
		newFilename := filepath.Base(matches[len(matches)-1])
		if newFilename == filename {
			continue
		}
		// Don't overwrite an existing entry.
		if _, exists := ps.Plans[newFilename]; exists {
			continue
		}

		ps.Plans[newFilename] = entry
		delete(ps.Plans, filename)
		rekeyed++
	}
	return rekeyed
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
	if err := os.MkdirAll(ps.Dir, 0o755); err != nil {
		return fmt.Errorf("create plan state dir: %w", err)
	}
	path := filepath.Join(ps.Dir, stateFile)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write plan state: %w", err)
	}
	return nil
}
