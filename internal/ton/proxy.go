package ton

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultProxyPort    = 8080
	defaultProxyHost    = "127.0.0.1"
	proxyPollInterval   = 500 * time.Millisecond
	proxyCheckTimeout   = 2 * time.Second
)

// ProxyStatus checks whether tonnet-proxy is running on the default port (8080).
// Returns: running bool, port int, error
func ProxyStatus() (bool, int, error) {
	return ProxyStatusOnPort(defaultProxyPort)
}

// ProxyStatusOnPort checks whether tonnet-proxy is running on the given port.
func ProxyStatusOnPort(port int) (bool, int, error) {
	url := fmt.Sprintf("http://%s:%d", defaultProxyHost, port)
	client := &http.Client{Timeout: proxyCheckTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return false, port, nil //nolint:nilerr // connection refused = not running
	}
	defer resp.Body.Close()
	return true, port, nil
}

// WaitForProxy polls the default proxy port every 500 ms until it responds
// or the context is cancelled / timeout expires.
func WaitForProxy(ctx context.Context, timeout time.Duration) error {
	return WaitForProxyOnPort(ctx, defaultProxyPort, timeout)
}

// WaitForProxyOnPort polls the given port until the proxy is available.
func WaitForProxyOnPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		running, _, err := ProxyStatusOnPort(port)
		if err != nil {
			return fmt.Errorf("proxy check error: %w", err)
		}
		if running {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("tonnet-proxy not available after %s on port %d", timeout, port)
		}
		time.Sleep(proxyPollInterval)
	}
}
