package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/TONresistor/agent-tonbrowser/internal/cdp"
	"github.com/TONresistor/agent-tonbrowser/internal/config"
)

// StatusResponse is the response from GET /status.
type StatusResponse struct {
	Connected bool `json:"connected"`
	Port      int  `json:"port"`
	Tab       int  `json:"tab"`
	PID       int  `json:"pid"`
}

// Response is the standard daemon response format.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// SocketPath returns the path to the daemon Unix socket.
func SocketPath() (string, error) {
	cfg, err := config.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("socket path: %w", err)
	}
	return filepath.Join(cfg, "daemon.sock"), nil
}

// PIDPath returns the path to the daemon PID file.
func PIDPath() (string, error) {
	cfg, err := config.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("pid path: %w", err)
	}
	return filepath.Join(cfg, "daemon.pid"), nil
}

var (
	daemonClient     *http.Client
	daemonClientOnce sync.Once
)

func getClient() *http.Client {
	daemonClientOnce.Do(func() {
		sockPath, _ := SocketPath()
		daemonClient = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sockPath)
				},
			},
			// No Timeout here — use context.WithTimeout per request instead
		}
	})
	return daemonClient
}

// do performs an HTTP request to the daemon and decodes the Response.
func do(method, path string, body any) (*Response, error) {
	client := getClient()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, "http://daemon"+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	var daemonResp Response
	if err := json.NewDecoder(resp.Body).Decode(&daemonResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !daemonResp.OK {
		return nil, fmt.Errorf("daemon error: %s", daemonResp.Error)
	}
	return &daemonResp, nil
}

// DaemonRunning checks if the daemon is alive by pinging GET /status.
func DaemonRunning() bool {
	resp, err := do(http.MethodGet, "/status", nil)
	return err == nil && resp.OK
}

// StartDaemon forks the current binary with "daemon start" subcommand.
// The child process is detached (new process group). Waits up to 5 seconds
// for the daemon to be responsive before returning.
func StartDaemon() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}

	cmd := exec.Command(exePath, "daemon", "start")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Poll for daemon to be responsive (up to 5 seconds)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if DaemonRunning() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within 5 seconds")
}

// StopDaemon sends POST /shutdown to the daemon.
func StopDaemon() error {
	_, err := do(http.MethodPost, "/shutdown", map[string]any{})
	if err != nil {
		return fmt.Errorf("stop daemon: %w", err)
	}
	return nil
}

// EnsureDaemon starts the daemon if not already running.
func EnsureDaemon() error {
	if DaemonRunning() {
		return nil
	}
	return StartDaemon()
}

// DaemonStatus returns the daemon status from GET /status.
func DaemonStatus() (*StatusResponse, error) {
	resp, err := do(http.MethodGet, "/status", nil)
	if err != nil {
		return nil, fmt.Errorf("daemon status: %w", err)
	}
	var status StatusResponse
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &status, nil
}

// Connect sends POST /connect to the daemon.
func Connect(port, tab int) error {
	_, err := do(http.MethodPost, "/connect", map[string]any{"port": port, "tab": tab})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	return nil
}

// Disconnect sends POST /disconnect to the daemon.
func Disconnect() error {
	_, err := do(http.MethodPost, "/disconnect", map[string]any{})
	if err != nil {
		return fmt.Errorf("disconnect: %w", err)
	}
	return nil
}

// Eval sends POST /eval and returns the result string.
func Eval(tab int, script string) (string, error) {
	resp, err := do(http.MethodPost, "/eval", map[string]any{"tab": tab, "script": script})
	if err != nil {
		return "", fmt.Errorf("eval: %w", err)
	}
	var result string
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("decode eval result: %w", err)
	}
	return result, nil
}

