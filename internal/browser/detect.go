package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/TONresistor/agent-tonbrowser/internal/config"
)

// executableName returns the expected browser executable name inside a managed install dir.
func executableName() string {
	switch runtime.GOOS {
	case "windows":
		return "TON Browser.exe"
	case "darwin":
		return filepath.Join("TON Browser.app", "Contents", "MacOS", "TON Browser")
	default:
		return "TON Browser.AppImage"
	}
}

// DetectInstalled checks {ConfigDir}/browsers/tonnet-{version}/ for a managed installation.
// If version is empty it returns the first valid installation found.
// Returns the path to the executable, or empty string if not found.
func DetectInstalled(version string) string {
	bdir, err := config.BrowserDir()
	if err != nil {
		return ""
	}

	check := func(dir string) string {
		exe := filepath.Join(dir, executableName())
		if _, err := os.Stat(exe); err == nil {
			return exe
		}
		return ""
	}

	if version != "" {
		return check(filepath.Join(bdir, "tonnet-"+version))
	}

	// Scan all tonnet-* subdirectories.
	entries, err := os.ReadDir(bdir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "tonnet-") {
			continue
		}
		if exe := check(filepath.Join(bdir, e.Name())); exe != "" {
			return exe
		}
	}
	return ""
}

// DetectSystem checks common system installation paths for Tonnet Browser.
// Returns the path to the executable, or empty string if not found.
func DetectSystem() string {
	var candidates []string

	switch runtime.GOOS {
	case "linux":
		home, _ := os.UserHomeDir()
		candidates = []string{
			"/usr/bin/tonnet-browser",
			"/opt/TON Browser/TON Browser",
			fmt.Sprintf("%s/.local/bin/tonnet-browser", home),
			fmt.Sprintf("%s/.local/bin/TON Browser", home),
			fmt.Sprintf("%s/Applications/TonnetBrowser.AppImage", home),
			fmt.Sprintf("%s/Applications/TON Browser.AppImage", home),
		}
	case "darwin":
		candidates = []string{
			"/Applications/TON Browser.app/Contents/MacOS/TON Browser",
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		candidates = []string{
			filepath.Join(localAppData, "Programs", "TON Browser", "TON Browser.exe"),
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
