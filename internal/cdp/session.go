package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// Session holds the chromedp allocator and browser context
type Session struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	// refMap stores @eN → BackendDOMNodeID mappings from the last snapshot
	refMap map[string]int64
}

// Connect establishes a CDP connection to a running Electron browser on the given port.
// Electron doesn't support Target.createTarget, so we discover existing page targets
// via /json/list and attach to the first one using WithTargetID.
func Connect(ctx context.Context, port int) (*Session, error) {
	pageTarget, err := discoverPageTarget(ctx, port)
	if err != nil {
		return nil, fmt.Errorf("cdp connect: %w", err)
	}

	// Connect to the page's WebSocket URL directly
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, pageTarget.WebSocketDebuggerURL)

	// Attach to the existing page target — do NOT create a new target (Electron doesn't support it)
	browserCtx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithTargetID(target.ID(pageTarget.ID)),
	)
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("cdp connect: %w", err)
	}

	return &Session{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         browserCtx,
		cancel:      cancel,
		refMap:      make(map[string]int64),
	}, nil
}

// ConnectToTarget connects to a specific page target by index (0-based).
// Use this to connect to WebContentsViews (tab content) instead of the main renderer.
func ConnectToTarget(ctx context.Context, port int, index int) (*Session, error) {
	targets, err := discoverAllPageTargets(ctx, port)
	if err != nil {
		return nil, fmt.Errorf("cdp connect: %w", err)
	}
	if index < 0 || index >= len(targets) {
		return nil, fmt.Errorf("cdp connect: target index %d out of range (have %d targets)", index, len(targets))
	}
	t := targets[index]

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, t.WebSocketDebuggerURL)
	browserCtx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithTargetID(target.ID(t.ID)),
	)
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("cdp connect: %w", err)
	}

	return &Session{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         browserCtx,
		cancel:      cancel,
		refMap:      make(map[string]int64),
	}, nil
}

// discoverAllPageTargets returns all "page" targets (excluding DevTools).
func discoverAllPageTargets(ctx context.Context, port int) ([]cdpTarget, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query %s: unexpected status %d", url, resp.StatusCode)
	}

	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("decode targets: %w", err)
	}

	var pages []cdpTarget
	for _, t := range targets {
		if t.Type == "page" && !strings.HasPrefix(t.URL, "devtools://") {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// cdpTarget represents a target from /json/list.
type cdpTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// discoverPageTarget queries /json/list on the given port and returns the
// first "page" target. This is necessary for Electron apps which don't
// support browser-level CDP operations like Target.createTarget.
func discoverPageTarget(ctx context.Context, port int) (*cdpTarget, error) {
	pages, err := discoverAllPageTargets(ctx, port)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no page target found on port %d", port)
	}
	return &pages[0], nil
}

// Disconnect releases the CDP session.
// IMPORTANT: We intentionally do NOT cancel chromedp contexts here because
// canceling them sends Target.detachFromTarget / Browser.close via WebSocket,
// which causes Electron to shut down (it interprets detach as exit signal).
// Instead, we just let the WebSocket connection close naturally when the
// CLI process exits. The contexts will be garbage collected.
func (s *Session) Disconnect() {
	// No-op: let the process exit handle cleanup.
	// Canceling s.cancel() or s.allocCancel() triggers Electron shutdown.
}

// Context returns the underlying chromedp context, useful for raw chromedp.Run calls.
func (s *Session) Context() context.Context {
	return s.ctx
}
