package cdp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// screenshotResult holds the raw CDP response for Page.captureScreenshot.
type screenshotResult struct {
	Data string `json:"data"` // base64-encoded image
}

// captureScreenshot is the shared implementation for Screenshot and FullScreenshot.
// If path is empty, a timestamped filename is generated using prefix.
func captureScreenshot(s *Session, path, prefix string, params map[string]any, timeout time.Duration) (string, error) {
	if path == "" {
		path = fmt.Sprintf("%s%s.png", prefix, time.Now().Format("20060102_150405"))
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	var result screenshotResult
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		executor := chromedp.FromContext(ctx)
		if executor == nil {
			return fmt.Errorf("no chromedp executor in context")
		}
		return executor.Target.Execute(ctx, "Page.captureScreenshot", params, &result)
	}))
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	buf, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return "", fmt.Errorf("screenshot decode: %w", err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return "", fmt.Errorf("screenshot write: %w", err)
	}
	return path, nil
}

// Screenshot captures the visible viewport and writes it to path.
// Uses the raw CDP executor to avoid chromedp action timeouts on Electron.
// If path is empty, a timestamped filename is generated in the current directory.
func Screenshot(s *Session, path string) (string, error) {
	return captureScreenshot(s, path, "screenshot_", map[string]any{"format": "png"}, 15*time.Second)
}

// FullScreenshot captures the full page by setting clip to the full document size.
// If path is empty, a timestamped filename is generated in the current directory.
func FullScreenshot(s *Session, path string) (string, error) {
	return captureScreenshot(s, path, "screenshot_full_", map[string]any{"format": "png", "captureBeyondViewport": true}, 30*time.Second)
}
