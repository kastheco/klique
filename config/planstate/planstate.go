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
	store        planstore.Store // non-nil when using remote backend
	project      string          // project name used with the remote store
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
	if renames := ps.reconcileFilenames(); len(renames) > 0 {
		_ = ps.save()
	}

	return ps, nil
}

// LoadWithStore creates a PlanState backed by a remote store.
// When store is non-nil, Plans and TopicEntries are populated from the remote
// store and all subsequent mutations write through to it instead of JSON.
// dir is retained for file operations (e.g. Rename moves the .md file on disk).
// When store is nil, this is equivalent to calling Load(dir).
func LoadWithStore(store planstore.Store, project, dir string) (*PlanState, error) {
	if store == nil {
		return Load(dir)
	}

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

	// Reconcile stale filenames: if a plan-state key doesn't match an actual
	// file on disk (e.g. date changed between planning and follow-up), fuzzy-
	// match by slug and rekey the entry. Persist renames back to the store.
	ps.reconcileFilenamesWithStore()

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
	if ps.store != nil {
		if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		return nil
	}
	return ps.Save()
}

// isValidStatus returns true if s is a recognised lifecycle status.
func isValidStatus(s Status) bool {
	switch s {
	case StatusReady, StatusPlanning, StatusImplementing, StatusReviewing, StatusDone, StatusCancelled:
		return true
	}
	return false
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
	if ps.store != nil {
		if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		return nil
	}
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
	if ps.store != nil {
		if err := ps.store.Create(ps.project, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		// Auto-create topic in remote store if needed
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
	entry := PlanEntry{
		Status:      StatusReady,
		Description: description,
		Branch:      branch,
		CreatedAt:   createdAt.UTC(),
	}
	ps.Plans[filename] = entry
	if ps.store != nil {
		if err := ps.store.Create(ps.project, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		return nil
	}
	return ps.save()
}

// Entry returns the PlanEntry for the given filename, and whether it exists.
func (ps *PlanState) Entry(filename string) (PlanEntry, bool) {
	entry, ok := ps.Plans[filename]
	return entry, ok
}

// SetTopic assigns a topic to an existing plan entry and persists to disk.
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
	if ps.store != nil {
		if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		// Auto-create topic in remote store if needed
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
	return ps.save()
}

// SetBranch assigns a branch name to an existing plan entry and persists to disk.
func (ps *PlanState) SetBranch(filename, branch string) error {
	entry, ok := ps.Plans[filename]
	if !ok {
		return fmt.Errorf("plan not found: %s", filename)
	}
	entry.Branch = branch
	ps.Plans[filename] = entry
	if ps.store != nil {
		if err := ps.store.Update(ps.project, filename, ps.toPlanstoreEntry(filename, entry)); err != nil {
			return fmt.Errorf("plan store: %w", err)
		}
		return nil
	}
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

	if ps.store != nil {
		if err := ps.store.Rename(ps.project, oldFilename, newFilename); err != nil {
			return "", fmt.Errorf("plan store: %w", err)
		}
		return newFilename, nil
	}
	return newFilename, ps.save()
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

// reconcileFilenamesWithStore is like reconcileFilenames but also persists
// renames to the remote store when one is configured.
func (ps *PlanState) reconcileFilenamesWithStore() {
	renames := ps.reconcileFilenames()
	if len(renames) == 0 || ps.store == nil {
		return
	}
	// Persist each rename to the remote store using Rename (not Update),
	// since the DB still has the old filename as its key.
	for _, rn := range renames {
		_ = ps.store.Rename(ps.project, rn.OldFilename, rn.NewFilename)
	}
}

// filenameRename records an old→new filename rekey performed by reconcileFilenames.
type filenameRename struct {
	OldFilename string
	NewFilename string
}

// reconcileFilenames checks each plan key against the actual files on disk.
// If the exact filename is missing but a file with the same slug (date-stripped
// suffix) exists, the entry is rekeyed to the real filename. Returns the list
// of renames performed so callers can persist them to the remote store.
func (ps *PlanState) reconcileFilenames() []filenameRename {
	var renames []filenameRename
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
		renames = append(renames, filenameRename{OldFilename: filename, NewFilename: newFilename})
	}
	return renames
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
// writing to the remote store.
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

func (ps *PlanState) save() error {
	if ps.store != nil {
		// Remote store: no-op here — mutations write through individually.
		// save() is called after each mutation; with a store backend the
		// individual Create/Update/Rename calls already persisted the change.
		return nil
	}
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
