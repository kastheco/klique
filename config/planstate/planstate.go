package planstate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/planstore"
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
	store        planstore.Store // always non-nil
	project      string          // project name used with the store
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

// Load creates a PlanState backed by the given store. Plans and TopicEntries are
// populated from the store. dir is retained for file operations (e.g. Rename moves
// the .md file on disk). The store is always required — there is no JSON fallback.
func Load(store planstore.Store, project, dir string) (*PlanState, error) {
	plans, err := store.List(project)
	if err != nil {
		return nil, fmt.Errorf("plan store: %w", err)
	}

	topics, err := store.ListTopics(project)
	if err != nil {
		return nil, fmt.Errorf("plan store: %w", err)
	}

	ps := &PlanState{
		Dir:          dir,
		Plans:        make(map[string]PlanEntry, len(plans)),
		TopicEntries: make(map[string]TopicEntry, len(topics)),
		store:        store,
		project:      project,
	}

	for _, e := range plans {
		ps.Plans[e.Filename] = PlanEntry{
			Status:      Status(e.Status),
			Description: e.Description,
			Branch:      e.Branch,
			Topic:       e.Topic,
			CreatedAt:   e.CreatedAt,
			Implemented: e.Implemented,
		}
	}

	for _, t := range topics {
		ps.TopicEntries[t.Name] = TopicEntry{CreatedAt: t.CreatedAt}
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

// List returns all plans (including done and cancelled), sorted by filename.
func (ps *PlanState) List() []PlanInfo {
	result := make([]PlanInfo, 0, len(ps.Plans))
	for filename, entry := range ps.Plans {
		result = append(result, PlanInfo{
			Filename:    filename,
			Status:      entry.Status,
			Description: entry.Description,
			Branch:      entry.Branch,
			Topic:       entry.Topic,
			CreatedAt:   entry.CreatedAt,
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

// ForceSetStatus overrides a plan's status regardless of FSM rules.
// Validates the status is a known value. Use only for manual overrides (e.g. kq plan set-status --force).
func (ps *PlanState) ForceSetStatus(filename string, status Status) error {
	if !isValidStatus(status) {
		return fmt.Errorf("invalid status %q: must be one of ready, planning, implementing, reviewing, done, cancelled", status)
	}
	if _, ok := ps.Plans[filename]; !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry := ps.Plans[filename]
	entry.Status = status
	ps.Plans[filename] = entry
	if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	return nil
}

// isValidStatus returns true if s is a recognised lifecycle status.
func isValidStatus(s Status) bool {
	switch s {
	case StatusReady, StatusPlanning, StatusImplementing, StatusReviewing, StatusDone, StatusCancelled:
		return true
	}
	return false
}

// setStatus updates a plan's status and persists to the store.
// Unexported: only for use within this package (tests). Production code must use planfsm.Transition.
func (ps *PlanState) setStatus(filename string, status Status) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	entry := ps.Plans[filename]
	entry.Status = status
	ps.Plans[filename] = entry
	if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	return nil
}

// CreateWithContent adds a new plan entry with markdown content stored in the backend.
// The plan entry is created with StatusReady, and the content is persisted via store.SetContent.
// Returns an error if the plan already exists.
func (ps *PlanState) CreateWithContent(filename, description, branch, topic string, createdAt time.Time, content string) error {
	if err := ps.Create(filename, description, branch, topic, createdAt); err != nil {
		return err
	}
	if err := ps.store.SetContent(ps.project, filename, content); err != nil {
		return fmt.Errorf("plan store set content: %w", err)
	}
	return nil
}

// GetContent retrieves the markdown content for the given plan filename from the store.
func (ps *PlanState) GetContent(filename string) (string, error) {
	return ps.store.GetContent(ps.project, filename)
}

// SetContent updates the markdown content for the given plan filename in the store.
func (ps *PlanState) SetContent(filename, content string) error {
	return ps.store.SetContent(ps.project, filename, content)
}

// Create adds a new plan entry to the state and auto-creates the topic if needed.
func (ps *PlanState) Create(filename, description, branch, topic string, createdAt time.Time) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	if _, exists := ps.Plans[filename]; exists {
		return fmt.Errorf("plan already exists: %s", filename)
	}
	entry := PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		Topic:       topic,
		CreatedAt:   createdAt.UTC(),
	}
	ps.Plans[filename] = entry
	// Auto-create topic entry if it doesn't exist
	if topic != "" {
		if ps.TopicEntries == nil {
			ps.TopicEntries = make(map[string]TopicEntry)
		}
		if _, exists := ps.TopicEntries[topic]; !exists {
			ps.TopicEntries[topic] = TopicEntry{CreatedAt: createdAt.UTC()}
		}
	}
	if err := ps.store.Create(ps.project, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	// Auto-create topic in store if needed
	if topic != "" {
		topicEntry := planstore.TopicEntry{Name: topic, CreatedAt: createdAt.UTC()}
		if err := ps.store.CreateTopic(ps.project, topicEntry); err != nil {
			// Ignore "already exists" errors for topics
			if !isAlreadyExistsError(err) {
				return fmt.Errorf("plan store: %w", err)
			}
		}
	}
	return nil
}

// Register adds a new plan entry with metadata and persists to the store.
// Returns an error if the plan already exists.
func (ps *PlanState) Register(filename, description, branch string, createdAt time.Time) error {
	if ps.Plans == nil {
		ps.Plans = make(map[string]PlanEntry)
	}
	if _, exists := ps.Plans[filename]; exists {
		return fmt.Errorf("plan already exists: %s", filename)
	}
	entry := PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		CreatedAt:   createdAt.UTC(),
	}
	ps.Plans[filename] = entry
	if err := ps.store.Create(ps.project, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	return nil
}

// Entry returns the PlanEntry for the given filename, and whether it exists.
func (ps *PlanState) Entry(filename string) (PlanEntry, bool) {
	entry, ok := ps.Plans[filename]
	return entry, ok
}

// SetTopic assigns a topic to an existing plan entry and persists to the store.
// If topic is non-empty and does not yet exist in TopicEntries, it is auto-created.
// Pass an empty string to remove the plan from any topic.
func (ps *PlanState) SetTopic(filename, topic string) error {
	entry, ok := ps.Plans[filename]
	if !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry.Topic = topic
	ps.Plans[filename] = entry
	// Auto-create topic entry if it doesn't exist
	if topic != "" {
		if ps.TopicEntries == nil {
			ps.TopicEntries = make(map[string]TopicEntry)
		}
		if _, exists := ps.TopicEntries[topic]; !exists {
			ps.TopicEntries[topic] = TopicEntry{CreatedAt: time.Now().UTC()}
		}
	}
	if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	// Auto-create topic in store if needed
	if topic != "" {
		topicEntry := planstore.TopicEntry{Name: topic, CreatedAt: ps.TopicEntries[topic].CreatedAt}
		if err := ps.store.CreateTopic(ps.project, topicEntry); err != nil {
			if !isAlreadyExistsError(err) {
				return fmt.Errorf("plan store: %w", err)
			}
		}
	}
	return nil
}

// SetBranch assigns a branch name to an existing plan entry and persists to the store.
func (ps *PlanState) SetBranch(filename, branch string) error {
	entry, ok := ps.Plans[filename]
	if !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry.Branch = branch
	ps.Plans[filename] = entry
	if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
		return fmt.Errorf("plan store: %w", err)
	}
	return nil
}

// Save is a no-op — all mutations write through to the store immediately.
// Retained for API compatibility.
func (ps *PlanState) Save() error {
	return nil
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

// Rename renames a plan by giving it a new display name slug.
// It renames the .md file on disk (if it exists), rekeys the planstate entry,
// and persists the updated state. The date prefix of the old filename is preserved.
// newName should be a human-readable name (e.g., "auth refactor") which will be
// slugified automatically. Returns the new filename on success.
func (ps *PlanState) Rename(oldFilename, newName string) (string, error) {
	entry, ok := ps.Plans[oldFilename]
	if !ok {
		return "", fmt.Errorf("plan not found: %s", oldFilename)
	}
	if newName == "" {
		return "", fmt.Errorf("new name cannot be empty")
	}

	// Build new filename preserving the date prefix (YYYY-MM-DD-) if present.
	newSlug := slugify(newName)
	if newSlug == "" {
		return "", fmt.Errorf("new name produced an empty slug")
	}
	var newFilename string
	if len(oldFilename) > 11 && oldFilename[4] == '-' && oldFilename[7] == '-' && oldFilename[10] == '-' {
		newFilename = oldFilename[:11] + newSlug + ".md"
	} else {
		newFilename = newSlug + ".md"
	}

	if newFilename == oldFilename {
		return oldFilename, nil // nothing to do
	}
	if _, exists := ps.Plans[newFilename]; exists {
		return "", fmt.Errorf("a plan named %q already exists", newFilename)
	}

	// Rename file on disk if it exists.
	oldPath := filepath.Join(ps.Dir, oldFilename)
	newPath := filepath.Join(ps.Dir, newFilename)
	if _, err := os.Stat(oldPath); err == nil {
		if err := os.Rename(oldPath, newPath); err != nil {
			return "", fmt.Errorf("rename plan file: %w", err)
		}
	}

	// Rekey the planstate entry.
	ps.Plans[newFilename] = entry
	delete(ps.Plans, oldFilename)

	if err := ps.store.Rename(ps.project, oldFilename, newFilename); err != nil {
		return "", fmt.Errorf("plan store: %w", err)
	}
	return newFilename, nil
}

// slugify converts a human name to a lowercase, hyphen-separated slug.
// "My Cool Feature!" → "my-cool-feature"
func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Replace any sequence of non-alphanumeric characters with a hyphen.
	result := make([]rune, 0, len(name))
	inHyphen := false
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			result = append(result, r)
			inHyphen = false
		} else if !inHyphen && len(result) > 0 {
			result = append(result, '-')
			inHyphen = true
		}
	}
	// Trim trailing hyphen.
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}

// isAlreadyExistsError returns true if the error indicates a duplicate resource.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists")
}

// toPlanstoreEntry converts a local PlanEntry to a planstore.PlanEntry for
// writing to the store.
func (ps *PlanState) toPlanstoreEntry(filename string, e PlanEntry) planstore.PlanEntry {
	return planstore.PlanEntry{
		Filename:    filename,
		Status:      planstore.Status(e.Status),
		Description: e.Description,
		Branch:      e.Branch,
		Topic:       e.Topic,
		CreatedAt:   e.CreatedAt,
		Implemented: e.Implemented,
	}
}
