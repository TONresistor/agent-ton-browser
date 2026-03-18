package browser

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schollz/progressbar/v3"

	"github.com/TONresistor/agent-tonbrowser/internal/config"
)

// Download fetches the Tonnet Browser binary for the current platform.
// If version is empty the latest version is used.
// Returns the path to the installed executable.
func Download(ctx context.Context, version string) (string, error) {
	if version == "" {
		var err error
		version, err = LatestVersion(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve latest version: %w", err)
		}
	}

	bdir, err := config.BrowserDir()
	if err != nil {
		return "", err
	}
	installDir := filepath.Join(bdir, "tonnet-"+version)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create install dir: %w", err)
	}

	artifact := ArtifactName(version)
	artifactURL := DownloadURL(version, artifact)
	destPath := filepath.Join(installDir, artifact)

	// Download SHA256SUMS.txt for checksum verification.
	expectedSum, err := fetchExpectedChecksum(ctx, version, artifact)
	if err != nil {
		// Non-fatal: continue without checksum verification.
		fmt.Fprintf(os.Stderr, "warning: could not fetch SHA256SUMS.txt: %v\n", err)
	}

	if err := downloadFile(ctx, artifactURL, destPath); err != nil {
		return "", fmt.Errorf("download browser: %w", err)
	}

	if expectedSum != "" {
		if err := verifyChecksum(destPath, expectedSum); err != nil {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("checksum mismatch: %w", err)
		}
	}

	return installBinary(ctx, destPath, installDir, version)
}

// downloadFile fetches url and writes it to dest, showing a progress bar.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer f.Close()

	bar := progressbar.DefaultBytes(resp.ContentLength, "downloading")
	if _, err := io.Copy(io.MultiWriter(f, bar), resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// fetchExpectedChecksum downloads SHA256SUMS.txt and finds the hash for artifact.
func fetchExpectedChecksum(ctx context.Context, version, artifact string) (string, error) {
	url := SHA256SumsURL(version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching SHA256SUMS.txt", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "<hash>  <filename>" or "<hash> <filename>"
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == artifact {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("artifact %q not found in SHA256SUMS.txt", artifact)
}

// verifyChecksum computes the SHA256 of path and compares against expected (hex).
func verifyChecksum(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("got %s, want %s", got, expected)
	}
	return nil
}

// installBinary post-processes the downloaded artifact for each platform.
// Returns the path to the runnable executable.
func installBinary(ctx context.Context, srcPath, installDir, version string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		return installAppImage(srcPath)
	case "darwin":
		return installDMG(ctx, srcPath, installDir)
	case "windows":
		return installExe(srcPath, installDir)
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// installAppImage makes the AppImage executable and returns its path.
func installAppImage(appImagePath string) (string, error) {
	if err := os.Chmod(appImagePath, 0o755); err != nil {
		return "", fmt.Errorf("chmod AppImage: %w", err)
	}
	return appImagePath, nil
}

// installDMG mounts the DMG, copies the .app bundle, then unmounts.
// Returns the path to the copied .app's executable.
func installDMG(ctx context.Context, dmgPath, installDir string) (string, error) {
	mountDir := filepath.Join(installDir, "mnt")
	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		return "", fmt.Errorf("create mount dir: %w", err)
	}

	// Mount the DMG.
	attach := exec.CommandContext(ctx, "hdiutil", "attach", dmgPath,
		"-mountpoint", mountDir, "-nobrowse", "-quiet")
	attach.Stdout = os.Stdout
	attach.Stderr = os.Stderr
	if err := attach.Run(); err != nil {
		return "", fmt.Errorf("hdiutil attach: %w", err)
	}
	defer func() {
		detach := exec.Command("hdiutil", "detach", mountDir, "-quiet", "-force")
		_ = detach.Run()
		_ = os.Remove(mountDir)
	}()

	// Find the .app bundle on the mounted volume.
	entries, err := os.ReadDir(mountDir)
	if err != nil {
		return "", fmt.Errorf("read mountpoint: %w", err)
	}
	var appSrc string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".app") {
			appSrc = filepath.Join(mountDir, e.Name())
			break
		}
	}
	if appSrc == "" {
		return "", fmt.Errorf("no .app bundle found in DMG")
	}

	appDest := filepath.Join(installDir, filepath.Base(appSrc))
	cp := exec.CommandContext(ctx, "cp", "-R", appSrc, appDest)
	cp.Stdout = os.Stdout
	cp.Stderr = os.Stderr
	if err := cp.Run(); err != nil {
		return "", fmt.Errorf("copy .app bundle: %w", err)
	}

	// Return the executable inside the .app.
	exeName := strings.TrimSuffix(filepath.Base(appSrc), ".app")
	exe := filepath.Join(appDest, "Contents", "MacOS", exeName)
	return exe, nil
}

// installExe places the NSIS installer in installDir.
// The user is expected to run the installer, or agent-tonbrowser will invoke it.
func installExe(exePath, installDir string) (string, error) {
	dest := filepath.Join(installDir, filepath.Base(exePath))
	if exePath != dest {
		if err := os.Rename(exePath, dest); err != nil {
			return "", fmt.Errorf("move installer: %w", err)
		}
	}
	return dest, nil
}