// Click sends POST /click.
func Click(tab int, selector string) error {
	_, err := do(http.MethodPost, "/click", map[string]any{"tab": tab, "selector": selector})
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

// Fill sends POST /fill.
func Fill(tab int, selector, text string) error {
	_, err := do(http.MethodPost, "/fill", map[string]any{"tab": tab, "selector": selector, "text": text})
	if err != nil {
		return fmt.Errorf("fill: %w", err)
	}
	return nil
}

// TypeText sends POST /type.
func TypeText(tab int, selector, text string) error {
	_, err := do(http.MethodPost, "/type", map[string]any{"tab": tab, "selector": selector, "text": text})
	if err != nil {
		return fmt.Errorf("type: %w", err)
	}
	return nil
}

// Press sends POST /press.
func Press(tab int, key string) error {
	_, err := do(http.MethodPost, "/press", map[string]any{"tab": tab, "key": key})
	if err != nil {
		return fmt.Errorf("press: %w", err)
	}
	return nil
}

// Scroll sends POST /scroll.
func Scroll(tab int, direction string, pixels int) error {
	_, err := do(http.MethodPost, "/scroll", map[string]any{"tab": tab, "direction": direction, "pixels": pixels})
	if err != nil {
		return fmt.Errorf("scroll: %w", err)
	}
	return nil
}

// Wait sends POST /wait.
func Wait(tab int, condition string) error {
	_, err := do(http.MethodPost, "/wait", map[string]any{"tab": tab, "condition": condition})
	if err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	return nil
}

// Screenshot sends POST /screenshot and returns the saved file path.
func Screenshot(tab int, path string, full bool) (string, error) {
	resp, err := do(http.MethodPost, "/screenshot", map[string]any{"tab": tab, "path": path, "full": full})
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	var result string
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("decode screenshot result: %w", err)
	}
	return result, nil
}

// Snapshot sends POST /snapshot and returns the formatted accessibility tree text.
func Snapshot(tab int, interactive, compact bool, depth int) (string, error) {
	resp, err := do(http.MethodPost, "/snapshot", map[string]any{
		"tab":         tab,
		"interactive": interactive,
		"compact":     compact,
		"depth":       depth,
	})
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	var result string
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("decode snapshot result: %w", err)
	}
	return result, nil
}

// Navigate sends POST /navigate.
func Navigate(tab int, url string) error {
	_, err := do(http.MethodPost, "/navigate", map[string]any{"tab": tab, "url": url})
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	return nil
}

// Back sends POST /back.
func Back(tab int) error {
	_, err := do(http.MethodPost, "/back", map[string]any{"tab": tab})
	if err != nil {
		return fmt.Errorf("back: %w", err)
	}
	return nil
}

// Forward sends POST /forward.
func Forward(tab int) error {
	_, err := do(http.MethodPost, "/forward", map[string]any{"tab": tab})
	if err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	return nil
}

// Reload sends POST /reload.
func Reload(tab int) error {
	_, err := do(http.MethodPost, "/reload", map[string]any{"tab": tab})
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	return nil
}

// GetURL sends GET /get?what=url and returns the current page URL.
func GetURL(tab int) (string, error) {
	resp, err := do(http.MethodGet, fmt.Sprintf("/get?what=url&tab=%d", tab), nil)
	if err != nil {
		return "", fmt.Errorf("get url: %w", err)
	}
	var result string
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("decode url result: %w", err)
	}
	return result, nil
}

// GetTitle sends GET /get?what=title and returns the current page title.
func GetTitle(tab int) (string, error) {
	resp, err := do(http.MethodGet, fmt.Sprintf("/get?what=title&tab=%d", tab), nil)
	if err != nil {
		return "", fmt.Errorf("get title: %w", err)
	}
	var result string
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("decode title result: %w", err)
	}
	return result, nil
}

// ListTabs sends GET /tabs and returns all open tabs.
func ListTabs() ([]cdp.TabInfo, error) {
	resp, err := do(http.MethodGet, "/tabs", nil)
	if err != nil {
		return nil, fmt.Errorf("list tabs: %w", err)
	}
	var tabs []cdp.TabInfo
	if err := json.Unmarshal(resp.Data, &tabs); err != nil {
		return nil, fmt.Errorf("decode tabs: %w", err)
	}
	return tabs, nil
}

// SwitchTab sends POST /tab/switch.
func SwitchTab(index int) error {
	_, err := do(http.MethodPost, "/tab/switch", map[string]any{"index": index})
	if err != nil {
		return fmt.Errorf("switch tab: %w", err)
	}
	return nil
}
