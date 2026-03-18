package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir returns the base config directory: {UserConfigDir}/agent-tonbrowser/
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}
	return filepath.Join(base, "agent-tonbrowser"), nil
}

// BrowserDir returns where browsers are stored: {ConfigDir}/browsers/
func BrowserDir() (string, error) {
	cfg, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "browsers"), nil
}

// StatePath returns the state file path: {ConfigDir}/state.json
func StatePath() (string, error) {
	cfg, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "state.json"), nil
}

// EnsureDirs creates the config and browser directories if they don't exist.
func EnsureDirs() error {
	bdir, err := BrowserDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		return fmt.Errorf("create browser dir: %w", err)
	}
	return nil
}
