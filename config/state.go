package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/log"
)

const (
	// StateFileName is the name of the JSON state file within the config dir.
	StateFileName = "state.json"
	// InstancesFileName is the legacy per-file instances store name.
	InstancesFileName = "instances.json"
)

// InstanceStorage is the interface for reading and writing serialised instance data.
type InstanceStorage interface {
	// SaveInstances persists the raw instance JSON blob.
	SaveInstances(instancesJSON json.RawMessage) error
	// GetInstances returns the current raw instance JSON blob.
	GetInstances() json.RawMessage
	// DeleteAllInstances resets the stored instances to an empty list.
	DeleteAllInstances() error
}

// AppState is the interface for reading and writing application-level state.
type AppState interface {
	// GetHelpScreensSeen returns the bitmask of help screens that have been shown.
	GetHelpScreensSeen() uint32
	// SetHelpScreensSeen stores an updated bitmask and persists it.
	SetHelpScreensSeen(seen uint32) error
}

// StateManager is the unified interface combining instance storage and app state.
type StateManager interface {
	InstanceStorage
	AppState
}

// State is the on-disk representation of application state.
type State struct {
	// HelpScreensSeen is a bitmask tracking which help screens the user has seen.
	HelpScreensSeen uint32 `json:"help_screens_seen"`
	// InstancesData holds the serialised instance list as a raw JSON value.
	InstancesData json.RawMessage `json:"instances"`
}

// DefaultState returns an initial State with no help screens seen and an empty instances list.
func DefaultState() *State {
	return &State{
		HelpScreensSeen: 0,
		InstancesData:   json.RawMessage("[]"),
	}
}

// LoadState reads state.json from the config directory. When the file is absent it
// creates and persists a default. On parse errors it returns a default without saving.
func LoadState() *State {
	dir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultState()
	}

	data, readErr := os.ReadFile(filepath.Join(dir, StateFileName))
	if readErr != nil {
		if os.IsNotExist(readErr) {
			def := DefaultState()
			if saveErr := SaveState(def); saveErr != nil {
				log.WarningLog.Printf("failed to save default state: %v", saveErr)
			}
			return def
		}
		log.WarningLog.Printf("failed to get state file: %v", readErr)
		return DefaultState()
	}

	var s State
	if unmarshalErr := json.Unmarshal(data, &s); unmarshalErr != nil {
		log.ErrorLog.Printf("failed to parse state file: %v", unmarshalErr)
		return DefaultState()
	}

	return &s
}

// SaveState serialises s as indented JSON and writes it to the config directory.
func SaveState(s *State) error {
	dir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return fmt.Errorf("failed to create config directory: %w", mkErr)
	}
	data, marshalErr := json.MarshalIndent(s, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal state: %w", marshalErr)
	}
	return os.WriteFile(filepath.Join(dir, StateFileName), data, 0644)
}

// SaveInstances implements InstanceStorage: replaces the stored instances and persists.
func (s *State) SaveInstances(instancesJSON json.RawMessage) error {
	s.InstancesData = instancesJSON
	return SaveState(s)
}

// GetInstances implements InstanceStorage: returns the raw instance JSON blob.
func (s *State) GetInstances() json.RawMessage {
	return s.InstancesData
}

// DeleteAllInstances implements InstanceStorage: resets instances to an empty list.
func (s *State) DeleteAllInstances() error {
	s.InstancesData = json.RawMessage("[]")
	return SaveState(s)
}

// GetHelpScreensSeen implements AppState: returns the seen-help-screens bitmask.
func (s *State) GetHelpScreensSeen() uint32 {
	return s.HelpScreensSeen
}

// SetHelpScreensSeen implements AppState: stores an updated bitmask and persists.
func (s *State) SetHelpScreensSeen(seen uint32) error {
	s.HelpScreensSeen = seen
	return SaveState(s)
}
