package cdp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const defaultActionTimeout = 10 * time.Second

// resolveSelector converts a CSS/XPath selector string to chromedp query params.
// Does NOT handle @eN refs — callers handle those separately via ResolveRef + resolveNodeObject.
func resolveSelector(selector string) (string, chromedp.QueryOption) {
	if strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//") {
		return selector, chromedp.BySearch
	}
	return selector, chromedp.ByQuery
}

// resolveNodeObject resolves a BackendDOMNodeID to a JS RemoteObject via CDP.
// This is the correct way to interact with nodes identified by the accessibility tree.
func resolveNodeObject(ctx context.Context, backendNodeID int64) (*runtime.RemoteObject, error) {
	obj, err := dom.ResolveNode().WithBackendNodeID(cdp.BackendNodeID(backendNodeID)).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve node %d: %w", backendNodeID, err)
	}
	return obj, nil
}

// callOnNode resolves a BackendDOMNodeID and calls a JS function on it.
func callOnNode(ctx context.Context, backendNodeID int64, fnDecl string) error {
	obj, err := resolveNodeObject(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, _, err = runtime.CallFunctionOn(fnDecl).WithObjectID(obj.ObjectID).Do(ctx)
	if err != nil {
		return fmt.Errorf("call function on node %d: %w", backendNodeID, err)
	}
	return nil
}

// Click clicks the element matching selector.
// Selector may be a CSS selector, XPath expression, or @eN ref.
func Click(s *Session, selector string) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	if strings.HasPrefix(selector, "@e") {
		nodeID, err := s.ResolveRef(selector)
		if err != nil {
			return fmt.Errorf("click: %w", err)
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return callOnNode(ctx, nodeID, "function() { this.click(); }")
		}))
	}

	sel, by := resolveSelector(selector)
	return chromedp.Run(ctx, chromedp.Click(sel, by))
}

// Fill clears the input and types new text.
func Fill(s *Session, selector, text string) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	if strings.HasPrefix(selector, "@e") {
		nodeID, err := s.ResolveRef(selector)
		if err != nil {
			return fmt.Errorf("fill: %w", err)
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			if err := callOnNode(ctx, nodeID, "function() { this.focus(); this.value = ''; }"); err != nil {
				return err
			}
			return chromedp.KeyEvent(text).Do(ctx)
		}))
	}

	sel, by := resolveSelector(selector)
	return chromedp.Run(ctx,
		// Click to focus, Ctrl+A to select all, then type replaces selection.
		// This works with React controlled inputs where chromedp.Clear fails.
		chromedp.Click(sel, by),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.KeyEvent("a", chromedp.KeyModifiers(2)).Do(ctx) // 2 = Ctrl
		}),
		chromedp.SendKeys(sel, text, by),
	)
}

// Type types text into the element without clearing first.
func Type(s *Session, selector, text string) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	if strings.HasPrefix(selector, "@e") {
		nodeID, err := s.ResolveRef(selector)
		if err != nil {
			return fmt.Errorf("type: %w", err)
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			if err := callOnNode(ctx, nodeID, "function() { this.focus(); }"); err != nil {
				return err
			}
			return chromedp.KeyEvent(text).Do(ctx)
		}))
	}

	sel, by := resolveSelector(selector)
	return chromedp.Run(ctx, chromedp.SendKeys(sel, text, by))
}

// keyNameMap maps human-readable key names to chromedp kb constants.
var keyNameMap = map[string]string{
	"Enter":     kb.Enter,
	"Tab":       kb.Tab,
	"Escape":    kb.Escape,
	"Backspace": kb.Backspace,
	"Delete":    kb.Delete,
	"ArrowUp":   kb.ArrowUp,
	"ArrowDown": kb.ArrowDown,
	"ArrowLeft": kb.ArrowLeft,
	"ArrowRight": kb.ArrowRight,
	"Home":      kb.Home,
	"End":       kb.End,
	"PageUp":    kb.PageUp,
	"PageDown":  kb.PageDown,
	"Space":     " ",
}

// Press sends a key press to the focused element.
// key should be a key name like "Enter", "Tab", "Escape", or a single character.
func Press(s *Session, key string) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	// Map named keys to kb constants
	if mapped, ok := keyNameMap[key]; ok {
		return chromedp.Run(ctx, chromedp.KeyEvent(mapped))
	}

	// Single character: send directly
	if runes := []rune(key); len(runes) == 1 {
		return chromedp.Run(ctx, chromedp.KeyEvent(key))
	}

	return fmt.Errorf("press: unknown key %q (use Enter, Tab, Escape, Backspace, Delete, ArrowUp/Down/Left/Right, Home, End, PageUp, PageDown, Space, or a single character)", key)
}

// Scroll scrolls the page in the given direction by the given number of pixels.
// direction: "up", "down", "left", "right"
func Scroll(s *Session, direction string, pixels int) error {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	var js string
	switch direction {
	case "down":
		js = fmt.Sprintf("window.scrollBy(0, %d)", pixels)
	case "up":
		js = fmt.Sprintf("window.scrollBy(0, -%d)", pixels)
	case "right":
		js = fmt.Sprintf("window.scrollBy(%d, 0)", pixels)
	case "left":
		js = fmt.Sprintf("window.scrollBy(-%d, 0)", pixels)
	default:
		return fmt.Errorf("scroll: unknown direction %q (use up/down/left/right)", direction)
	}

	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// Wait waits for a condition:
//   - CSS selector string → waits until the element is visible
//   - "load" → waits until document.readyState === 'complete'
//   - "network" → waits until network is idle (readyState complete)
//   - numeric string like "500" → sleeps for that many milliseconds
func Wait(s *Session, condition string) error {
	// Numeric: sleep N ms (context-aware)
	var ms int
	if n, err := fmt.Sscanf(condition, "%d", &ms); n == 1 && err == nil {
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return nil
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	switch condition {
	case "load", "network":
		return chromedp.Run(ctx, chromedp.Poll(
			`document.readyState === 'complete'`,
			nil,
			chromedp.WithPollingInterval(200*time.Millisecond),
		))
	default:
		// Treat as CSS selector
		return chromedp.Run(ctx, chromedp.WaitVisible(condition, chromedp.ByQuery))
	}
}

// Eval executes JavaScript in the page context and returns the result as a string.
func Eval(s *Session, script string) (string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, defaultActionTimeout)
	defer cancel()

	var result any
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
		return "", fmt.Errorf("eval: %w", err)
	}
	return fmt.Sprintf("%v", result), nil
}

