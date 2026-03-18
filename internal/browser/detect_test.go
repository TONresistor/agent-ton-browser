package browser

import "testing"

func TestDetectInstalled_NonExistentVersion(t *testing.T) {
	// A version that will never be installed — must return empty string.
	result := DetectInstalled("99.99.99-nonexistent")
	if result != "" {
		t.Errorf("DetectInstalled with non-existent version: expected '', got %q", result)
	}
}

func TestDetectInstalled_EmptyVersion_NoBrowser(t *testing.T) {
	// With version="", DetectInstalled scans all tonnet-* dirs.
	// On a fresh system / CI runner without Tonnet Browser, result is "".
	// We only verify there is no panic.
	_ = DetectInstalled("")
}

func TestDetectSystem_NoBrowser(t *testing.T) {
	// DetectSystem scans well-known OS paths.
	// On CI or a system without Tonnet Browser installed, returns "".
	// We only verify there is no panic.
	_ = DetectSystem()
}
