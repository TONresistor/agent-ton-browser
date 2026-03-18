package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error: %v", err)
	}
	if dir == "" {
		t.Error("ConfigDir() returned empty string")
	}
}

func TestBrowserDir(t *testing.T) {
	dir, err := BrowserDir()
	if err != nil {
		t.Fatalf("BrowserDir() error: %v", err)
	}
	if !strings.HasSuffix(dir, "browsers") {
		t.Errorf("BrowserDir() = %q, want suffix 'browsers'", dir)
	}
}

func TestStatePath(t *testing.T) {
	path, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath() error: %v", err)
	}
	if !strings.HasSuffix(path, "state.json") {
		t.Errorf("StatePath() = %q, want suffix 'state.json'", path)
	}
}

func TestEnsureDirs(t *testing.T) {
	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	bdir, err := BrowserDir()
	if err != nil {
		t.Fatalf("BrowserDir() error: %v", err)
	}
	if _, err := os.Stat(bdir); err != nil {
		t.Errorf("BrowserDir %q does not exist after EnsureDirs(): %v", bdir, err)
	}
}
