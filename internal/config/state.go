package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// State holds the runtime state of the managed browser process.
type State struct {
	PID       int    `json:"pid"`
	CDPPort   int    `json:"cdp_port"`
	WSURL     string `json:"ws_url,omitempty"`
	StartedAt string `json:"started_at"`
	Version   string `json:"version"`
}

// LoadState reads state.json and returns the persisted State.
// Returns nil and no error when the file does not exist.
func LoadState() (*State, error) {
	path, err := StatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// SaveState writes s to state.json atomically (write temp + rename).
func SaveState(s *State) error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// ClearState removes state.json.
func ClearState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove state: %w", err)
	}
	return nil
}

// IsRunning checks whether the PID stored in s is still alive.
func (s *State) IsRunning() bool {
	if s == nil || s.PID == 0 {
		return false
	}
	proc, err := os.FindProcess(s.PID)
	if err != nil {
		return false
	}
	return isProcessAlive(proc)
}
