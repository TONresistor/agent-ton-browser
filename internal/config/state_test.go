package config

import (
	"os"
	"testing"
)

// saveAndRestore saves the current state.json (if any) and restores it after the test.
// This prevents tests from clobbering a real running instance's state.
func saveAndRestore(t *testing.T) {
	t.Helper()
	path, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}
	original, readErr := os.ReadFile(path)
	t.Cleanup(func() {
		if readErr != nil {
			// No original file existed — remove whatever the test wrote.
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, original, 0o644)
		}
	})
}

func TestSaveLoadStateRoundtrip(t *testing.T) {
	saveAndRestore(t)

	want := &State{
		PID:       1234,
		CDPPort:   9222,
		WSURL:     "ws://127.0.0.1:9222/devtools/browser/abc",
		StartedAt: "2025-01-01T00:00:00Z",
		Version:   "1.4.2",
	}

	if err := SaveState(want); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	got, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadState() returned nil, expected state")
	}
	if got.PID != want.PID {
		t.Errorf("PID: got %d, want %d", got.PID, want.PID)
	}
	if got.CDPPort != want.CDPPort {
		t.Errorf("CDPPort: got %d, want %d", got.CDPPort, want.CDPPort)
	}
	if got.WSURL != want.WSURL {
		t.Errorf("WSURL: got %q, want %q", got.WSURL, want.WSURL)
	}
	if got.StartedAt != want.StartedAt {
		t.Errorf("StartedAt: got %q, want %q", got.StartedAt, want.StartedAt)
	}
	if got.Version != want.Version {
		t.Errorf("Version: got %q, want %q", got.Version, want.Version)
	}
}

func TestLoadState_NoFile(t *testing.T) {
	saveAndRestore(t)
	_ = ClearState() // ensure file does not exist

	state, err := LoadState()
	if err != nil {
		t.Errorf("LoadState() with no file: unexpected error: %v", err)
	}
	if state != nil {
		t.Errorf("LoadState() with no file: expected nil, got %+v", state)
	}
}

func TestClearState(t *testing.T) {
	saveAndRestore(t)

	s := &State{PID: 1, CDPPort: 9222, StartedAt: "2025-01-01T00:00:00Z", Version: "1.0"}
	if err := SaveState(s); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}
	if err := ClearState(); err != nil {
		t.Errorf("ClearState() error: %v", err)
	}
	// Second call on missing file must not error
	if err := ClearState(); err != nil {
		t.Errorf("ClearState() on missing file: unexpected error: %v", err)
	}
}

func TestState_IsRunning_ZeroPID(t *testing.T) {
	s := &State{PID: 0}
	if s.IsRunning() {
		t.Error("IsRunning() with PID=0 should return false")
	}
}

func TestState_IsRunning_Nil(t *testing.T) {
	var s *State
	if s.IsRunning() {
		t.Error("IsRunning() on nil State should return false")
	}
}

func TestState_IsRunning_NonExistentPID(t *testing.T) {
	// 9999999 exceeds Linux's max PID (4194304) so it cannot exist.
	// On other platforms it is also very unlikely.
	s := &State{PID: 9999999}
	if s.IsRunning() {
		t.Errorf("IsRunning() with PID=9999999 should return false (process cannot exist)")
	}
}
