package browser

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/TONresistor/agent-tonbrowser/internal/config"
)

// Launch starts Tonnet Browser with --remote-debugging-port=port.
// It waits until the browser prints "DevTools listening on ws://..." to stderr,
// then persists the PID and port to state.json and returns the CDP WebSocket URL.
func Launch(ctx context.Context, exePath string, port int, version string) (string, error) {
	state, err := config.LoadState()
	if err != nil {
		return "", fmt.Errorf("load state: %w", err)
	}
	if state != nil && state.IsRunning() {
		return state.WSURL, nil
	}

	cmd := exec.CommandContext(ctx, exePath,
		fmt.Sprintf("--remote-debugging-port=%d", port),
	)
	cmd.Env = append(os.Environ(), "ELECTRON_NO_ATTACH_CONSOLE=1")
	setSysProcAttr(cmd) // platform-specific: Setpgid=true on Unix

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start browser: %w", err)
	}

	// Parse stderr for the DevTools URL, with a timeout.
	wsURLCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "DevTools listening on ") {
				wsURLCh <- strings.TrimPrefix(line, "DevTools listening on ")
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		} else {
			errCh <- fmt.Errorf("browser exited before emitting DevTools URL")
		}
	}()

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	var wsURL string
	select {
	case wsURL = <-wsURLCh:
	case err := <-errCh:
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("wait for CDP: %w", err)
	case <-timer.C:
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timeout waiting for browser CDP URL")
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", ctx.Err()
	}

	if err := config.SaveState(&config.State{
		PID:       cmd.Process.Pid,
		CDPPort:   port,
		WSURL:     wsURL,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
	}); err != nil {
		return "", fmt.Errorf("save state: %w", err)
	}

	return wsURL, nil
}

// Close kills the browser process whose PID is recorded in state.json,
// then removes the state file.
func Close() error {
	state, err := config.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if state == nil {
		return nil
	}
	proc, err := os.FindProcess(state.PID)
	if err != nil {
		_ = config.ClearState()
		return nil
	}
	if err := killProcess(proc); err != nil {
		return fmt.Errorf("kill browser: %w", err)
	}
	return config.ClearState()
}

