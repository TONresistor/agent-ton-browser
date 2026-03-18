package cdp

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/target"
)

// TabInfo describes a browser tab/target.
type TabInfo struct {
	Index  int
	ID     target.ID
	Type   string // "page", "webview", "background_page"
	Title  string
	URL    string
}

// ListTabs returns all open tabs (page + webview types only).
// Background pages and service workers are excluded.
func ListTabs(s *Session) ([]TabInfo, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	var infos []*target.Info
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		infos, err = target.GetTargets().Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("list tabs: %w", err)
	}

	var tabs []TabInfo
	idx := 0
	for _, info := range infos {
		switch info.Type {
		case "page", "webview":
			tabs = append(tabs, TabInfo{
				Index: idx,
				ID:    info.TargetID,
				Type:  string(info.Type),
				Title: info.Title,
				URL:   info.URL,
			})
			idx++
		}
	}
	return tabs, nil
}

// SwitchTab switches the session context to the tab at the given index
// (as returned by ListTabs). The session's active context is updated.
func SwitchTab(s *Session, index int) error {
	tabs, err := ListTabs(s)
	if err != nil {
		return fmt.Errorf("switch tab: %w", err)
	}
	if index < 0 || index >= len(tabs) {
		return fmt.Errorf("switch tab: index %d out of range (0-%d)", index, len(tabs)-1)
	}
	return switchToTarget(s, tabs[index].ID)
}

// switchToTarget updates the session context to point at the given target.
func switchToTarget(s *Session, id target.ID) error {
	newCtx, newCancel := chromedp.NewContext(s.allocCtx, chromedp.WithTargetID(id))
	if err := chromedp.Run(newCtx); err != nil {
		newCancel()
		return fmt.Errorf("switch target: %w", err)
	}
	// Cancel old context and replace
	s.cancel()
	s.ctx = newCtx
	s.cancel = newCancel
	return nil
}
