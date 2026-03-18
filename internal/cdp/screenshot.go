package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// AnnotationLabel describes an interactive element annotated in an AnnotatedScreenshot.
type AnnotationLabel struct {
	Index int    `json:"index"`
	Tag   string `json:"tag"`
	Text  string `json:"text"`
	Href  string `json:"href,omitempty"`
}

// AnnotatedScreenshot captures a screenshot with numbered labels on interactive elements.
// Injects JS overlays (red numbered boxes at top-left of each interactive element),
// takes the screenshot, then removes the overlays.
// Returns the path and a legend mapping index to element info.
func AnnotatedScreenshot(s *Session, path string) (string, []AnnotationLabel, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	const injectJS = `(function() {
		var elems = document.querySelectorAll('a, button, input, select, textarea, [role="button"], [role="link"], [onclick]');
		var labels = [];
		var count = 0;
		elems.forEach(function(el) {
			var rect = el.getBoundingClientRect();
			if (rect.width <= 0 || rect.height <= 0) return;
			var div = document.createElement('div');
			div.setAttribute('data-annotation', 'true');
			div.style.cssText = 'position:fixed;left:' + rect.left + 'px;top:' + rect.top + 'px;' +
				'background:red;color:white;font-size:11px;font-weight:bold;' +
				'z-index:2147483647;padding:1px 3px;pointer-events:none;line-height:1.2;';
			div.textContent = count;
			document.body.appendChild(div);
			labels.push({
				index: count,
				tag: el.tagName.toLowerCase(),
				text: el.textContent.trim().substring(0, 50),
				href: el.href || ''
			});
			count++;
		});
		return JSON.stringify(labels);
	})()`

	var labelsJSON string
	if err := chromedp.Run(ctx, chromedp.Evaluate(injectJS, &labelsJSON)); err != nil {
		return "", nil, fmt.Errorf("annotated screenshot: inject overlays: %w", err)
	}

	capturedPath, err := captureScreenshot(s, path, "screenshot_annotated_", map[string]any{"format": "png"}, 15*time.Second)
	if err != nil {
		return "", nil, err
	}

	const cleanupJS = `document.querySelectorAll('[data-annotation]').forEach(function(el) { el.remove(); })`
	_ = chromedp.Run(ctx, chromedp.Evaluate(cleanupJS, nil))

	var labels []AnnotationLabel
	if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
		return capturedPath, nil, fmt.Errorf("annotated screenshot: parse labels: %w", err)
	}

	return capturedPath, labels, nil
}
