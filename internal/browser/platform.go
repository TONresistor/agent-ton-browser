package browser

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/google/go-github/v67/github"
)

const (
	githubOwner = "TONresistor"
	githubRepo  = "Tonnet-Browser"
)

// ArtifactName returns the release artifact filename for the current OS/arch.
//
//	Linux:   "TON Browser-{version}.AppImage"
//	Windows: "TON Browser Setup {version}.exe"
//	macOS:   "TON Browser-{version}.dmg"
func ArtifactName(version string) string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("TON Browser Setup %s.exe", version)
	case "darwin":
		return fmt.Sprintf("TON Browser-%s.dmg", version)
	default: // linux
		return fmt.Sprintf("TON Browser-%s.AppImage", version)
	}
}

// DownloadURL returns the GitHub release download URL for a given version and artifact.
func DownloadURL(version, artifact string) string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/v%s/%s",
		githubOwner, githubRepo, version, artifact,
	)
}

// SHA256SumsURL returns the URL for the SHA256SUMS.txt asset of a given version.
func SHA256SumsURL(version string) string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/v%s/SHA256SUMS.txt",
		githubOwner, githubRepo, version,
	)
}

// LatestVersion queries the GitHub Releases API and returns the latest published
// version string (without the "v" prefix, e.g. "1.4.2").
func LatestVersion(ctx context.Context) (string, error) {
	client := github.NewClient(nil)
	rel, _, err := client.Repositories.GetLatestRelease(ctx, githubOwner, githubRepo)
	if err != nil {
		return "", fmt.Errorf("query latest release: %w", err)
	}
	tag := rel.GetTagName()
	version := strings.TrimPrefix(tag, "v")
	if version == "" {
		return "", fmt.Errorf("empty tag name from GitHub release")
	}
	return version, nil
}
