package cdp

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

const defaultNavTimeout = 30 * time.Second

// Navigate goes to the given URL and waits for the page to load.
func Navigate(s *Session, url string) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultNavTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		return fmt.Errorf("navigate %q: %w", url, err)
	}
	return nil
}

// Back goes back one step in the browser history.
func Back(s *Session) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultNavTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.NavigateBack()); err != nil {
		return fmt.Errorf("navigate back: %w", err)
	}
	return nil
}

// Forward goes forward one step in the browser history.
func Forward(s *Session) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultNavTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.NavigateForward()); err != nil {
		return fmt.Errorf("navigate forward: %w", err)
	}
	return nil
}

// Reload reloads the current page.
func Reload(s *Session) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultNavTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	return nil
}

// GetURL returns the current page URL.
func GetURL(s *Session) (string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	var url string
	if err := chromedp.Run(ctx, chromedp.Location(&url)); err != nil {
		return "", fmt.Errorf("get url: %w", err)
	}
	return url, nil
}

// GetTitle returns the current page title.
func GetTitle(s *Session) (string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		return "", fmt.Errorf("get title: %w", err)
	}
	return title, nil
}
