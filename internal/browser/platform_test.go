package browser

import (
	"strings"
	"testing"
)

func TestArtifactName(t *testing.T) {
	tests := []struct {
		version string
	}{
		{"1.4.2"},
		{"2.0.0"},
		{"0.1.0"},
	}
	for _, tc := range tests {
		name := ArtifactName(tc.version)
		if name == "" {
			t.Errorf("ArtifactName(%q) returned empty string", tc.version)
		}
		if !strings.Contains(name, tc.version) {
			t.Errorf("ArtifactName(%q) = %q, expected to contain version", tc.version, name)
		}
	}
}

func TestDownloadURL(t *testing.T) {
	version := "1.4.2"
	artifact := ArtifactName(version)
	url := DownloadURL(version, artifact)

	if !strings.Contains(url, "github.com") {
		t.Errorf("DownloadURL() = %q, expected to contain 'github.com'", url)
	}
	if !strings.Contains(url, version) {
		t.Errorf("DownloadURL() = %q, expected to contain version %q", url, version)
	}
	if !strings.Contains(url, artifact) {
		t.Errorf("DownloadURL() = %q, expected to contain artifact %q", url, artifact)
	}
}

func TestSHA256SumsURL(t *testing.T) {
	version := "1.4.2"
	url := SHA256SumsURL(version)

	if !strings.Contains(url, "github.com") {
		t.Errorf("SHA256SumsURL() = %q, expected to contain 'github.com'", url)
	}
	if !strings.Contains(url, version) {
		t.Errorf("SHA256SumsURL() = %q, expected to contain version %q", url, version)
	}
	if !strings.Contains(url, "SHA256SUMS.txt") {
		t.Errorf("SHA256SumsURL() = %q, expected to contain 'SHA256SUMS.txt'", url)
	}
}
